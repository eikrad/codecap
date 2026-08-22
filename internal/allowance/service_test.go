// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"context"
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

	got, err := svc.Resolve(context.Background(), accountHome)
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

func TestServiceResolveSignedOutWhenRefreshRejected(t *testing.T) {
	accountHome := t.TempDir()
	writeCreds(t, accountHome, 1)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth/token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(tokenServer.Close)

	svc := NewService(NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = tokenServer.Client()
	svc.APIBaseURL = tokenServer.URL
	svc.TokenBaseURL = tokenServer.URL
	svc.Now = func() time.Time { return time.Unix(10, 0).UTC() }

	got, err := svc.Resolve(context.Background(), accountHome)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Face != snapshot.FaceSignedOut {
		t.Fatalf("face: got %q want signed_out", got.Face)
	}
	if got.UsageCredit != "none" {
		t.Fatalf("usage credit: got %q want none", got.UsageCredit)
	}
}

func TestServiceResolveSignedOutWhenUnauthorizedRefreshRejected(t *testing.T) {
	accountHome := t.TempDir()
	writeCreds(t, accountHome, time.Now().Add(time.Hour).UnixMilli())

	usageCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/oauth/usage":
			usageCalls++
			w.WriteHeader(http.StatusUnauthorized)
		case "/v1/oauth/token":
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	svc := NewService(NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = server.Client()
	svc.APIBaseURL = server.URL
	svc.TokenBaseURL = server.URL

	got, err := svc.Resolve(context.Background(), accountHome)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if usageCalls == 0 {
		t.Fatal("expected usage fetch before refresh attempt")
	}
	if got.Face != snapshot.FaceSignedOut {
		t.Fatalf("face: got %q want signed_out", got.Face)
	}
}

func TestServiceResolveUnknownAllowanceWhenFetchFailsWithoutLastKnownCache(t *testing.T) {
	accountHome := t.TempDir()
	writeCreds(t, accountHome, time.Now().Add(time.Hour).UnixMilli())

	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(usageServer.Close)

	svc := NewService(NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = usageServer.Client()
	svc.APIBaseURL = usageServer.URL
	svc.TokenBaseURL = usageServer.URL

	got, err := svc.Resolve(context.Background(), accountHome)
	// The face is still usable, but the reason is reported rather than dropped:
	// the caller cannot otherwise tell "no Allowance" from "could not fetch it".
	if err == nil {
		t.Fatal("expected the fetch failure to be reported")
	}
	if got.Face != snapshot.FaceUnknownAllowance {
		t.Fatalf("face: got %q want unknown_allowance", got.Face)
	}
	if got.UsageCredit != "none" {
		t.Fatalf("usage credit: got %q want none", got.UsageCredit)
	}
}

func TestServiceResolveDoesNotBurnRefreshGrantOnTransientStatus(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			accountHome := t.TempDir()
			writeCreds(t, accountHome, time.Now().Add(time.Hour).UnixMilli())

			var refreshes int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/oauth/token" {
					refreshes++
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"access_token":"new","refresh_token":"new-refresh","expires_in":3600}`))
					return
				}
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(status)
			}))
			t.Cleanup(server.Close)

			svc := NewService(NewLastKnownStore(t.TempDir()))
			svc.HTTPClient = server.Client()
			svc.APIBaseURL = server.URL
			svc.TokenBaseURL = server.URL

			got, err := svc.Resolve(context.Background(), accountHome)
			if err == nil {
				t.Fatal("expected a transient failure to be reported")
			}
			// A 403 from rate limiting used to be read as "the token is bad",
			// so every poll tick rotated the refresh grant.
			if refreshes != 0 {
				t.Fatalf("status %d must not force a refresh, got %d", status, refreshes)
			}
			if got.Face != snapshot.FaceUnknownAllowance {
				t.Fatalf("face: got %q want unknown_allowance", got.Face)
			}
			if wait, ok := RetryAfter(err); !ok || wait != 2*time.Minute {
				t.Fatalf("Retry-After: got %v (%v) want 2m", wait, ok)
			}
		})
	}
}

func TestServiceResolveRefreshesOnUnauthorized(t *testing.T) {
	accountHome := t.TempDir()
	writeCreds(t, accountHome, time.Now().Add(time.Hour).UnixMilli())

	var refreshes, usageCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth/token" {
			refreshes++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"access_token":"new","refresh_token":"new-refresh","expires_in":3600}`))
			return
		}
		usageCalls++
		if usageCalls == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":10,"resets_at":"2026-08-20T12:00:00Z"}}`))
	}))
	t.Cleanup(server.Close)

	svc := NewService(NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = server.Client()
	svc.APIBaseURL = server.URL
	svc.TokenBaseURL = server.URL

	got, err := svc.Resolve(context.Background(), accountHome)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if refreshes != 1 {
		t.Fatalf("401 should force exactly one refresh, got %d", refreshes)
	}
	if got.Face != snapshot.FaceReady {
		t.Fatalf("face: got %q want ready", got.Face)
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

	got, err := svc.Resolve(context.Background(), accountHome)
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
