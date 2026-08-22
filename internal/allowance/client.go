// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eikrad/codecap/internal/httpx"
)

const (
	DefaultAPIBaseURL = "https://api.anthropic.com"
	usagePath         = "/api/oauth/usage"
	betaHeader        = "oauth-2025-04-20"
	// User-Agent must look like Claude Code or the usage endpoint rate-limits harshly.
	// See decision D1 in docs/plan-hardening.md: this is a suspension risk carried
	// by every user, and it is what makes the 403 handling below matter.
	userAgent = "claude-code/2.1.0"
)

var (
	// errUnauthorized means the access token is not valid. It is the only
	// condition that justifies burning a refresh grant.
	errUnauthorized = errors.New("oauth usage unauthorized")
	// errTransient means try again later: rate limiting, a WAF, or a server
	// error. Forcing a refresh for these rotated the grant once a minute.
	errTransient = errors.New("oauth usage temporarily unavailable")
)

// TransientError carries a server-suggested wait, from Retry-After.
type TransientError struct {
	RetryAfter time.Duration
	err        error
}

func (e *TransientError) Error() string { return e.err.Error() }
func (e *TransientError) Unwrap() error { return e.err }

// Client fetches Allowance from Anthropic's unofficial OAuth usage endpoint.
type Client struct {
	httpClient *http.Client
	apiBaseURL string
}

func NewClient(httpClient *http.Client, apiBaseURL string) *Client {
	if httpClient == nil {
		httpClient = httpx.New()
	}
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = DefaultAPIBaseURL
	}
	return &Client{
		httpClient: httpClient,
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
	}
}

func (c *Client) FetchUsage(ctx context.Context, accessToken string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+usagePath, nil)
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
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, fmt.Errorf("read usage response: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return Result{}, fmt.Errorf("%w: status %d", errUnauthorized, resp.StatusCode)
	case resp.StatusCode == http.StatusForbidden,
		resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode >= 500:
		// 403 used to be treated as "token is bad", which forced a refresh on
		// every poll tick against an endpoint that was rate-limiting us.
		return Result{}, &TransientError{
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			err:        fmt.Errorf("%w: status %d", errTransient, resp.StatusCode),
		}
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return Result{}, fmt.Errorf("usage status %d: %s", resp.StatusCode, truncate(body))
	}
	return FromUsagePayload(body)
}

func IsUnauthorized(err error) bool {
	return errors.Is(err, errUnauthorized)
}

func IsTransient(err error) bool {
	return errors.Is(err, errTransient)
}

// RetryAfter reports a server-requested wait, if the error carries one.
func RetryAfter(err error) (time.Duration, bool) {
	var transient *TransientError
	if errors.As(err, &transient) && transient.RetryAfter > 0 {
		return transient.RetryAfter, true
	}
	return 0, false
}

func parseRetryAfter(value string) time.Duration {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(trimmed); err == nil {
		if wait := time.Until(when); wait > 0 {
			return wait
		}
	}
	return 0
}

// truncate keeps an unexpected upstream body out of the journal at full size.
func truncate(b []byte) string {
	const max = 200
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
