// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package accounthome

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenCredentialsRejectsUnsafeStoresForBothAgents(t *testing.T) {
	for _, name := range []string{"auth.json", ".credentials.json"} {
		for _, kind := range []string{"symlink", "hardlink", "fifo", "oversized"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				home := t.TempDir()
				path := filepath.Join(home, name)
				switch kind {
				case "symlink", "hardlink":
					target := filepath.Join(t.TempDir(), "credentials")
					if err := os.WriteFile(target, nil, 0o600); err != nil {
						t.Fatal(err)
					}
					link := os.Link
					if kind == "symlink" {
						link = os.Symlink
					}
					if err := link(target, path); err != nil {
						t.Fatal(err)
					}
				case "fifo":
					if err := syscall.Mkfifo(path, 0o600); err != nil {
						t.Fatal(err)
					}
				case "oversized":
					if err := os.WriteFile(path, nil, 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.Truncate(path, MaxCredentialsBytes+1); err != nil {
						t.Fatal(err)
					}
				}
				file, err := OpenCredentials(home, name)
				if file != nil {
					_ = file.Close()
				}
				if !errors.Is(err, ErrCredentialsUnsafe) {
					t.Fatalf("unsafe store accepted: %v", err)
				}
			})
		}
	}
}
