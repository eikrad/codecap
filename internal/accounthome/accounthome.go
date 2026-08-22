// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// Package accounthome validates the Account Home path that arrives over D-Bus.
//
// The path is a plain string argument from any session-bus peer, and the helper
// uses it as a filesystem base for reads, writes, temp files, renames, directory
// walks and inotify watches. Validation here is the first of two defences; the
// second is refusing to follow a symlinked credentials file (internal/creds).
package accounthome

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// MaxLength caps the path. D-Bus permits strings up to the 128 MB message limit,
// and an Account Home becomes a permanent map key in the watcher and poller
// registries.
const MaxLength = 4096

var (
	ErrTooLong     = errors.New("account home path too long")
	ErrNotAbsolute = errors.New("account home path is not absolute")
	ErrNotAPath    = errors.New("account home path is not usable")
)

// Validate normalizes an Account Home. An empty string is valid and means
// Unbound. Anything returned is absolute and cleaned, so it can be joined
// against without re-checking for traversal.
func Validate(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) > MaxLength {
		return "", fmt.Errorf("%w: %d bytes", ErrTooLong, len(trimmed))
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", fmt.Errorf("%w: contains NUL", ErrNotAPath)
	}
	if !filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("%w: %q", ErrNotAbsolute, trimmed)
	}

	cleaned := filepath.Clean(trimmed)
	if cleaned == "/" {
		// Nothing good comes of walking the whole filesystem, and the Account
		// label derived from it would render as ".".
		return "", fmt.Errorf("%w: %q", ErrNotAPath, trimmed)
	}
	return cleaned, nil
}
