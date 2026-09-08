// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// The applet's compact representation: the Session ring, and the click that
// opens the popup.
//
// A custom compactRepresentation replaces Plasma's DefaultCompactRepresentation,
// which is where click-to-expand lives, so it has to be restored by hand or the
// expanded face is unreachable by mouse. It was not, and the popup could not be
// opened at all (finding P-C3, one of the four criticals of 2026-08-22).
//
// It lives here rather than inline in main.qml so that a test runner can
// instantiate it and deliver a real click. PlasmoidItem only works inside
// Plasma's applet machinery, so nothing that stays in main.qml can be executed
// by a test -- the same reason SnapshotSource and CompactRing were extracted,
// and the reason the missing MouseArea went unnoticed through a green pipeline.
//
// It takes plain properties and emits `activated`; deciding what that means is
// main.qml's job, because `expanded` belongs to the applet.

import QtQuick
import org.kde.kirigami as Kirigami

MouseArea {
    id: root

    property string face: "unbound"
    property bool helperAvailable: false
    property bool isBound: false
    property real usedPercent: 0
    property bool stale: false

    // Whether the ring is drawn rather than a face icon. Exposed so a test can
    // tell a working compact face from an empty box that merely accepts clicks.
    readonly property bool showsRing: ring.showRing

    // The panel gives this its size, so implicit size comes from the ring.
    implicitWidth: ring.implicitWidth
    implicitHeight: ring.implicitHeight

    acceptedButtons: Qt.LeftButton

    // Not `expanded = !expanded`: this component does not own that state, and a
    // property it toggled would have to be bound back to the applet's own.
    signal activated

    onClicked: root.activated()

    CompactRing {
        id: ring
        anchors.fill: parent
        face: root.face
        helperAvailable: root.helperAvailable
        isBound: root.isBound
        usedPercent: root.usedPercent
        stale: root.stale
        showNumeral: root.width >= Kirigami.Units.gridUnit * 2.5
    }
}
