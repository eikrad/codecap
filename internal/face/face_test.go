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

func TestClassifyDirectoryWithoutCredentialsIsSignedOut(t *testing.T) {
	dir := t.TempDir()
	got := Classify(dir)
	if got.Face != snapshot.FaceSignedOut {
		t.Fatalf("expected %q, got %q", snapshot.FaceSignedOut, got.Face)
	}
}

func TestClassifyDirectoryWithEmptyCredentialsIsSignedOut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFileName)
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	got := Classify(dir)
	if got.Face != snapshot.FaceSignedOut {
		t.Fatalf("expected %q, got %q", snapshot.FaceSignedOut, got.Face)
	}
}

func TestClassifyDirectoryWithOAuthAccessTokenIsUnknownAllowance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFileName)
	payload := `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"refresh","expiresAt":9999999999999}}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	got := Classify(dir)
	if got.Face != snapshot.FaceUnknownAllowance {
		t.Fatalf("expected %q, got %q", snapshot.FaceUnknownAllowance, got.Face)
	}
}
