// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usagewatch

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	}, 20*time.Millisecond)
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
	manager := NewManager(func(home string) { changed <- home }, 20*time.Millisecond)
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
	manager := NewManager(func(string) {}, 20*time.Millisecond)
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

	manager := NewManager(func(string) {}, 20*time.Millisecond)
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
	manager := NewManager(func(string) {}, 20*time.Millisecond)
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

func TestNotificationsAreCoalesced(t *testing.T) {
	accountHome := t.TempDir()
	logDir := filepath.Join(accountHome, "projects", "demo")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}

	var mu sync.Mutex
	var calls int
	manager := NewManager(func(string) {
		mu.Lock()
		calls++
		mu.Unlock()
	}, 150*time.Millisecond)
	t.Cleanup(func() { _ = manager.Close() })
	manager.Ensure(accountHome)

	// Claude Code appends to a log many times a second. One notification per
	// event made the plasmoid re-read the whole corpus each time.
	logPath := filepath.Join(logDir, "events.jsonl")
	for i := 0; i < 40; i++ {
		if err := os.WriteFile(logPath, []byte(fmt.Sprintf("{\"n\":%d}\n", i)), 0o600); err != nil {
			t.Fatalf("write log: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	got := calls
	mu.Unlock()

	if got == 0 {
		t.Fatal("expected at least one notification")
	}
	if got > 3 {
		t.Fatalf("40 writes produced %d notifications, expected them to coalesce", got)
	}
}
