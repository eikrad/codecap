// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eikrad/codecap/internal/snapshot"
)

func TestFromUsagePayloadMapsSessionWeeklyAndUsageCredit(t *testing.T) {
	payload := []byte(`{
		"five_hour": {"utilization": 35.5, "resets_at": "2026-08-20T15:00:00Z"},
		"seven_day": {"utilization": 14.0, "resets_at": "2026-08-27T12:00:00Z"},
		"extra_usage": {"is_enabled": true, "monthly_limit": 100, "used_credits": 12.5, "utilization": 12.5}
	}`)

	got, err := FromUsagePayload(payload)
	if err != nil {
		t.Fatalf("map payload: %v", err)
	}

	if got.Session.UsedPercent != 35.5 {
		t.Fatalf("session used_percent: got %v want 35.5", got.Session.UsedPercent)
	}
	wantSessionReset := time.Date(2026, 8, 20, 15, 0, 0, 0, time.UTC).Unix()
	if got.Session.ResetsAt != wantSessionReset {
		t.Fatalf("session resets_at: got %d want %d", got.Session.ResetsAt, wantSessionReset)
	}
	if got.Session.Stale {
		t.Fatal("fresh map should not be stale")
	}

	if got.Weekly.UsedPercent != 14.0 {
		t.Fatalf("weekly used_percent: got %v want 14.0", got.Weekly.UsedPercent)
	}
	wantWeeklyReset := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC).Unix()
	if got.Weekly.ResetsAt != wantWeeklyReset {
		t.Fatalf("weekly resets_at: got %d want %d", got.Weekly.ResetsAt, wantWeeklyReset)
	}

	if got.UsageCredit != "enabled" {
		t.Fatalf("usage_credit: got %q want enabled", got.UsageCredit)
	}
}

func TestFromUsagePayloadMapsUsageCreditStates(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{
			name: "disabled",
			json: `{"five_hour":null,"seven_day":null,"extra_usage":{"is_enabled":false}}`,
			want: "none",
		},
		{
			name: "available",
			json: `{"extra_usage":{"is_enabled":true,"monthly_limit":100,"used_credits":0,"utilization":0}}`,
			want: "available",
		},
		{
			name: "exhausted",
			json: `{"extra_usage":{"is_enabled":true,"monthly_limit":100,"used_credits":100,"utilization":100}}`,
			want: "exhausted",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FromUsagePayload([]byte(tc.json))
			if err != nil {
				t.Fatalf("map payload: %v", err)
			}
			if got.UsageCredit != tc.want {
				t.Fatalf("usage_credit: got %q want %q", got.UsageCredit, tc.want)
			}
		})
	}
}

func TestFromUsagePayloadMapsNullBucketsToEmptyWindows(t *testing.T) {
	payload := []byte(`{
		"five_hour": null,
		"seven_day": null,
		"extra_usage": {"is_enabled": false}
	}`)

	got, err := FromUsagePayload(payload)
	if err != nil {
		t.Fatalf("map payload: %v", err)
	}
	if got.Session.UsedPercent != 0 || got.Session.ResetsAt != 0 {
		t.Fatalf("session window: got %+v want empty", got.Session)
	}
	if got.Weekly.UsedPercent != 0 || got.Weekly.ResetsAt != 0 {
		t.Fatalf("weekly window: got %+v want empty", got.Weekly)
	}
	if got.UsageCredit != "none" {
		t.Fatalf("usage_credit: got %q want none", got.UsageCredit)
	}
}

func TestFromUsagePayloadRejectsInvalidJSON(t *testing.T) {
	if _, err := FromUsagePayload([]byte(`not-json`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// The Account Home behind the reported bug: the vendor UI showed
// "Usage credits $4.04 of $4.00", the widget showed only the word "exhausted".
// 4.04 of 4.00 is 101%, so the status was right all along — what was missing
// were the amounts it was derived from.
func TestUsageCreditSpendKeepsTheAmounts(t *testing.T) {
	cases := []struct {
		name      string
		json      string
		wantUsed  float64
		wantLimit float64
	}{
		{
			name:      "over the ceiling",
			json:      `{"extra_usage":{"is_enabled":true,"monthly_limit":4.00,"used_credits":4.04}}`,
			wantUsed:  4.04,
			wantLimit: 4.00,
		},
		{
			name:      "part way through",
			json:      `{"extra_usage":{"is_enabled":true,"monthly_limit":20,"used_credits":5.5}}`,
			wantUsed:  5.5,
			wantLimit: 20,
		},
		{
			// A ceiling that is not in force must not be reported as headroom.
			name:      "disabled reports nothing",
			json:      `{"extra_usage":{"is_enabled":false,"monthly_limit":20,"used_credits":5}}`,
			wantUsed:  0,
			wantLimit: 0,
		},
		{
			name:      "absent block",
			json:      `{"five_hour":null}`,
			wantUsed:  0,
			wantLimit: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FromUsagePayload([]byte(tc.json))
			if err != nil {
				t.Fatalf("FromUsagePayload: %v", err)
			}
			if got.UsageCreditSpend.UsedUSD != tc.wantUsed {
				t.Errorf("used = %v, want %v", got.UsageCreditSpend.UsedUSD, tc.wantUsed)
			}
			if got.UsageCreditSpend.LimitUSD != tc.wantLimit {
				t.Errorf("limit = %v, want %v", got.UsageCreditSpend.LimitUSD, tc.wantLimit)
			}
		})
	}
}

// The reason the amounts were added as a field instead of reshaping
// UsageCredit: a cache written by an older helper has to keep working, or an
// offline desktop drops from Last-Known to Unknown Allowance on upgrade.
func TestLastKnownReadsACachePredatingUsageCreditSpend(t *testing.T) {
	dir := t.TempDir()
	store := NewLastKnownStore(dir)
	accountHome := "/home/someone/.claude"

	old := `{
		"account_home": "` + accountHome + `",
		"fetched_at": 1787756882,
		"session_allowance": {"used_percent": 37, "resets_at": 1787771400, "stale": false},
		"weekly_allowance": {"used_percent": 48, "resets_at": 1788156000, "stale": false},
		"usage_credit": "exhausted"
	}`
	path := store.pathFor(accountHome)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatalf("write legacy cache: %v", err)
	}

	cached, ok, err := store.Load(accountHome, time.Unix(1787756900, 0).UTC())
	if err != nil {
		t.Fatalf("Load on a pre-upgrade cache: %v", err)
	}
	if !ok {
		t.Fatal("pre-upgrade cache was discarded; offline would fall back to Unknown Allowance")
	}
	if cached.UsageCredit != "exhausted" {
		t.Errorf("usage credit = %q, want %q", cached.UsageCredit, "exhausted")
	}
	if cached.UsageCreditSpend != (snapshot.UsageCreditSpend{}) {
		t.Errorf("missing amounts should read as zero, got %+v", cached.UsageCreditSpend)
	}
}
