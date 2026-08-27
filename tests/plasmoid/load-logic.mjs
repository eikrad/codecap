// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const logicPath = path.join(__dirname, "..", "..", "plasmoid", "contents", "ui", "logic.js");

export function loadLogic() {
    const source = fs.readFileSync(logicPath, "utf8").replace(/^\.pragma library\s*\n/m, "");
    const fn = new Function(`
        ${source}
        return {
            emptySnapshot,
            expandPath,
            stripFileScheme,
            finiteNumber,
            normalizeWindow,
            normalizeDegraded,
            normalizePeriod,
            normalizeSnapshot,
            parseSnapshot,
            extractSnapshotPayload,
            describePayload,
            localFaceSnapshot,
            staleSnapshot,
            showAllowanceBars,
            compactIcon,
            formatTimeToReset,
            allowanceFillColor,
            localeCurrencyCode,
            effectiveCurrency,
            parseEcbRates,
            usdToDisplayRate,
            formatMoney,
            formatTokens
        };
    `);
    return fn();
}
