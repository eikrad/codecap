// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Captured verbatim from Codex 0.153.4 account/rateLimits/read on 2026-10-28.
// Only top-level fields unrelated to Allowance were omitted; no credential,
// account identifier, or email address is retained.
func capturedLimits(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/rate-limits.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCodexAllowanceUsesTheRealSessionAndWeeklyWindows(t *testing.T) {
	r, err := FromRateLimits(capturedLimits(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Session.UsedPercent != 31 || r.Weekly.UsedPercent != 7 || r.Session.ResetsAt != 1793218884 || r.Weekly.ResetsAt != 1793787621 || r.Session.Stale {
		t.Fatalf("unexpected Allowance: %+v", r)
	}
}

func TestIncompleteOrDifferentWindowsCannotLookLikeRealZeroPercent(t *testing.T) {
	for _, replacement := range []struct{ old, new string }{
		{`"usedPercent": 31`, `"missingPercent": 31`},
		{`"windowDurationMins": 300`, `"windowDurationMins": 15`},
		{`"secondary": {`, `"missingSecondary": {`},
		{`"codex": {`, `"other": {`},
	} {
		data := strings.ReplaceAll(string(capturedLimits(t)), replacement.old, replacement.new)
		if _, err := FromRateLimits([]byte(data)); err == nil {
			t.Fatalf("accepted incomplete Allowance after %s", replacement.new)
		}
	}
}

func TestEmptyMultiBucketViewDoesNotReuseLegacyAllowance(t *testing.T) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(capturedLimits(t), &payload); err != nil {
		t.Fatal(err)
	}
	payload["rateLimitsByLimitId"] = json.RawMessage(`{}`)
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FromRateLimits(data); err == nil {
		t.Fatal("empty current bucket view reused potentially stale legacy Allowance")
	}
}

func TestAppServerHandshakeReadsAllowanceWithoutLoggingIn(t *testing.T) {
	// The envelope is a protocol stand-in. The rate-limit result is the real
	// captured artifact above, not a second hand-written vendor fixture.
	out := bytes.NewBufferString("{\"id\":1,\"result\":{}}\n{\"method\":\"account/updated\",\"params\":{}}\n{\"id\":2,\"result\":{\"account\":{\"type\":\"chatgpt\"}}}\n")
	if err := json.NewEncoder(out).Encode(map[string]any{"id": 3, "result": json.RawMessage(capturedLimits(t))}); err != nil {
		t.Fatal(err)
	}
	var requests bytes.Buffer
	r, err := readAllowance(out, &requests)
	if err != nil || r.Session.UsedPercent != 31 {
		t.Fatalf("%+v, %v", r, err)
	}
	var methods []string
	decoder := json.NewDecoder(&requests)
	for decoder.More() {
		var req struct {
			Method string `json:"method"`
			Params struct {
				Refresh bool `json:"refreshToken"`
			} `json:"params"`
		}
		if err := decoder.Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Params.Refresh {
			t.Fatal("must not proactively refresh credentials")
		}
		methods = append(methods, req.Method)
	}
	if strings.Join(methods, ",") != "initialize,initialized,account/read,account/rateLimits/read" {
		t.Fatal(methods)
	}
}

func TestSignedOutAndProtocolErrorsStayExplicit(t *testing.T) {
	_, err := readAllowance(strings.NewReader("{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"account\":null}}\n"), &bytes.Buffer{})
	if !errors.Is(err, ErrSignedOut) {
		t.Fatal(err)
	}
	_, err = readAllowance(strings.NewReader("{\"id\":1,\"error\":{\"message\":\"secret-example\"}}\n"), &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "secret-example") {
		t.Fatalf("upstream errors must be explicit without exposing their contents: %v", err)
	}
}

func TestUnsafeCodexCredentialsAreRejectedWithoutBlocking(t *testing.T) {
	home := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(home, "auth.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := FetchAllowance(context.Background(), home); err == nil {
		t.Fatal("accepted FIFO")
	}
	if time.Since(start) > time.Second {
		t.Fatal("blocked on FIFO")
	}
}

func TestCodexChildIsKilledAtTheCallerDeadline(t *testing.T) {
	// A real child process stands in for an unresponsive app server. This
	// exercises CommandContext and cleanup rather than mocking a timer.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte("#!/bin/sh\nwhile :; do read -r line || exit 0; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := FetchAllowance(ctx, t.TempDir()); err == nil {
		t.Fatal("unresponsive child must fail")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("child outlived deadline")
	}
}
