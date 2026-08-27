// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// Everything the applet does on D-Bus: the GetSnapshot call, the in-flight
// guard, the push signal, the service watch and the safety-net poll.
//
// It lives outside main.qml so it can be instantiated without a PlasmoidItem.
// PlasmoidItem only works inside Plasma's applet machinery — Plasmoid.configuration
// is null anywhere else — so as long as this code sat in main.qml, nothing could
// execute it, and the class of bug that made up half the criticals of 2026-08-22
// (a signal handler with a name nothing calls) had no gate at all. Taking plain
// properties instead is what tests/plasmoid/qml-dbus drives against a stub helper.
//
// The same reasoning as CompactRing and AllowanceBar, which took plain properties
// for the same reason and are tested for the same reason.

import QtQuick
import org.kde.plasma.workspace.dbus as DBus
import "logic.js" as Logic

Item {
    id: source

    // An already-resolved path — the tilde is expanded and any file:// scheme is
    // stripped before it gets here (finding P-C1). Empty means Unbound: no bus
    // call is made at all and the face is decided locally.
    property string accountHome: ""

    // Empty means "the helper's own zone". Qt's JS engine has no Intl, so QML
    // cannot produce an IANA id, and the display name it can produce resolves to
    // UTC — which moved every day, week and month boundary for every user outside
    // it (finding C-1).
    property string timezone: ""

    // The locale's first day of week, as the helper's `i` argument.
    property int weekStart: 1

    // A safety net behind the Changed signal, not the primary clock.
    property int pollInterval: 30000

    readonly property bool isBound: accountHome.trim() !== ""

    property var snapshot: Logic.emptySnapshot()

    // False means the call itself failed — an activatable service that could not
    // be activated. DBusServiceWatcher.registered cannot answer this: it means
    // "registered right now", and the helper is D-Bus activated, so the first
    // call is what starts it (design.md:38, finding A-1).
    property bool helperReachable: true

    // Replies from a superseded request or a previous Account Home are dropped.
    property int requestSeq: 0

    onAccountHomeChanged: refresh()
    Component.onCompleted: refresh()

    // Which face to show is decided in logic.js, which is the only plasmoid code
    // CI can execute; what is left here is the assignment. Assigning the
    // property fires the change signal and mutating the object afterwards does
    // not, so nothing may touch the snapshot after this line (finding P-H3).
    function applyLocalFace(face) {
        snapshot = Logic.localFaceSnapshot(face, source.accountHome)
    }

    // A failed or unreadable call must not blank the popup. ADR 0006 keeps
    // Last-Known Allowance visible with staleness shown.
    function markSnapshotStale() {
        snapshot = Logic.staleSnapshot(source.snapshot, source.accountHome)
    }

    function applySnapshotPayload(result) {
        const payload = Logic.extractSnapshotPayload(result)
        const parsed = Logic.parseSnapshot(payload)
        if (parsed === null) {
            // Unreadable helper output is Unknown Allowance, not Unbound. Say
            // what arrived: the reply's shape is the only thing that identifies
            // which case went unhandled, and a silent fallback here looked
            // exactly like a helper that was not running.
            console.warn("codecap: could not read the helper reply;",
                         "outer:", Logic.describePayload(result),
                         "unwrapped:", Logic.describePayload(payload))
            markSnapshotStale()
            return
        }
        snapshot = parsed
    }

    function refresh() {
        if (!isBound) {
            applyLocalFace("unbound")
            return
        }

        const home = accountHome
        const seq = ++requestSeq

        DBus.SessionBus.asyncCall({
            "service": "dev.codecap.Helper",
            "path": "/dev/codecap/Helper",
            "iface": "dev.codecap.Helper",
            "member": "GetSnapshot",
            // No signature on purpose. The library derives one from
            // introspection, and dbusconnection.cpp builds it as
            // '(' + types + ')' — the parenthesised struct form, as in the
            // documented example "(u)" for a single uint32. Passing a bare
            // "ssi" here made the encoder read only the first type and the
            // call failed, which showed up as "Helper unavailable" while
            // busctl worked fine. "(ssi)" is very likely correct and would
            // save one round-trip per poll, but it cannot be tested without a
            // Plasma session, and after phase 3 that round-trip costs nothing.
            "arguments": [home, source.timezone, source.weekStart]
        }, function(result) {
            if (seq !== source.requestSeq || home !== source.accountHome) {
                return
            }
            source.helperReachable = true
            source.applySnapshotPayload(result)
        }, function(failure) {
            if (seq !== source.requestSeq || home !== source.accountHome) {
                return
            }
            // The call itself failing is the signal that the helper is not
            // there — an activatable service that cannot be activated.
            source.helperReachable = false
            // Say why. A silent reject is what made "Helper unavailable" mean
            // four different things at once.
            console.warn("codecap: GetSnapshot failed:",
                         failure && failure.error && failure.error.message
                             ? failure.error.message
                             : failure)
            source.markSnapshotStale()
        })
    }

    DBus.DBusServiceWatcher {
        id: helperWatcher
        busType: DBus.BusType.Session
        watchedService: "dev.codecap.Helper"
    }

    Connections {
        target: helperWatcher
        function onRegisteredChanged() {
            source.refresh()
        }
    }

    DBus.SignalWatcher {
        enabled: source.isBound
        busType: DBus.BusType.Session
        service: "dev.codecap.Helper"
        path: "/dev/codecap/Helper"
        iface: "dev.codecap.Helper"

        // SignalWatcher has no receivedSignal signal: it looks up a function
        // named "dbus" + the member name and calls it with the decoded
        // arguments. Any other name is silently never called (finding P-C2) —
        // which is why tests/plasmoid/qml-dbus asserts a stub's Changed signal
        // moves the numbers here.
        function dbusChanged(accountHome) {
            if (accountHome === source.accountHome) {
                source.refresh()
            }
        }
    }

    Timer {
        interval: source.pollInterval
        running: source.isBound
        repeat: true
        onTriggered: source.refresh()
    }
}
