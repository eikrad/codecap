// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtTest
import org.kde.kirigami as Kirigami
import "../../../plasmoid/contents/ui" as Ui

TestCase {
    id: testCase
    name: "WidgetMarker"
    visible: true
    width: 100
    height: 100
    property var ring

    Component {
        id: ringComponent
        Ui.CompactRing {
            width: 36
            height: 36
            face: "ready"
            helperAvailable: true
            isBound: true
            usedPercent: 37
        }
    }

    function init() {
        // Plasma supplies i18n; qmltestrunner does not. Exercise the real
        // component without replacing any platform global.
        ignoreWarning(/ReferenceError: i18n is not defined/)
        ring = createTemporaryObject(ringComponent, testCase)
        verify(ring !== null)
    }

    function test_marker_is_opt_in() {
        var marker = findChild(ring, "widgetMarker")
        verify(marker !== null, "the actual compact Face must contain the marker")
        compare(marker.visible, false)
    }

    function test_marker_color_does_not_change_allowance_colors() {
        ring.markerEnabled = true
        ring.markerColor = Qt.rgba(0.2, 0.4, 0.6, 1)
        var marker = findChild(ring, "widgetMarker")
        compare(marker.visible, true)
        compare(marker.color, ring.markerColor)
        compare(ring.ringColor, Kirigami.Theme.highlightColor)
        ignoreWarning(/ReferenceError: i18n is not defined/)
        ring.usedPercent = 100
        compare(ring.ringColor, Kirigami.Theme.negativeTextColor)
        compare(marker.color, ring.markerColor)
        ring.stale = true
        compare(ring.ringColor, Kirigami.Theme.disabledTextColor)
        compare(marker.color, ring.markerColor)
    }

    function test_marker_fits_panel_and_tray_sizes() {
        ring.markerEnabled = true
        var marker = findChild(ring, "widgetMarker")
        for (var i = 0; i < 3; ++i) {
            var size = [24, 36, 72][i]
            ring.width = size
            ring.height = size
            verify(marker.width >= 4)
            verify(marker.x >= 0 && marker.y >= 0)
            verify(marker.x + marker.width <= ring.width)
            verify(marker.y + marker.height <= ring.height)
        }
    }

    function test_marker_stays_visible_when_the_account_needs_attention() {
        ring.markerEnabled = true
        ring.face = "signed_out"
        compare(ring.showRing, false)
        compare(findChild(ring, "widgetMarker").visible, true)
    }
}
