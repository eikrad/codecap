// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/eikrad/codecap/internal/allowance"
	"github.com/eikrad/codecap/internal/creds"
	"github.com/eikrad/codecap/internal/httpx"
)

// dumpUsage prints the Allowance endpoint's reply for one Account Home.
//
// The endpoint is undocumented and every fixture in internal/allowance is
// invented, so the mapping has never been checked against a real payload. Two
// open questions need one: Usage Credit reports "exhausted" for an Account
// whose extra budget is not spent, which points at a unit mismatch between
// used_credits and monthly_limit; and the mapping reads only five_hour and
// seven_day, while the subscription also has a monthly limit that may or may
// not be in here under some name.
//
// This runs as the user, not as the helper, so nobody has to hand credentials
// to anyone. It goes through creds.EnsureAccessToken, which refreshes under the
// credentials lock — that lock is what stops a concurrent helper poll from
// rotating the same grant twice and signing the Account out.
func dumpUsage(ctx context.Context, out io.Writer, accountHome string) error {
	if strings.TrimSpace(accountHome) == "" {
		return fmt.Errorf("usage: codecap dump-usage <account-home>")
	}

	httpClient := httpx.New()
	oauth, err := creds.EnsureAccessToken(ctx, accountHome, httpClient, allowance.DefaultTokenBaseURL, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("access token for %s: %w", accountHome, err)
	}

	body, err := allowance.NewClient(httpClient, allowance.DefaultAPIBaseURL).
		FetchRawUsage(ctx, oauth.AccessToken)
	if err != nil {
		return fmt.Errorf("fetch usage: %w", err)
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("decode usage payload: %w", err)
	}

	pretty, err := json.MarshalIndent(redact(decoded), "", "  ")
	if err != nil {
		return fmt.Errorf("format usage payload: %w", err)
	}
	_, err = fmt.Fprintf(out, "%s\n%s\n", redactionNote(decoded), pretty)
	return err
}

// sensitiveKey decides what never reaches the terminal. It matches on
// substrings rather than exact names on purpose: the point of this command is
// to reveal fields nobody has seen, so an unknown key holding a token has to be
// caught by shape, not by a list of names someone remembered to update.
func sensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, needle := range []string{
		"token", "secret", "password", "credential", "authorization",
		"email", "apikey", "api_key", "key",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// redact replaces sensitive values but keeps their keys, so the payload's shape
// stays visible — a removed key would hide the very thing this command exists
// to show.
func redact(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, inner := range typed {
			if sensitiveKey(key) {
				out[key] = "<redacted>"
				continue
			}
			out[key] = redact(inner)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, inner := range typed {
			out[i] = redact(inner)
		}
		return out
	default:
		return value
	}
}

// redactionNote names what was hidden, so a reader can tell "no such field"
// apart from "field withheld" without re-running anything.
func redactionNote(value any) string {
	hidden := collectRedacted(value, "")
	if len(hidden) == 0 {
		return "// nothing redacted"
	}
	sort.Strings(hidden)
	return "// redacted: " + strings.Join(hidden, ", ")
}

func collectRedacted(value any, path string) []string {
	var found []string
	switch typed := value.(type) {
	case map[string]any:
		for key, inner := range typed {
			here := key
			if path != "" {
				here = path + "." + key
			}
			if sensitiveKey(key) {
				found = append(found, here)
				continue
			}
			found = append(found, collectRedacted(inner, here)...)
		}
	case []any:
		for i, inner := range typed {
			found = append(found, collectRedacted(inner, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return found
}
