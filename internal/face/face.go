// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package face

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/eikrad/codecap/internal/agent"
	"github.com/eikrad/codecap/internal/creds"
	"github.com/eikrad/codecap/internal/snapshot"
)

// CredentialsFileName is Claude Code's Account Home OAuth store on Linux.
const CredentialsFileName = creds.CredentialsFileName

func Classify(accountHome string) snapshot.Snapshot {
	trimmed := strings.TrimSpace(accountHome)
	if trimmed == "" {
		return baseSnapshot(snapshot.FaceUnbound, "")
	}

	info, err := os.Stat(trimmed)
	if err != nil || !info.IsDir() {
		return baseSnapshot(snapshot.FaceSignedOut, trimmed)
	}
	a, err := agent.Detect(trimmed)
	if err != nil {
		return baseSnapshot(snapshot.FaceUnknownAllowance, trimmed)
	}
	if a == agent.Codex {
		// Codex owns its authentication (including keyring storage). The
		// helper's background app-server read resolves Signed Out; local
		// Consumed Usage can be shown while that read is pending.
		return baseSnapshot(snapshot.FaceUnknownAllowance, trimmed)
	}

	if _, err := creds.Load(trimmed); err != nil {
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
		SchemaVersion: snapshot.SchemaVersion,
		Face:          face,
		AccountHome:   accountHome,
		AccountLabel:  accountLabel,
		UsageCredit:   "none",
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
