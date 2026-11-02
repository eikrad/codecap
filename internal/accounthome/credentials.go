// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package accounthome

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrCredentialsUnsafe identifies a credential path that cannot safely be read.
var ErrCredentialsUnsafe = errors.New("credentials file is not safe to use")

// MaxCredentialsBytes bounds an Agent's small JSON credential store.
const MaxCredentialsBytes = 1 << 20

// OpenCredentials is the shared Account Home file-opening policy. A returned
// descriptor is a bounded regular file owned by this user with one link.
// The caller owns closing it; no credential contents are interpreted here.
func OpenCredentials(home, name string) (*os.File, error) {
	home, err := Validate(home)
	if err != nil {
		return nil, err
	}
	if home == "" || name == "" || name == "." || name == ".." || name != filepath.Base(name) {
		return nil, fmt.Errorf("%w: invalid Account Home or credential filename", ErrCredentialsUnsafe)
	}
	// O_NONBLOCK is essential: a FIFO blocks before Stat could reject it.
	file, err := os.OpenFile(filepath.Join(home, name), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("%w: symbolic link", ErrCredentialsUnsafe)
		}
		return nil, fmt.Errorf("open credentials: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat credentials: %w", err)
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Size() > MaxCredentialsBytes || !ok || int64(owner.Uid) != int64(os.Getuid()) || owner.Nlink != 1 {
		_ = file.Close()
		return nil, ErrCredentialsUnsafe
	}
	return file, nil
}
