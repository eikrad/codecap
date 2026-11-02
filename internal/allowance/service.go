// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/eikrad/codecap/internal/agent"
	"github.com/eikrad/codecap/internal/codex"
	"github.com/eikrad/codecap/internal/creds"
	"github.com/eikrad/codecap/internal/httpx"
	"github.com/eikrad/codecap/internal/snapshot"
)

const DefaultTokenBaseURL = "https://console.anthropic.com"

// SnapshotFields is Allowance data merged into a GetSnapshot response.
type SnapshotFields struct {
	Face             snapshot.Face
	SessionAllowance snapshot.AllowanceWindow
	WeeklyAllowance  snapshot.AllowanceWindow
	UsageCredit      string
	UsageCreditSpend snapshot.UsageCreditSpend
	FetchedAt        int64
}

// Service resolves live or Last-Known Allowance for an Account Home.
type Service struct {
	HTTPClient   *http.Client
	APIBaseURL   string
	TokenBaseURL string
	LastKnown    *LastKnownStore
	Now          func() time.Time
	CodexFetch   func(context.Context, string) (codex.Result, error)
}

func NewService(lastKnown *LastKnownStore) *Service {
	return &Service{
		HTTPClient:   httpx.New(),
		APIBaseURL:   DefaultAPIBaseURL,
		TokenBaseURL: DefaultTokenBaseURL,
		LastKnown:    lastKnown,
		Now:          func() time.Time { return time.Now().UTC() },
		CodexFetch:   codex.FetchAllowance,
	}
}

func (s *Service) Resolve(ctx context.Context, accountHome string) (SnapshotFields, error) {
	now := s.Now()
	a, err := agent.Detect(accountHome)
	if err != nil {
		return s.fallback(accountHome, now, err)
	}
	var result Result
	if a == agent.Codex {
		var fetched codex.Result
		fetched, err = s.CodexFetch(ctx, accountHome)
		result = Result{Session: fetched.Session, Weekly: fetched.Weekly, UsageCredit: "none"}
	} else {
		result, err = s.fetchClaude(ctx, accountHome, now)
	}
	if errors.Is(err, codex.ErrSignedOut) || errors.Is(err, creds.ErrRefreshRejected) {
		return SnapshotFields{Face: snapshot.FaceSignedOut, UsageCredit: "none"}, fmt.Errorf("account home %s is signed out: %w", accountHome, err)
	}
	if err != nil {
		return s.fallback(accountHome, now, err)
	}
	return s.remember(accountHome, now, result)
}

func (s *Service) fetchClaude(ctx context.Context, accountHome string, now time.Time) (Result, error) {
	oauth, err := creds.EnsureAccessToken(ctx, accountHome, s.HTTPClient, s.TokenBaseURL, now)
	if err != nil {
		return Result{}, err
	}
	client := NewClient(s.HTTPClient, s.APIBaseURL)
	result, err := client.FetchUsage(ctx, oauth.AccessToken)
	// Only a 401 justifies refreshing the grant. A 403/429 must back off.
	if IsUnauthorized(err) {
		oauth, err = creds.RefreshAccessToken(ctx, accountHome, s.HTTPClient, s.TokenBaseURL)
		if err != nil {
			return Result{}, err
		}
		return client.FetchUsage(ctx, oauth.AccessToken)
	}
	return result, err
}

func (s *Service) remember(accountHome string, now time.Time, result Result) (SnapshotFields, error) {
	fetchedAt := now.Unix()
	fields := SnapshotFields{
		Face: snapshot.FaceReady, SessionAllowance: result.Session, WeeklyAllowance: result.Weekly,
		UsageCredit: result.UsageCredit, UsageCreditSpend: result.UsageCreditSpend, FetchedAt: fetchedAt,
	}
	if s.LastKnown != nil {
		if saveErr := s.LastKnown.Save(accountHome, CachedAllowance{
			FetchedAt:        fetchedAt,
			Session:          result.Session,
			Weekly:           result.Weekly,
			UsageCredit:      result.UsageCredit,
			UsageCreditSpend: result.UsageCreditSpend,
		}); saveErr != nil {
			// Not fatal: the live result stands, only the offline fallback is
			// missing. Reported so a broken cache directory is visible.
			return fields, fmt.Errorf("cache last-known allowance: %w", saveErr)
		}
	}

	return fields, nil
}

func (s *Service) fallback(accountHome string, now time.Time, cause error) (SnapshotFields, error) {
	if s.LastKnown == nil {
		return SnapshotFields{Face: snapshot.FaceUnknownAllowance, UsageCredit: "none"}, fmt.Errorf("allowance unavailable: %w", cause)
	}
	cached, ok, err := s.LastKnown.Load(accountHome, now)
	if err != nil {
		return SnapshotFields{Face: snapshot.FaceUnknownAllowance, UsageCredit: "none"}, err
	}
	if !ok {
		return SnapshotFields{Face: snapshot.FaceUnknownAllowance, UsageCredit: "none"}, fmt.Errorf("allowance unavailable: %w", cause)
	}
	return SnapshotFields{
		Face:             snapshot.FaceReady,
		SessionAllowance: cached.Session,
		WeeklyAllowance:  cached.Weekly,
		UsageCredit:      cached.UsageCredit,
		UsageCreditSpend: cached.UsageCreditSpend,
		FetchedAt:        cached.FetchedAt,
	}, cause
}
