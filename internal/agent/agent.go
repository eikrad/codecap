// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// Package agent identifies the Agent owning an Account Home, without reading
// credentials. The directory name is user chosen and is not an identity.
package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Agent string

const (
	Claude Agent = "claude"
	Codex  Agent = "codex"
)

func Detect(home string) (Agent, error) {
	exists := func(name string) (bool, error) {
		_, err := os.Lstat(filepath.Join(home, name))
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return err == nil, err
	}
	claude, err := exists(".credentials.json")
	if err != nil {
		return "", err
	}
	codex, err := exists("auth.json")
	if err != nil {
		return "", err
	}
	// Signed-out and log-only Account Homes still have an identifiable layout.
	if !claude && !codex {
		claude, err = exists("projects")
		if err != nil {
			return "", err
		}
		codex, err = exists("sessions")
		if err != nil {
			return "", err
		}
		if !codex {
			codex, err = exists("archived_sessions")
		}
		if err != nil {
			return "", err
		}
	}
	if claude && codex {
		return "", fmt.Errorf("ambiguous Account Home: both Claude and Codex artifacts")
	}
	if codex {
		return Codex, nil
	}
	// Preserve the v1 behaviour for an empty or not-yet-created Claude home.
	return Claude, nil
}

func (a Agent) LogDirectories() []string {
	if a == Codex {
		return []string{"sessions", "archived_sessions"}
	}
	return []string{"projects"}
}
