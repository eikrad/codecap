// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package face

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/eikrad/codecap/internal/snapshot"
)

const LoginSentinelName = "login"

var stat = os.Stat

func Classify(accountHome string) snapshot.Snapshot {
	trimmed := strings.TrimSpace(accountHome)
	if trimmed == "" {
		return baseSnapshot(snapshot.FaceUnbound, "")
	}

	info, err := stat(trimmed)
	if err != nil || !info.IsDir() {
		return baseSnapshot(snapshot.FaceSignedOut, trimmed)
	}

	loginPath := filepath.Join(trimmed, LoginSentinelName)
	if _, err := stat(loginPath); err != nil {
		return baseSnapshot(snapshot.FaceSignedOut, trimmed)
	}

	return baseSnapshot(snapshot.FaceUnknownAllowance, trimmed)
}

func baseSnapshot(face snapshot.Face, accountHome string) snapshot.Snapshot {
	accountLabel := ""
	if accountHome != "" {
		accountLabel = filepath.Base(strings.TrimRight(accountHome, "/"))
	}

	return snapshot.Snapshot{
		Face:         face,
		AccountHome:  accountHome,
		AccountLabel: accountLabel,
		UsageCredit:  "none",
		ConsumedUsage: snapshot.ConsumedUsage{
			Session: snapshot.ConsumedPeriod{},
			Today:   snapshot.ConsumedPeriod{},
			Week:    snapshot.ConsumedPeriod{},
			Month:   snapshot.ConsumedPeriod{},
		},
		SessionAllowance: snapshot.AllowanceWindow{},
		WeeklyAllowance:  snapshot.AllowanceWindow{},
		FetchedAt:        0,
	}
}
