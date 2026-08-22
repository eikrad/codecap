// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package creds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestLoadReadsOAuthAccessToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFileName)
	payload := `{
		"claudeAiOauth": {
			"accessToken": "sk-ant-oat01-aaa",
			"refreshToken": "sk-ant-ort01-bbb",
			"expiresAt": 9999999999999,
			"subscriptionType": "pro",
			"rateLimitTier": "default_raven"
		}
	}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	oauth, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if oauth.AccessToken != "sk-ant-oat01-aaa" {
		t.Fatalf("access token mismatch: %q", oauth.AccessToken)
	}
	if oauth.RefreshToken != "sk-ant-ort01-bbb" {
		t.Fatalf("refresh token mismatch: %q", oauth.RefreshToken)
	}
}

func TestLoadParsesExpiresAtEpochSeconds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFileName)
	const epochSeconds int64 = 1_700_000_000
	payload := `{
		"claudeAiOauth": {
			"accessToken": "sk-ant-oat01-aaa",
			"refreshToken": "sk-ant-ort01-bbb",
			"expiresAt": 1700000000
		}
	}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	oauth, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := time.Unix(epochSeconds, 0).UTC()
	if !oauth.ExpiresAt.Equal(want) {
		t.Fatalf("expiresAt: got %v want %v", oauth.ExpiresAt, want)
	}
}

func TestLoadParsesExpiresAtEpochMilliseconds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFileName)
	const epochMillis int64 = 1_700_000_000_000
	payload := `{
		"claudeAiOauth": {
			"accessToken": "sk-ant-oat01-aaa",
			"refreshToken": "sk-ant-ort01-bbb",
			"expiresAt": 1700000000000
		}
	}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	oauth, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := time.UnixMilli(epochMillis).UTC()
	if !oauth.ExpiresAt.Equal(want) {
		t.Fatalf("expiresAt: got %v want %v", oauth.ExpiresAt, want)
	}
}

func TestEnsureAccessTokenRefreshesAndPreservesSiblingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFileName)
	payload := `{
		"claudeAiOauth": {
			"accessToken": "sk-ant-oat01-old",
			"refreshToken": "sk-ant-ort01-old",
			"expiresAt": 1,
			"subscriptionType": "pro",
			"rateLimitTier": "default_raven",
			"scopes": ["user:inference"]
		}
	}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/oauth/token" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "sk-ant-oat01-new",
			"refresh_token": "sk-ant-ort01-new",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(server.Close)

	oauth, err := EnsureAccessToken(context.Background(), dir, server.Client(), server.URL, time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatalf("ensure access token: %v", err)
	}
	if oauth.AccessToken != "sk-ant-oat01-new" {
		t.Fatalf("expected refreshed access token, got %q", oauth.AccessToken)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reread credentials: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode credentials: %v", err)
	}
	oauthObj := decoded["claudeAiOauth"].(map[string]any)
	if oauthObj["subscriptionType"] != "pro" {
		t.Fatalf("lost subscriptionType: %#v", oauthObj["subscriptionType"])
	}
	if oauthObj["rateLimitTier"] != "default_raven" {
		t.Fatalf("lost rateLimitTier: %#v", oauthObj["rateLimitTier"])
	}
	if oauthObj["accessToken"] != "sk-ant-oat01-new" {
		t.Fatalf("accessToken not updated on disk: %#v", oauthObj["accessToken"])
	}
}

func writeValidCreds(t *testing.T, dir string, expiresAt int64) string {
	t.Helper()
	path := filepath.Join(dir, CredentialsFileName)
	body := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test","refreshToken":"refresh","expiresAt":%d}}`, expiresAt)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	return path
}

func TestLoadRefusesSymlinkedCredentials(t *testing.T) {
	// The sharp case: a caller points the helper at a directory whose
	// .credentials.json is a symlink to the real one. Following it would read
	// the real tokens, and the rename in saveOAuthTokens replaces the symlink
	// itself, so the refreshed tokens would land in the caller's directory.
	realHome := t.TempDir()
	writeValidCreds(t, realHome, time.Now().Add(time.Hour).UnixMilli())

	attackerHome := t.TempDir()
	if err := os.Symlink(filepath.Join(realHome, CredentialsFileName),
		filepath.Join(attackerHome, CredentialsFileName)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := Load(attackerHome); !errors.Is(err, ErrCredentialsUnsafe) {
		t.Fatalf("expected ErrCredentialsUnsafe, got %v", err)
	}
}

func TestLoadRefusesNonRegularCredentials(t *testing.T) {
	accountHome := t.TempDir()
	path := filepath.Join(accountHome, CredentialsFileName)
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := Load(accountHome)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a FIFO to be refused")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Load blocked on a FIFO instead of refusing it")
	}
}

func TestLockCredentialsRecoversFromALeftoverLockFile(t *testing.T) {
	accountHome := t.TempDir()
	writeValidCreds(t, accountHome, time.Now().Add(time.Hour).UnixMilli())

	// The old scheme used O_CREATE|O_EXCL and removed the file on the way out,
	// so a SIGKILL left this behind and every later refresh failed until
	// someone deleted it by hand. flock is released by the kernel instead.
	stale := filepath.Join(accountHome, ".credentials.json.lock")
	if err := os.WriteFile(stale, []byte("999999\n"), 0o600); err != nil {
		t.Fatalf("plant stale lock: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		unlock, err := lockCredentials(accountHome)
		if unlock != nil {
			unlock()
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a leftover lock file must not block: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("lockCredentials blocked on a leftover lock file")
	}
}

func TestLockCredentialsIsExclusive(t *testing.T) {
	accountHome := t.TempDir()
	writeValidCreds(t, accountHome, time.Now().Add(time.Hour).UnixMilli())

	unlock, err := lockCredentials(accountHome)
	if err != nil {
		t.Fatalf("take lock: %v", err)
	}

	blocked := make(chan struct{})
	go func() {
		second, err := lockCredentials(accountHome)
		if err == nil {
			second()
		}
		close(blocked)
	}()

	select {
	case <-blocked:
		t.Fatal("a second lock was granted while the first was held")
	case <-time.After(150 * time.Millisecond):
	}

	unlock()
	select {
	case <-blocked:
	case <-time.After(3 * time.Second):
		t.Fatal("the lock was not released")
	}
}

func TestSaveSweepsOrphanTempFiles(t *testing.T) {
	accountHome := t.TempDir()
	writeValidCreds(t, accountHome, time.Now().Add(-time.Hour).UnixMilli())

	// A process killed between CreateTemp and Rename leaves a complete, valid
	// token set behind in the Account Home.
	orphan := filepath.Join(accountHome, ".credentials.orphan.tmp")
	if err := os.WriteFile(orphan, []byte(`{"claudeAiOauth":{"accessToken":"leaked"}}`), 0o600); err != nil {
		t.Fatalf("plant orphan: %v", err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatalf("age orphan: %v", err)
	}

	if err := saveOAuthTokens(accountHome, OAuth{
		AccessToken:  "fresh",
		RefreshToken: "fresh-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the orphan temp file to be swept, stat gave %v", err)
	}
}
