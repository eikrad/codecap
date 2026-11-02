// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAccountHomeIdentityUsesArtifactsRatherThanFolderName(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := Detect(home)
	if err != nil || a != Codex {
		t.Fatalf("got %q, %v", a, err)
	}
	if err := os.WriteFile(filepath.Join(home, ".credentials.json"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Detect(home); err == nil {
		t.Fatal("ambiguous Account Home must not silently select an Agent")
	}
}
