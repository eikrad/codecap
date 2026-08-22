// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package dbusapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eikrad/codecap/internal/allowance"
	"github.com/eikrad/codecap/internal/snapshot"
)

func TestGetSnapshotFillsConsumedUsageForSignedInAccountHome(t *testing.T) {
	accountHome := t.TempDir()
	creds := `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"refresh","expiresAt":9999999999999}}`
	if err := os.WriteFile(filepath.Join(accountHome, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatalf("create credentials: %v", err)
	}

	logDir := filepath.Join(accountHome, "projects", "demo")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}
	stamp := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	line := fmt.Sprintf(`{"type":"assistant","uuid":"snap-1","timestamp":%q,"message":{"model":"claude-sonnet","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`, stamp)
	if err := os.WriteFile(filepath.Join(logDir, "events.jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	server := &Server{}
	payload, derr := server.GetSnapshot(accountHome, "UTC", 1)
	if derr != nil {
		t.Fatalf("GetSnapshot failed: %v", derr)
	}

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.Face != snapshot.FaceUnknownAllowance {
		t.Fatalf("expected face %q, got %q", snapshot.FaceUnknownAllowance, snap.Face)
	}
	if snap.ConsumedUsage.Session.Tokens != 150 {
		t.Fatalf("expected session tokens 150, got %+v", snap.ConsumedUsage)
	}
	if snap.FetchedAt == 0 {
		t.Fatal("expected fetched_at to be set")
	}
}

func TestGetSnapshotFillsAllowanceWhenVendorFetchSucceeds(t *testing.T) {
	accountHome := t.TempDir()
	creds := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"refresh","expiresAt":%d}}`, time.Now().Add(time.Hour).UnixMilli())
	if err := os.WriteFile(filepath.Join(accountHome, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatalf("create credentials: %v", err)
	}

	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"five_hour": map[string]any{"utilization": 19.0, "resets_at": "2026-08-20T22:00:00Z"},
			"seven_day": map[string]any{"utilization": 51.0, "resets_at": "2026-08-27T22:00:00Z"},
			"extra_usage": map[string]any{
				"is_enabled":    true,
				"utilization":   5.0,
				"used_credits":  5.0,
				"monthly_limit": 100.0,
			},
		})
	}))
	t.Cleanup(usageServer.Close)

	svc := allowance.NewService(allowance.NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = usageServer.Client()
	svc.APIBaseURL = usageServer.URL
	svc.TokenBaseURL = usageServer.URL

	server := &Server{allowance: svc}
	payload, derr := server.GetSnapshot(accountHome, "UTC", 1)
	if derr != nil {
		t.Fatalf("GetSnapshot failed: %v", derr)
	}

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.Face != snapshot.FaceReady {
		t.Fatalf("expected ready, got %q", snap.Face)
	}
	if snap.SessionAllowance.UsedPercent != 19.0 {
		t.Fatalf("session allowance: %+v", snap.SessionAllowance)
	}
	if snap.WeeklyAllowance.UsedPercent != 51.0 {
		t.Fatalf("weekly allowance: %+v", snap.WeeklyAllowance)
	}
	if snap.UsageCredit != "enabled" {
		t.Fatalf("usage credit: %q", snap.UsageCredit)
	}
}

func TestGetSnapshotSignedOutWhenRefreshRejected(t *testing.T) {
	accountHome := t.TempDir()
	creds := `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"refresh","expiresAt":1}}`
	if err := os.WriteFile(filepath.Join(accountHome, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatalf("create credentials: %v", err)
	}

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth/token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(tokenServer.Close)

	svc := allowance.NewService(allowance.NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = tokenServer.Client()
	svc.APIBaseURL = tokenServer.URL
	svc.TokenBaseURL = tokenServer.URL
	svc.Now = func() time.Time { return time.Unix(10, 0).UTC() }

	server := &Server{allowance: svc}
	payload, derr := server.GetSnapshot(accountHome, "UTC", 1)
	if derr != nil {
		t.Fatalf("GetSnapshot failed: %v", derr)
	}

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.Face != snapshot.FaceSignedOut {
		t.Fatalf("expected signed_out, got %q", snap.Face)
	}
	if snap.ConsumedUsage.Session.Tokens != 0 {
		t.Fatalf("signed out should not fill consumed usage, got %+v", snap.ConsumedUsage)
	}
}

func TestGetSnapshotReadyIncludesAllowanceAndConsumedUsage(t *testing.T) {
	accountHome := t.TempDir()
	creds := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"refresh","expiresAt":%d}}`, time.Now().Add(time.Hour).UnixMilli())
	if err := os.WriteFile(filepath.Join(accountHome, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatalf("create credentials: %v", err)
	}

	logDir := filepath.Join(accountHome, "projects", "demo")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}
	stamp := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	line := fmt.Sprintf(`{"type":"assistant","uuid":"ready-both","timestamp":%q,"message":{"model":"claude-sonnet","usage":{"input_tokens":80,"output_tokens":20,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`, stamp)
	if err := os.WriteFile(filepath.Join(logDir, "events.jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"five_hour":   map[string]any{"utilization": 42.0, "resets_at": "2026-08-20T22:00:00Z"},
			"seven_day":   map[string]any{"utilization": 11.0, "resets_at": "2026-08-27T22:00:00Z"},
			"extra_usage": map[string]any{"is_enabled": false},
		})
	}))
	t.Cleanup(usageServer.Close)

	svc := allowance.NewService(allowance.NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = usageServer.Client()
	svc.APIBaseURL = usageServer.URL
	svc.TokenBaseURL = usageServer.URL

	server := &Server{allowance: svc}
	payload, derr := server.GetSnapshot(accountHome, "UTC", 1)
	if derr != nil {
		t.Fatalf("GetSnapshot failed: %v", derr)
	}

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.Face != snapshot.FaceReady {
		t.Fatalf("expected ready, got %q", snap.Face)
	}
	if snap.SessionAllowance.UsedPercent != 42.0 {
		t.Fatalf("session allowance: %+v", snap.SessionAllowance)
	}
	if snap.ConsumedUsage.Session.Tokens != 100 {
		t.Fatalf("consumed usage session: %+v", snap.ConsumedUsage.Session)
	}
	if snap.FetchedAt == 0 {
		t.Fatal("expected fetched_at to be set")
	}
}

func TestGetSnapshotLeavesConsumedUsageEmptyWhenUnbound(t *testing.T) {
	server := &Server{}
	payload, derr := server.GetSnapshot("   ", "UTC", 1)
	if derr != nil {
		t.Fatalf("GetSnapshot failed: %v", derr)
	}

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.Face != snapshot.FaceUnbound {
		t.Fatalf("expected unbound, got %q", snap.Face)
	}
	if snap.ConsumedUsage.Month.Tokens != 0 {
		t.Fatalf("expected empty consumed usage, got %+v", snap.ConsumedUsage)
	}
}
