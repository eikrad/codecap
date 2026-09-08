// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// These run logic.js inside a real QML engine. The Node tests are faster and
// cover more cases, but they cannot see the two things that matter here: QML
// types are not JavaScript types (StandardPaths returns a url object, not a
// string), and QML's Number.toLocaleString has different defaults from
// ECMAScript's. Both of those shipped as bugs.
import QtQuick
import QtTest
import QtCore
import "../../../plasmoid/contents/ui/logic.js" as Logic

TestCase {
    name: "LogicInQmlEngine"

    function test_stripFileScheme_accepts_a_url_object() {
        // The regression: `(value || "").trim()` throws on a url, because a url
        // is truthy and has no trim(). A string fixture cannot catch it.
        var home = StandardPaths.writableLocation(StandardPaths.HomeLocation)
        verify(typeof home === "object", "precondition: QML gives us a url object")

        var path = Logic.stripFileScheme(home)
        compare(typeof path, "string")
        verify(path.indexOf("file://") !== 0, "the scheme must be gone")
        compare(path.charAt(0), "/")
    }

    function test_stripFileScheme_edges() {
        compare(Logic.stripFileScheme(undefined), "")
        compare(Logic.stripFileScheme(null), "")
        compare(Logic.stripFileScheme(""), "")
        compare(Logic.stripFileScheme("/home/me"), "/home/me")
        compare(Logic.stripFileScheme(Qt.url("file:///home/me")), "/home/me")
    }

    function test_expandPath_produces_something_the_helper_can_stat() {
        var home = StandardPaths.writableLocation(StandardPaths.HomeLocation)
        var resolved = Logic.expandPath("~/.claude", home)

        verify(resolved.indexOf("file://") !== 0, "a file:// url must never reach the helper")
        compare(resolved.charAt(0), "/")
        verify(resolved.length > "/.claude".length, "the home directory must be in there")
        verify(/\.claude$/.test(resolved))
    }

    function test_formatTokens_has_no_decimals() {
        // QML defaults this call to format "f" with precision 2, unlike
        // ECMAScript, so tokens rendered as "1,234,567.00".
        compare(Logic.formatTokens(1234567, Qt.locale("en_US")), "1,234,567")
        compare(Logic.formatTokens(0, Qt.locale("en_US")), "0")
    }

    function test_formatMoney_uses_the_locale() {
        compare(Logic.formatMoney(10, 7.45, "DKK", false, Qt.locale("da_DK")), "74,50 DKK")
        compare(Logic.formatMoney(3.5, 1, "USD", true, Qt.locale("en_US")), "3.50 USD")
    }

    function test_a_url_value_has_no_toLocalFile() {
        // The folder picker wrote `selectedFolder.toLocalFile()`. QML url
        // values do not have that method, so the handler threw and choosing a
        // folder silently did nothing. stripFileScheme is the conversion.
        var u = Qt.url("file:///home/me/.claude")
        compare(typeof u, "object")
        // The missing property is the assertion. qmllint resolves QUrl, so it
        // reports the very absence this line exists to prove — and with
        // --max-warnings 0 that would fail the build for being right.
        // qmllint disable missing-property
        compare(typeof u.toLocalFile, "undefined")
        // qmllint enable missing-property
        compare(Logic.stripFileScheme(u), "/home/me/.claude")
    }

    function test_parseSnapshot_survives_real_engine_types() {
        compare(Logic.parseSnapshot("not-json"), null)
        var snap = Logic.parseSnapshot('{"schema_version":1,"face":"ready"}')
        compare(snap.face, "ready")
        compare(snap.consumed_usage.today.tokens, 0)
    }

    function test_an_array_reply_is_unreadable_not_unbound() {
        // typeof [] is "object" in every JS engine, and the guard that rejects
        // it uses Array.isArray — which this pins in the engine that actually
        // runs it, not in Node. Without the guard the payload became the empty
        // snapshot, whose face is "unbound": the widget would tell the user to
        // choose an Account Home because the helper misbehaved.
        compare(Logic.parseSnapshot("[]"), null)
        compare(Logic.parseSnapshot('[{"face":"ready"}]'), null)
        compare(Logic.parseSnapshot("null"), null)
    }

    // These decide what the widget shows when the helper gives it nothing
    // usable. They ran nowhere until they moved out of main.qml.
    function test_staleSnapshot_keeps_last_known_and_marks_it() {
        var ready = Logic.parseSnapshot('{"face":"ready","session_allowance":{"used_percent":37},"weekly_allowance":{"used_percent":61}}')
        var stale = Logic.staleSnapshot(ready, "/home/me/.claude")

        compare(stale.face, "ready")
        compare(stale.session_allowance.used_percent, 37)
        verify(stale.session_allowance.stale)
        verify(stale.weekly_allowance.stale)
        // The snapshot still assigned to the property must not have moved.
        verify(!ready.session_allowance.stale)
    }

    function test_staleSnapshot_falls_back_to_unknown_allowance() {
        compare(Logic.staleSnapshot(null, "/home/me/.claude").face, "unknown_allowance")
        compare(Logic.staleSnapshot(Logic.emptySnapshot(), "/home/me/.claude").face, "unknown_allowance")
    }

    function test_localFaceSnapshot_names_the_home_only_when_bound() {
        compare(Logic.localFaceSnapshot("unbound", "/home/me/.claude").account_home, "")
        compare(Logic.localFaceSnapshot("signed_out", "/home/me/.claude").account_home, "/home/me/.claude")
    }
}
