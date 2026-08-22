// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usage

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildAccountHome writes a log corpus shaped like a real ~/.claude/projects.
func buildAccountHome(tb testing.TB, files, perFile int) string {
	tb.Helper()
	home := tb.TempDir()
	base := time.Now().UTC().Add(-30 * 24 * time.Hour)
	filler := strings.Repeat("x", 400)

	for f := 0; f < files; f++ {
		dir := filepath.Join(home, "projects", fmt.Sprintf("-home-user-proj%02d", f))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatalf("mkdir: %v", err)
		}
		var buf strings.Builder
		for i := 0; i < perFile; i++ {
			at := base.Add(time.Duration(rand.Int63n(int64(30 * 24 * time.Hour))))
			fmt.Fprintf(&buf,
				`{"type":"assistant","uuid":"%d-%d","timestamp":%q,"message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":1200,"output_tokens":800,"cache_creation_input_tokens":15000,"cache_read_input_tokens":120000},"content":[{"type":"text","text":%q}]}}`+"\n",
				f, i, at.Format(time.RFC3339Nano), filler)
		}
		if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte(buf.String()), 0o600); err != nil {
			tb.Fatalf("write log: %v", err)
		}
	}
	return home
}

// BenchmarkComputeCold is the pre-cache behaviour: every refresh re-read and
// re-decoded every log. Measured at 2.54 s and 825 MB per call on a 311 MB
// Account Home in docs/audit-2026-08-22.md.
func BenchmarkComputeCold(b *testing.B) {
	home := buildAccountHome(b, 40, 1500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := NewCache(DefaultRates()).Compute(home, "UTC", 1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkComputeWarm is a refresh with nothing changed, which is what a
// GetSnapshot or an Allowance poll actually hits.
func BenchmarkComputeWarm(b *testing.B) {
	home := buildAccountHome(b, 40, 1500)
	cache := NewCache(DefaultRates())
	if _, err := cache.Compute(home, "UTC", 1); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := cache.Compute(home, "UTC", 1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkComputeOneFileAppended is an active Claude Code session: one log is
// being written to, the rest are untouched.
func BenchmarkComputeOneFileAppended(b *testing.B) {
	home := buildAccountHome(b, 40, 1500)
	cache := NewCache(DefaultRates())
	if _, err := cache.Compute(home, "UTC", 1); err != nil {
		b.Fatal(err)
	}
	active := filepath.Join(home, "projects", "-home-user-proj00", "session.jsonl")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		file, err := os.OpenFile(active, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = fmt.Fprintf(file, `{"type":"assistant","uuid":"live-%d","timestamp":%q,"message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`+"\n",
			i, time.Now().UTC().Format(time.RFC3339Nano))
		_ = file.Close()
		b.StartTimer()

		if _, err := cache.Compute(home, "UTC", 1); err != nil {
			b.Fatal(err)
		}
	}
}
