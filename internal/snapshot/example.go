// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package snapshot

// Faces is every Face the helper can put on the bus. A new Face belongs in
// this list, because the golden documents are keyed on it.
func Faces() []Face {
	return []Face{FaceUnbound, FaceSignedOut, FaceUnknownAllowance, FaceReady}
}

// Example is one snapshot per Face. The numbers are chosen so that reading the
// wrong window is obvious: 37% is the Session that shipped painted red, and the
// token counts differ per period. testdata holds json.Marshal of these values,
// which is the document GetSnapshot sends.
func Example(face Face) Snapshot {
	base := Snapshot{
		SchemaVersion: SchemaVersion,
		Face:          face,
		AccountHome:   "/home/me/.claude",
		AccountLabel:  "stub@example.invalid",
		UsageCredit:   "none",
		FetchedAt:     1756200000,
	}

	// Signed Out is the base: a chosen Account Home and label, Usage Credit
	// "none", and no degraded reason. The other Faces vary from that.
	switch face {
	case FaceUnbound:
		base.AccountHome = ""
		base.AccountLabel = ""
	case FaceUnknownAllowance:
		base.Degraded = []string{DegradedAllowance}
		base.ConsumedUsage.Today = ConsumedPeriod{ListPriceUSD: 3.5, Tokens: 222}
	case FaceReady:
		base.SessionAllowance = AllowanceWindow{UsedPercent: 37, ResetsAt: 1756203600}
		base.WeeklyAllowance = AllowanceWindow{UsedPercent: 61, ResetsAt: 1756720000}
		base.UsageCredit = "enabled"
		base.UsageCreditSpend = UsageCreditSpend{UsedUSD: 4.04, LimitUSD: 4}
		base.ConsumedUsage = ConsumedUsage{
			Session: ConsumedPeriod{ListPriceUSD: 1.25, Tokens: 111},
			Today:   ConsumedPeriod{ListPriceUSD: 3.5, Tokens: 222},
			Week:    ConsumedPeriod{ListPriceUSD: 12.75, Tokens: 333},
			Month:   ConsumedPeriod{ListPriceUSD: 40, Tokens: 444},
		}
	}

	return base
}
