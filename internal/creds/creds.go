// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package creds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	CredentialsFileName = ".credentials.json"
	credentialsLockName = ".credentials.json.lock"
	refreshSkew         = 30 * time.Second
)

// OAuth holds the Claude Code OAuth grant used for Allowance fetches.
type OAuth struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

func Load(accountHome string) (OAuth, error) {
	path := filepath.Join(accountHome, CredentialsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return OAuth{}, fmt.Errorf("read credentials: %w", err)
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
// Account Home credentials when needed. tokenURL is typically
// https://console.anthropic.com (tests pass an httptest base).
func EnsureAccessToken(accountHome string, httpClient *http.Client, tokenBaseURL string, now time.Time) (OAuth, error) {
	unlock, err := lockCredentials(accountHome)
	if err != nil {
		return OAuth{}, err
	}
	defer unlock()

	oauth, err := Load(accountHome)
	if err != nil {
		return OAuth{}, err
	}
	if oauth.ExpiresAt.After(now.Add(refreshSkew)) {
		return oauth, nil
	}
	if strings.TrimSpace(oauth.RefreshToken) == "" {
		return OAuth{}, fmt.Errorf("credentials missing refreshToken")
	}

	refreshed, err := refreshOAuth(httpClient, tokenBaseURL, oauth.RefreshToken)
	if err != nil {
		return OAuth{}, err
	}
	if err := saveOAuthTokens(accountHome, refreshed); err != nil {
		return OAuth{}, err
	}
	return refreshed, nil
}

// RefreshAccessToken forces an OAuth refresh and rewrites Account Home credentials.
func RefreshAccessToken(accountHome string, httpClient *http.Client, tokenBaseURL string) (OAuth, error) {
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
	refreshed, err := refreshOAuth(httpClient, tokenBaseURL, oauth.RefreshToken)
	if err != nil {
		return OAuth{}, err
	}
	if err := saveOAuthTokens(accountHome, refreshed); err != nil {
		return OAuth{}, err
	}
	return refreshed, nil
}

func refreshOAuth(httpClient *http.Client, tokenBaseURL, refreshToken string) (OAuth, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(tokenBaseURL, "/")+"/v1/oauth/token", bytes.NewReader(body))
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
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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
		ExpiresAt:    time.Now().UTC().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

func saveOAuthTokens(accountHome string, oauth OAuth) error {
	path := filepath.Join(accountHome, CredentialsFileName)
	data, err := os.ReadFile(path)
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

	tmp, err := os.CreateTemp(accountHome, ".credentials.*.tmp")
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
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace credentials: %w", err)
	}
	return nil
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

func lockCredentials(accountHome string) (func(), error) {
	lockPath := filepath.Join(accountHome, credentialsLockName)
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("create credentials lock: %w", err)
		}
		if time.Now().After(deadline) {
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
