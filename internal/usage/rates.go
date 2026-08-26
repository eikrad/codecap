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

// DefaultRates returns Anthropic's published list rates for Claude families.
// Order matters: first Match substring wins (opus before sonnet before haiku).
//
// KNOWN STALE — the base rates below are a family-wide approximation and do not
// track model versions. `opus` matches every Opus ever released, so an Opus 4.1
// rate is applied to a current Opus; the same holds for the Sonnet and Haiku
// rows. Measured against one real session, the resulting List Price came out
// several times the figure the vendor reported for the same tokens.
//
// Fixing it needs per-model-ID cards and a current price list, which is a
// separate change. The lifetime split below is correct and independent of it:
// whatever the base rate turns out to be, a one-hour write is 2x it and a
// five-minute write is 1.25x, and pricing both at 1.25x understated the cache
// write line by about half on this desktop's logs (86% of writes were one-hour).
func DefaultRates() Rates {
	return Rates{
		Cards: []RateCard{
			{Match: "opus", InputUSDPerM: 15.0, OutputUSDPerM: 75.0, CacheWrite5mUSDPerM: 18.75, CacheWrite1hUSDPerM: 30.0, CacheReadUSDPerM: 1.50},
			{Match: "sonnet", InputUSDPerM: 3.0, OutputUSDPerM: 15.0, CacheWrite5mUSDPerM: 3.75, CacheWrite1hUSDPerM: 6.0, CacheReadUSDPerM: 0.30},
			{Match: "haiku", InputUSDPerM: 0.80, OutputUSDPerM: 4.0, CacheWrite5mUSDPerM: 1.0, CacheWrite1hUSDPerM: 1.60, CacheReadUSDPerM: 0.08},
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
