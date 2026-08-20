// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/eikrad/codecap/internal/creds"
	"github.com/eikrad/codecap/internal/snapshot"
)

const DefaultTokenBaseURL = "https://console.anthropic.com"

// SnapshotFields is Allowance data merged into a GetSnapshot response.
type SnapshotFields struct {
	Face             snapshot.Face
	SessionAllowance snapshot.AllowanceWindow
	WeeklyAllowance  snapshot.AllowanceWindow
	UsageCredit      string
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
		HTTPClient:   http.DefaultClient,
		APIBaseURL:   DefaultAPIBaseURL,
		TokenBaseURL: DefaultTokenBaseURL,
		LastKnown:    lastKnown,
		Now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Resolve(accountHome string) (SnapshotFields, error) {
	now := s.Now()
	oauth, err := creds.EnsureAccessToken(accountHome, s.HTTPClient, s.TokenBaseURL, now)
	if err != nil {
		if errors.Is(err, creds.ErrRefreshRejected) {
			return SnapshotFields{Face: snapshot.FaceSignedOut, UsageCredit: "none"}, nil
		}
		return s.fallback(accountHome, now, err)
	}

	client := NewClient(s.HTTPClient, s.APIBaseURL)
	result, err := client.FetchUsage(oauth.AccessToken)
	if err != nil {
		if IsUnauthorized(err) {
			oauth, refreshErr := creds.RefreshAccessToken(accountHome, s.HTTPClient, s.TokenBaseURL)
			if refreshErr != nil {
				if errors.Is(refreshErr, creds.ErrRefreshRejected) {
					return SnapshotFields{Face: snapshot.FaceSignedOut, UsageCredit: "none"}, nil
				}
				return s.fallback(accountHome, now, refreshErr)
			}
			result, err = client.FetchUsage(oauth.AccessToken)
		}
		if err != nil {
			return s.fallback(accountHome, now, err)
		}
	}

	fetchedAt := now.Unix()
	if s.LastKnown != nil {
		_ = s.LastKnown.Save(accountHome, CachedAllowance{
			FetchedAt:   fetchedAt,
			Session:     result.Session,
			Weekly:      result.Weekly,
			UsageCredit: result.UsageCredit,
		})
	}

	return SnapshotFields{
		Face:             snapshot.FaceReady,
		SessionAllowance: result.Session,
		WeeklyAllowance:  result.Weekly,
		UsageCredit:      result.UsageCredit,
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
		return SnapshotFields{Face: snapshot.FaceUnknownAllowance, UsageCredit: "none"}, nil
	}
	return SnapshotFields{
		Face:             snapshot.FaceReady,
		SessionAllowance: cached.Session,
		WeeklyAllowance:  cached.Weekly,
		UsageCredit:      cached.UsageCredit,
		FetchedAt:        cached.FetchedAt,
	}, nil
}
