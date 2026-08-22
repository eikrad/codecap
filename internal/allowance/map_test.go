// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"testing"
	"time"
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
