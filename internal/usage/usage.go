// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/eikrad/codecap/internal/snapshot"
)

const sessionWindow = 5 * time.Hour

type tokenUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type usageEvent struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Model string     `json:"model"`
		Usage tokenUsage `json:"usage"`
	} `json:"message"`
}

type period struct {
	start time.Time
	end   time.Time
}

// periods are the four reported windows. Session is a rolling five hours in
// absolute time; the others are wall-clock boundaries in the desktop's zone.
type periods struct {
	session period
	today   period
	week    period
	month   period
}

func periodsAt(now time.Time, loc *time.Location, weekStart int32) periods {
	nowInLoc := now.In(loc)
	return periods{
		session: period{start: now.Add(-sessionWindow), end: now},
		today:   period{start: dayStart(nowInLoc), end: nowInLoc},
		week:    period{start: weekStartAt(nowInLoc, normalizeWeekStart(weekStart)), end: nowInLoc},
		month: period{
			start: time.Date(nowInLoc.Year(), nowInLoc.Month(), 1, 0, 0, 0, 0, loc),
			end:   nowInLoc,
		},
	}
}

// Compute aggregates Consumed Usage for an Account Home using DefaultRates.
//
// This builds a throwaway Cache, so it re-reads every log. Long-lived callers
// should hold a Cache and call its Compute instead.
func Compute(accountHome, timezone string, weekStart int32) (snapshot.ConsumedUsage, error) {
	return ComputeWithRates(accountHome, timezone, weekStart, DefaultRates())
}

// ComputeWithRates is the List Price seam: callers can swap the published rate table.
func ComputeWithRates(accountHome, timezone string, weekStart int32, rates Rates) (snapshot.ConsumedUsage, error) {
	return computeAt(accountHome, timezone, weekStart, time.Now().UTC(), rates)
}

func computeAt(accountHome, timezone string, weekStart int32, now time.Time, rates Rates) (snapshot.ConsumedUsage, error) {
	return NewCache(rates).computeAt(accountHome, timezone, weekStart, now)
}

// resolveLocation maps the bus timezone argument to a location.
//
// An empty string means "use the helper's own zone". Qt's JS engine has no Intl,
// so the plasmoid cannot produce an IANA id; it runs in the same session, so
// time.Local is the same zone the user sees. An unresolvable name is an error
// rather than a silent fallback to UTC, which moved every day, week and month
// boundary for users outside UTC without telling anyone.
func resolveLocation(timezone string) (*time.Location, error) {
	trimmed := strings.TrimSpace(timezone)
	if trimmed == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(trimmed)
	if err != nil {
		return nil, fmt.Errorf("resolve timezone %q: %w", trimmed, err)
	}
	return loc, nil
}

func listJSONLFiles(accountHome string) ([]string, error) {
	projectsDir := filepath.Join(accountHome, "projects")
	info, err := os.Stat(projectsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var files []string
	err = filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		// WalkDir reports the entry type from lstat, so this also rejects
		// symlinks, FIFOs and device nodes. A FIFO named *.jsonl blocked
		// os.Open until a writer appeared, which parked a D-Bus handler
		// goroutine forever.
		if !d.Type().IsRegular() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// parseEvents reads one log file into retained events.
func parseEvents(path string, rates Rates, cutoff time.Time) ([]event, error) {
	// O_NOFOLLOW because the walk saw an lstat, not the target; O_NONBLOCK so
	// that opening anything that is not a plain file cannot block. Both are
	// belt and braces over the IsRegular check in listJSONLFiles, since the
	// tree can change between the walk and the open.
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}

	return parseEventsFrom(file, rates, cutoff)
}

func parseEventsFrom(reader io.Reader, rates Rates, cutoff time.Time) ([]event, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var events []event
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var decoded usageEvent
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			continue
		}
		if decoded.Type != "assistant" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, decoded.Timestamp)
		if err != nil {
			continue
		}
		// Nothing older than the longest reported window can ever be counted.
		if at.Before(cutoff) {
			continue
		}

		usageCounts := decoded.Message.Usage
		events = append(events, event{
			at:   at,
			uuid: decoded.UUID,
			tokens: usageCounts.InputTokens + usageCounts.OutputTokens +
				usageCounts.CacheCreationInputTokens + usageCounts.CacheReadInputTokens,
			priceUSD: estimateListPriceUSD(rates, decoded.Message.Model, usageCounts),
		})
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return events, nil
}

func estimateListPriceUSD(rates Rates, model string, usage tokenUsage) float64 {
	card := rates.forModel(model)
	return (float64(usage.InputTokens)/1_000_000.0)*card.InputUSDPerM +
		(float64(usage.OutputTokens)/1_000_000.0)*card.OutputUSDPerM +
		(float64(usage.CacheCreationInputTokens)/1_000_000.0)*card.CacheWriteUSDPerM +
		(float64(usage.CacheReadInputTokens)/1_000_000.0)*card.CacheReadUSDPerM
}

func normalizeWeekStart(weekStart int32) time.Weekday {
	switch weekStart {
	case 1:
		return time.Monday
	case 2:
		return time.Tuesday
	case 3:
		return time.Wednesday
	case 4:
		return time.Thursday
	case 5:
		return time.Friday
	case 6:
		return time.Saturday
	case 0, 7:
		return time.Sunday
	default:
		return time.Monday
	}
}

func weekStartAt(now time.Time, weekStart time.Weekday) time.Time {
	start := dayStart(now)
	for start.Weekday() != weekStart {
		start = start.AddDate(0, 0, -1)
	}
	return start
}

func dayStart(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func inPeriod(t time.Time, p period) bool {
	return (t.Equal(p.start) || t.After(p.start)) && (t.Equal(p.end) || t.Before(p.end))
}

func addPeriod(dst *snapshot.ConsumedPeriod, src snapshot.ConsumedPeriod) {
	dst.ListPriceUSD += src.ListPriceUSD
	dst.Tokens += src.Tokens
}
