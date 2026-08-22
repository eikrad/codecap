// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// Package httpx builds the HTTP client the helper uses for vendor calls.
package httpx

import (
	"net/http"
	"time"
)

// Timeout bounds a whole vendor request. http.DefaultClient has no timeout at
// all, so a blackholed connection parked a goroutine forever — while holding
// the credentials lock.
const Timeout = 15 * time.Second

// New returns the client every vendor call should use.
//
// Redirects are not followed: the refresh grant travels in a POST body, and Go
// replays request bodies on 307 and 308. It strips Authorization across hosts
// but has no such protection for a body, so a redirect away from
// console.anthropic.com would hand over the refresh token.
func New() *http.Client {
	return &http.Client{
		Timeout: Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
