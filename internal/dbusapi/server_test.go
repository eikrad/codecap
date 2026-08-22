// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package dbusapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eikrad/codecap/internal/allowance"
	"github.com/eikrad/codecap/internal/snapshot"
	"github.com/godbus/dbus/v5/introspect"
)

// snapshotAfterRefresh drives the shape ADR 0011 implies: the first call is
// served from an empty cache and kicks a background refresh, the second reads
// what that refresh stored. GetSnapshot itself never blocks on the network or
// on the filesystem.
func snapshotAfterRefresh(t *testing.T, server *Server, accountHome string) snapshot.Snapshot {
	t.Helper()

	first, derr := server.GetSnapshot(accountHome, "UTC", 1)
	if derr != nil {
		t.Fatalf("GetSnapshot (cold): %v", derr)
	}
	var cold snapshot.Snapshot
	if err := json.Unmarshal([]byte(first), &cold); err != nil {
		t.Fatalf("unmarshal cold snapshot: %v", err)
	}
	if len(cold.Degraded) == 0 {
		t.Fatal("a cold call should say the snapshot is not complete yet")
	}

	// scheduleRefresh registers with the WaitGroup before GetSnapshot returns,
	// so this cannot race.
	server.refreshes.Wait()

	second, derr := server.GetSnapshot(accountHome, "UTC", 1)
	if derr != nil {
		t.Fatalf("GetSnapshot (warm): %v", derr)
	}
	var warm snapshot.Snapshot
	if err := json.Unmarshal([]byte(second), &warm); err != nil {
		t.Fatalf("unmarshal warm snapshot: %v", err)
	}
	return warm
}

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

	server := NewServer(context.Background(), nil, nil)
	defer func() { _ = server.Close() }()
	snap := snapshotAfterRefresh(t, server, accountHome)

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

	server := NewServer(context.Background(), nil, svc)
	defer func() { _ = server.Close() }()
	snap := snapshotAfterRefresh(t, server, accountHome)

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

	server := NewServer(context.Background(), nil, svc)
	defer func() { _ = server.Close() }()
	snap := snapshotAfterRefresh(t, server, accountHome)

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

	server := NewServer(context.Background(), nil, svc)
	defer func() { _ = server.Close() }()
	snap := snapshotAfterRefresh(t, server, accountHome)

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
	server := NewServer(context.Background(), nil, nil)
	defer func() { _ = server.Close() }()
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

func TestIntrospectionMatchesTheDeclaredSignature(t *testing.T) {
	// This XML is the contract the plasmoid's asyncCall signature has to match,
	// and it is hand-written. A wrong type or a reordered argument is invisible
	// everywhere else until a real call fails at runtime.
	node := IntrospectNode()

	var iface *introspect.Interface
	for i := range node.Interfaces {
		if node.Interfaces[i].Name == InterfaceName {
			iface = &node.Interfaces[i]
		}
	}
	if iface == nil {
		t.Fatalf("interface %q is not published", InterfaceName)
	}

	var method *introspect.Method
	for i := range iface.Methods {
		if iface.Methods[i].Name == "GetSnapshot" {
			method = &iface.Methods[i]
		}
	}
	if method == nil {
		t.Fatal("GetSnapshot is not published")
	}

	var in, out []string
	for _, arg := range method.Args {
		if arg.Direction == "out" {
			out = append(out, arg.Type)
			continue
		}
		in = append(in, arg.Type)
	}
	if got := strings.Join(in, ""); got != "ssi" {
		t.Fatalf("input signature is %q, the plasmoid sends ssi", got)
	}
	if got := strings.Join(out, ""); got != "s" {
		t.Fatalf("output signature is %q, want s", got)
	}

	// Reflection over the Go method has to agree with the XML, or godbus
	// rejects calls that introspection says are valid.
	methodType, ok := reflect.TypeOf(&Server{}).MethodByName("GetSnapshot")
	if !ok {
		t.Fatal("Server has no exported GetSnapshot method")
	}
	// Receiver, then accountHome, timezone, weekStart.
	if got := methodType.Type.NumIn(); got != 4 {
		t.Fatalf("GetSnapshot takes %d arguments, the XML declares 3", got-1)
	}
	if got := methodType.Type.In(3).Kind(); got != reflect.Int32 {
		t.Fatalf("weekStart is %v in Go but \"i\" in the XML", got)
	}

	var signals []string
	for _, signal := range iface.Signals {
		signals = append(signals, signal.Name)
	}
	if len(signals) != 1 || signals[0] != "Changed" {
		t.Fatalf("published signals are %v, the plasmoid listens for Changed", signals)
	}
}

func TestGetSnapshotRejectsAnUnusableAccountHome(t *testing.T) {
	server := NewServer(context.Background(), nil, nil)
	defer func() { _ = server.Close() }()

	for _, home := range []string{".claude", "~/.claude", "/", "/home/me/\x00.claude"} {
		if _, derr := server.GetSnapshot(home, "UTC", 1); derr == nil {
			t.Fatalf("GetSnapshot(%q) was accepted", home)
		}
	}
}

func TestGetSnapshotIsSafeUnderConcurrentCalls(t *testing.T) {
	// godbus dispatches every method call in its own goroutine, and the design
	// puts the same applet on the panel, the desktop and the tray, so
	// concurrent calls for one Account Home are the normal case.
	accountHome := t.TempDir()
	creds := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"refresh","expiresAt":%d}}`,
		time.Now().Add(time.Hour).UnixMilli())
	if err := os.WriteFile(filepath.Join(accountHome, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatalf("create credentials: %v", err)
	}
	logDir := filepath.Join(accountHome, "projects", "demo")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}
	stamp := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	line := fmt.Sprintf(`{"type":"assistant","uuid":"conc-1","timestamp":%q,"message":{"model":"claude-sonnet","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`, stamp)
	if err := os.WriteFile(filepath.Join(logDir, "events.jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":10,"resets_at":"2026-08-20T12:00:00Z"}}`))
	}))
	t.Cleanup(usageServer.Close)

	svc := allowance.NewService(allowance.NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = usageServer.Client()
	svc.APIBaseURL = usageServer.URL
	svc.TokenBaseURL = usageServer.URL

	server := NewServer(context.Background(), nil, svc)
	defer func() { _ = server.Close() }()

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload, derr := server.GetSnapshot(accountHome, "UTC", 1)
			if derr != nil {
				t.Errorf("GetSnapshot: %v", derr)
				return
			}
			var snap snapshot.Snapshot
			if err := json.Unmarshal([]byte(payload), &snap); err != nil {
				t.Errorf("unmarshal: %v", err)
			}
		}()
	}
	// The poller resolves the same Account Home from its own goroutine.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = server.pollAccountHome(context.Background(), accountHome)
		}()
	}
	wg.Wait()
}

func TestGetSnapshotDoesNotBlockOnASlowVendor(t *testing.T) {
	accountHome := t.TempDir()
	creds := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"refresh","expiresAt":%d}}`,
		time.Now().Add(time.Hour).UnixMilli())
	if err := os.WriteFile(filepath.Join(accountHome, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatalf("create credentials: %v", err)
	}

	release := make(chan struct{})
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":10,"resets_at":"2026-08-20T12:00:00Z"}}`))
	}))
	t.Cleanup(func() {
		close(release)
		vendor.Close()
	})

	svc := allowance.NewService(allowance.NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = vendor.Client()
	svc.APIBaseURL = vendor.URL
	svc.TokenBaseURL = vendor.URL

	server := NewServer(context.Background(), nil, svc)
	t.Cleanup(func() { _ = server.Close() })

	// ADR 0011 puts polling in the helper. Doing the fetch inline meant a slow
	// or hung vendor froze the widget and could outlast the D-Bus timeout.
	start := time.Now()
	payload, derr := server.GetSnapshot(accountHome, "UTC", 1)
	elapsed := time.Since(start)
	if derr != nil {
		t.Fatalf("GetSnapshot: %v", derr)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("GetSnapshot took %v while the vendor was hanging", elapsed)
	}

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if snap.SchemaVersion != snapshot.SchemaVersion {
		t.Fatalf("schema_version is %d, want %d", snap.SchemaVersion, snapshot.SchemaVersion)
	}
	if !slices.Contains(snap.Degraded, snapshot.DegradedPending) {
		t.Fatalf("a cold snapshot should be marked pending, got %v", snap.Degraded)
	}
}

func TestGetSnapshotServesTheCacheOnceRefreshed(t *testing.T) {
	accountHome := t.TempDir()
	creds := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"refresh","expiresAt":%d}}`,
		time.Now().Add(time.Hour).UnixMilli())
	if err := os.WriteFile(filepath.Join(accountHome, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatalf("create credentials: %v", err)
	}

	var fetches int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetches, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":42,"resets_at":"2026-08-20T12:00:00Z"}}`))
	}))
	t.Cleanup(vendor.Close)

	svc := allowance.NewService(allowance.NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = vendor.Client()
	svc.APIBaseURL = vendor.URL
	svc.TokenBaseURL = vendor.URL

	server := NewServer(context.Background(), nil, svc)
	t.Cleanup(func() { _ = server.Close() })

	snap := snapshotAfterRefresh(t, server, accountHome)
	if snap.SessionAllowance.UsedPercent != 42 {
		t.Fatalf("session allowance: got %v want 42", snap.SessionAllowance.UsedPercent)
	}
	if len(snap.Degraded) != 0 {
		t.Fatalf("a refreshed snapshot should not be degraded, got %v", snap.Degraded)
	}

	after := atomic.LoadInt32(&fetches)
	// Ten more calls inside the TTL must not touch the vendor at all.
	for i := 0; i < 10; i++ {
		if _, derr := server.GetSnapshot(accountHome, "UTC", 1); derr != nil {
			t.Fatalf("GetSnapshot: %v", derr)
		}
	}
	server.refreshes.Wait()
	if got := atomic.LoadInt32(&fetches); got != after {
		t.Fatalf("cached calls triggered %d extra vendor fetches", got-after)
	}
}

func TestFillFromCacheReaggregatesWhenTheWeekStartChanges(t *testing.T) {
	accountHome := t.TempDir()
	server := NewServer(context.Background(), nil, nil)
	t.Cleanup(func() { _ = server.Close() })

	state := server.state(accountHome)
	state.mu.Lock()
	state.key = usageKey{timezone: "UTC", weekStart: 1}
	state.consumed = snapshot.ConsumedUsage{Week: snapshot.ConsumedPeriod{Tokens: 500}}
	state.consumedOK = true
	state.consumedAt = time.Now()
	state.mu.Unlock()

	snap := snapshot.Snapshot{Face: snapshot.FaceReady, AccountHome: accountHome}
	// A different week start is a different aggregation, so the cached value
	// must not be served for it.
	server.fillFromCache(&snap, usageKey{timezone: "UTC", weekStart: 0})

	if snap.ConsumedUsage.Week.Tokens != 0 {
		t.Fatalf("cached usage was reused for a different week start: %+v", snap.ConsumedUsage)
	}
	if !slices.Contains(snap.Degraded, snapshot.DegradedPending) {
		t.Fatalf("expected the snapshot to be marked incomplete, got %v", snap.Degraded)
	}
}

func TestSignedOutBacksThePollerOff(t *testing.T) {
	accountHome := t.TempDir()
	creds := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"refresh","expiresAt":%d}}`,
		time.Now().Add(-time.Hour).UnixMilli())
	if err := os.WriteFile(filepath.Join(accountHome, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatalf("create credentials: %v", err)
	}

	var refreshAttempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth/token" {
			atomic.AddInt32(&refreshAttempts, 1)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	svc := allowance.NewService(allowance.NewLastKnownStore(t.TempDir()))
	svc.HTTPClient = server.Client()
	svc.APIBaseURL = server.URL
	svc.TokenBaseURL = server.URL

	helper := NewServer(context.Background(), nil, svc)
	t.Cleanup(func() { _ = helper.Close() })

	// A rejected grant does not become valid by asking again every minute, and
	// each attempt is a request against the vendor.
	err := helper.pollAccountHome(context.Background(), accountHome)
	if err == nil {
		t.Fatal("a signed-out account must be reported so the poller backs off")
	}

	// The face is still usable and still served.
	state := helper.state(accountHome)
	state.mu.Lock()
	face := state.allowanceFields.Face
	state.mu.Unlock()
	if face != snapshot.FaceSignedOut {
		t.Fatalf("face: got %q want signed_out", face)
	}
	if atomic.LoadInt32(&refreshAttempts) == 0 {
		t.Fatal("expected the refresh to have been attempted once")
	}
}
