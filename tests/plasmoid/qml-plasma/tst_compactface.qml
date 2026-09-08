// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// P-C3: the compact representation had no MouseArea at all, so the popup could
// not be opened -- one of the four criticals of 2026-08-22 that meant the widget
// did not work. A custom compactRepresentation replaces Plasma's
// DefaultCompactRepresentation, which is where click-to-expand normally lives,
// so it has to be restored by hand or the expanded face is unreachable.
//
// It sat in main.qml, which no test runner can instantiate: PlasmoidItem only
// works inside Plasma's applet machinery. CompactFace is that representation
// taking plain properties, for the same reason SnapshotSource and CompactRing
// do -- so a click can actually be delivered to it here.
//
// Needs Kirigami (CompactRing resolves theme colours), so it lives in
// qml-plasma, which skips itself where the import is missing.
import QtQuick
import QtTest
import "../../../plasmoid/contents/ui" as Ui

TestCase {
    id: testCase
    name: "CompactFace"
    when: windowShown
    // A TestCase is invisible by default and children inherit that, so a
    // MouseArea inside one never receives input and every mouseClick silently
    // does nothing. Both of these are needed before a click can land.
    visible: true
    width: 200
    height: 200

    property int activations: 0

    Ui.CompactFace {
        id: face
        width: 40
        height: 40
        face: "ready"
        helperAvailable: true
        isBound: true
        usedPercent: 37
        stale: false
        onActivated: testCase.activations++
    }

    function init() {
        testCase.activations = 0
    }

    // The assertion the missing MouseArea would have failed. A click on the
    // compact face has to reach the applet, or the popup cannot be opened.
    function test_a_click_on_the_compact_face_asks_to_expand() {
        mouseClick(face)
        compare(testCase.activations, 1)
    }

    // Every click toggles, so opening and closing both work from the panel.
    function test_each_click_asks_again() {
        mouseClick(face)
        mouseClick(face)
        compare(testCase.activations, 2)
    }

    // The ring is what the panel actually shows; an empty clickable box would
    // pass the assertions above and show nothing.
    function test_the_face_draws_the_session_ring() {
        verify(face.implicitWidth > 0)
        verify(face.implicitHeight > 0)
        verify(face.showsRing)
    }
}
