// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

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
}

func NewService(lastKnown *LastKnownStore) *Service {
	return &Service{
		HTTPClient:   httpx.New(),
		APIBaseURL:   DefaultAPIBaseURL,
		TokenBaseURL: DefaultTokenBaseURL,
		LastKnown:    lastKnown,
		Now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Resolve(ctx context.Context, accountHome string) (SnapshotFields, error) {
	now := s.Now()
	oauth, err := creds.EnsureAccessToken(ctx, accountHome, s.HTTPClient, s.TokenBaseURL, now)
	if err != nil {
		if errors.Is(err, creds.ErrRefreshRejected) {
			// Signed Out is a usable face, not a failure — but the cause is
			// reported so it reaches the journal, and so the poller backs off
			// instead of re-attempting a rejected grant every minute.
			return SnapshotFields{Face: snapshot.FaceSignedOut, UsageCredit: "none"},
				fmt.Errorf("account home %s is signed out: %w", accountHome, err)
		}
		return s.fallback(accountHome, now, err)
	}

	client := NewClient(s.HTTPClient, s.APIBaseURL)
	result, err := client.FetchUsage(ctx, oauth.AccessToken)
	if err != nil {
		// Only a 401 justifies burning the refresh grant. A 403 or 429 means
		// wait, and forcing a refresh for those rotated the grant every minute.
		if IsUnauthorized(err) {
			refreshedOAuth, refreshErr := creds.RefreshAccessToken(ctx, accountHome, s.HTTPClient, s.TokenBaseURL)
			if refreshErr != nil {
				if errors.Is(refreshErr, creds.ErrRefreshRejected) {
					return SnapshotFields{Face: snapshot.FaceSignedOut, UsageCredit: "none"},
						fmt.Errorf("account home %s is signed out: %w", accountHome, refreshErr)
				}
				return s.fallback(accountHome, now, refreshErr)
			}
			result, err = client.FetchUsage(ctx, refreshedOAuth.AccessToken)
		}
		if err != nil {
			return s.fallback(accountHome, now, err)
		}
	}

	fetchedAt := now.Unix()
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
			return SnapshotFields{
				Face:             snapshot.FaceReady,
				SessionAllowance: result.Session,
				WeeklyAllowance:  result.Weekly,
				UsageCredit:      result.UsageCredit,
				UsageCreditSpend: result.UsageCreditSpend,
				FetchedAt:        fetchedAt,
			}, fmt.Errorf("cache last-known allowance: %w", saveErr)
		}
	}

	return SnapshotFields{
		Face:             snapshot.FaceReady,
		SessionAllowance: result.Session,
		WeeklyAllowance:  result.Weekly,
		UsageCredit:      result.UsageCredit,
		UsageCreditSpend: result.UsageCreditSpend,
		FetchedAt:        fetchedAt,
	}, nil
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
	}, nil
}
