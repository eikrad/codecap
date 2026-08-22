// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtQuick.Layouts
import QtCore
import org.kde.kirigami as Kirigami
import org.kde.plasma.components as PlasmaComponents
import org.kde.plasma.plasmoid
import org.kde.plasma.workspace.dbus as DBus
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

    readonly property bool isBound: resolvedAccountHome().trim() !== ""
    property var snapshot: Logic.emptySnapshot()
    property int nowUnix: Math.floor(Date.now() / 1000)
    property real fxRate: 1.0
    property bool fxUsingUsdFallback: false
    property string fxNote: ""

    // Replies from a superseded request or a previous Account Home are dropped.
    property int snapshotRequestSeq: 0

    readonly property string displayCurrency: Logic.effectiveCurrency(
        configuredCurrency,
        Qt.locale().name
    )
    readonly property bool helperAvailable: helperWatcher.registered

    onConfiguredAccountHomeChanged: updateSnapshot()
    onConfiguredCurrencyChanged: refreshFxRate()

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

    // Assigning the property fires the change signal; mutating the object
    // afterwards does not, so every face has to be built before it is assigned.
    function applyLocalFace(face) {
        const next = Logic.emptySnapshot()
        next.face = face
        if (face !== "unbound") {
            next.account_home = resolvedAccountHome()
        }
        snapshot = next
    }

    // A failed or unreadable call must not blank the popup. ADR 0006 keeps
    // Last-Known Allowance visible with staleness shown.
    function markSnapshotStale() {
        const previous = root.snapshot
        if (previous && previous.face === "ready") {
            const next = Logic.normalizeSnapshot(previous)
            next.session_allowance.stale = true
            next.weekly_allowance.stale = true
            snapshot = next
            return
        }
        applyLocalFace("unknown_allowance")
    }

    function applySnapshotPayload(result) {
        let payload = result
        if (payload && payload.value !== undefined) {
            payload = payload.value
        }
        if (Array.isArray(payload) && payload.length > 0) {
            payload = payload[0]
        }
        const parsed = Logic.parseSnapshot(payload)
        if (parsed === null) {
            // Unreadable helper output is Unknown Allowance, not Unbound.
            markSnapshotStale()
            return
        }
        snapshot = parsed
        nowUnix = Math.floor(Date.now() / 1000)
    }

    function updateSnapshot() {
        if (!isBound) {
            applyLocalFace("unbound")
            return
        }

        if (!helperAvailable) {
            applyLocalFace("unknown_allowance")
            return
        }

        const home = resolvedAccountHome()
        const seq = ++snapshotRequestSeq

        DBus.SessionBus.asyncCall({
            "service": "dev.codecap.Helper",
            "path": "/dev/codecap/Helper",
            "iface": "dev.codecap.Helper",
            "member": "GetSnapshot",
            // Without a signature the call first costs an Introspect round-trip,
            // and an unencoded argument can fail to match the declared ssi.
            "signature": "ssi",
            // An empty timezone means "the helper's own zone". Qt's JS engine
            // has no Intl, so QML cannot produce an IANA id, and the display
            // name it can produce resolves to UTC.
            "arguments": [home, "", weekStart()]
        }, function(result) {
            if (seq !== root.snapshotRequestSeq || home !== root.resolvedAccountHome()) {
                return
            }
            root.applySnapshotPayload(result)
        }, function() {
            if (seq !== root.snapshotRequestSeq || home !== root.resolvedAccountHome()) {
                return
            }
            root.markSnapshotStale()
        })
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

        // SignalWatcher has no receivedSignal signal: it looks up a function
        // named "dbus" + the member name and calls it with the decoded
        // arguments. Any other name is silently never called.
        function dbusChanged(accountHome) {
            if (accountHome === root.resolvedAccountHome()) {
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
            helperAvailable: root.helperAvailable
            isBound: root.isBound
            usedPercent: root.snapshot.session_allowance.used_percent
            stale: root.snapshot.session_allowance.stale
            usageCredit: root.snapshot.usage_credit
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
