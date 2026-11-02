// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// Package codex reads Allowance through the installed Codex app server. Codex
// retains ownership of its login and refresh flow; no token crosses D-Bus.
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/eikrad/codecap/internal/accounthome"
	"github.com/eikrad/codecap/internal/httpx"
	"github.com/eikrad/codecap/internal/snapshot"
)

var ErrSignedOut = errors.New("signed out of Codex Account")

type Result struct{ Session, Weekly snapshot.AllowanceWindow }

type window struct {
	Used    *float64 `json:"usedPercent"`
	Minutes *int64   `json:"windowDurationMins"`
	Reset   *int64   `json:"resetsAt"`
}
type limits struct {
	ID        string  `json:"limitId"`
	Primary   *window `json:"primary"`
	Secondary *window `json:"secondary"`
}

// FromRateLimits maps only the Codex bucket and only the window durations the
// widget labels Session and Weekly. Unknown shapes are errors, never 0% bars.
// Fields verified against codex 0.153.4 generate-json-schema and official docs:
// https://learn.chatgpt.com/docs/app-server#rate-limits-chatgpt
func FromRateLimits(data []byte) (Result, error) {
	var payload struct {
		Legacy limits            `json:"rateLimits"`
		ByID   map[string]limits `json:"rateLimitsByLimitId"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return Result{}, fmt.Errorf("decode Codex Allowance: %w", err)
	}
	bucket := payload.Legacy
	if payload.ByID != nil {
		var ok bool
		bucket, ok = payload.ByID["codex"]
		if !ok {
			return Result{}, errors.New("missing Codex Allowance bucket")
		}
	} else if bucket.ID != "" && bucket.ID != "codex" {
		return Result{}, errors.New("missing Codex Allowance bucket")
	}
	valid := func(w *window, minutes int64) bool {
		return w != nil && w.Used != nil && *w.Used >= 0 && *w.Used <= 100 && w.Minutes != nil && *w.Minutes == minutes && w.Reset != nil && *w.Reset > 0
	}
	if !valid(bucket.Primary, 300) || !valid(bucket.Secondary, 10080) {
		return Result{}, errors.New("unavailable Codex Session or Weekly Allowance, or unsupported duration")
	}
	return Result{Session: snapshot.AllowanceWindow{UsedPercent: *bucket.Primary.Used, ResetsAt: *bucket.Primary.Reset}, Weekly: snapshot.AllowanceWindow{UsedPercent: *bucket.Secondary.Used, ResetsAt: *bucket.Secondary.Reset}}, nil
}

// FetchAllowance bounds both the child process lifetime and protocol output.
// It never starts a thread, runs a prompt, or requests a login/refresh itself.
func FetchAllowance(ctx context.Context, home string) (Result, error) {
	home, err := accounthome.Validate(home)
	if err != nil || home == "" {
		return Result{}, errors.New("invalid Codex Account Home")
	}
	// Reject unsafe filesystem auth stores before Codex opens them. Keyring
	// auth need not have an auth.json at all. O_NONBLOCK prevents FIFO hangs.
	file, err := accounthome.OpenCredentials(home, "auth.json")
	if err == nil {
		_ = file.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, fmt.Errorf("check Codex credentials: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, httpx.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "codex", "app-server", "--listen", "stdio://")
	cmd.Dir = "/"
	cmd.Env = append(os.Environ(), "CODEX_HOME="+home)
	cmd.Stderr = io.Discard // Raw upstream errors can include account details.
	cmd.WaitDelay = time.Second
	in, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return Result{}, err
	}
	if err := cmd.Start(); err != nil {
		_ = in.Close()
		_ = out.Close()
		return Result{}, fmt.Errorf("start Codex app server: %w", err)
	}
	defer func() { _ = in.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	return readAllowance(out, in)
}

func readAllowance(out io.Reader, in io.Writer) (Result, error) {
	scanner := bufio.NewScanner(io.LimitReader(out, 4<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	encoder := json.NewEncoder(in)
	call := func(id int, method string, params any) (json.RawMessage, error) {
		if err := encoder.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			return nil, err
		}
		for scanner.Scan() {
			var reply struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(scanner.Bytes(), &reply) != nil {
				return nil, errors.New("invalid Codex protocol response")
			}
			if reply.ID == nil || *reply.ID != id {
				continue
			}
			if len(reply.Error) > 0 && string(reply.Error) != "null" {
				return nil, errors.New("failed Codex app-server request")
			}
			return reply.Result, nil
		}
		return nil, errors.New("unavailable Codex app-server response or exceeded output bound")
	}
	if _, err := call(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "codecap", "version": "0.2.0"}}); err != nil {
		return Result{}, err
	}
	if err := encoder.Encode(map[string]string{"method": "initialized"}); err != nil {
		return Result{}, err
	}
	account, err := call(2, "account/read", map[string]bool{"refreshToken": false})
	if err != nil {
		return Result{}, err
	}
	var auth struct {
		Account json.RawMessage `json:"account"`
	}
	if json.Unmarshal(account, &auth) != nil || len(auth.Account) == 0 {
		return Result{}, errors.New("unavailable Codex account response")
	}
	if string(auth.Account) == "null" {
		return Result{}, ErrSignedOut
	}
	raw, err := call(3, "account/rateLimits/read", nil)
	if err != nil {
		return Result{}, err
	}
	return FromRateLimits(raw)
}
