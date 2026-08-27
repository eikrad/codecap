// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// Phase 5.3 of docs/plan-hardening.md: the applet's own SnapshotSource, on a
// private session bus, talking to a stub dev.codecap.Helper (tests/helperstub).
//
// This is the gate for the class of bug that made up half the criticals of
// 2026-08-22: a handler with a name nothing calls. `onReceivedSignal` was not a
// typo, it was what anyone would guess, and SignalWatcher looks up a function
// named "dbus" + the member instead — so the push signal arrived at a function
// that did not exist, no warning, no error, and the widget simply never
// updated. Nothing in this repo could see that. A stub can: it changes its
// answer, emits Changed, and this file asserts the numbers moved.
//
// Run it with `make test-plasmoid-qml-dbus`, which starts the bus and the stub.
// It needs org.kde.plasma.workspace.dbus, a Plasma 6 module, so it skips itself
// anywhere that module is missing — CI included.
import QtQuick
import QtTest
import org.kde.plasma.workspace.dbus as DBus
import "../../../plasmoid/contents/ui" as Ui
import "../../../plasmoid/contents/ui/logic.js" as Logic

TestCase {
    id: testCase
    name: "SnapshotSourceOverDBus"

    // Never opened. The stub answers for whatever path it is handed; the real
    // helper's own validation of it is tested in Go.
    readonly property string homeA: "/tmp/codecap-harness/a"
    readonly property string homeB: "/tmp/codecap-harness/b"

    property bool controlDone: false
    property var controlResult: undefined
    property string controlError: ""

    // Every snapshot this suite has seen, in order. A negative assertion needs
    // this: "the superseded reply never landed" cannot be shown by looking at
    // the final value.
    property var observedHomes: []

    Ui.SnapshotSource {
        id: source
        // The safety-net poll would add calls the assertions below count, and
        // it is not what any of them are testing.
        pollInterval: 3600000
    }

    Connections {
        target: source
        function onSnapshotChanged() {
            // Copied, not pushed into: mutating a var property in place fires
            // no change signal, which is the discipline this suite exists to
            // defend (finding P-H3).
            const next = testCase.observedHomes.slice()
            next.push(source.snapshot.account_home)
            testCase.observedHomes = next
        }
    }

    // control calls the stub's scripting interface and waits for the reply, so
    // a test reads as a sequence rather than as nested callbacks.
    function control(member, args) {
        testCase.controlDone = false
        testCase.controlResult = undefined
        testCase.controlError = ""

        DBus.SessionBus.asyncCall({
            "service": "dev.codecap.Helper",
            "path": "/dev/codecap/Stub",
            "iface": "dev.codecap.Stub",
            "member": member,
            "arguments": args
        }, function(result) {
            testCase.controlResult = result
            testCase.controlDone = true
        }, function(failure) {
            testCase.controlError = member + " failed: " + Logic.describePayload(failure)
            testCase.controlDone = true
        })

        tryVerify(function() { return testCase.controlDone }, 5000,
                  "the stub did not answer " + member + "; is tests/helperstub running on this bus?")
        verify(testCase.controlError === "", testCase.controlError)
        return testCase.controlResult
    }

    // The same unwrapping the applet does on a GetSnapshot reply, against a
    // real D-Bus reply value rather than a fixture — which no test has done
    // before, and which is the reason extractSnapshotPayload accepts three
    // shapes in the first place.
    function callLog() {
        const raw = Logic.extractSnapshotPayload(control("CallLog", []))
        if (typeof raw !== "string" || raw === "") {
            return []
        }
        return JSON.parse(raw)
    }

    function callCount() {
        return callLog().length
    }

    function bindAndWaitForReady(home, percent) {
        control("SetFace", ["ready"])
        control("SetSessionPercent", [percent])
        source.accountHome = home
        tryVerify(function() {
            return source.snapshot.face === "ready"
                && source.snapshot.account_home === home
                && source.snapshot.session_allowance.used_percent === percent
        }, 5000, "the first GetSnapshot never produced a ready face")
    }

    function init() {
        source.accountHome = ""
        source.timezone = ""
        source.weekStart = 1
        testCase.observedHomes = []
        control("Reset", [])
    }

    // The one this whole phase exists for. Nothing asks the applet to refresh;
    // only the helper's push signal can move the number.
    function test_a_changed_signal_reaches_the_applet() {
        bindAndWaitForReady(homeA, 37)

        control("SetSessionPercent", [88])
        compare(source.snapshot.session_allowance.used_percent, 37,
                "the applet refreshed without being told to; this test would prove nothing")

        control("EmitChanged", [homeA])

        tryVerify(function() {
            return source.snapshot.session_allowance.used_percent === 88
        }, 5000,
        "Changed never reached the applet. SignalWatcher calls a function named "
        + "\"dbus\" + the member name — a handler called anything else is never "
        + "invoked, with no warning and no error (finding P-C2).")
    }

    function test_changed_for_another_account_home_is_ignored() {
        bindAndWaitForReady(homeA, 37)
        const before = callCount()

        control("EmitChanged", [homeB])
        wait(300)
        compare(callCount(), before,
                "a Changed for a different Account Home caused a refresh")

        // Not a sleep dressed up as an assertion: the same mechanism must still
        // fire for the bound home, or the test above would pass on a dead
        // watcher too.
        control("EmitChanged", [homeA])
        tryVerify(function() { return callCount() > before }, 5000,
                  "Changed for the bound Account Home did not cause a refresh")
    }

    // ADR 0006: a failed call keeps Last-Known Allowance visible with staleness
    // shown. Blanking the popup was finding P-M4.
    function test_a_rejected_call_keeps_last_known_visible() {
        bindAndWaitForReady(homeA, 37)

        control("SetFailure", ["helper is not there"])
        control("EmitChanged", [homeA])

        tryCompare(source, "helperReachable", false, 5000)
        compare(source.snapshot.face, "ready")
        compare(source.snapshot.session_allowance.used_percent, 37)
        verify(source.snapshot.session_allowance.stale)
        verify(source.snapshot.weekly_allowance.stale)
        compare(source.snapshot.account_home, homeA)
    }

    // Finding P-M6. Unbound tells the user they have not chosen an Account
    // Home; that is a different thing from a helper that answered with rubbish,
    // and showing it made a broken helper look like a configuration mistake.
    function test_an_unreadable_reply_is_unknown_allowance_not_unbound() {
        control("SetRaw", ["not-json"])
        source.accountHome = homeA

        tryVerify(function() {
            return source.snapshot.face === "unknown_allowance"
        }, 5000, "an unreadable reply did not land on Unknown Allowance")
        compare(source.snapshot.account_home, homeA)
        // The call succeeded. Only its content was unusable, and the helper is
        // demonstrably there — saying otherwise would be a third wrong answer.
        compare(source.helperReachable, true)
    }

    function test_an_array_reply_is_unreadable_too() {
        // typeof [] is "object", so this reached normalizeSnapshot, matched no
        // face, and came back as the empty snapshot — face "unbound".
        control("SetRaw", ["[]"])
        source.accountHome = homeA

        tryVerify(function() {
            return source.snapshot.face === "unknown_allowance"
        }, 5000, "an array reply was read as Unbound rather than as unreadable")
    }

    function test_no_account_home_makes_no_bus_call() {
        source.accountHome = ""
        wait(300)

        compare(source.snapshot.face, "unbound")
        compare(source.snapshot.account_home, "")
        compare(callCount(), 0, "the helper was called with no Account Home bound")
    }

    // The bus contract of ADR 0011, as the helper receives it.
    function test_the_helper_receives_what_the_contract_says() {
        source.weekStart = 7
        bindAndWaitForReady(homeA, 37)

        const calls = callLog()
        verify(calls.length >= 1, "no GetSnapshot arrived at the stub")
        const first = calls[0]
        compare(first.account_home, homeA)
        // Empty means "the helper's own zone". Qt's JS engine has no Intl, so
        // QML cannot produce an IANA id, and the display name it can produce
        // resolves to UTC — which moved every day, week and month boundary for
        // every user outside it (finding C-1).
        compare(first.timezone, "")
        compare(first.week_start, 7)
    }

    // Finding P-M3. A reply for an Account Home the widget has since left must
    // not be shown, and the final value alone cannot prove it never was.
    function test_a_superseded_reply_is_dropped() {
        control("SetSessionPercent", [37])
        control("SetDelayMs", [700])

        source.accountHome = homeA
        source.accountHome = homeB
        control("SetDelayMs", [0])

        tryVerify(function() {
            return source.snapshot.face === "ready"
                && source.snapshot.account_home === homeB
        }, 10000, "the reply for the current Account Home never arrived")

        compare(testCase.observedHomes.indexOf(homeA), -1,
                "a reply for a superseded Account Home was shown: "
                + JSON.stringify(testCase.observedHomes))
    }
}
