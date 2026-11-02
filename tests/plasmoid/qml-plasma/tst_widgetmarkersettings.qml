// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtTest
import org.kde.kirigami as Kirigami
import "../../../plasmoid/contents/ui" as Ui

TestCase {
    id: testCase
    name: "WidgetMarkerSettings"
    visible: true
    property var settings

    Component {
        id: settingsComponent
        Ui.WidgetMarkerSettings {}
    }

    function createSettings() {
        // Exactly the two control labels and dialog title need Plasma's
        // translation context. No platform global is stubbed here.
        for (var i = 0; i < 3; ++i) {
            ignoreWarning(/ReferenceError: i18n is not defined/)
        }
        var item = createTemporaryObject(settingsComponent, testCase)
        verify(item !== null)
        return item
    }

    function init() {
        settings = createSettings()
    }

    function test_defaults_follow_the_desktop_accent_with_marker_disabled() {
        compare(settings.markerEnabled, false)
        compare(settings.markerColor, "")
        compare(settings.effectiveColor, Kirigami.Theme.highlightColor)
        compare(findChild(settings, "markerColorButton").enabled, false)
    }

    function test_accepted_color_round_trips_as_a_config_string() {
        var selected = Qt.rgba(0.2, 0.4, 0.6, 1)
        var dialog = findChild(settings, "markerColorDialog")
        dialog.selectedColor = selected
        dialog.accepted()
        verify(/^#[0-9a-f]{6,8}$/i.test(settings.markerColor))
        compare(settings.effectiveColor, selected)

        var reloaded = createSettings()
        reloaded.markerColor = settings.markerColor
        compare(reloaded.effectiveColor, selected)
    }

    function test_cancelling_and_disabling_preserve_the_saved_color() {
        settings.markerEnabled = true
        var dialog = findChild(settings, "markerColorDialog")
        dialog.selectedColor = Qt.rgba(0.2, 0.4, 0.6, 1)
        dialog.accepted()
        var saved = settings.markerColor
        dialog.selectedColor = Qt.rgba(0.7, 0.3, 0.1, 1)
        dialog.rejected()
        compare(settings.markerColor, saved)
        settings.markerEnabled = false
        compare(settings.markerColor, saved)
        settings.markerEnabled = true
        compare(settings.markerColor, saved)
    }

    function test_two_widgets_keep_independent_colors() {
        var other = createSettings()
        var dialog = findChild(settings, "markerColorDialog")
        dialog.selectedColor = Qt.rgba(0.2, 0.4, 0.6, 1)
        dialog.accepted()
        compare(other.markerColor, "")
        compare(other.markerEnabled, false)
    }
}
