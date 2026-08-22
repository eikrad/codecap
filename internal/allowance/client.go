// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	DefaultAPIBaseURL = "https://api.anthropic.com"
	usagePath         = "/api/oauth/usage"
	betaHeader        = "oauth-2025-04-20"
	// User-Agent must look like Claude Code or the usage endpoint rate-limits harshly.
	userAgent = "claude-code/2.1.0"
)

var errUnauthorized = errors.New("oauth usage unauthorized")

// Client fetches Allowance from Anthropic's unofficial OAuth usage endpoint.
type Client struct {
	httpClient *http.Client
	apiBaseURL string
}

func NewClient(httpClient *http.Client, apiBaseURL string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = DefaultAPIBaseURL
	}
	return &Client{
		httpClient: httpClient,
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
	}
}

func (c *Client) FetchUsage(accessToken string) (Result, error) {
	req, err := http.NewRequest(http.MethodGet, c.apiBaseURL+usagePath, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", betaHeader)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("usage request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, fmt.Errorf("read usage response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Result{}, fmt.Errorf("%w: status %d", errUnauthorized, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("usage status %d: %s", resp.StatusCode, string(body))
	}
	return FromUsagePayload(body)
}

func IsUnauthorized(err error) bool {
	return errors.Is(err, errUnauthorized)
}
