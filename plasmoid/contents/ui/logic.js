// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

.pragma library

// QML's StandardPaths returns a url, not a path, so homeDir arrives as
// "file:///home/you". The helper stats the string it is given, so a scheme has
// to be gone before anything is joined to it.
function stripFileScheme(value) {
    // QML hands this a url object, not a string. `value || ""` keeps the url
    // (it is truthy) and url has no trim(), so coercing first is the whole
    // point of this function existing.
    if (value === undefined || value === null) {
        return ""
    }
    var trimmed = String(value).trim()
    if (trimmed.indexOf("file://") !== 0) {
        return trimmed
    }
    var path = trimmed.substring(7)
    try {
        return decodeURIComponent(path)
    } catch (e) {
        return path
    }
}

function expandPath(path, homeDir) {
    var trimmed = stripFileScheme(path)
    var home = stripFileScheme(homeDir)
    if (trimmed === "") {
        return ""
    }
    if (trimmed === "~") {
        return home
    }
    if (trimmed.indexOf("~/") === 0) {
        return home + trimmed.substring(1)
    }
    return trimmed
}

function emptySnapshot() {
    return {
        schema_version: 0,
        degraded: [],
        face: "unbound",
        account_home: "",
        account_label: "",
        session_allowance: { used_percent: 0, resets_at: 0, stale: false },
        weekly_allowance: { used_percent: 0, resets_at: 0, stale: false },
        usage_credit: "none",
        usage_credit_spend: { used_usd: 0, limit_usd: 0 },
        consumed_usage: {
            session: { list_price_usd: 0, tokens: 0 },
            today: { list_price_usd: 0, tokens: 0 },
            week: { list_price_usd: 0, tokens: 0 },
            month: { list_price_usd: 0, tokens: 0 }
        },
        fetched_at: 0
    }
}

function finiteNumber(value) {
    var n = Number(value)
    return isFinite(n) ? n : 0
}

function normalizeWindow(value) {
    if (!value || typeof value !== "object") {
        return { used_percent: 0, resets_at: 0, stale: false }
    }
    return {
        used_percent: finiteNumber(value.used_percent),
        resets_at: finiteNumber(value.resets_at),
        stale: !!value.stale
    }
}

// An older helper sends no usage_credit_spend at all, so this has to survive the
// field being absent rather than assume the two are upgraded together.
function normalizeUsageCreditSpend(value) {
    if (!value || typeof value !== "object") {
        return { used_usd: 0, limit_usd: 0 }
    }
    return {
        used_usd: finiteNumber(value.used_usd),
        limit_usd: finiteNumber(value.limit_usd)
    }
}

function normalizePeriod(value) {
    if (!value || typeof value !== "object") {
        return { list_price_usd: 0, tokens: 0 }
    }
    return {
        list_price_usd: finiteNumber(value.list_price_usd),
        tokens: finiteNumber(value.tokens)
    }
}

// The helper reports why a snapshot is incomplete as stable codes, never as
// message text: the wording belongs here, and an error string could carry a
// filesystem path across the bus.
function normalizeDegraded(value) {
    if (!Array.isArray(value)) {
        return []
    }
    var reasons = []
    for (var i = 0; i < value.length; i++) {
        if (typeof value[i] === "string" && value[i] !== "") {
            reasons.push(value[i])
        }
    }
    return reasons
}

// Helper and plasmoid are installed separately and versioned separately, so a
// payload may be partial or carry unexpected types. Every field is filled in
// and coerced; nothing downstream should ever see undefined or NaN.
function normalizeSnapshot(decoded) {
    var base = emptySnapshot()
    if (!decoded || typeof decoded !== "object") {
        return base
    }
    if (typeof decoded.face === "string" && decoded.face !== "") {
        base.face = decoded.face
    }
    base.account_home = typeof decoded.account_home === "string" ? decoded.account_home : ""
    base.account_label = typeof decoded.account_label === "string" ? decoded.account_label : ""
    base.usage_credit = typeof decoded.usage_credit === "string" && decoded.usage_credit !== ""
        ? decoded.usage_credit : "none"
    base.fetched_at = finiteNumber(decoded.fetched_at)
    base.schema_version = finiteNumber(decoded.schema_version)
    base.degraded = normalizeDegraded(decoded.degraded)
    base.session_allowance = normalizeWindow(decoded.session_allowance)
    base.weekly_allowance = normalizeWindow(decoded.weekly_allowance)
    base.usage_credit_spend = normalizeUsageCreditSpend(decoded.usage_credit_spend)

    var consumed = decoded.consumed_usage
    if (!consumed || typeof consumed !== "object") {
        consumed = {}
    }
    base.consumed_usage = {
        session: normalizePeriod(consumed.session),
        today: normalizePeriod(consumed.today),
        week: normalizePeriod(consumed.week),
        month: normalizePeriod(consumed.month)
    }
    return base
}

// The D-Bus reply value's shape is documented only by example, and the example
// uses a method that returns a dict. For a single out-argument of type "s" the
// value can arrive as the string itself, as a one-element list, or as an object
// keyed by the out-argument name. Accept all three rather than assuming one.
function extractSnapshotPayload(result) {
    var payload = result
    if (payload && typeof payload === "object" && payload.value !== undefined) {
        payload = payload.value
    }
    if (Array.isArray(payload)) {
        payload = payload.length > 0 ? payload[0] : ""
    }
    if (payload && typeof payload === "object") {
        if (typeof payload.snapshotJSON === "string") {
            return payload.snapshotJSON
        }
        for (var key in payload) {
            if (typeof payload[key] === "string") {
                return payload[key]
            }
        }
    }
    return payload
}

// signalAccountHome reads the Account Home out of a Changed signal argument.
//
// SignalWatcher does not hand the handler a bare string. It decodes a "s"
// argument to {value: "..."}, so `accountHome === source.accountHome` was false
// for every signal the helper ever sent, with no warning and no error -- the
// same shape extractSnapshotPayload exists to unwrap on the reply side, and the
// same class of finding as P-C2. Verified against a real SignalWatcher on a
// private session bus in tests/plasmoid/qml-dbus.
//
// Returns null when no string can be read at all. "" would be indistinguishable
// from a genuinely empty Account Home, and the caller has to tell those apart.
function signalAccountHome(argument) {
    var value = argument
    if (value && typeof value === "object" && !Array.isArray(value) && value.value !== undefined) {
        value = value.value
    }
    if (Array.isArray(value)) {
        value = value.length > 0 ? value[0] : undefined
    }
    return typeof value === "string" ? value : null
}

// describePayload is for the log line when a reply cannot be read: the shape is
// the only thing that identifies which of the cases above went unhandled.
function describePayload(value) {
    if (value === null || value === undefined) {
        return String(value)
    }
    if (typeof value !== "object") {
        return typeof value + " " + String(value).substring(0, 120)
    }
    if (Array.isArray(value)) {
        return "array[" + value.length + "]"
    }
    var keys = []
    for (var key in value) {
        keys.push(key)
    }
    return "object{" + keys.join(",") + "}"
}

// Returns null when the payload cannot be read at all. An unreadable reply is
// not the same thing as "no Account Home chosen", and the caller is the only
// place that knows which face to show instead.
function parseSnapshot(jsonString) {
    if (typeof jsonString !== "string" || jsonString.trim() === "") {
        return null
    }
    try {
        var decoded = JSON.parse(jsonString)
        // An array is typeof "object", so without the check a reply of "[]"
        // reached normalizeSnapshot, which found no face on it and returned the
        // empty snapshot — face "unbound". That tells the user they have not
        // chosen an Account Home when what actually happened is that the helper
        // sent something unreadable. That is finding P-M6 by another route.
        if (!decoded || typeof decoded !== "object" || Array.isArray(decoded)) {
            return null
        }
        return normalizeSnapshot(decoded)
    } catch (e) {
        return null
    }
}

// The face for a state the plasmoid decides on its own, with no helper reply
// involved: no Account Home bound, or a call that produced nothing usable.
//
// It returns the snapshot rather than assigning one, because assigning a var
// property fires the change signal and mutating it afterwards does not — every
// face has to be complete before it is assigned (finding P-H3). Building it
// here also means the decision is testable: a .qml file is not code CI can run.
function localFaceSnapshot(face, accountHome) {
    var next = emptySnapshot()
    next.face = face
    if (face !== "unbound") {
        next.account_home = accountHome
    }
    return next
}

// What to show when the call failed, or its reply could not be read.
//
// ADR 0006 keeps Last-Known Allowance visible with staleness shown, so a ready
// snapshot survives with both windows marked stale rather than being blanked
// (finding P-M4). Anything else becomes Unknown Allowance — never Unbound,
// which would claim the user has not chosen an Account Home (finding P-M6).
function staleSnapshot(previous, accountHome) {
    if (previous && previous.face === "ready") {
        var next = normalizeSnapshot(previous)
        next.session_allowance.stale = true
        next.weekly_allowance.stale = true
        return next
    }
    return localFaceSnapshot("unknown_allowance", accountHome)
}

function showAllowanceBars(face) {
    return face === "ready"
}

function compactIcon(face, helperAvailable, isBound) {
    if (!isBound) {
        return "folder-open-symbolic"
    }
    if (!helperAvailable) {
        return "question-symbolic"
    }
    switch (face) {
    case "unbound":
        return "folder-open-symbolic"
    case "signed_out":
        return "unlock-symbolic"
    case "ready":
        return ""
    default:
        return "question-symbolic"
    }
}

function formatTimeToReset(resetsAtUnix, nowUnix) {
    if (!resetsAtUnix || resetsAtUnix <= nowUnix) {
        return ""
    }
    var seconds = resetsAtUnix - nowUnix
    var hours = Math.floor(seconds / 3600)
    var minutes = Math.floor((seconds % 3600) / 60)
    if (hours > 0) {
        return hours + "h " + minutes + "m"
    }
    return minutes + "m"
}

// Colour reports the fill it sits on and nothing else: the ring is the Session
// Allowance, and the expanded bars are Session and Weekly. Usage Credit is a
// separate, monthly thing — letting "exhausted" force red painted a Session at
// 37% as if it were spent, which is the opposite of what the ring is for. It
// stays a label in the expanded view. Visual revision 2026-08-26, see
// design.md; this closes D3 in docs/plan-hardening.md.
function allowanceFillColor(usedPercent, stale, colors) {
    if (stale) {
        return colors.disabled
    }
    if (usedPercent >= 100) {
        return colors.negative
    }
    if (usedPercent >= 80) {
        return colors.neutral
    }
    return colors.highlight
}

// Tray auto-hide (Plasma::Types::ItemStatus). Returns a stable name that QML
// maps onto PlasmaCore.Types — logic.js cannot import Plasma modules, and
// keeping the decision here is what lets CI assert the 80% band without a
// desktop. Faces that need the user to act stay visible; a calm Session can
// hide. Finding P-M14.
function trayStatus(face, usedPercent, stale) {
    if (face === "unbound" || face === "signed_out") {
        return "needsAttention"
    }
    if (face !== "ready") {
        return "active"
    }
    if (stale || usedPercent >= 100) {
        return "needsAttention"
    }
    if (usedPercent >= 80) {
        return "active"
    }
    return "passive"
}

function localeCurrencyCode(localeName) {
    var map = {
        "da_DK": "DKK",
        "de_DE": "EUR",
        "en_US": "USD",
        "en_GB": "GBP",
        "fr_FR": "EUR",
        "sv_SE": "SEK",
        "nb_NO": "NOK",
        "nn_NO": "NOK"
    }
    if (map[localeName]) {
        return map[localeName]
    }
    if (localeName.indexOf("de_") === 0 || localeName.indexOf("fr_") === 0 || localeName.indexOf("es_") === 0 || localeName.indexOf("it_") === 0) {
        return "EUR"
    }
    return "USD"
}

function effectiveCurrency(configCurrency, localeName) {
    var trimmed = (configCurrency || "").trim()
    if (trimmed !== "") {
        return trimmed.toUpperCase()
    }
    return localeCurrencyCode(localeName)
}

function parseEcbRates(xml) {
    var rates = { EUR: 1.0 }
    // The published feed uses double quotes; single quotes are legal XML too.
    var re = /currency=["']([A-Z]{3})["']\s+rate=["']([0-9.]+)["']/g
    var match
    while ((match = re.exec(xml)) !== null) {
        rates[match[1]] = parseFloat(match[2])
    }
    return rates
}

// Returns 0 when no rate is available, so the caller can show USD visibly
// instead of silently labelling USD amounts with another currency code.
function usdToDisplayRate(rates, targetCurrency) {
    if (targetCurrency === "USD") {
        return 1.0
    }
    if (!rates || !rates.USD) {
        return 0
    }
    if (targetCurrency === "EUR") {
        return 1.0 / rates.USD
    }
    if (!rates[targetCurrency]) {
        return 0
    }
    return rates[targetCurrency] / rates.USD
}

function formatMoney(amountUsd, rate, currencyCode, usingUsdFallback, locale) {
    var amount = amountUsd * rate
    var formatted = amount.toLocaleString(locale, "f", 2)
    if (usingUsdFallback) {
        return formatted + " USD"
    }
    return formatted + " " + currencyCode
}

// QML's Number.toLocaleString defaults to format "f" with precision 2, unlike
// ECMAScript's, so the precision has to be given explicitly for whole tokens.
function formatTokens(tokens, locale) {
    return finiteNumber(tokens).toLocaleString(locale, "f", 0)
}
