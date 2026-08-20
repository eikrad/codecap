// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package dbusapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eikrad/codecap/internal/snapshot"
)

func TestGetSnapshotFillsConsumedUsageForSignedInAccountHome(t *testing.T) {
	accountHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(accountHome, "login"), []byte{}, 0o600); err != nil {
		t.Fatalf("create login sentinel: %v", err)
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
