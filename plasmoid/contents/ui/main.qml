// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.plasma.components as PlasmaComponents
import org.kde.plasma.plasmoid
import org.kde.plasma.workspace.dbus as DBus
import "logic.js" as Logic

PlasmoidItem {
    id: root

    property string accountHome: plasmoid.configuration.accountHome || ""
    property bool isBound: accountHome.trim() !== ""
    property var snapshot: Logic.emptySnapshot()
    property int nowUnix: Math.floor(Date.now() / 1000)
    property real fxRate: plasmoid.configuration.fxRateUsdToDisplay || 1.0
    property bool fxUsingUsdFallback: plasmoid.configuration.fxUsingUsdFallback || false
    property string fxNote: ""

    readonly property string displayCurrency: Logic.effectiveCurrency(
        plasmoid.configuration.displayCurrency,
        Qt.locale().name
    )
    readonly property bool helperAvailable: helperWatcher.registered

    DBus.DBusServiceWatcher {
        id: helperWatcher
        busType: DBus.BusType.Session
        watchedService: "dev.codecap.Helper"
    }

    Timer {
        interval: 30000
        running: true
        repeat: true
        onTriggered: root.nowUnix = Math.floor(Date.now() / 1000)
    }

    Timer {
        id: fxTimer
        interval: 60000
        running: true
        repeat: true
        onTriggered: root.refreshFxRate()
    }

    function weekStart() {
        return Qt.locale().firstDayOfWeek
    }

    function timezoneId() {
        const now = new Date()
        const fallback = now.toString().match(/\(([^)]+)\)$/)
        if (fallback && fallback.length > 1) {
            return fallback[1]
        }
        return "UTC"
    }

    function applySnapshotPayload(payload) {
        snapshot = Logic.parseSnapshot(payload)
        nowUnix = Math.floor(Date.now() / 1000)
    }

    function updateSnapshot() {
        if (!isBound) {
            snapshot = Logic.emptySnapshot()
            snapshot.face = "unbound"
            return
        }

        if (!helperAvailable) {
            snapshot = Logic.emptySnapshot()
            snapshot.face = "unknown_allowance"
            snapshot.account_home = accountHome
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
            applySnapshotPayload(payload)
        }, function() {
            snapshot = Logic.emptySnapshot()
            snapshot.face = "unknown_allowance"
            snapshot.account_home = accountHome
        })
    }

    function refreshFxRate() {
        const currency = displayCurrency
        if (currency === "USD") {
            fxRate = 1.0
            fxUsingUsdFallback = false
            fxNote = ""
            plasmoid.configuration.fxRateUsdToDisplay = 1.0
            plasmoid.configuration.fxUsingUsdFallback = false
            plasmoid.configuration.fxFetchedAt = nowUnix
            return
        }

        const fetchedAt = plasmoid.configuration.fxFetchedAt || 0
        if (nowUnix - fetchedAt < 86400 && plasmoid.configuration.fxRateUsdToDisplay > 0) {
            fxRate = plasmoid.configuration.fxRateUsdToDisplay
            fxUsingUsdFallback = plasmoid.configuration.fxUsingUsdFallback || false
            fxNote = fxUsingUsdFallback ? i18n("Display currency rate unavailable; showing USD.") : ""
            return
        }

        const xhr = new XMLHttpRequest()
        xhr.onreadystatechange = function() {
            if (xhr.readyState !== XMLHttpRequest.DONE) {
                return
            }
            if (xhr.status === 200) {
                const rates = Logic.parseEcbRates(xhr.responseText)
                const rate = Logic.usdToDisplayRate(rates, currency)
                if (rate > 0) {
                    fxRate = rate
                    fxUsingUsdFallback = false
                    fxNote = ""
                    plasmoid.configuration.fxRateUsdToDisplay = rate
                    plasmoid.configuration.fxUsingUsdFallback = false
                    plasmoid.configuration.fxFetchedAt = Math.floor(Date.now() / 1000)
                    return
                }
            }
            fxRate = 1.0
            fxUsingUsdFallback = true
            fxNote = i18n("Display currency rate unavailable; showing USD.")
            plasmoid.configuration.fxRateUsdToDisplay = 1.0
            plasmoid.configuration.fxUsingUsdFallback = true
        }
        xhr.open("GET", "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml")
        xhr.send()
    }

    function formatListPrice(usd) {
        return Logic.formatMoney(usd, fxRate, displayCurrency, fxUsingUsdFallback, Qt.locale())
    }

    function usageCreditLabel() {
        switch (snapshot.usage_credit) {
        case "enabled":
            return i18n("Usage credit enabled")
        case "available":
            return i18n("Usage credit available")
        case "exhausted":
            return i18n("Usage credit exhausted")
        default:
            return ""
        }
    }

    Component.onCompleted: {
        refreshFxRate()
        updateSnapshot()
    }

    DBus.SignalWatcher {
        id: helperChangedWatcher
        enabled: root.isBound && root.helperAvailable
        busType: DBus.BusType.Session
        service: "dev.codecap.Helper"
        path: "/dev/codecap/Helper"
        iface: "dev.codecap.Helper"

        function onReceivedSignal(message) {
            if (message.member !== "Changed") {
                return
            }
            const changedHome = message.arguments.length > 0 ? message.arguments[0] : ""
            if (changedHome === root.accountHome) {
                root.updateSnapshot()
            }
        }
    }

    Timer {
        interval: 30000
        running: root.isBound && root.helperAvailable
        repeat: true
        onTriggered: root.updateSnapshot()
    }

    onAccountHomeChanged: updateSnapshot()

    Connections {
        target: helperWatcher
        function onRegisteredChanged() {
            root.updateSnapshot()
        }
    }

    toolTipMainText: {
        if (!isBound) {
            return i18n("No Account Home")
        }
        if (!helperAvailable || snapshot.face === "unknown_allowance") {
            return i18n("Unknown Allowance")
        }
        if (snapshot.face === "signed_out") {
            return i18n("Signed out")
        }
        if (snapshot.face !== "ready") {
            return i18n("codecap")
        }
        var resetText = Logic.formatTimeToReset(snapshot.session_allowance.resets_at, nowUnix)
        var line = i18n("Session %1%", Math.round(snapshot.session_allowance.used_percent))
        if (resetText !== "") {
            line += " · " + resetText
        }
        if (snapshot.session_allowance.stale) {
            line += " · " + i18n("Last-Known")
        }
        return line
    }

    toolTipSubText: {
        if (snapshot.face === "ready" && snapshot.account_label !== "") {
            return snapshot.account_label
        }
        return ""
    }

    compactRepresentation: CompactRing {
        face: root.snapshot.face
        helperAvailable: root.helperAvailable
        isBound: root.isBound
        usedPercent: root.snapshot.session_allowance.used_percent
        stale: root.snapshot.session_allowance.stale
        usageCredit: root.snapshot.usage_credit
        showNumeral: width >= Kirigami.Units.gridUnit * 2.5
    }

    fullRepresentation: Kirigami.ScrollablePage {
        title: i18n("codecap")

        ColumnLayout {
            width: parent.width
            spacing: Kirigami.Units.largeSpacing

            Kirigami.PlaceholderMessage {
                Layout.fillWidth: true
                visible: !root.isBound
                icon.name: "folder-open-symbolic"
                text: i18n("No Account Home selected.")
                explanation: i18n("Open widget configuration and choose an Account Home (for example ~/.claude).")
            }

            Kirigami.PlaceholderMessage {
                Layout.fillWidth: true
                visible: root.isBound && !root.helperAvailable
                icon.name: "question-symbolic"
                text: i18n("Helper unavailable.")
                explanation: i18n("Install or run the codecap helper to fetch Allowance.")
            }

            Kirigami.PlaceholderMessage {
                Layout.fillWidth: true
                visible: root.isBound && root.helperAvailable && root.snapshot.face === "signed_out"
                icon.name: "unlock-symbolic"
                text: i18n("Signed out.")
                explanation: i18n("Sign in with Claude Code for this Account Home.")
            }

            PlasmaComponents.Label {
                Layout.fillWidth: true
                visible: root.isBound && root.snapshot.account_label !== ""
                text: root.snapshot.account_label
                font.bold: true
            }

            PlasmaComponents.Label {
                Layout.fillWidth: true
                visible: root.fxNote !== ""
                opacity: 0.75
                wrapMode: Text.Wrap
                text: root.fxNote
            }

            AllowanceBar {
                Layout.fillWidth: true
                visible: root.isBound && Logic.showAllowanceBars(root.snapshot.face)
                title: i18n("Session Allowance")
                usedPercent: root.snapshot.session_allowance.used_percent
                resetsAt: root.snapshot.session_allowance.resets_at
                stale: root.snapshot.session_allowance.stale
                usageCredit: root.snapshot.usage_credit
                nowUnix: root.nowUnix
            }

            AllowanceBar {
                Layout.fillWidth: true
                visible: root.isBound && Logic.showAllowanceBars(root.snapshot.face)
                title: i18n("Weekly Allowance")
                usedPercent: root.snapshot.weekly_allowance.used_percent
                resetsAt: root.snapshot.weekly_allowance.resets_at
                stale: root.snapshot.weekly_allowance.stale
                usageCredit: root.snapshot.usage_credit
                nowUnix: root.nowUnix
            }

            PlasmaComponents.Label {
                Layout.fillWidth: true
                visible: root.isBound && root.usageCreditLabel() !== "" && root.snapshot.face === "ready"
                text: root.usageCreditLabel()
            }

            Kirigami.PlaceholderMessage {
                Layout.fillWidth: true
                visible: root.isBound && root.helperAvailable && root.snapshot.face === "unknown_allowance"
                icon.name: "question-symbolic"
                text: i18n("Unknown Allowance.")
                explanation: i18n("Allowance is not available yet. Consumed Usage from local logs may still update.")
            }

            Kirigami.FormLayout {
                Layout.fillWidth: true
                visible: root.isBound && (root.snapshot.face === "ready" || root.snapshot.face === "unknown_allowance")

                PlasmaComponents.Label {
                    Kirigami.FormData.label: i18n("Session")
                    text: root.formatListPrice(root.snapshot.consumed_usage.session.list_price_usd) + "\n" + i18n("%1 tokens", Logic.formatTokens(root.snapshot.consumed_usage.session.tokens, Qt.locale()))
                    wrapMode: Text.Wrap
                }

                PlasmaComponents.Label {
                    Kirigami.FormData.label: i18n("Today")
                    text: root.formatListPrice(root.snapshot.consumed_usage.today.list_price_usd) + "\n" + i18n("%1 tokens", Logic.formatTokens(root.snapshot.consumed_usage.today.tokens, Qt.locale()))
                    wrapMode: Text.Wrap
                }

                PlasmaComponents.Label {
                    Kirigami.FormData.label: i18n("This week")
                    text: root.formatListPrice(root.snapshot.consumed_usage.week.list_price_usd) + "\n" + i18n("%1 tokens", Logic.formatTokens(root.snapshot.consumed_usage.week.tokens, Qt.locale()))
                    wrapMode: Text.Wrap
                }

                PlasmaComponents.Label {
                    Kirigami.FormData.label: i18n("This month")
                    text: root.formatListPrice(root.snapshot.consumed_usage.month.list_price_usd) + "\n" + i18n("%1 tokens", Logic.formatTokens(root.snapshot.consumed_usage.month.tokens, Qt.locale()))
                    wrapMode: Text.Wrap
                }
            }
        }
    }
}
