// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// Compute aggregates Consumed Usage for an Account Home using DefaultRates.
func Compute(accountHome, timezone string, weekStart int32) (snapshot.ConsumedUsage, error) {
	return ComputeWithRates(accountHome, timezone, weekStart, DefaultRates())
}

// ComputeWithRates is the List Price seam: callers can swap the published rate table.
func ComputeWithRates(accountHome, timezone string, weekStart int32, rates Rates) (snapshot.ConsumedUsage, error) {
	return computeAt(accountHome, timezone, weekStart, time.Now().UTC(), rates)
}

func computeAt(accountHome, timezone string, weekStart int32, now time.Time, rates Rates) (snapshot.ConsumedUsage, error) {
	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		loc = time.UTC
	}

	normalizedWeekStart := normalizeWeekStart(weekStart)
	nowInLoc := now.In(loc)

	session := period{start: now.Add(-sessionWindow), end: now}
	today := period{start: dayStart(nowInLoc), end: nowInLoc}
	week := period{start: weekStartAt(nowInLoc, normalizedWeekStart), end: nowInLoc}
	month := period{
		start: time.Date(nowInLoc.Year(), nowInLoc.Month(), 1, 0, 0, 0, 0, loc),
		end:   nowInLoc,
	}

	files, err := listJSONLFiles(accountHome)
	if err != nil {
		return snapshot.ConsumedUsage{}, err
	}

	seen := make(map[string]struct{})
	var usage snapshot.ConsumedUsage
	for _, jsonlPath := range files {
		fileUsage, fileErr := consumeFile(jsonlPath, session, today, week, month, loc, rates, seen)
		if fileErr != nil {
			continue
		}
		addPeriod(&usage.Session, fileUsage.Session)
		addPeriod(&usage.Today, fileUsage.Today)
		addPeriod(&usage.Week, fileUsage.Week)
		addPeriod(&usage.Month, fileUsage.Month)
	}

	return usage, nil
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
		if strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func consumeFile(path string, session, today, week, month period, loc *time.Location, rates Rates, seen map[string]struct{}) (snapshot.ConsumedUsage, error) {
	file, err := os.Open(path)
	if err != nil {
		return snapshot.ConsumedUsage{}, err
	}
	defer func() { _ = file.Close() }()

	return consumeReader(file, session, today, week, month, loc, rates, seen)
}

func consumeReader(reader io.Reader, session, today, week, month period, loc *time.Location, rates Rates, seen map[string]struct{}) (snapshot.ConsumedUsage, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var usage snapshot.ConsumedUsage
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event usageEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type != "assistant" {
			continue
		}
		if event.UUID != "" {
			if _, ok := seen[event.UUID]; ok {
				continue
			}
			seen[event.UUID] = struct{}{}
		}

		eventTime, err := time.Parse(time.RFC3339Nano, event.Timestamp)
		if err != nil {
			continue
		}

		eventInLoc := eventTime.In(loc)
		tokens := event.Message.Usage.InputTokens +
			event.Message.Usage.OutputTokens +
			event.Message.Usage.CacheCreationInputTokens +
			event.Message.Usage.CacheReadInputTokens
		listPriceUSD := estimateListPriceUSD(rates, event.Message.Model, event.Message.Usage)
		periodUsage := snapshot.ConsumedPeriod{
			ListPriceUSD: listPriceUSD,
			Tokens:       tokens,
		}

		if inPeriod(eventTime, session) {
			addPeriod(&usage.Session, periodUsage)
		}
		if inPeriod(eventInLoc, today) {
			addPeriod(&usage.Today, periodUsage)
		}
		if inPeriod(eventInLoc, week) {
			addPeriod(&usage.Week, periodUsage)
		}
		if inPeriod(eventInLoc, month) {
			addPeriod(&usage.Month, periodUsage)
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return snapshot.ConsumedUsage{}, err
	}
	return usage, nil
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
