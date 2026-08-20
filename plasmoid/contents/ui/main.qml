// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.plasma.components as PlasmaComponents
import org.kde.plasma.plasmoid
import org.kde.plasma.workspace.dbus as DBus

PlasmoidItem {
    id: root

    property string accountHome: plasmoid.configuration.accountHome || ""
    property bool isBound: accountHome.trim() !== ""
    property string faceText: "unbound"
    property string snapshotText: ""
    property string statusText: ""

    DBus.DBusServiceWatcher {
        id: helperWatcher
        busType: DBus.BusType.Session
        watchedService: "dev.codecap.Helper"
    }

    function weekStart() {
        return Qt.locale().firstDayOfWeek
    }

    function timezoneId() {
        let now = new Date()
        let fallback = now.toString().match(/\(([^)]+)\)$/)
        if (fallback && fallback.length > 1) {
            return fallback[1]
        }
        return "UTC"
    }

    function updateSnapshot() {
        if (!isBound) {
            faceText = "unbound"
            snapshotText = ""
            statusText = i18n("Pick an Account Home in Configure.")
            return
        }

        if (!helperWatcher.registered) {
            faceText = "unknown_allowance"
            snapshotText = ""
            statusText = i18n("Helper not installed or not running.")
            return
        }

        DBus.SessionBus.asyncCall({
            "service": "dev.codecap.Helper",
            "path": "/dev/codecap/Helper",
            "iface": "dev.codecap.Helper",
            "member": "GetSnapshot",
            "arguments": [accountHome, timezoneId(), weekStart()]
        }, function(result) {
            let payload = result
            if (payload && payload.value !== undefined) {
                payload = payload.value
            }
            if (Array.isArray(payload) && payload.length > 0) {
                payload = payload[0]
            }
            let decoded = JSON.parse(payload)
            faceText = decoded.face || "unknown_allowance"
            snapshotText = JSON.stringify(decoded)
            statusText = i18n("Snapshot loaded from helper.")
        }, function(err) {
            faceText = "unknown_allowance"
            snapshotText = ""
            statusText = i18n("D-Bus call failed: %1", err.error ? err.error.message : err)
        })
    }

    Component.onCompleted: updateSnapshot()
    onAccountHomeChanged: updateSnapshot()

    Connections {
        target: helperWatcher
        function onRegisteredChanged() {
            root.updateSnapshot()
        }
    }

    Plasmoid.compactRepresentation: Kirigami.Icon {
        source: root.isBound ? (helperWatcher.registered ? "view-statistics" : "question-symbolic") : "folder-open-symbolic"
    }

    Plasmoid.fullRepresentation: Kirigami.ScrollablePage {
        title: i18n("codecap smoke")

        ColumnLayout {
            width: parent.width
            spacing: Kirigami.Units.smallSpacing

            Kirigami.PlaceholderMessage {
                Layout.fillWidth: true
                visible: !root.isBound
                icon.name: "folder-open-symbolic"
                text: i18n("No Account Home selected.")
                explanation: i18n("Open widget configuration and set Account Home (for example ~/.claude).")
            }

            Kirigami.PlaceholderMessage {
                Layout.fillWidth: true
                visible: root.isBound && !helperWatcher.registered
                icon.name: "question-symbolic"
                text: i18n("Helper unavailable.")
                explanation: i18n("Install or run the codecap helper to fetch snapshots.")
            }

            PlasmaComponents.Label {
                Layout.fillWidth: true
                visible: root.isBound
                text: i18n("Face: %1", root.faceText)
                wrapMode: Text.Wrap
            }

            PlasmaComponents.Label {
                Layout.fillWidth: true
                visible: root.statusText.length > 0
                text: root.statusText
                wrapMode: Text.Wrap
            }

            PlasmaComponents.Label {
                Layout.fillWidth: true
                visible: root.snapshotText.length > 0
                text: root.snapshotText
                wrapMode: Text.WrapAnywhere
            }
        }
    }
}
