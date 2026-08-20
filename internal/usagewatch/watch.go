// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usagewatch

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type Manager struct {
	mu       sync.Mutex
	watchers map[string]*watchInstance
	onChange func(accountHome string)
}

type watchInstance struct {
	watcher *fsnotify.Watcher
	done    chan struct{}
}

func NewManager(onChange func(accountHome string)) *Manager {
	return &Manager{
		watchers: make(map[string]*watchInstance),
		onChange: onChange,
	}
}

func (m *Manager) Ensure(accountHome string) {
	if accountHome == "" {
		return
	}

	m.mu.Lock()
	if _, ok := m.watchers[accountHome]; ok {
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
		watcher: watcher,
		done:    make(chan struct{}),
	}

	m.mu.Lock()
	if _, ok := m.watchers[accountHome]; ok {
		m.mu.Unlock()
		_ = watcher.Close()
		return
	}
	m.watchers[accountHome] = instance
	m.mu.Unlock()

	go m.loop(accountHome, projectsDir, instance)
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for accountHome, instance := range m.watchers {
		close(instance.done)
		if err := instance.watcher.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close watcher for %s: %w", accountHome, err))
		}
	}
	m.watchers = make(map[string]*watchInstance)

	if len(errs) == 0 {
		return nil
	}
	return errorsJoin(errs...)
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
				m.onChange(accountHome)
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
			return err
		}
		if !d.IsDir() {
			return nil
		}
		return watcher.Add(path)
	})
}

func isUnderDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	if rel == ".." || startsWithParent(rel) {
		return false
	}
	return true
}

func startsWithParent(path string) bool {
	return len(path) >= 3 && path[0:3] == ".."+string(filepath.Separator)
}

func errorsJoin(errs ...error) error {
	filtered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	msg := filtered[0].Error()
	for i := 1; i < len(filtered); i++ {
		msg += "; " + filtered[i].Error()
	}
	return fmt.Errorf("%s", msg)
}
