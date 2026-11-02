// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usage

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/eikrad/codecap/internal/snapshot"
)

const (
	// retention bounds how far back parsed events are kept. The longest window
	// reported is the current month, so 40 days covers it with room for a week
	// that started in the previous one.
	retention = 40 * 24 * time.Hour

	// maxTrackedFiles caps the walk. A real Account Home has tens of logs.
	maxTrackedFiles = 4096

	// maxFileBytes caps one log file.
	maxFileBytes = 512 << 20
)

// event is one assistant message, parsed once and kept until it ages out.
type event struct {
	at               time.Time
	identity         string
	replaceEarlier   bool
	tokens           int64
	priceUSD         float64
	priceUnavailable bool
}

// fileEntry is the parsed form of one log file, valid while the file's size and
// modification time are unchanged.
type fileEntry struct {
	size    int64
	modTime time.Time
	events  []event
}

// Cache turns Consumed Usage from a full re-read of every log into a re-read of
// the files that actually changed.
//
// The aggregation windows move — the Session window is rolling and day, week and
// month boundaries roll over — so running totals cannot be kept. Parsed events
// can be, and re-aggregating them costs no I/O and no JSON decoding. During an
// active Claude Code session exactly one log is being appended to, so a refresh
// re-reads one file instead of all of them.
//
// Rates are fixed for the life of a Cache: prices are computed when an event is
// parsed. Swap the rate table by building a new Cache.
type Cache struct {
	mu    sync.Mutex
	files map[string]*fileEntry
	rates Rates
}

func NewCache(rates Rates) *Cache {
	if len(rates.Cards) == 0 {
		rates = DefaultRates()
	}
	return &Cache{
		files: make(map[string]*fileEntry),
		rates: rates,
	}
}

// Compute aggregates Consumed Usage for an Account Home.
func (c *Cache) Compute(accountHome, timezone string, weekStart int32) (snapshot.ConsumedUsage, error) {
	return c.computeAt(accountHome, timezone, weekStart, time.Now().UTC())
}

func (c *Cache) computeAt(accountHome, timezone string, weekStart int32, now time.Time) (snapshot.ConsumedUsage, error) {
	loc, err := resolveLocation(timezone)
	if err != nil {
		return snapshot.ConsumedUsage{}, err
	}

	files, err := listJSONLFiles(accountHome)
	if err != nil {
		return snapshot.ConsumedUsage{}, err
	}
	if len(files) > maxTrackedFiles {
		return snapshot.ConsumedUsage{}, fmt.Errorf("too many Consumed Usage log files (limit %d)", maxTrackedFiles)
	}
	// Keep duplicate selection and floating-point accumulation deterministic.
	sort.Strings(files)

	// Cached event slices must stay stable until aggregation finishes; a
	// concurrent refresh may prune or replace them.
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := now.Add(-retention)
	events, err := c.refresh(files, cutoff)
	if err != nil {
		return snapshot.ConsumedUsage{}, err
	}

	return aggregate(events, periodsAt(now, loc, weekStart)), nil
}

// refresh re-parses only the files whose size or modification time changed, and
// returns every retained event in file order.
func (c *Cache) refresh(files []string, cutoff time.Time) ([][]event, error) {
	live := make(map[string]struct{}, len(files))
	batches := make([][]event, 0, len(files))

	for _, path := range files {
		live[path] = struct{}{}

		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("stat Consumed Usage log: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() > maxFileBytes {
			return nil, fmt.Errorf("unreadable Consumed Usage log %s: not a bounded regular file", path)
		}

		entry, ok := c.files[path]
		// A file that shrank was truncated or replaced, so its cached events no
		// longer describe it.
		if ok && entry.size == info.Size() && entry.modTime.Equal(info.ModTime()) {
			entry.events = pruneBefore(entry.events, cutoff)
			batches = append(batches, entry.events)
			continue
		}

		parsed, err := parseEvents(path, c.rates, cutoff)
		if err != nil {
			// A partial sum looks authoritative. Preserve the last successful
			// computation at the caller and report this failed refresh instead.
			return nil, fmt.Errorf("read Consumed Usage log %q: %w", path, err)
		}
		c.files[path] = &fileEntry{size: info.Size(), modTime: info.ModTime(), events: parsed}
		batches = append(batches, parsed)
	}

	// Drop files that disappeared, so a long-lived helper does not accumulate
	// events from deleted projects.
	for path := range c.files {
		if _, ok := live[path]; !ok {
			delete(c.files, path)
		}
	}
	return batches, nil
}

func pruneBefore(events []event, cutoff time.Time) []event {
	keep := 0
	for _, ev := range events {
		if !ev.at.Before(cutoff) {
			events[keep] = ev
			keep++
		}
	}
	clear(events[keep:])
	return events[:keep]
}

// aggregate sums retained events into the reported windows, deduplicating by
// identity before adding any period. Parsers explicitly mark request totals
// that may be replaced by a later content block; identities are opaque here.
func aggregate(batches [][]event, windows periods) snapshot.ConsumedUsage {
	var usage snapshot.ConsumedUsage
	indices := make(map[string]int)
	var unique []event
	for _, batch := range batches {
		for _, ev := range batch {
			if ev.identity != "" {
				if index, ok := indices[ev.identity]; ok {
					if ev.replaceEarlier && !ev.at.Before(unique[index].at) {
						unique[index] = ev
					}
					continue
				}
				indices[ev.identity] = len(unique)
			}
			unique = append(unique, ev)
		}
	}

	for _, ev := range unique {
		amount := snapshot.ConsumedPeriod{ListPriceUSD: ev.priceUSD, Tokens: ev.tokens, ListPriceUnavailable: ev.priceUnavailable}
		if inPeriod(ev.at, windows.session) {
			addPeriod(&usage.Session, amount)
		}
		if inPeriod(ev.at, windows.today) {
			addPeriod(&usage.Today, amount)
		}
		if inPeriod(ev.at, windows.week) {
			addPeriod(&usage.Week, amount)
		}
		if inPeriod(ev.at, windows.month) {
			addPeriod(&usage.Month, amount)
		}
	}
	return usage
}
