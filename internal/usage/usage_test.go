// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usage

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/eikrad/codecap/internal/snapshot"
)

func TestComputeAtAggregatesPeriodsByTimezoneAndWeekStart(t *testing.T) {
	accountHome := t.TempDir()
	logDir := filepath.Join(accountHome, "projects", "sample")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	lines := []string{
		`{"type":"assistant","uuid":"a1","timestamp":"2026-08-20T11:00:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":100,"cache_read_input_tokens":50}}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-08-20T01:00:00Z","message":{"model":"claude-opus","usage":{"input_tokens":2000,"output_tokens":1000,"cache_creation_input_tokens":200,"cache_read_input_tokens":100}}}`,
		`{"type":"assistant","uuid":"a3","timestamp":"2026-08-15T10:00:00Z","message":{"model":"claude-haiku","usage":{"input_tokens":3000,"output_tokens":1500,"cache_creation_input_tokens":300,"cache_read_input_tokens":150}}}`,
	}
	logPath := filepath.Join(logDir, "events.jsonl")
	if err := os.WriteFile(logPath, []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	got, err := computeAt(accountHome, "Europe/Copenhagen", 1, now, DefaultRates())
	if err != nil {
		t.Fatalf("compute usage: %v", err)
	}

	if got.Session.Tokens != 1650 {
		t.Fatalf("session tokens mismatch: got %d, want %d", got.Session.Tokens, 1650)
	}
	if got.Today.Tokens != 4950 {
		t.Fatalf("today tokens mismatch: got %d, want %d", got.Today.Tokens, 4950)
	}
	if got.Week.Tokens != 4950 {
		t.Fatalf("week tokens mismatch: got %d, want %d", got.Week.Tokens, 4950)
	}
	if got.Month.Tokens != 9900 {
		t.Fatalf("month tokens mismatch: got %d, want %d", got.Month.Tokens, 9900)
	}

	if got.Session.ListPriceUSD <= 0 {
		t.Fatalf("expected positive session price, got %f", got.Session.ListPriceUSD)
	}
	if got.Month.ListPriceUSD <= got.Week.ListPriceUSD {
		t.Fatalf("month price should exceed week price: month=%f week=%f", got.Month.ListPriceUSD, got.Week.ListPriceUSD)
	}
}

func TestComputeIncludesNestedSubagentLogs(t *testing.T) {
	accountHome := t.TempDir()
	parentDir := filepath.Join(accountHome, "projects", "demo")
	subDir := filepath.Join(parentDir, "conv-1", "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subagents: %v", err)
	}

	parent := `{"type":"assistant","uuid":"parent-1","timestamp":"2026-08-20T11:00:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	sub := `{"type":"assistant","uuid":"sub-1","isSidechain":true,"timestamp":"2026-08-20T11:01:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":200,"output_tokens":25,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	if err := os.WriteFile(filepath.Join(parentDir, "conv-1.jsonl"), []byte(parent+"\n"), 0o600); err != nil {
		t.Fatalf("write parent log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.jsonl"), []byte(sub+"\n"), 0o600); err != nil {
		t.Fatalf("write subagent log: %v", err)
	}

	got, err := computeAt(accountHome, "UTC", 1, time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC), DefaultRates())
	if err != nil {
		t.Fatalf("compute usage: %v", err)
	}
	if got.Session.Tokens != 375 {
		t.Fatalf("expected parent+subagent tokens 375, got %d", got.Session.Tokens)
	}
}

func TestComputeDedupesRepeatedEventUUID(t *testing.T) {
	accountHome := t.TempDir()
	logDir := filepath.Join(accountHome, "projects", "sample")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	line := `{"type":"assistant","uuid":"same","timestamp":"2026-08-20T11:00:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":100,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	if err := os.WriteFile(filepath.Join(logDir, "a.jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "b.jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	got, err := computeAt(accountHome, "UTC", 1, time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC), DefaultRates())
	if err != nil {
		t.Fatalf("compute usage: %v", err)
	}
	if got.Session.Tokens != 100 {
		t.Fatalf("expected uuid-deduped tokens 100, got %d", got.Session.Tokens)
	}
}

func TestAListPriceFollowsTheRateTableItWasGiven(t *testing.T) {
	accountHome := t.TempDir()
	logDir := filepath.Join(accountHome, "projects", "sample")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	line := `{"type":"assistant","uuid":"r1","timestamp":"2026-08-20T11:00:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":1000000,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	if err := os.WriteFile(filepath.Join(logDir, "events.jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	rates := Rates{Cards: []RateCard{{Match: "sonnet", InputUSDPerM: 10.0}}}
	got, err := computeAt(accountHome, "UTC", 1, time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC), rates)
	if err != nil {
		t.Fatalf("compute usage: %v", err)
	}
	if got.Session.ListPriceUSD != 10.0 {
		t.Fatalf("expected custom list price 10, got %f", got.Session.ListPriceUSD)
	}
}

func TestComputeAtReturnsZeroWhenProjectsMissing(t *testing.T) {
	accountHome := t.TempDir()
	got, err := computeAt(accountHome, "UTC", 1, time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC), DefaultRates())
	if err != nil {
		t.Fatalf("compute usage: %v", err)
	}
	if got.Session.Tokens != 0 || got.Today.Tokens != 0 || got.Week.Tokens != 0 || got.Month.Tokens != 0 {
		t.Fatalf("expected zero usage without projects/, got %+v", got)
	}
}

func TestComputeAtWeekBoundaryExcludesPriorWeekEvents(t *testing.T) {
	accountHome := t.TempDir()
	logDir := filepath.Join(accountHome, "projects", "sample")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC) // Thursday
	lines := []string{
		`{"type":"assistant","uuid":"prior-week","timestamp":"2026-08-16T12:00:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":900,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		`{"type":"assistant","uuid":"this-week","timestamp":"2026-08-18T12:00:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":100,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
	}
	if err := os.WriteFile(filepath.Join(logDir, "events.jsonl"), []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	got, err := computeAt(accountHome, "UTC", 1, now, DefaultRates())
	if err != nil {
		t.Fatalf("compute usage: %v", err)
	}
	if got.Week.Tokens != 100 {
		t.Fatalf("week tokens: got %d want 100 (Monday-start week excludes Sunday Aug 16)", got.Week.Tokens)
	}
	if got.Month.Tokens != 1000 {
		t.Fatalf("month tokens: got %d want 1000", got.Month.Tokens)
	}
}

func TestComputeAtIgnoresInvalidLines(t *testing.T) {
	accountHome := t.TempDir()
	logDir := filepath.Join(accountHome, "projects", "sample")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	logPath := filepath.Join(logDir, "events.jsonl")
	lines := []string{
		`not-json`,
		`{"type":"user","timestamp":"2026-08-20T11:00:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":999}}}`,
		`{"type":"assistant","timestamp":"bad-time","message":{"model":"claude-sonnet","usage":{"input_tokens":999}}}`,
	}
	if err := os.WriteFile(logPath, []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	got, err := computeAt(accountHome, "UTC", 1, time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC), DefaultRates())
	if err != nil {
		t.Fatalf("compute usage: %v", err)
	}
	if got.Month.Tokens != 0 {
		t.Fatalf("expected zero tokens, got %d", got.Month.Tokens)
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, line := range lines {
		out += line + "\n"
	}
	return out
}

func TestResolveLocationEmptyMeansHelperLocalZone(t *testing.T) {
	loc, err := resolveLocation("")
	if err != nil {
		t.Fatalf("resolve empty timezone: %v", err)
	}
	if loc != time.Local {
		t.Fatalf("expected time.Local for the empty timezone, got %v", loc)
	}

	loc, err = resolveLocation("  ")
	if err != nil {
		t.Fatalf("resolve blank timezone: %v", err)
	}
	if loc != time.Local {
		t.Fatalf("expected time.Local for a blank timezone, got %v", loc)
	}
}

func TestResolveLocationAcceptsIANAIDsAndRejectsDisplayNames(t *testing.T) {
	if _, err := resolveLocation("Europe/Copenhagen"); err != nil {
		t.Fatalf("resolve IANA id: %v", err)
	}

	// These are what Date.prototype.toString() produces. Treating them as a
	// silent UTC fallback moved every day, week and month boundary for users
	// outside UTC.
	for _, name := range []string{
		"Central European Summer Time",
		"CEST",
		"Coordinated Universal Time",
		"not a zone",
	} {
		if _, err := resolveLocation(name); err == nil {
			t.Fatalf("expected %q to be rejected, got no error", name)
		}
	}
}

func TestComputeAtRejectsUnresolvableTimezone(t *testing.T) {
	accountHome := t.TempDir()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	if _, err := computeAt(accountHome, "Central European Summer Time", 1, now, DefaultRates()); err == nil {
		t.Fatal("expected an unresolvable timezone to be reported, got no error")
	}
}

func TestComputeAtEmptyTimezoneMatchesTheHelperLocalZone(t *testing.T) {
	accountHome := t.TempDir()
	logDir := filepath.Join(accountHome, "projects", "sample")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	line := `{"type":"assistant","uuid":"tz1","timestamp":"2026-08-19T23:30:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":1000,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	if err := os.WriteFile(filepath.Join(logDir, "events.jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	// time.Local is fixed at process start, so this asserts the equivalence
	// rather than a particular zone: an empty argument must behave exactly like
	// naming the zone the helper is running in.
	explicit, err := computeAt(accountHome, time.Local.String(), 1, now, DefaultRates())
	if err != nil {
		t.Fatalf("compute with explicit local zone: %v", err)
	}
	implicit, err := computeAt(accountHome, "", 1, now, DefaultRates())
	if err != nil {
		t.Fatalf("compute with empty zone: %v", err)
	}
	if implicit != explicit {
		t.Fatalf("empty timezone should equal %q: got %+v want %+v",
			time.Local.String(), implicit, explicit)
	}
}

func TestComputeAtDayBoundaryFollowsTheGivenZone(t *testing.T) {
	accountHome := t.TempDir()
	logDir := filepath.Join(accountHome, "projects", "sample")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	// 23:30 UTC on the 19th is 01:30 on the 20th in Copenhagen, so the two
	// zones disagree about which day this belongs to. Falling back to UTC when
	// a timezone cannot be resolved is what made this invisible.
	line := `{"type":"assistant","uuid":"tz2","timestamp":"2026-08-19T23:30:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":1000,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	if err := os.WriteFile(filepath.Join(logDir, "events.jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	utc, err := computeAt(accountHome, "UTC", 1, now, DefaultRates())
	if err != nil {
		t.Fatalf("compute in UTC: %v", err)
	}
	if utc.Today.Tokens != 0 {
		t.Fatalf("in UTC the event falls on the previous day, got %d tokens today", utc.Today.Tokens)
	}

	copenhagen, err := computeAt(accountHome, "Europe/Copenhagen", 1, now, DefaultRates())
	if err != nil {
		t.Skipf("zoneinfo unavailable: %v", err)
	}
	if copenhagen.Today.Tokens != 1000 {
		t.Fatalf("in Europe/Copenhagen the event falls on today, got %d tokens", copenhagen.Today.Tokens)
	}
}

func TestComputeAtSkipsAFifoNamedLikeALog(t *testing.T) {
	accountHome := t.TempDir()
	logDir := filepath.Join(accountHome, "projects", "sample")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	line := `{"type":"assistant","uuid":"ok1","timestamp":"2026-08-20T11:00:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":100,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	if err := os.WriteFile(filepath.Join(logDir, "real.jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	// os.Open on a FIFO blocks until a writer appears. One of these in
	// projects/ parked a D-Bus handler goroutine forever, and every later call
	// leaked another one.
	if err := syscall.Mkfifo(filepath.Join(logDir, "trap.jsonl"), 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}

	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	done := make(chan snapshot.ConsumedUsage, 1)
	go func() {
		got, err := computeAt(accountHome, "UTC", 1, now, DefaultRates())
		if err != nil {
			t.Errorf("compute: %v", err)
		}
		done <- got
	}()

	select {
	case got := <-done:
		if got.Today.Tokens != 100 {
			t.Fatalf("the real log should still be counted, got %d tokens", got.Today.Tokens)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("computeAt blocked on a FIFO named *.jsonl")
	}
}

func TestComputeAtSkipsASymlinkedLog(t *testing.T) {
	accountHome := t.TempDir()
	logDir := filepath.Join(accountHome, "projects", "sample")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "elsewhere.jsonl")
	line := `{"type":"assistant","uuid":"out1","timestamp":"2026-08-20T11:00:00Z","message":{"model":"claude-sonnet","usage":{"input_tokens":999,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	if err := os.WriteFile(outside, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write outside log: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(logDir, "linked.jsonl")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	got, err := computeAt(accountHome, "UTC", 1, now, DefaultRates())
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if got.Today.Tokens != 0 {
		t.Fatalf("a symlink out of the Account Home must not be read, got %d tokens", got.Today.Tokens)
	}
}

// A one-hour cache entry costs 2x base input to write; a five-minute entry
// costs 1.25x. The logs report the split, and pricing the whole line at the
// five-minute rate understated cache writes by about half on the desktop this
// was found on — 86% of its writes were one-hour.
func TestEstimateListPriceUSDPricesCacheWritesByLifetime(t *testing.T) {
	rates := DefaultRates()
	const million = 1_000_000

	oneHourOnly := tokenUsage{
		CacheCreationInputTokens: million,
		CacheCreation:            &cacheCreation{Ephemeral1hInputTokens: million},
	}
	fiveMinuteOnly := tokenUsage{
		CacheCreationInputTokens: million,
		CacheCreation:            &cacheCreation{Ephemeral5mInputTokens: million},
	}

	gotOneHour := estimateListPriceUSD(rates, "claude-opus-4-1", oneHourOnly)
	gotFiveMinute := estimateListPriceUSD(rates, "claude-opus-4-1", fiveMinuteOnly)

	if gotOneHour != 30.0 {
		t.Errorf("one-hour write of 1M tokens = %v, want 30.0", gotOneHour)
	}
	if gotFiveMinute != 18.75 {
		t.Errorf("five-minute write of 1M tokens = %v, want 18.75", gotFiveMinute)
	}
	if gotOneHour <= gotFiveMinute {
		t.Errorf("a one-hour write must cost more than a five-minute one: %v vs %v", gotOneHour, gotFiveMinute)
	}

	// The real shape: both buckets populated in one event.
	mixed := tokenUsage{
		CacheCreationInputTokens: million,
		CacheCreation: &cacheCreation{
			Ephemeral1hInputTokens: 800_000,
			Ephemeral5mInputTokens: 200_000,
		},
	}
	want := 0.8*30.0 + 0.2*18.75
	if got := estimateListPriceUSD(rates, "claude-opus-4-1", mixed); got != want {
		t.Errorf("mixed write = %v, want %v", got, want)
	}
}

func TestCacheWriteSplitHandlesPayloadsWithoutTheSplit(t *testing.T) {
	// A Claude Code old enough not to report cache_creation. The lifetime is
	// genuinely unknown, so it lands in the cheaper bucket — the behaviour this
	// code had before the split existed.
	legacy := tokenUsage{CacheCreationInputTokens: 500}
	oneHour, fiveMinute := legacy.cacheWriteSplit()
	if oneHour != 0 || fiveMinute != 500 {
		t.Errorf("legacy payload split = (%d, %d), want (0, 500)", oneHour, fiveMinute)
	}

	// A total larger than its own split means the payload grew a bucket this
	// code does not know about. The remainder must still be counted.
	partial := tokenUsage{
		CacheCreationInputTokens: 1000,
		CacheCreation:            &cacheCreation{Ephemeral1hInputTokens: 600, Ephemeral5mInputTokens: 300},
	}
	oneHour, fiveMinute = partial.cacheWriteSplit()
	if oneHour != 600 || fiveMinute != 400 {
		t.Errorf("partial split = (%d, %d), want (600, 400) — the 100 unaccounted tokens must not vanish", oneHour, fiveMinute)
	}

	// Tokens reported only in the split, with no total, still price.
	noTotal := tokenUsage{CacheCreation: &cacheCreation{Ephemeral1hInputTokens: 700}}
	oneHour, fiveMinute = noTotal.cacheWriteSplit()
	if oneHour != 700 || fiveMinute != 0 {
		t.Errorf("split without total = (%d, %d), want (700, 0)", oneHour, fiveMinute)
	}
}
