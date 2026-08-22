// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestComputeWithRatesUsesCustomListPriceTable(t *testing.T) {
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
