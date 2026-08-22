// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usagewatch

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// MaxWatchers bounds how many Account Homes are watched at once.
//
// Each one is an inotify instance plus one watch per subdirectory, and inotify
// instances are a per-user resource: exhausting them breaks file watching for
// the whole desktop session, not just this helper. Account Homes arrive as
// D-Bus arguments, so the registry has to be bounded.
const MaxWatchers = 8

// DefaultDebounce coalesces a burst of filesystem events into one notification.
//
// Claude Code appends to a log many times a second. Every event used to emit a
// Changed signal, each of which made the plasmoid call GetSnapshot, which
// re-read the entire log corpus — the watcher fed the exact work it was meant
// to avoid.
const DefaultDebounce = time.Second

type Manager struct {
	mu       sync.Mutex
	watchers map[string]*watchInstance
	onChange func(accountHome string)
	debounce time.Duration
	now      func() time.Time
}

type watchInstance struct {
	watcher  *fsnotify.Watcher
	done     chan struct{}
	lastSeen time.Time

	timerMu sync.Mutex
	timer   *time.Timer
}

func NewManager(onChange func(accountHome string), debounce time.Duration) *Manager {
	if debounce <= 0 {
		debounce = DefaultDebounce
	}
	return &Manager{
		watchers: make(map[string]*watchInstance),
		onChange: onChange,
		debounce: debounce,
		now:      func() time.Time { return time.Now() },
	}
}

// notify schedules one notification per debounce window. The first event in a
// burst arms the timer; the rest are dropped.
func (m *Manager) notify(accountHome string, instance *watchInstance) {
	instance.timerMu.Lock()
	defer instance.timerMu.Unlock()
	if instance.timer != nil {
		return
	}
	instance.timer = time.AfterFunc(m.debounce, func() {
		instance.timerMu.Lock()
		instance.timer = nil
		instance.timerMu.Unlock()

		select {
		case <-instance.done:
			return
		default:
		}
		m.onChange(accountHome)
	})
}

func (instance *watchInstance) stopTimer() {
	instance.timerMu.Lock()
	defer instance.timerMu.Unlock()
	if instance.timer != nil {
		instance.timer.Stop()
		instance.timer = nil
	}
}

func (m *Manager) Ensure(accountHome string) {
	if accountHome == "" {
		return
	}

	m.mu.Lock()
	if instance, ok := m.watchers[accountHome]; ok {
		instance.lastSeen = m.now()
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	projectsDir := filepath.Join(accountHome, "projects")
	info, err := os.Stat(projectsDir)
	if err != nil || !info.IsDir() {
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("create usage watcher for %s: %v", accountHome, err)
		return
	}

	if err := addRecursive(watcher, projectsDir); err != nil {
		log.Printf("watch usage directory %s: %v", projectsDir, err)
		_ = watcher.Close()
		return
	}

	instance := &watchInstance{
		watcher:  watcher,
		done:     make(chan struct{}),
		lastSeen: m.now(),
	}

	m.mu.Lock()
	if existing, ok := m.watchers[accountHome]; ok {
		existing.lastSeen = m.now()
		m.mu.Unlock()
		_ = watcher.Close()
		return
	}
	m.evictLocked()
	m.watchers[accountHome] = instance
	m.mu.Unlock()

	go m.loop(accountHome, projectsDir, instance)
}

// evictLocked closes the least recently requested watcher to make room.
func (m *Manager) evictLocked() {
	if len(m.watchers) < MaxWatchers {
		return
	}
	oldestKey := ""
	var oldestSeen time.Time
	for key, instance := range m.watchers {
		if oldestKey == "" || instance.lastSeen.Before(oldestSeen) {
			oldestKey = key
			oldestSeen = instance.lastSeen
		}
	}
	if oldestKey == "" {
		return
	}
	log.Printf("dropping usage watch for %s: watch limit %d reached", oldestKey, MaxWatchers)
	m.closeInstanceLocked(oldestKey)
}

func (m *Manager) closeInstanceLocked(accountHome string) {
	instance, ok := m.watchers[accountHome]
	if !ok {
		return
	}
	close(instance.done)
	instance.stopTimer()
	_ = instance.watcher.Close()
	delete(m.watchers, accountHome)
}

// Forget stops watching one Account Home.
func (m *Manager) Forget(accountHome string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeInstanceLocked(accountHome)
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for accountHome, instance := range m.watchers {
		close(instance.done)
		instance.stopTimer()
		if err := instance.watcher.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close watcher for %s: %w", accountHome, err))
		}
	}
	m.watchers = make(map[string]*watchInstance)
	return errors.Join(errs...)
}

func (m *Manager) loop(accountHome, projectsDir string, instance *watchInstance) {
	for {
		select {
		case <-instance.done:
			return
		case event, ok := <-instance.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Remove) == 0 {
				continue
			}

			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if err := addRecursive(instance.watcher, event.Name); err != nil {
						log.Printf("add nested watch %s: %v", event.Name, err)
					}
				}
			}

			if isUnderDir(event.Name, projectsDir) {
				m.notify(accountHome, instance)
			}
		case err, ok := <-instance.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("usage watcher error for %s: %v", accountHome, err)
		}
	}
}

func addRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// One unreadable project directory must not abort the whole watch.
			if path == root {
				return err
			}
			log.Printf("skip watch for %s: %v", path, err)
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if err := watcher.Add(path); err != nil {
			if path == root {
				return err
			}
			log.Printf("skip watch for %s: %v", path, err)
		}
		return nil
	})
}

func isUnderDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return filepath.IsLocal(rel) || rel == "."
}
