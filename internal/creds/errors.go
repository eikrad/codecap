// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package creds

import "errors"

// ErrRefreshRejected means the OAuth refresh grant is gone and re-login is required.
var ErrRefreshRejected = errors.New("oauth refresh rejected")

// ErrCredentialsUnsafe means the credentials path is not a plain file this user
// owns — a symlink, a FIFO, a hard link, or someone else's file.
var ErrCredentialsUnsafe = errors.New("credentials file is not safe to use")
