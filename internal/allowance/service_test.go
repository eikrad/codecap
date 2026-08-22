// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eikrad/codecap/internal/snapshot"
)

func TestServiceResolveReadyOnLiveFetch(t *testing.T) {
	accountHome := t.TempDir()
	writeCreds(t, accountHome, time.Now().Add(time.Hour).UnixMilli())

	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"five_hour": map[string]any{"utilization": 8.0, "resets_at": "2026-08-20T20:00:00Z"},
			"seven_day": map[string]any{"utilization": 21.0, "resets_at": "2026-08-27T20:00:00Z"},
			"extra_usage": map[string]any{
				"is_enabled": false,
			},
		})
	}))
	t.Cleanup(usageServer.Close)

	svc := NewService(NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = usageServer.Client()
	svc.APIBaseURL = usageServer.URL
	svc.TokenBaseURL = usageServer.URL

	got, err := svc.Resolve(accountHome)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Face != snapshot.FaceReady {
		t.Fatalf("face: got %q", got.Face)
	}
	if got.SessionAllowance.UsedPercent != 8.0 {
		t.Fatalf("session: %+v", got.SessionAllowance)
	}
	if got.UsageCredit != "none" {
		t.Fatalf("usage credit: %q", got.UsageCredit)
	}
}

func TestServiceResolveUsesLastKnownWhenFetchFails(t *testing.T) {
	accountHome := t.TempDir()
	writeCreds(t, accountHome, time.Now().Add(time.Hour).UnixMilli())
	cache := NewLastKnownStore(t.TempDir())
	if err := cache.Save(accountHome, CachedAllowance{
		FetchedAt:   time.Now().Unix(),
		Session:     snapshotWindow(33, 1770000000),
		Weekly:      snapshotWindow(44, 1771000000),
		UsageCredit: "enabled",
	}); err != nil {
		t.Fatalf("seed last-known: %v", err)
	}

	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(usageServer.Close)

	svc := NewService(cache)
	svc.HTTPClient = usageServer.Client()
	svc.APIBaseURL = usageServer.URL
	svc.TokenBaseURL = usageServer.URL

	got, err := svc.Resolve(accountHome)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Face != snapshot.FaceReady {
		t.Fatalf("face: got %q", got.Face)
	}
	if !got.SessionAllowance.Stale || got.SessionAllowance.UsedPercent != 33 {
		t.Fatalf("expected stale last-known session, got %+v", got.SessionAllowance)
	}
}

func writeCreds(t *testing.T, accountHome string, expiresAtMs int64) {
	t.Helper()
	payload := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":      "sk-ant-oat01-test",
			"refreshToken":     "sk-ant-ort01-test",
			"expiresAt":        expiresAtMs,
			"subscriptionType": "pro",
			"rateLimitTier":    "default_raven",
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	if err := os.WriteFile(filepath.Join(accountHome, ".credentials.json"), data, 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
}

func snapshotWindow(used float64, resetsAt int64) snapshot.AllowanceWindow {
	return snapshot.AllowanceWindow{UsedPercent: used, ResetsAt: resetsAt}
}
