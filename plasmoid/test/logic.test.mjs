// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import assert from "node:assert/strict";
import test from "node:test";
import { loadLogic } from "./load-logic.mjs";

const Logic = loadLogic();
const colors = { disabled: "gray", negative: "red", neutral: "orange", highlight: "blue" };

test("parseSnapshot returns unbound empty snapshot for invalid JSON", () => {
    const snap = Logic.parseSnapshot("not-json");
    assert.equal(snap.face, "unbound");
    assert.equal(snap.consumed_usage.session.tokens, 0);
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
});

test("formatTimeToReset omits elapsed or missing reset times", () => {
    assert.equal(Logic.formatTimeToReset(0, 1000), "");
    assert.equal(Logic.formatTimeToReset(900, 1000), "");
    assert.equal(Logic.formatTimeToReset(4600, 1000), "1h 0m");
    assert.equal(Logic.formatTimeToReset(1300, 1000), "5m");
});

test("allowanceFillColor maps utilization stale and usage credit states", () => {
    assert.equal(Logic.allowanceFillColor(10, true, "none", colors), "gray");
    assert.equal(Logic.allowanceFillColor(100, false, "none", colors), "red");
    assert.equal(Logic.allowanceFillColor(100, false, "exhausted", colors), "red");
    assert.equal(Logic.allowanceFillColor(85, false, "enabled", colors), "orange");
    assert.equal(Logic.allowanceFillColor(20, false, "enabled", colors), "blue");
});

test("effectiveCurrency prefers config override then locale mapping", () => {
    assert.equal(Logic.effectiveCurrency(" eur ", "en_US"), "EUR");
    assert.equal(Logic.localeCurrencyCode("da_DK"), "DKK");
    assert.equal(Logic.localeCurrencyCode("de_DE"), "EUR");
    assert.equal(Logic.localeCurrencyCode("en_US"), "USD");
    assert.equal(Logic.effectiveCurrency("", "sv_SE"), "SEK");
});

test("parseEcbRates and usdToDisplayRate convert USD list price", () => {
    const xml = `<?xml version="1.0"?>
        <Cube>
            <Cube currency='USD' rate='1.08'/>
            <Cube currency='DKK' rate='7.45'/>
        </Cube>`;
    const rates = Logic.parseEcbRates(xml);
    assert.equal(rates.EUR, 1.0);
    assert.equal(rates.USD, 1.08);
    assert.equal(rates.DKK, 7.45);
    assert.ok(Math.abs(Logic.usdToDisplayRate(rates, "EUR") - (1 / 1.08)) < 1e-9);
    assert.ok(Math.abs(Logic.usdToDisplayRate(rates, "DKK") - (7.45 / 1.08)) < 1e-9);
    assert.equal(Logic.usdToDisplayRate(rates, "USD"), 1.0);
});

test("formatMoney and formatTokens use locale formatting seam", () => {
    const original = Number.prototype.toLocaleString;
    Number.prototype.toLocaleString = function (_locale, _fmt, digits) {
        return this.toFixed(digits ?? 2);
    };
    try {
        assert.equal(Logic.formatMoney(10, 7.45, "DKK", false, "da_DK"), "74.50 DKK");
        assert.equal(Logic.formatMoney(3.5, 1, "USD", true, "en_US"), "3.50 USD");
        assert.equal(Logic.formatTokens(1234567, "en_US"), "1234567.00");
    } finally {
        Number.prototype.toLocaleString = original;
    }
});
