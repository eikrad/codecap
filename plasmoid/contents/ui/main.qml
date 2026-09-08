// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtQuick.Layouts
import QtCore
import org.kde.kirigami as Kirigami
import org.kde.plasma.components as PlasmaComponents
import org.kde.plasma.core as PlasmaCore
import org.kde.plasma.plasmoid
import "logic.js" as Logic

PlasmoidItem {
    id: root

    // QML's StandardPaths returns a url, so this is "file:///home/you" until
    // the scheme is stripped.
    readonly property string homeDir: Logic.stripFileScheme(StandardPaths.writableLocation(StandardPaths.HomeLocation))

    // Plasma::Applet has no configurationChanged signal, so configuration is
    // mirrored into real properties whose own change signals do fire.
    readonly property string configuredAccountHome: Plasmoid.configuration.accountHome
    readonly property string configuredCurrency: Plasmoid.configuration.displayCurrency

    function resolvedAccountHome() {
        return Logic.expandPath(configuredAccountHome, homeDir)
    }

    property int nowUnix: Math.floor(Date.now() / 1000)
    property real fxRate: 1.0
    property bool fxUsingUsdFallback: false
    property string fxNote: ""

    readonly property bool isBound: snapshotSource.isBound
    readonly property var snapshot: snapshotSource.snapshot
    readonly property bool helperReachable: snapshotSource.helperReachable

    // Tray auto-hide. Names come from logic.js so the 80% band is unit-tested;
    // the mapping onto PlasmaCore.Types is the only part that has to live here
    // (finding P-M14). Confirmed against plasma/plasma.h ItemStatus and the
    // in-tree kdeconnect applet.
    Plasmoid.status: {
        var name = Logic.trayStatus(
            snapshot.face,
            snapshot.session_allowance.used_percent,
            snapshot.session_allowance.stale
        )
        if (name === "needsAttention") {
            return PlasmaCore.Types.NeedsAttentionStatus
        }
        if (name === "active") {
            return PlasmaCore.Types.ActiveStatus
        }
        return PlasmaCore.Types.PassiveStatus
    }
    // The reported windows are read against this, so a fresh snapshot deserves
    // a fresh clock rather than waiting up to 30 s for the ticker.
    onSnapshotChanged: nowUnix = Math.floor(Date.now() / 1000)

    readonly property string displayCurrency: Logic.effectiveCurrency(
        configuredCurrency,
        Qt.locale().name
    )
    // The Account Home is bound through SnapshotSource.accountHome, whose own
    // change handler refreshes; Plasma::Applet has no configurationChanged
    // signal, which is what the mirrored properties above are for.
    onConfiguredCurrencyChanged: refreshFxRate()

    Timer {
        interval: 30000
        running: true
        repeat: true
        onTriggered: root.nowUnix = Math.floor(Date.now() / 1000)
    }

    Timer {
        id: fxTimer
        // The rate is cached for a day; a minute timer just re-checked the
        // cache, and on failure re-fetched, 1440 times a day.
        interval: 3600000
        running: true
        repeat: true
        onTriggered: root.refreshFxRate()
    }

    function weekStart() {
        return Qt.locale().firstDayOfWeek
    }

    // Everything on the bus lives in SnapshotSource, which takes plain
    // properties so that tests/plasmoid/qml-dbus can drive it against a stub
    // helper. PlasmoidItem cannot be instantiated outside Plasma's applet
    // machinery, so nothing that stays in this file can be executed by a test.
    SnapshotSource {
        id: snapshotSource
        accountHome: root.resolvedAccountHome()
        weekStart: root.weekStart()
    }

    function applyFxRate(rate, usingFallback) {
        fxRate = rate
        fxUsingUsdFallback = usingFallback
        fxNote = usingFallback ? i18n("Display currency rate unavailable; showing USD.") : ""
    }

    function cacheFxRate(rate, usingFallback) {
        // Recorded on failure too, so a failed fetch backs off for a day
        // instead of retrying on every timer tick.
        Plasmoid.configuration.fxRateUsdToDisplay = rate
        Plasmoid.configuration.fxUsingUsdFallback = usingFallback
        Plasmoid.configuration.fxCurrency = root.displayCurrency
        Plasmoid.configuration.fxFetchedAt = Math.floor(Date.now() / 1000)
    }

    function refreshFxRate() {
        const currency = displayCurrency
        if (currency === "USD") {
            // No fetch and no config write: this branch used to dirty the
            // applet config once a minute for the whole session.
            applyFxRate(1.0, false)
            return
        }

        const fetchedAt = Plasmoid.configuration.fxFetchedAt || 0
        const cachedCurrency = Plasmoid.configuration.fxCurrency || ""
        // A cached rate is only usable for the currency it was fetched for.
        if (cachedCurrency === currency
                && nowUnix - fetchedAt < 86400
                && Plasmoid.configuration.fxRateUsdToDisplay > 0) {
            applyFxRate(Plasmoid.configuration.fxRateUsdToDisplay,
                        Plasmoid.configuration.fxUsingUsdFallback || false)
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
                // 0 means no rate was found. Showing USD amounts under another
                // currency code would be worse than saying so.
                if (rate > 0) {
                    root.applyFxRate(rate, false)
                    root.cacheFxRate(rate, false)
                    return
                }
            }
            root.applyFxRate(1.0, true)
            root.cacheFxRate(1.0, true)
        }
        xhr.open("GET", "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml")
        xhr.send()
    }

    function formatListPrice(usd) {
        return Logic.formatMoney(usd, fxRate, displayCurrency, fxUsingUsdFallback, Qt.locale())
    }

    // The helper says why a snapshot is incomplete; the wording is ours.
    // allowance_unavailable is deliberately not listed: the Unknown Allowance
    // placeholder already says that, and two notices for one fact is noise.
    function degradedNote() {
        const reasons = snapshot.degraded || []
        if (reasons.indexOf("usage_unavailable") >= 0) {
            return i18n("Consumed Usage could not be read from the local logs.")
        }
        if (reasons.indexOf("pending") >= 0) {
            return i18n("Updating…")
        }
        return ""
    }

    // Usage Credit is money, so it is reported as money: the vendor's own view
    // says "$4.04 of $4.00", and the status word alone could not say how much
    // was left or how far past the ceiling the Account had gone.
    //
    // Deliberately not converted to Display Currency. That is scoped to List
    // Price, which is an estimate; this is what the vendor actually bills, and
    // an ECB conversion would print a figure nobody is charged.
    function usageCreditLabel() {
        if (snapshot.usage_credit === "none") {
            return ""
        }
        var spend = snapshot.usage_credit_spend
        if (spend && spend.limit_usd > 0) {
            return i18n("Usage credit: %1 of %2",
                        Logic.formatMoney(spend.used_usd, 1.0, "USD", true, Qt.locale()),
                        Logic.formatMoney(spend.limit_usd, 1.0, "USD", true, Qt.locale()))
        }
        // A helper too old to send the amounts, or a limit the vendor did not
        // report. The status word is less useful, but it is not wrong.
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

    Component.onCompleted: refreshFxRate()

    toolTipMainText: {
        if (!isBound) {
            return i18n("No Account Home")
        }
        if (!helperReachable || snapshot.face === "unknown_allowance") {
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

    // A custom compactRepresentation replaces DefaultCompactRepresentation,
    // which is where Plasma's click-to-expand lives, so it has to be restored
    // here or the expanded face is unreachable by mouse.
    compactRepresentation: MouseArea {
        id: compactRoot

        implicitWidth: ring.implicitWidth
        implicitHeight: ring.implicitHeight
        acceptedButtons: Qt.LeftButton
        onClicked: root.expanded = !root.expanded

        CompactRing {
            id: ring
            anchors.fill: parent
            face: root.snapshot.face
            helperAvailable: root.helperReachable
            isBound: root.isBound
            usedPercent: root.snapshot.session_allowance.used_percent
            stale: root.snapshot.session_allowance.stale
            showNumeral: compactRoot.width >= Kirigami.Units.gridUnit * 2.5
        }
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
                visible: root.isBound && !root.helperReachable
                icon.name: "question-symbolic"
                text: i18n("Helper unavailable.")
                explanation: i18n("Install or run the codecap helper to fetch Allowance.")
            }

            Kirigami.PlaceholderMessage {
                Layout.fillWidth: true
                visible: root.isBound && root.helperReachable && root.snapshot.face === "signed_out"
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
                visible: root.degradedNote() !== ""
                opacity: 0.75
                wrapMode: Text.Wrap
                text: root.degradedNote()
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
                nowUnix: root.nowUnix
            }

            AllowanceBar {
                Layout.fillWidth: true
                visible: root.isBound && Logic.showAllowanceBars(root.snapshot.face)
                title: i18n("Weekly Allowance")
                usedPercent: root.snapshot.weekly_allowance.used_percent
                resetsAt: root.snapshot.weekly_allowance.resets_at
                stale: root.snapshot.weekly_allowance.stale
                nowUnix: root.nowUnix
            }

            PlasmaComponents.Label {
                Layout.fillWidth: true
                visible: root.isBound && root.usageCreditLabel() !== "" && root.snapshot.face === "ready"
                text: root.usageCreditLabel()
            }

            Kirigami.PlaceholderMessage {
                Layout.fillWidth: true
                visible: root.isBound && root.helperReachable && root.snapshot.face === "unknown_allowance"
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
