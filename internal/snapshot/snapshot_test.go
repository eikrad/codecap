// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package snapshot

import (
	"encoding/json"
	"testing"
)

func TestSnapshotMarshalsExpectedTopLevelFields(t *testing.T) {
	s := Snapshot{Face: FaceUnknownAllowance}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	for _, key := range []string{
		"face",
		"account_home",
		"account_label",
		"session_allowance",
		"weekly_allowance",
		"usage_credit",
		"consumed_usage",
		"fetched_at",
	} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q", key)
		}
	}
}
