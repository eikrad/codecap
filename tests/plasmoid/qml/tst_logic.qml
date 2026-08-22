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
        compare(typeof u.toLocalFile, "undefined")
        compare(Logic.stripFileScheme(u), "/home/me/.claude")
    }

    function test_parseSnapshot_survives_real_engine_types() {
        compare(Logic.parseSnapshot("not-json"), null)
        var snap = Logic.parseSnapshot('{"schema_version":1,"face":"ready"}')
        compare(snap.face, "ready")
        compare(snap.consumed_usage.today.tokens, 0)
    }
}
