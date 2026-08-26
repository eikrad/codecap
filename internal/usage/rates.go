// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usage

import "strings"

// Rates is the published Anthropic List Price table in USD per million tokens.
// Swap or extend this at the Compute seam; v1 does not fetch prices over the network.
type Rates struct {
	Cards []RateCard
}

// RateCard matches a model name substring (case-insensitive) to per-million USD rates.
//
// Cache writes are priced by the lifetime they are written with, so there are
// two write rates rather than one. The vendor's published multipliers over base
// input are 1.25x for a five-minute entry and 2x for a one-hour entry; the
// figures below are those multipliers applied to InputUSDPerM.
type RateCard struct {
	Match               string
	InputUSDPerM        float64
	OutputUSDPerM       float64
	CacheWrite5mUSDPerM float64
	CacheWrite1hUSDPerM float64
	CacheReadUSDPerM    float64
}

// DefaultRates returns Anthropic's published list rates.
//
// Checked against the published price list on 2026-08-26. Every rate here is
// quoted, not derived — the write columns are published per model rather than
// computed from a multiplier, so they are copied as printed.
//
// Order matters: the first Match substring wins, so version-specific cards must
// precede their family's catch-all. A bare "opus" card first would have priced a
// current Opus at a retired model's rate, which is exactly the bug this replaces:
// the previous table's three rows were Opus 4.1, Sonnet 4.5, and Haiku 3.5
// rates, two of them retired models, applied to every generation since.
//
// The catch-all rows carry the current generation's price. A model newer than
// this table therefore gets today's rate rather than a decade-old one — wrong if
// prices move, but wrong by a smaller margin, and in the direction that ages
// better. Keeping this current is a maintenance point at every model release.
func DefaultRates() Rates {
	return Rates{
		Cards: []RateCard{
			// Retired, but they still appear in older logs, and a log is
			// historical: an event from an Opus 4.1 session was billed at Opus
			// 4.1 rates no matter what the model costs now.
			//
			// Note the two ID shapes. Claude 4 and later put the version after
			// the family (`claude-opus-4-1-20250805`); Claude 3 put it before
			// (`claude-3-5-haiku-20241022`), so the Haiku 3 cards match on the
			// leading version instead.
			{Match: "opus-4-1", InputUSDPerM: 15.0, OutputUSDPerM: 75.0, CacheWrite5mUSDPerM: 18.75, CacheWrite1hUSDPerM: 30.0, CacheReadUSDPerM: 1.50},
			{Match: "opus-4-2025", InputUSDPerM: 15.0, OutputUSDPerM: 75.0, CacheWrite5mUSDPerM: 18.75, CacheWrite1hUSDPerM: 30.0, CacheReadUSDPerM: 1.50},
			{Match: "3-5-haiku", InputUSDPerM: 0.80, OutputUSDPerM: 4.0, CacheWrite5mUSDPerM: 1.0, CacheWrite1hUSDPerM: 1.60, CacheReadUSDPerM: 0.08},
			{Match: "3-haiku", InputUSDPerM: 0.80, OutputUSDPerM: 4.0, CacheWrite5mUSDPerM: 1.0, CacheWrite1hUSDPerM: 1.60, CacheReadUSDPerM: 0.08},

			// Sonnet 5 is cheaper than the 4.x Sonnets it follows, so the
			// version-specific card has to come first or it inherits their rate.
			// The $2/$10 shipped as introductory pricing and is now standard —
			// the increase to $3/$15 announced for 2026-09-01 was cancelled.
			{Match: "sonnet-5", InputUSDPerM: 2.0, OutputUSDPerM: 10.0, CacheWrite5mUSDPerM: 2.50, CacheWrite1hUSDPerM: 4.0, CacheReadUSDPerM: 0.20},

			{Match: "fable", InputUSDPerM: 10.0, OutputUSDPerM: 50.0, CacheWrite5mUSDPerM: 12.50, CacheWrite1hUSDPerM: 20.0, CacheReadUSDPerM: 1.0},
			{Match: "mythos", InputUSDPerM: 10.0, OutputUSDPerM: 50.0, CacheWrite5mUSDPerM: 12.50, CacheWrite1hUSDPerM: 20.0, CacheReadUSDPerM: 1.0},

			// Current generation. Opus 4.5 through Opus 5 all price the same;
			// Sonnet 4.5 and 4.6 share theirs; Haiku 4.5 is alone in its family.
			{Match: "opus", InputUSDPerM: 5.0, OutputUSDPerM: 25.0, CacheWrite5mUSDPerM: 6.25, CacheWrite1hUSDPerM: 10.0, CacheReadUSDPerM: 0.50},
			{Match: "sonnet", InputUSDPerM: 3.0, OutputUSDPerM: 15.0, CacheWrite5mUSDPerM: 3.75, CacheWrite1hUSDPerM: 6.0, CacheReadUSDPerM: 0.30},
			{Match: "haiku", InputUSDPerM: 1.0, OutputUSDPerM: 5.0, CacheWrite5mUSDPerM: 1.25, CacheWrite1hUSDPerM: 2.0, CacheReadUSDPerM: 0.10},
		},
	}
}

func (r Rates) forModel(model string) RateCard {
	lower := strings.ToLower(model)
	for _, card := range r.Cards {
		if strings.Contains(lower, card.Match) {
			return card
		}
	}
	if len(r.Cards) > 0 {
		// Prefer sonnet-like default when the table includes it.
		for _, card := range r.Cards {
			if card.Match == "sonnet" {
				return card
			}
		}
		return r.Cards[0]
	}
	return RateCard{}
}
