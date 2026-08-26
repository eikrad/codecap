// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import "github.com/eikrad/codecap/internal/snapshot"

// Fixture is one snapshot per Face, built as a snapshot.Snapshot and marshalled
// by the helper's own struct tags rather than typed out as JSON.
//
// AGENTS.md forbids hand-writing a sample of an external format, and the reason
// applies here even though both sides are ours: a hand-written payload would be
// written from the same reading of the contract as the code it is testing, so a
// renamed field would pass. Going through the struct means the field names in
// this harness are the field names the helper actually sends, by construction.
//
// The values are chosen, not real. They are picked so that a wrong one is
// visible rather than plausible — 37 % is the Session that shipped painted red
// (finding D3), and the token counts are distinct per window so a test cannot
// pass by reading the wrong one.
func Fixture(face string) snapshot.Snapshot {
	base := snapshot.Snapshot{
		SchemaVersion: snapshot.SchemaVersion,
		Face:          snapshot.Face(face),
		AccountLabel:  "stub@example.invalid",
		UsageCredit:   "none",
		FetchedAt:     1756200000,
	}

	switch snapshot.Face(face) {
	case snapshot.FaceReady:
		base.SessionAllowance = snapshot.AllowanceWindow{UsedPercent: 37, ResetsAt: 1756203600}
		base.WeeklyAllowance = snapshot.AllowanceWindow{UsedPercent: 61, ResetsAt: 1756720000}
		base.UsageCredit = "enabled"
		base.UsageCreditSpend = snapshot.UsageCreditSpend{UsedUSD: 4.04, LimitUSD: 4}
		base.ConsumedUsage = snapshot.ConsumedUsage{
			Session: snapshot.ConsumedPeriod{ListPriceUSD: 1.25, Tokens: 111},
			Today:   snapshot.ConsumedPeriod{ListPriceUSD: 3.5, Tokens: 222},
			Week:    snapshot.ConsumedPeriod{ListPriceUSD: 12.75, Tokens: 333},
			Month:   snapshot.ConsumedPeriod{ListPriceUSD: 40, Tokens: 444},
		}
	case snapshot.FaceUnknownAllowance:
		// Allowance is unavailable but the local logs were read: the partial
		// case the expanded view still has to fill in.
		base.Degraded = []string{snapshot.DegradedAllowance}
		base.ConsumedUsage = snapshot.ConsumedUsage{
			Today: snapshot.ConsumedPeriod{ListPriceUSD: 3.5, Tokens: 222},
		}
	case snapshot.FaceSignedOut:
		base.AccountLabel = ""
	case snapshot.FaceUnbound:
		base.AccountLabel = ""
	}

	return base
}
