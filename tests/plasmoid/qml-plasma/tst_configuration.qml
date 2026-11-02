// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtTest

TestCase {
    id: testCase
    name: "ConfigurationPage"
    visible: true
    width: 640
    height: 480
    property var page: null

    SignalSpy { id: enabledSpy; target: testCase.page; signalName: "cfg_widgetMarkerEnabledChanged" }
    SignalSpy { id: colorSpy; target: testCase.page; signalName: "cfg_widgetMarkerColorChanged" }

    function makePage(properties) {
        // The real page needs the translation context that Plasma supplies.
        // Ignore only that known harness omission; no global is replaced.
        for (var i = 0; i < 10; ++i) {
            ignoreWarning(/ReferenceError: i18n is not defined/)
        }
        var component = Qt.createComponent(Qt.resolvedUrl("../../../plasmoid/contents/ui/configGeneral.qml"))
        var item = createTemporaryObject(component, testCase, properties)
        verify(item !== null, "Plasma must be able to instantiate the actual configuration page")
        component.destroy()
        return item
    }

    function cleanup() { page = null }

    function test_marker_changes_emit_the_signals_plasma_monitors() {
        page = makePage({cfg_widgetMarkerEnabled: false, cfg_widgetMarkerColor: ""})
        verify(enabledSpy.valid)
        verify(colorSpy.valid)
        enabledSpy.clear()
        colorSpy.clear()
        var picker = findChild(page, "markerColorDialog")
        verify(picker !== null)
        var settings = findChild(page, "markerColorButton").parent
        settings.markerEnabled = true
        compare(page.cfg_widgetMarkerEnabled, true)
        verify(page.hasOwnProperty("cfg_widgetMarkerEnabled"))
        compare(enabledSpy.count, 1)
        picker.selectedColor = Qt.rgba(0.2, 0.4, 0.6, 1)
        picker.accepted()
        compare(page.cfg_widgetMarkerColor, "#336699")
        verify(page.hasOwnProperty("cfg_widgetMarkerColor"))
        compare(colorSpy.count, 1)
    }

    function test_saved_marker_preferences_are_restored_on_the_actual_page() {
        page = makePage({cfg_widgetMarkerEnabled: true, cfg_widgetMarkerColor: "#336699"})
        compare(page.cfg_widgetMarkerEnabled, true)
        compare(page.cfg_widgetMarkerColor, "#336699")
    }
}
