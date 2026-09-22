// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// The bus-contract block in design.md is a sketch, with Face names written as
// alternatives, so it cannot be byte-compared to a Face document. Its keys
// still have to be the keys those documents send. A field added on one side
// only is the drift this exists to catch.
func TestBusContractSketchHasTheSameKeysAsTheFaceDocuments(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("..", "..", "docs", "design.md"))
	if err != nil {
		t.Fatalf("read design.md: %v", err)
	}
	raw, err := busContractJSON(text)
	if err != nil {
		t.Fatal(err)
	}
	var sketch any
	if err := json.Unmarshal(raw, &sketch); err != nil {
		t.Fatalf("bus-contract JSON: %v", err)
	}
	documented := map[string]bool{}
	for _, key := range objectPaths(sketch) {
		documented[key] = true
	}

	sent := map[string]bool{}
	for _, face := range Faces() {
		data, err := os.ReadFile(filepath.Join("testdata", string(face)+".json"))
		if err != nil {
			t.Fatalf("read %s: %v", face, err)
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("unmarshal %s: %v", face, err)
		}
		for _, key := range objectPaths(doc) {
			sent[key] = true
		}
	}

	for key := range sent {
		if !documented[key] {
			t.Errorf("Face documents send %q, the bus-contract sketch does not list it", key)
		}
	}
	for key := range documented {
		if !sent[key] {
			t.Errorf("bus-contract sketch lists %q, no Face document sends it", key)
		}
	}
}

func busContractJSON(design []byte) ([]byte, error) {
	const open = "```json\n"
	start := strings.Index(string(design), open)
	if start < 0 {
		return nil, fmt.Errorf("design.md has no json fence")
	}
	rest := string(design)[start+len(open):]
	end := strings.Index(rest, "\n```")
	if end < 0 {
		return nil, fmt.Errorf("design.md json fence is not closed")
	}
	return []byte(rest[:end]), nil
}

func objectPaths(v any) []string {
	var paths []string
	var walk func(prefix string, val any)
	walk = func(prefix string, val any) {
		obj, ok := val.(map[string]any)
		if !ok {
			if prefix != "" {
				paths = append(paths, prefix)
			}
			return
		}
		keys := make([]string, 0, len(obj))
		for key := range obj {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			if _, nested := obj[key].(map[string]any); nested {
				walk(next, obj[key])
				continue
			}
			paths = append(paths, next)
		}
	}
	walk("", v)
	sort.Strings(paths)
	return paths
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
