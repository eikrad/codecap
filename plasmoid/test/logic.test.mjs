// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import assert from "node:assert/strict";
import test from "node:test";
import { loadLogic } from "./load-logic.mjs";

const Logic = loadLogic();
const colors = { disabled: "gray", negative: "red", neutral: "orange", highlight: "blue" };

test("stripFileScheme turns a QML url into a path", () => {
    assert.equal(Logic.stripFileScheme("file:///home/me"), "/home/me");
    assert.equal(Logic.stripFileScheme("file:///home/my%20name"), "/home/my name");
    assert.equal(Logic.stripFileScheme("/home/me"), "/home/me");
    assert.equal(Logic.stripFileScheme("  /home/me  "), "/home/me");
    assert.equal(Logic.stripFileScheme(""), "");
    assert.equal(Logic.stripFileScheme(undefined), "");
});

test("expandPath resolves tilde and home-relative paths", () => {
    assert.equal(Logic.expandPath("", "/home/me"), "");
    assert.equal(Logic.expandPath("~", "/home/me"), "/home/me");
    assert.equal(Logic.expandPath("~/.claude", "/home/me"), "/home/me/.claude");
    assert.equal(Logic.expandPath("/home/me/.claude", "/home/me"), "/home/me/.claude");
});

test("expandPath copes with the file:// url QML hands it", () => {
    // StandardPaths.writableLocation returns a url, so homeDir arrives as a
    // file:// string. Without stripping it, ~/.claude expands to something the
    // helper cannot stat and every user who types the documented path is told
    // they are signed out.
    assert.equal(Logic.expandPath("~/.claude", "file:///home/me"), "/home/me/.claude");
    assert.equal(Logic.expandPath("~", "file:///home/me"), "/home/me");
    assert.equal(Logic.expandPath("file:///home/me/.claude", "file:///home/me"), "/home/me/.claude");
});

test("parseSnapshot returns null when the payload cannot be read", () => {
    assert.equal(Logic.parseSnapshot("not-json"), null);
    assert.equal(Logic.parseSnapshot(""), null);
    assert.equal(Logic.parseSnapshot("   "), null);
    assert.equal(Logic.parseSnapshot(undefined), null);
    assert.equal(Logic.parseSnapshot({ face: "ready" }), null);
    assert.equal(Logic.parseSnapshot("42"), null);
    assert.equal(Logic.parseSnapshot("null"), null);
});

test("parseSnapshot normalizes a well-formed payload", () => {
    const snap = Logic.parseSnapshot('{"face":"ready","account_home":"/home/me/.claude"}');
    assert.equal(snap.face, "ready");
    assert.equal(snap.account_home, "/home/me/.claude");
    assert.equal(snap.consumed_usage.month.tokens, 0);
});

test("normalizeSnapshot fills defaults and preserves allowance windows", () => {
    const snap = Logic.normalizeSnapshot({
        face: "ready",
        account_home: "/home/me/.claude",
        session_allowance: { used_percent: 12, resets_at: 100, stale: false },
        weekly_allowance: { used_percent: 34, resets_at: 200, stale: true },
        consumed_usage: {
            session: { list_price_usd: 1.2, tokens: 500 }
        }
    });
    assert.equal(snap.face, "ready");
    assert.equal(snap.account_home, "/home/me/.claude");
    assert.equal(snap.session_allowance.used_percent, 12);
    assert.equal(snap.weekly_allowance.stale, true);
    assert.equal(snap.consumed_usage.session.tokens, 500);
});

test("normalizeSnapshot fills the periods a partial payload leaves out", () => {
    // A shallow copy here leaves consumed_usage.today undefined, and every
    // label reading .today.list_price_usd throws.
    const snap = Logic.normalizeSnapshot({
        face: "ready",
        consumed_usage: { session: { list_price_usd: 1.2, tokens: 500 } }
    });
    for (const period of ["today", "week", "month"]) {
        assert.deepEqual(snap.consumed_usage[period], { list_price_usd: 0, tokens: 0 });
    }
});

test("normalizeSnapshot coerces junk into usable numbers", () => {
    const snap = Logic.normalizeSnapshot({
        face: 7,
        account_home: null,
        usage_credit: 3,
        fetched_at: "later",
        session_allowance: { used_percent: "oops" },
        weekly_allowance: "not-an-object",
        consumed_usage: { session: { tokens: null } }
    });
    assert.equal(snap.face, "unbound");
    assert.equal(snap.account_home, "");
    assert.equal(snap.usage_credit, "none");
    assert.equal(snap.fetched_at, 0);
    assert.equal(snap.session_allowance.used_percent, 0);
    assert.equal(snap.session_allowance.stale, false);
    assert.deepEqual(snap.weekly_allowance, { used_percent: 0, resets_at: 0, stale: false });
    assert.equal(snap.consumed_usage.session.tokens, 0);
    for (const value of [
        snap.session_allowance.used_percent,
        snap.weekly_allowance.resets_at,
        snap.consumed_usage.session.list_price_usd
    ]) {
        assert.ok(Number.isFinite(value), "no NaN may reach a binding");
    }
});

test("showAllowanceBars is true only for ready face", () => {
    assert.equal(Logic.showAllowanceBars("ready"), true);
    assert.equal(Logic.showAllowanceBars("unknown_allowance"), false);
    assert.equal(Logic.showAllowanceBars("signed_out"), false);
});

test("compactIcon reflects bind state helper availability and face", () => {
    assert.equal(Logic.compactIcon("ready", true, true), "");
    assert.equal(Logic.compactIcon("signed_out", true, true), "unlock-symbolic");
    assert.equal(Logic.compactIcon("ready", false, true), "question-symbolic");
    assert.equal(Logic.compactIcon("ready", true, false), "folder-open-symbolic");
    assert.equal(Logic.compactIcon("unbound", true, true), "folder-open-symbolic");
    assert.equal(Logic.compactIcon("something-new", true, true), "question-symbolic");
});

test("formatTimeToReset omits elapsed or missing reset times", () => {
    assert.equal(Logic.formatTimeToReset(0, 1000), "");
    assert.equal(Logic.formatTimeToReset(900, 1000), "");
    assert.equal(Logic.formatTimeToReset(1000, 1000), "");
    assert.equal(Logic.formatTimeToReset(4600, 1000), "1h 0m");
    assert.equal(Logic.formatTimeToReset(1300, 1000), "5m");
});

test("allowanceFillColor maps utilization stale and usage credit states", () => {
    assert.equal(Logic.allowanceFillColor(10, true, "none", colors), "gray");
    assert.equal(Logic.allowanceFillColor(100, false, "none", colors), "red");
    assert.equal(Logic.allowanceFillColor(100, false, "exhausted", colors), "red");
    assert.equal(Logic.allowanceFillColor(85, false, "enabled", colors), "orange");
    assert.equal(Logic.allowanceFillColor(20, false, "enabled", colors), "blue");
    // Band edges. Whether Usage Credit alone colours the ring is decision D3 in
    // docs/plan-hardening.md; these pin only what is settled today.
    assert.equal(Logic.allowanceFillColor(80, false, "none", colors), "orange");
    assert.equal(Logic.allowanceFillColor(79.9, false, "none", colors), "blue");
    assert.equal(Logic.allowanceFillColor(100, true, "exhausted", colors), "gray");
});

test("effectiveCurrency prefers config override then locale mapping", () => {
    assert.equal(Logic.effectiveCurrency(" eur ", "en_US"), "EUR");
    assert.equal(Logic.localeCurrencyCode("da_DK"), "DKK");
    assert.equal(Logic.localeCurrencyCode("de_DE"), "EUR");
    assert.equal(Logic.localeCurrencyCode("en_US"), "USD");
    assert.equal(Logic.effectiveCurrency("", "sv_SE"), "SEK");
    assert.equal(Logic.localeCurrencyCode("de_AT"), "EUR");
    assert.equal(Logic.localeCurrencyCode("it_IT"), "EUR");
    assert.equal(Logic.localeCurrencyCode("ja_JP"), "USD");
});

test("parseEcbRates reads the double-quoted attributes the ECB publishes", () => {
    const xml = `<?xml version="1.0" encoding="UTF-8"?>
<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01" xmlns="http://www.ecb.int/vocabulary/2002-08-01/eurofxref">
    <Cube>
        <Cube time="2026-08-21">
            <Cube currency="USD" rate="1.0846"/>
            <Cube currency="DKK" rate="7.4589"/>
        </Cube>
    </Cube>
</gesmes:Envelope>`;
    const rates = Logic.parseEcbRates(xml);
    assert.equal(rates.EUR, 1.0);
    assert.equal(rates.USD, 1.0846);
    assert.equal(rates.DKK, 7.4589);
});

test("parseEcbRates also reads single-quoted attributes", () => {
    const xml = "<Cube currency='USD' rate='1.08'/><Cube currency='DKK' rate='7.45'/>";
    const rates = Logic.parseEcbRates(xml);
    assert.equal(rates.USD, 1.08);
    assert.equal(rates.DKK, 7.45);
});

test("usdToDisplayRate converts through EUR", () => {
    const rates = { EUR: 1.0, USD: 1.08, DKK: 7.45 };
    assert.ok(Math.abs(Logic.usdToDisplayRate(rates, "EUR") - (1 / 1.08)) < 1e-9);
    assert.ok(Math.abs(Logic.usdToDisplayRate(rates, "DKK") - (7.45 / 1.08)) < 1e-9);
    assert.equal(Logic.usdToDisplayRate(rates, "USD"), 1.0);
});

test("usdToDisplayRate returns 0 when no rate is available", () => {
    // 1.0 here would be indistinguishable from a real USD rate, and the caller
    // would label raw USD amounts with the user's currency code.
    assert.equal(Logic.usdToDisplayRate({ EUR: 1.0 }, "DKK"), 0);
    assert.equal(Logic.usdToDisplayRate({ EUR: 1.0, USD: 1.08 }, "XYZ"), 0);
    assert.equal(Logic.usdToDisplayRate({}, "DKK"), 0);
    assert.equal(Logic.usdToDisplayRate(null, "DKK"), 0);
    assert.equal(Logic.usdToDisplayRate(Logic.parseEcbRates("<garbage/>"), "DKK"), 0);
});

test("formatMoney and formatTokens use locale formatting seam", () => {
    const original = Number.prototype.toLocaleString;
    Number.prototype.toLocaleString = function (_locale, _fmt, digits) {
        return this.toFixed(digits ?? 2);
    };
    try {
        assert.equal(Logic.formatMoney(10, 7.45, "DKK", false, "da_DK"), "74.50 DKK");
        assert.equal(Logic.formatMoney(3.5, 1, "USD", true, "en_US"), "3.50 USD");
        // QML defaults this call to 2 decimals; tokens are whole numbers.
        assert.equal(Logic.formatTokens(1234567, "en_US"), "1234567");
        assert.equal(Logic.formatTokens(undefined, "en_US"), "0");
    } finally {
        Number.prototype.toLocaleString = original;
    }
});

test("normalizeSnapshot carries the schema version and degraded reasons", () => {
    const snap = Logic.normalizeSnapshot({
        schema_version: 1,
        face: "ready",
        degraded: ["pending", "usage_unavailable"]
    });
    assert.equal(snap.schema_version, 1);
    assert.deepEqual(snap.degraded, ["pending", "usage_unavailable"]);
});

test("normalizeDegraded tolerates anything the helper might send", () => {
    // A separately installed helper of another version is the normal case, so
    // the field may be absent, the wrong type, or partly junk.
    assert.deepEqual(Logic.normalizeDegraded(undefined), []);
    assert.deepEqual(Logic.normalizeDegraded(null), []);
    assert.deepEqual(Logic.normalizeDegraded("pending"), []);
    assert.deepEqual(Logic.normalizeDegraded([1, "", "pending", null, "x"]), ["pending", "x"]);
    assert.deepEqual(Logic.normalizeSnapshot({ face: "ready" }).degraded, []);
});
