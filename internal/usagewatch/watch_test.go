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

func TestEnsureNotifiesForADirectoryCreatedAfterTheWatchStarts(t *testing.T) {
	accountHome := t.TempDir()
	projects := filepath.Join(accountHome, "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}

	changed := make(chan string, 16)
	manager := NewManager(func(home string) { changed <- home })
	t.Cleanup(func() { _ = manager.Close() })
	manager.Ensure(accountHome)

	// This is the normal case when Claude Code starts a new project, and it is
	// the whole point of the recursive watch.
	newProject := filepath.Join(projects, "fresh")
	if err := os.MkdirAll(newProject, 0o755); err != nil {
		t.Fatalf("mkdir new project: %v", err)
	}
	// Drain the event for the directory itself.
	waitForChange(t, changed)

	if err := os.WriteFile(filepath.Join(newProject, "events.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	if got := waitForChange(t, changed); got != accountHome {
		t.Fatalf("notified for %q, want %q", got, accountHome)
	}
}

func waitForChange(t *testing.T, changed <-chan string) string {
	t.Helper()
	select {
	case home := <-changed:
		return home
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a change notification")
		return ""
	}
}

func TestEnsureIsBoundedAndEvictsTheOldest(t *testing.T) {
	manager := NewManager(func(string) {})
	t.Cleanup(func() { _ = manager.Close() })

	homes := make([]string, 0, MaxWatchers+3)
	for i := 0; i < MaxWatchers+3; i++ {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, "projects"), 0o755); err != nil {
			t.Fatalf("mkdir projects: %v", err)
		}
		homes = append(homes, home)
		manager.Ensure(home)
	}

	manager.mu.Lock()
	watching := len(manager.watchers)
	_, oldestStillThere := manager.watchers[homes[0]]
	_, newestThere := manager.watchers[homes[len(homes)-1]]
	manager.mu.Unlock()

	// inotify instances are a per-user resource: exhausting them breaks file
	// watching for the whole desktop session, not just this helper.
	if watching > MaxWatchers {
		t.Fatalf("watching %d Account Homes, cap is %d", watching, MaxWatchers)
	}
	if oldestStillThere {
		t.Fatal("expected the least recently requested watch to be evicted")
	}
	if !newestThere {
		t.Fatal("expected the most recent watch to be kept")
	}
}

func TestForgetStopsWatching(t *testing.T) {
	accountHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(accountHome, "projects"), 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}

	manager := NewManager(func(string) {})
	t.Cleanup(func() { _ = manager.Close() })
	manager.Ensure(accountHome)
	manager.Forget(accountHome)

	manager.mu.Lock()
	_, ok := manager.watchers[accountHome]
	manager.mu.Unlock()
	if ok {
		t.Fatal("Forget should drop the watch")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	accountHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(accountHome, "projects"), 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}
	manager := NewManager(func(string) {})
	manager.Ensure(accountHome)

	if err := manager.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestIsUnderDirRejectsEscapes(t *testing.T) {
	cases := map[string]bool{
		"/a/b/c.jsonl":   true,
		"/a":             true,
		"/a/../b/c":      false,
		"/other/c.jsonl": false,
	}
	for path, want := range cases {
		if got := isUnderDir(path, "/a"); got != want {
			t.Fatalf("isUnderDir(%q, \"/a\") = %v, want %v", path, got, want)
		}
	}
}
