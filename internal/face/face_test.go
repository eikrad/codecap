// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package face

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eikrad/codecap/internal/snapshot"
)

func TestClassifyEmptyAccountHomeIsUnbound(t *testing.T) {
	got := Classify("   ")
	if got.Face != snapshot.FaceUnbound {
		t.Fatalf("expected %q, got %q", snapshot.FaceUnbound, got.Face)
	}
	if got.AccountLabel != "" {
		t.Fatalf("expected empty account label, got %q", got.AccountLabel)
	}
}

func TestClassifyMissingDirectoryIsSignedOut(t *testing.T) {
	got := Classify("/definitely/not/present")
	if got.Face != snapshot.FaceSignedOut {
		t.Fatalf("expected %q, got %q", snapshot.FaceSignedOut, got.Face)
	}
}

func TestClassifyDirectoryWithoutLoginSentinelIsSignedOut(t *testing.T) {
	dir := t.TempDir()
	got := Classify(dir)
	if got.Face != snapshot.FaceSignedOut {
		t.Fatalf("expected %q, got %q", snapshot.FaceSignedOut, got.Face)
	}
}

func TestClassifyDirectoryWithLoginSentinelIsUnknownAllowance(t *testing.T) {
	dir := t.TempDir()
	loginPath := filepath.Join(dir, LoginSentinelName)
	if err := os.WriteFile(loginPath, []byte{}, 0o600); err != nil {
		t.Fatalf("create login sentinel: %v", err)
	}

	got := Classify(dir)
	if got.Face != snapshot.FaceUnknownAllowance {
		t.Fatalf("expected %q, got %q", snapshot.FaceUnknownAllowance, got.Face)
	}
}
