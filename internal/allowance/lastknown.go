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
	"syscall"
	"time"

	"github.com/eikrad/codecap/internal/snapshot"
)

// LastKnownTTL is how long a cached Allowance remains usable after fetch failure.
const LastKnownTTL = time.Hour

// clockSkewAllowance is how far ahead of now a cached timestamp may sit before
// the entry is treated as unusable rather than immortal.
const clockSkewAllowance = 5 * time.Minute

// ErrCacheUnsafe means the cache directory is not a private directory we own.
var ErrCacheUnsafe = errors.New("last-known cache directory is not safe to use")

// CachedAllowance is Last-Known Allowance persisted outside Account Home.
type CachedAllowance struct {
	AccountHome string                   `json:"account_home"`
	FetchedAt   int64                    `json:"fetched_at"`
	Session     snapshot.AllowanceWindow `json:"session_allowance"`
	Weekly      snapshot.AllowanceWindow `json:"weekly_allowance"`
	UsageCredit string                   `json:"usage_credit"`
	// Absent in caches written before this field existed, which decode to the
	// zero value rather than failing — that is why it was added instead of
	// reshaping UsageCredit.
	UsageCreditSpend snapshot.UsageCreditSpend `json:"usage_credit_spend"`
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
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create last-known cache dir: %w", err)
	}
	// MkdirAll succeeds on a directory that already exists without checking who
	// owns it, and the filenames here are a predictable hash of the Account
	// Home. On the /tmp fallback root that is a directory another user can have
	// created first.
	if err := checkPrivateDir(dir); err != nil {
		return err
	}

	value.AccountHome = accountHome
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal last-known: %w", err)
	}

	// A fixed ".tmp" name let two concurrent saves interleave on the same path
	// and leave torn JSON behind; O_EXCL with a random name cannot.
	tmp, err := os.CreateTemp(dir, "last-known-*.tmp")
	if err != nil {
		return fmt.Errorf("create last-known temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod last-known temp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write last-known temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close last-known temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace last-known: %w", err)
	}
	return nil
}

// checkPrivateDir refuses a cache directory that is not ours or is writable by
// anyone else.
func checkPrivateDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("stat last-known cache dir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a directory", ErrCacheUnsafe, dir)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%w: %s is group- or world-writable", ErrCacheUnsafe, dir)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if int64(stat.Uid) != int64(os.Getuid()) {
			return fmt.Errorf("%w: %s is not owned by this user", ErrCacheUnsafe, dir)
		}
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

	// The file records which Account Home it belongs to, but nothing checked it,
	// so a planted or misfiled entry was served as if it were this account's.
	if cached.AccountHome != accountHome {
		return CachedAllowance{}, false, nil
	}

	fetchedAt := time.Unix(cached.FetchedAt, 0).UTC()
	age := now.Sub(fetchedAt)
	// A one-sided comparison let a future timestamp produce a negative age, so
	// the entry never expired and was served as live for good.
	if age > LastKnownTTL || age < -clockSkewAllowance {
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
