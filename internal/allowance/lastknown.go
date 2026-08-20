// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eikrad/codecap/internal/snapshot"
)

// LastKnownTTL is how long a cached Allowance remains usable after fetch failure.
const LastKnownTTL = time.Hour

// CachedAllowance is Last-Known Allowance persisted outside Account Home.
type CachedAllowance struct {
	AccountHome string                   `json:"account_home"`
	FetchedAt   int64                    `json:"fetched_at"`
	Session     snapshot.AllowanceWindow `json:"session_allowance"`
	Weekly      snapshot.AllowanceWindow `json:"weekly_allowance"`
	UsageCredit string                   `json:"usage_credit"`
}

// LastKnownStore reads and writes Last-Known Allowance under a cache root
// (normally $XDG_CACHE_HOME/codecap or ~/.cache/codecap).
type LastKnownStore struct {
	root string
}

func NewLastKnownStore(root string) *LastKnownStore {
	return &LastKnownStore{root: root}
}

// DefaultCacheRoot returns the XDG cache directory for codecap.
func DefaultCacheRoot() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "codecap")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "codecap-cache")
	}
	return filepath.Join(home, ".cache", "codecap")
}

func (s *LastKnownStore) Save(accountHome string, value CachedAllowance) error {
	path := s.pathFor(accountHome)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create last-known cache dir: %w", err)
	}

	value.AccountHome = accountHome
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal last-known: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write last-known temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace last-known: %w", err)
	}
	return nil
}

func (s *LastKnownStore) Load(accountHome string, now time.Time) (CachedAllowance, bool, error) {
	path := s.pathFor(accountHome)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CachedAllowance{}, false, nil
		}
		return CachedAllowance{}, false, fmt.Errorf("read last-known: %w", err)
	}

	var cached CachedAllowance
	if err := json.Unmarshal(data, &cached); err != nil {
		return CachedAllowance{}, false, fmt.Errorf("decode last-known: %w", err)
	}

	fetchedAt := time.Unix(cached.FetchedAt, 0).UTC()
	if now.Sub(fetchedAt) > LastKnownTTL {
		return CachedAllowance{}, false, nil
	}

	cached.Session.Stale = true
	cached.Weekly.Stale = true
	return cached, true, nil
}

func (s *LastKnownStore) pathFor(accountHome string) string {
	sum := sha256.Sum256([]byte(accountHome))
	name := hex.EncodeToString(sum[:16]) + ".json"
	return filepath.Join(s.root, "last-known", name)
}
