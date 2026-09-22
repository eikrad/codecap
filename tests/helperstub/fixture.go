// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import "github.com/eikrad/codecap/internal/snapshot"

// Fixture is the shared Face example. A second set of numbers here would let
// the harness pass while the document the plasmoid parses had drifted.
func Fixture(face string) snapshot.Snapshot {
	return snapshot.Example(snapshot.Face(face))
}
