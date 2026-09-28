// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package snapshot

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// go test ./internal/snapshot -update rewrites testdata from Example. The
// documents are generated, never edited: a contract change is a change to
// Example or to the struct tags, and this is how it reaches the files.
var update = flag.Bool("update", false, "rewrite testdata/<face>.json from Example")

// The document on disk is the contract the plasmoid reads. It has to be the
// bytes json.Marshal emits for that Face, because that is what GetSnapshot
// puts on the bus. A renamed struct tag or a hand-edited file fails here.
func TestEachFaceHasTheDocumentTheHelperSends(t *testing.T) {
	for _, face := range Faces() {
		t.Run(string(face), func(t *testing.T) {
			sent, err := json.Marshal(Example(face))
			if err != nil {
				t.Fatalf("marshal %s: %v", face, err)
			}
			if *update {
				if err := os.WriteFile(facePath(face), sent, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			onDisk := readFace(t, face)
			if !bytes.Equal(onDisk, sent) {
				t.Fatalf("testdata/%s.json is not the JSON the helper sends (go test ./internal/snapshot -update)\nfile: %s\nsent: %s", face, onDisk, sent)
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
	objectPaths("", sketch, documented)

	sent := map[string]bool{}
	for _, face := range Faces() {
		var doc any
		if err := json.Unmarshal(readFace(t, face), &doc); err != nil {
			t.Fatalf("unmarshal %s: %v", face, err)
		}
		objectPaths("", doc, sent)
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

func facePath(face Face) string {
	return filepath.Join("testdata", string(face)+".json")
}

func readFace(t *testing.T, face Face) []byte {
	t.Helper()
	data, err := os.ReadFile(facePath(face))
	if err != nil {
		t.Fatalf("read %s: %v", face, err)
	}
	return data
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

// objectPaths adds the dotted path of every leaf under v to paths. Arrays are
// leaves: degraded is one key whatever it holds.
func objectPaths(prefix string, v any, paths map[string]bool) {
	obj, ok := v.(map[string]any)
	if !ok {
		paths[prefix] = true
		return
	}
	for key, val := range obj {
		if prefix != "" {
			key = prefix + "." + key
		}
		objectPaths(key, val, paths)
	}
}
