// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// These load the real applet components, so they need Kirigami — which CI does
// not install. They live outside tests/plasmoid/qml for that reason and run
// from `make test-plasmoid-qml-plasma`, which skips itself when the import is
// missing. tests/plasmoid/qml stays importable with Qt alone.
//
// What this file covers that logic.js tests cannot: that the QML wiring feeding
// allowanceFillColor is intact, and that the ring resolves a Kirigami.Theme
// colour rather than an undefined property.
import QtQuick
import QtTest
import org.kde.kirigami as Kirigami
import "../../../plasmoid/contents/ui" as Ui

TestCase {
    name: "RingColour"

    Ui.CompactRing {
        id: ring
        face: "ready"
        helperAvailable: true
        isBound: true
        usedPercent: 37
        stale: false
    }

    // 37% is the Session that shipped red: the Account Home behind it had
    // Usage Credit "exhausted", and the ring took its colour from that instead
    // of from the window it draws. Usage Credit is no longer a property on this
    // component, so the old behaviour is now a load error rather than a wrong
    // colour — this pins the outcome, not the removed path.
    function test_a_mid_band_session_draws_the_accent() {
        verify(ring.showRing)
        verify(ring.ringColor !== Kirigami.Theme.negativeTextColor)
        compare(ring.ringColor, Kirigami.Theme.highlightColor)
    }

    function test_bands_follow_the_session_fill() {
        ring.usedPercent = 85
        compare(ring.ringColor, Kirigami.Theme.neutralTextColor)
        ring.usedPercent = 100
        compare(ring.ringColor, Kirigami.Theme.negativeTextColor)
        ring.usedPercent = 37
        ring.stale = true
        compare(ring.ringColor, Kirigami.Theme.disabledTextColor)
        ring.stale = false
    }
}
