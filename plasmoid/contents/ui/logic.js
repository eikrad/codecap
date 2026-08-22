// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

.pragma library

function expandPath(path, homeDir) {
    var trimmed = (path || "").trim()
    if (trimmed === "") {
        return ""
    }
    if (trimmed === "~") {
        return homeDir || ""
    }
    if (trimmed.indexOf("~/") === 0) {
        return (homeDir || "") + trimmed.substring(1)
    }
    return trimmed
}

function emptySnapshot() {
    return {
        face: "unbound",
        account_home: "",
        account_label: "",
        session_allowance: { used_percent: 0, resets_at: 0, stale: false },
        weekly_allowance: { used_percent: 0, resets_at: 0, stale: false },
        usage_credit: "none",
        consumed_usage: {
            session: { list_price_usd: 0, tokens: 0 },
            today: { list_price_usd: 0, tokens: 0 },
            week: { list_price_usd: 0, tokens: 0 },
            month: { list_price_usd: 0, tokens: 0 }
        },
        fetched_at: 0
    }
}

function normalizeSnapshot(decoded) {
    var base = emptySnapshot()
    if (!decoded || typeof decoded !== "object") {
        return base
    }
    base.face = decoded.face || base.face
    base.account_home = decoded.account_home || ""
    base.account_label = decoded.account_label || ""
    base.usage_credit = decoded.usage_credit || "none"
    base.fetched_at = decoded.fetched_at || 0
    if (decoded.session_allowance) {
        base.session_allowance = decoded.session_allowance
    }
    if (decoded.weekly_allowance) {
        base.weekly_allowance = decoded.weekly_allowance
    }
    if (decoded.consumed_usage) {
        base.consumed_usage = decoded.consumed_usage
    }
    return base
}

function parseSnapshot(jsonString) {
    try {
        return normalizeSnapshot(JSON.parse(jsonString))
    } catch (e) {
        return emptySnapshot()
    }
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

function allowanceFillColor(usedPercent, stale, usageCredit, colors) {
    if (stale) {
        return colors.disabled
    }
    if (usedPercent >= 100 || usageCredit === "exhausted") {
        return colors.negative
    }
    if (usageCredit !== "none" && usedPercent >= 100) {
        return colors.negative
    }
    if (usedPercent >= 80) {
        return colors.neutral
    }
    return colors.highlight
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
    var re = /currency='([A-Z]{3})'\s+rate='([0-9.]+)'/g
    var match
    while ((match = re.exec(xml)) !== null) {
        rates[match[1]] = parseFloat(match[2])
    }
    return rates
}

function usdToDisplayRate(rates, targetCurrency) {
    if (targetCurrency === "USD") {
        return 1.0
    }
    if (!rates.USD) {
        return 1.0
    }
    if (targetCurrency === "EUR") {
        return 1.0 / rates.USD
    }
    if (!rates[targetCurrency]) {
        return 1.0
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

function formatTokens(tokens, locale) {
    return tokens.toLocaleString(locale)
}
