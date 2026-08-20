// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usagewatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureNotifiesWhenProjectLogChanges(t *testing.T) {
	accountHome := t.TempDir()
	projectsDir := filepath.Join(accountHome, "projects", "demo")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}

	changed := make(chan string, 1)
	manager := NewManager(func(home string) {
		select {
		case changed <- home:
		default:
		}
	})
	t.Cleanup(func() { _ = manager.Close() })

	manager.Ensure(accountHome)

	logPath := filepath.Join(projectsDir, "events.jsonl")
	if err := os.WriteFile(logPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	select {
	case home := <-changed:
		if home != accountHome {
			t.Fatalf("expected account home %q, got %q", accountHome, home)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Changed notification")
	}
}
