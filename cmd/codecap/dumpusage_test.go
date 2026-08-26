// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The command prints an undocumented payload to a terminal, so redaction is the
// part that has to hold for fields nobody has seen yet.
func TestRedactHidesSecretsAndKeepsShape(t *testing.T) {
	raw := `{
		"five_hour": {"utilization": 37, "resets_at": "2026-08-26T18:30:00Z"},
		"extra_usage": {"is_enabled": true, "monthly_limit": 4.04, "used_credits": 400},
		"oauth_account": {
			"email_address": "someone@example.com",
			"uuid": "abc-123",
			"nested": [{"access_token": "sk-live-xxx"}]
		},
		"some_future_field": {"apiKey": "leak-me", "harmless": 1}
	}`

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	out, err := json.Marshal(redact(decoded))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)

	for _, secret := range []string{"someone@example.com", "sk-live-xxx", "leak-me"} {
		if strings.Contains(got, secret) {
			t.Errorf("redacted output still contains %q:\n%s", secret, got)
		}
	}

	// Shape has to survive: the command exists to reveal unknown fields, and a
	// dropped key would hide exactly what is being looked for.
	for _, key := range []string{
		"five_hour", "utilization", "extra_usage", "monthly_limit",
		"used_credits", "oauth_account", "some_future_field", "harmless",
	} {
		if !strings.Contains(got, key) {
			t.Errorf("redaction dropped key %q:\n%s", key, got)
		}
	}

	// Values that answer the open questions must NOT be redacted.
	if !strings.Contains(got, "4.04") || !strings.Contains(got, "400") {
		t.Errorf("redaction hid the Usage Credit amounts, which is the point of the dump:\n%s", got)
	}
}

func TestRedactionNoteNamesWhatWasHidden(t *testing.T) {
	var decoded any
	if err := json.Unmarshal([]byte(`{"a":{"access_token":"x"},"b":2}`), &decoded); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	note := redactionNote(decoded)
	if !strings.Contains(note, "a.access_token") {
		t.Errorf("note should name the hidden path, got %q", note)
	}

	var clean any
	if err := json.Unmarshal([]byte(`{"b":2}`), &clean); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if note := redactionNote(clean); !strings.Contains(note, "nothing redacted") {
		t.Errorf("clean payload should say so, got %q", note)
	}
}

func TestDumpUsageRejectsAnEmptyAccountHome(t *testing.T) {
	if err := dumpUsage(t.Context(), nil, "  "); err == nil {
		t.Fatal("expected an error for a blank account home")
	}
}
