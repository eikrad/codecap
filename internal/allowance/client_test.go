// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchUsageReturnsMappedResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/usage" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-ant-oat01-test" {
			t.Fatalf("authorization: %q", got)
		}
		if got := r.Header.Get("anthropic-beta"); got != "oauth-2025-04-20" {
			t.Fatalf("anthropic-beta: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"five_hour": map[string]any{"utilization": 12.0, "resets_at": "2026-08-20T18:00:00Z"},
			"seven_day": map[string]any{"utilization": 40.0, "resets_at": "2026-08-27T18:00:00Z"},
			"extra_usage": map[string]any{
				"is_enabled":    true,
				"monthly_limit": 100.0,
				"used_credits":  0.0,
				"utilization":   0.0,
			},
		})
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.Client(), server.URL)
	got, err := client.FetchUsage("sk-ant-oat01-test")
	if err != nil {
		t.Fatalf("fetch usage: %v", err)
	}
	if got.Session.UsedPercent != 12.0 {
		t.Fatalf("session percent: %v", got.Session.UsedPercent)
	}
	if got.Weekly.UsedPercent != 40.0 {
		t.Fatalf("weekly percent: %v", got.Weekly.UsedPercent)
	}
	if got.UsageCredit != "available" {
		t.Fatalf("usage credit: %q", got.UsageCredit)
	}
}

func TestFetchUsageUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.Client(), server.URL)
	_, err := client.FetchUsage("bad")
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if !IsUnauthorized(err) {
		t.Fatalf("expected unauthorized marker, got %v", err)
	}
}
