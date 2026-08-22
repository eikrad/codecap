// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLastKnownRoundTripWithinTTL(t *testing.T) {
	dir := t.TempDir()
	store := NewLastKnownStore(dir)
	accountHome := "/home/user/.claude"

	original := CachedAllowance{
		FetchedAt:   time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC).Unix(),
		Session:     Result{}.Session,
		Weekly:      Result{}.Weekly,
		UsageCredit: "enabled",
	}
	original.Session.UsedPercent = 22
	original.Session.ResetsAt = 1770000000
	original.Weekly.UsedPercent = 40
	original.Weekly.ResetsAt = 1771000000

	if err := store.Save(accountHome, original); err != nil {
		t.Fatalf("save last-known: %v", err)
	}

	now := time.Unix(original.FetchedAt, 0).Add(30 * time.Minute)
	got, ok, err := store.Load(accountHome, now)
	if err != nil {
		t.Fatalf("load last-known: %v", err)
	}
	if !ok {
		t.Fatal("expected last-known hit within TTL")
	}
	if !got.Session.Stale || !got.Weekly.Stale {
		t.Fatal("last-known windows must be marked stale")
	}
	if got.Session.UsedPercent != 22 || got.Weekly.UsedPercent != 40 {
		t.Fatalf("unexpected windows: %+v", got)
	}
	if got.UsageCredit != "enabled" {
		t.Fatalf("usage credit: got %q", got.UsageCredit)
	}
	if got.FetchedAt != original.FetchedAt {
		t.Fatalf("fetched_at: got %d want %d", got.FetchedAt, original.FetchedAt)
	}
}

func TestLastKnownExpiredAfterTTL(t *testing.T) {
	dir := t.TempDir()
	store := NewLastKnownStore(dir)
	accountHome := "/tmp/account"

	cached := CachedAllowance{
		FetchedAt:   time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC).Unix(),
		UsageCredit: "none",
	}
	if err := store.Save(accountHome, cached); err != nil {
		t.Fatalf("save: %v", err)
	}

	now := time.Unix(cached.FetchedAt, 0).Add(61 * time.Minute)
	_, ok, err := store.Load(accountHome, now)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if ok {
		t.Fatal("expected miss after TTL")
	}
}

func TestLastKnownMissWhenMissing(t *testing.T) {
	store := NewLastKnownStore(t.TempDir())
	_, ok, err := store.Load("/missing", time.Now())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if ok {
		t.Fatal("expected miss")
	}
}

func TestLastKnownPathIsStableForAccountHome(t *testing.T) {
	dir := t.TempDir()
	store := NewLastKnownStore(dir)
	path := store.pathFor("/home/user/.claude")
	if filepath.Base(filepath.Dir(path)) == "" {
		t.Fatal("expected nested path")
	}
	if filepath.Ext(path) != ".json" {
		t.Fatalf("expected json cache file, got %s", path)
	}
}
