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

// cacheCreation splits a cache write by the lifetime it was written with. The
// two are priced differently — a one-hour entry costs more to write than a
// five-minute one — so the split has to survive as far as the rate card.
type cacheCreation struct {
	Ephemeral1hInputTokens int64 `json:"ephemeral_1h_input_tokens"`
	Ephemeral5mInputTokens int64 `json:"ephemeral_5m_input_tokens"`
}

type tokenUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	// CacheCreationInputTokens is the total, and CacheCreation is that same
	// total split by lifetime. Across 3761 events in three Account Homes the
	// two always agreed, so the total is treated as authoritative for token
	// reporting and the split is used only to price it.
	CacheCreationInputTokens int64          `json:"cache_creation_input_tokens"`
	CacheCreation            *cacheCreation `json:"cache_creation"`
	CacheReadInputTokens     int64          `json:"cache_read_input_tokens"`
}

// cacheWriteSplit reports one-hour and five-minute cache writes separately.
//
// The pointer is nil for an event written by a Claude Code old enough not to
// report the split. Everything then falls into the five-minute bucket, which is
// what this code assumed before the split existed — a low estimate for anyone
// actually using one-hour caching, but the alternative is inventing a lifetime
// the log does not record.
func (u tokenUsage) cacheWriteSplit() (oneHour, fiveMinute int64) {
	if u.CacheCreation == nil {
		return 0, u.CacheCreationInputTokens
	}
	oneHour = u.CacheCreation.Ephemeral1hInputTokens
	fiveMinute = u.CacheCreation.Ephemeral5mInputTokens
	// A total that disagrees with its own split means the payload changed shape.
	// Trusting the total keeps the token count right and puts the unexplained
	// remainder in the cheaper bucket rather than silently dropping it.
	if remainder := u.CacheCreationInputTokens - (oneHour + fiveMinute); remainder > 0 {
		fiveMinute += remainder
	}
	return oneHour, fiveMinute
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

// computeAt is the cold-read seam the tests drive: a throwaway Cache, an
// injected clock and an injected rate table.
//
// The package exports no one-shot Compute. It had two — Compute and
// ComputeWithRates — and nothing outside the package ever called either;
// dbusapi holds a Cache, which is the whole point of having one, and the tests
// need the clock this takes. Exported wrappers over a constructor plus a method
// are surface, not seams.
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
	oneHourWrite, fiveMinuteWrite := usage.cacheWriteSplit()
	return (float64(usage.InputTokens)/1_000_000.0)*card.InputUSDPerM +
		(float64(usage.OutputTokens)/1_000_000.0)*card.OutputUSDPerM +
		(float64(oneHourWrite)/1_000_000.0)*card.CacheWrite1hUSDPerM +
		(float64(fiveMinuteWrite)/1_000_000.0)*card.CacheWrite5mUSDPerM +
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
