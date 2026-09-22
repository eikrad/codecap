// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package snapshot

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The document on disk is the contract the plasmoid reads. It has to be the
// bytes json.Marshal emits for that Face, because that is what GetSnapshot
// puts on the bus. A renamed struct tag or a hand-edited file fails here.
func TestEachFaceHasTheDocumentTheHelperSends(t *testing.T) {
	for _, face := range Faces() {
		t.Run(string(face), func(t *testing.T) {
			path := filepath.Join("testdata", string(face)+".json")
			onDisk, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			sent, err := json.Marshal(Example(face))
			if err != nil {
				t.Fatalf("marshal %s: %v", face, err)
			}
			if !bytes.Equal(onDisk, sent) {
				t.Fatalf("testdata/%s.json is not the JSON the helper sends\nfile: %s\nsent: %s", face, onDisk, sent)
			}

			var decoded Snapshot
			if err := json.Unmarshal(onDisk, &decoded); err != nil {
				t.Fatalf("unmarshal %s: %v", face, err)
			}
			again, err := json.Marshal(decoded)
			if err != nil {
				t.Fatalf("remarshal %s: %v", face, err)
			}
			if !bytes.Equal(onDisk, again) {
				t.Fatalf("testdata/%s.json does not round-trip\nfile: %s\nagain: %s", face, onDisk, again)
			}
			if decoded.Face != face {
				t.Fatalf("document face %q, file is named %s", decoded.Face, face)
			}
			if decoded.SchemaVersion != SchemaVersion {
				t.Fatalf("schema_version %d, want %d", decoded.SchemaVersion, SchemaVersion)
			}
		})
	}
}

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
