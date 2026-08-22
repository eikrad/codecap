// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package creds

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/eikrad/codecap/internal/httpx"
)

const (
	CredentialsFileName = ".credentials.json"
	credentialsLockName = ".credentials.json.lock"
	credentialsTempGlob = ".credentials.*.tmp"

	refreshSkew = 30 * time.Second

	// Longer than httpx.Timeout so a waiter does not give up while the holder
	// is doing a refresh that is still within its own deadline.
	lockTimeout = httpx.Timeout + 5*time.Second

	// The file holds one small JSON object.
	maxCredentialsBytes = 1 << 20

	// Orphan temp files hold a complete, valid token set.
	tempFileMaxAge = 5 * time.Minute
)

// OAuth holds the Claude Code OAuth grant used for Allowance fetches.
type OAuth struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// openCredentials opens the Account Home credentials file, refusing anything
// that is not a plain file owned by this user.
//
// O_NOFOLLOW is the load-bearing part. The path comes from a D-Bus argument, and
// saveOAuthTokens renames over it — rename replaces a symlink itself, not its
// target, so following one would read the real tokens and write the refreshed
// ones into a directory the caller chose.
func openCredentials(accountHome string) (*os.File, error) {
	path := filepath.Join(accountHome, CredentialsFileName)
	// O_NONBLOCK matters as much as O_NOFOLLOW here: opening a FIFO for reading
	// blocks until a writer appears, which would happen before the regular-file
	// check below could reject it.
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("%w: %s is a symbolic link", ErrCredentialsUnsafe, path)
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat credentials: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrCredentialsUnsafe, path)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if int64(stat.Uid) != int64(os.Getuid()) {
			_ = file.Close()
			return nil, fmt.Errorf("%w: %s is not owned by this user", ErrCredentialsUnsafe, path)
		}
		if uint64(stat.Nlink) != 1 {
			_ = file.Close()
			return nil, fmt.Errorf("%w: %s is hard-linked", ErrCredentialsUnsafe, path)
		}
	}
	return file, nil
}

func readCredentials(accountHome string) ([]byte, error) {
	file, err := openCredentials(accountHome)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxCredentialsBytes))
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	return data, nil
}

func Load(accountHome string) (OAuth, error) {
	data, err := readCredentials(accountHome)
	if err != nil {
		return OAuth{}, err
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return OAuth{}, fmt.Errorf("decode credentials: %w", err)
	}
	rawOAuth, ok := root["claudeAiOauth"]
	if !ok {
		return OAuth{}, fmt.Errorf("credentials missing claudeAiOauth")
	}

	var oauthObj map[string]any
	if err := json.Unmarshal(rawOAuth, &oauthObj); err != nil {
		return OAuth{}, fmt.Errorf("decode claudeAiOauth: %w", err)
	}

	access, _ := oauthObj["accessToken"].(string)
	refresh, _ := oauthObj["refreshToken"].(string)
	if strings.TrimSpace(access) == "" {
		return OAuth{}, fmt.Errorf("credentials missing accessToken")
	}

	expiresAt, err := parseExpiresAt(oauthObj["expiresAt"])
	if err != nil {
		return OAuth{}, err
	}

	return OAuth{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresAt:    expiresAt,
	}, nil
}

// EnsureAccessToken returns a non-expired access token, refreshing and rewriting
// Account Home credentials when needed. tokenBaseURL is typically
// https://console.anthropic.com (tests pass an httptest base).
func EnsureAccessToken(ctx context.Context, accountHome string, httpClient *http.Client, tokenBaseURL string, now time.Time) (OAuth, error) {
	// Fast path, no lock: the file is replaced by rename, so a reader sees
	// either the old token or the new one, never a torn write. This also keeps
	// concurrent GetSnapshot calls off the lock entirely in the common case.
	if oauth, err := Load(accountHome); err == nil && oauth.ExpiresAt.After(now.Add(refreshSkew)) {
		return oauth, nil
	}

	unlock, err := lockCredentials(accountHome)
	if err != nil {
		return OAuth{}, err
	}
	defer unlock()

	oauth, err := Load(accountHome)
	if err != nil {
		return OAuth{}, err
	}
	// Someone else may have refreshed while we waited for the lock.
	if oauth.ExpiresAt.After(now.Add(refreshSkew)) {
		return oauth, nil
	}
	if strings.TrimSpace(oauth.RefreshToken) == "" {
		return OAuth{}, fmt.Errorf("credentials missing refreshToken")
	}

	// The refresh runs under the lock on purpose: two processes rotating the
	// same grant would invalidate each other. It is bounded by the client
	// timeout, which is what makes holding a lock across it acceptable.
	refreshed, err := refreshOAuth(ctx, httpClient, tokenBaseURL, oauth.RefreshToken, now)
	if err != nil {
		return OAuth{}, err
	}
	if err := saveOAuthTokens(accountHome, refreshed); err != nil {
		return OAuth{}, err
	}
	return refreshed, nil
}

// RefreshAccessToken forces an OAuth refresh and rewrites Account Home credentials.
func RefreshAccessToken(ctx context.Context, accountHome string, httpClient *http.Client, tokenBaseURL string) (OAuth, error) {
	unlock, err := lockCredentials(accountHome)
	if err != nil {
		return OAuth{}, err
	}
	defer unlock()

	oauth, err := Load(accountHome)
	if err != nil {
		return OAuth{}, err
	}
	if strings.TrimSpace(oauth.RefreshToken) == "" {
		return OAuth{}, fmt.Errorf("credentials missing refreshToken")
	}
	refreshed, err := refreshOAuth(ctx, httpClient, tokenBaseURL, oauth.RefreshToken, time.Now().UTC())
	if err != nil {
		return OAuth{}, err
	}
	if err := saveOAuthTokens(accountHome, refreshed); err != nil {
		return OAuth{}, err
	}
	return refreshed, nil
}

func refreshOAuth(ctx context.Context, httpClient *http.Client, tokenBaseURL, refreshToken string, now time.Time) (OAuth, error) {
	if httpClient == nil {
		httpClient = httpx.New()
	}
	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	})
	if err != nil {
		return OAuth{}, fmt.Errorf("encode refresh request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(tokenBaseURL, "/")+"/v1/oauth/token", bytes.NewReader(body))
	if err != nil {
		return OAuth{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := httpClient.Do(req)
	if err != nil {
		return OAuth{}, fmt.Errorf("refresh token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return OAuth{}, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return OAuth{}, fmt.Errorf("%w: refresh rejected with status %d", ErrRefreshRejected, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return OAuth{}, fmt.Errorf("refresh token status %d: %s", resp.StatusCode, truncate(respBody))
	}

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return OAuth{}, fmt.Errorf("decode refresh response: %w", err)
	}
	if payload.AccessToken == "" {
		return OAuth{}, fmt.Errorf("refresh response missing access_token")
	}
	if payload.RefreshToken == "" {
		payload.RefreshToken = refreshToken
	}
	if payload.ExpiresIn <= 0 {
		payload.ExpiresIn = 3600
	}

	return OAuth{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		ExpiresAt:    now.UTC().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

func saveOAuthTokens(accountHome string, oauth OAuth) error {
	path := filepath.Join(accountHome, CredentialsFileName)
	data, err := readCredentials(accountHome)
	if err != nil {
		return fmt.Errorf("read credentials for save: %w", err)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("decode credentials for save: %w", err)
	}
	rawOAuth, ok := root["claudeAiOauth"]
	if !ok {
		return fmt.Errorf("credentials missing claudeAiOauth")
	}

	var oauthObj map[string]any
	if err := json.Unmarshal(rawOAuth, &oauthObj); err != nil {
		return fmt.Errorf("decode claudeAiOauth for save: %w", err)
	}

	oauthObj["accessToken"] = oauth.AccessToken
	oauthObj["refreshToken"] = oauth.RefreshToken
	oauthObj["expiresAt"] = oauth.ExpiresAt.UTC().UnixMilli()

	updatedOAuth, err := json.Marshal(oauthObj)
	if err != nil {
		return err
	}
	root["claudeAiOauth"] = updatedOAuth

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')

	sweepStaleTempFiles(accountHome)

	tmp, err := os.CreateTemp(accountHome, credentialsTempGlob)
	if err != nil {
		return fmt.Errorf("create credentials temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Rename replaces a symlink rather than its target, so refuse to install
	// over anything that is no longer a plain file.
	if info, err := os.Lstat(path); err != nil {
		return fmt.Errorf("stat credentials before replace: %w", err)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrCredentialsUnsafe, path)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace credentials: %w", err)
	}
	return syncDir(accountHome)
}

// syncDir makes the rename durable. Without it a crash can leave the directory
// entry pointing at nothing, which costs the user their Claude login.
func syncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open account home for sync: %w", err)
	}
	defer func() { _ = handle.Close() }()
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("sync account home: %w", err)
	}
	return nil
}

// sweepStaleTempFiles removes orphans left by a process killed mid-write. Each
// one holds a complete, valid token set.
func sweepStaleTempFiles(accountHome string) {
	matches, err := filepath.Glob(filepath.Join(accountHome, credentialsTempGlob))
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-tempFileMaxAge)
	for _, match := range matches {
		info, err := os.Lstat(match)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(match)
		}
	}
}

func parseExpiresAt(raw any) (time.Time, error) {
	switch v := raw.(type) {
	case float64:
		return expiresFromNumber(int64(v)), nil
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return time.Time{}, err
		}
		return expiresFromNumber(n), nil
	case nil:
		return time.Time{}, nil
	default:
		return time.Time{}, fmt.Errorf("unexpected expiresAt type %T", raw)
	}
}

func expiresFromNumber(n int64) time.Time {
	// Claude Code writes epoch milliseconds when >= 1e11.
	if n >= 100_000_000_000 {
		return time.UnixMilli(n).UTC()
	}
	return time.Unix(n, 0).UTC()
}

// lockCredentials takes an advisory lock on a lock file next to the credentials.
//
// flock is held by the open file description, so the kernel releases it when the
// process dies. The previous O_CREATE|O_EXCL scheme left a lock file behind on
// any SIGKILL, and every later refresh then failed until someone deleted it by
// hand. The lock file itself is deliberately never removed: unlinking it would
// let a second process create a new one and hold a lock on a different inode.
func lockCredentials(accountHome string) (func(), error) {
	lockPath := filepath.Join(accountHome, credentialsLockName)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open credentials lock: %w", err)
	}

	release := func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = file.Close()
			return nil, fmt.Errorf("lock credentials: %w", err)
		}
		if time.Now().After(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("credentials lock busy")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func truncate(b []byte) string {
	const max = 200
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
