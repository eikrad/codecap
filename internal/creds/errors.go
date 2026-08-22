// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package creds

import "errors"

// ErrRefreshRejected means the OAuth refresh grant is gone and re-login is required.
var ErrRefreshRejected = errors.New("oauth refresh rejected")
