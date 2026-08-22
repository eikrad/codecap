// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package accounthome

import (
	"strings"
	"testing"
)

func TestValidateAcceptsAndNormalizes(t *testing.T) {
	cases := map[string]string{
		"/home/me/.claude":   "/home/me/.claude",
		"/home/me/.claude/":  "/home/me/.claude",
		"  /home/me/.claude": "/home/me/.claude",
		"/home/me//.claude":  "/home/me/.claude",
		"/home/me/./.claude": "/home/me/.claude",
		"":                   "",
		"   ":                "",
	}
	for input, want := range cases {
		got, err := Validate(input)
		if err != nil {
			t.Fatalf("Validate(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("Validate(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidateRejectsUnusablePaths(t *testing.T) {
	cases := []string{
		".claude",              // relative
		"~/.claude",            // the plasmoid expands this, the helper must not see it
		"/",                    // whole filesystem, and the label would render as "."
		"/home/me/\x00.claude", // NUL
		strings.Repeat("/a", MaxLength),
	}
	for _, input := range cases {
		if got, err := Validate(input); err == nil {
			t.Fatalf("Validate(%q) = %q, expected an error", input, got)
		}
	}
}

func TestValidateResolvesTraversal(t *testing.T) {
	// The result is joined against without further checks, so it must not be
	// able to climb out of wherever it appears to point.
	got, err := Validate("/home/me/../../etc/.claude")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if strings.Contains(got, "..") {
		t.Fatalf("Validate left a traversal in %q", got)
	}
	if got != "/etc/.claude" {
		t.Fatalf("Validate = %q, want /etc/.claude", got)
	}
}
