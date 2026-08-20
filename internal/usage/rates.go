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
type RateCard struct {
	Match             string
	InputUSDPerM      float64
	OutputUSDPerM     float64
	CacheWriteUSDPerM float64
	CacheReadUSDPerM  float64
}

// DefaultRates returns Anthropic's published list rates for Claude families.
// Order matters: first Match substring wins (opus before sonnet before haiku).
func DefaultRates() Rates {
	return Rates{
		Cards: []RateCard{
			{Match: "opus", InputUSDPerM: 15.0, OutputUSDPerM: 75.0, CacheWriteUSDPerM: 18.75, CacheReadUSDPerM: 1.50},
			{Match: "sonnet", InputUSDPerM: 3.0, OutputUSDPerM: 15.0, CacheWriteUSDPerM: 3.75, CacheReadUSDPerM: 0.30},
			{Match: "haiku", InputUSDPerM: 0.80, OutputUSDPerM: 4.0, CacheWriteUSDPerM: 1.0, CacheReadUSDPerM: 0.08},
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
