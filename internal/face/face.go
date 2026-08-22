// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package face

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/eikrad/codecap/internal/snapshot"
)

// CredentialsFileName is Claude Code's Account Home OAuth store on Linux.
const CredentialsFileName = ".credentials.json"

var (
	stat     = os.Stat
	readFile = os.ReadFile
)

func Classify(accountHome string) snapshot.Snapshot {
	trimmed := strings.TrimSpace(accountHome)
	if trimmed == "" {
		return baseSnapshot(snapshot.FaceUnbound, "")
	}

	info, err := stat(trimmed)
	if err != nil || !info.IsDir() {
		return baseSnapshot(snapshot.FaceSignedOut, trimmed)
	}

	if !hasUsableOAuth(trimmed) {
		return baseSnapshot(snapshot.FaceSignedOut, trimmed)
	}

	return baseSnapshot(snapshot.FaceUnknownAllowance, trimmed)
}

func hasUsableOAuth(accountHome string) bool {
	path := filepath.Join(accountHome, CredentialsFileName)
	data, err := readFile(path)
	if err != nil {
		return false
	}

	var payload struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}
	return strings.TrimSpace(payload.ClaudeAiOauth.AccessToken) != ""
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
