// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package creds

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	oauth, err := EnsureAccessToken(dir, server.Client(), server.URL, time.Unix(10, 0).UTC())
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
