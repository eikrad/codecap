// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usage

import "testing"

// The bug this pins: the table held three family-wide cards carrying Opus 4.1,
// Sonnet 4.5 and Haiku 3.5 rates — two of them retired models — and priced every
// generation since at those. `claude-opus-5` was billed at $15/MTok input
// against a published $5.
//
// Model IDs are the ones actually present in this desktop's logs, plus the
// retired shapes that older logs can still carry.
func TestDefaultRatesPriceCurrentModelsAtCurrentRates(t *testing.T) {
	cases := []struct {
		model       string
		wantInput   float64
		wantOutput  float64
		wantRead    float64
		wantWrite1h float64
	}{
		{"claude-opus-5", 5.0, 25.0, 0.50, 10.0},
		{"claude-opus-4-8", 5.0, 25.0, 0.50, 10.0},
		{"claude-opus-4-5", 5.0, 25.0, 0.50, 10.0},
		{"claude-sonnet-5", 2.0, 10.0, 0.20, 4.0},
		{"claude-sonnet-4-6", 3.0, 15.0, 0.30, 6.0},
		{"claude-haiku-4-5-20251001", 1.0, 5.0, 0.10, 2.0},
		{"claude-fable-5", 10.0, 50.0, 1.0, 20.0},

		// Retired. An old log event was billed at the rate of its day.
		{"claude-opus-4-1-20250805", 15.0, 75.0, 1.50, 30.0},
		{"claude-opus-4-20250514", 15.0, 75.0, 1.50, 30.0},
		{"claude-3-5-haiku-20241022", 0.80, 4.0, 0.08, 1.60},
		{"claude-3-haiku-20240307", 0.80, 4.0, 0.08, 1.60},
	}

	rates := DefaultRates()
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			card := rates.forModel(tc.model)
			if card.InputUSDPerM != tc.wantInput {
				t.Errorf("input = %v, want %v (matched card %q)", card.InputUSDPerM, tc.wantInput, card.Match)
			}
			if card.OutputUSDPerM != tc.wantOutput {
				t.Errorf("output = %v, want %v (matched card %q)", card.OutputUSDPerM, tc.wantOutput, card.Match)
			}
			if card.CacheReadUSDPerM != tc.wantRead {
				t.Errorf("cache read = %v, want %v (matched card %q)", card.CacheReadUSDPerM, tc.wantRead, card.Match)
			}
			if card.CacheWrite1hUSDPerM != tc.wantWrite1h {
				t.Errorf("1h cache write = %v, want %v (matched card %q)", card.CacheWrite1hUSDPerM, tc.wantWrite1h, card.Match)
			}
		})
	}
}

// Card order is load-bearing: a family catch-all placed before its
// version-specific cards would swallow them. These two are the pairs where the
// version-specific rate differs from the family's, so they are the ones order
// can actually break.
func TestVersionSpecificCardsWinOverTheirFamily(t *testing.T) {
	rates := DefaultRates()

	if sonnet5, sonnet46 := rates.forModel("claude-sonnet-5"), rates.forModel("claude-sonnet-4-6"); sonnet5.InputUSDPerM >= sonnet46.InputUSDPerM {
		t.Errorf("Sonnet 5 ($2) must be cheaper than Sonnet 4.6 ($3); got %v and %v", sonnet5.InputUSDPerM, sonnet46.InputUSDPerM)
	}
	if opus5, opus41 := rates.forModel("claude-opus-5"), rates.forModel("claude-opus-4-1-20250805"); opus5.InputUSDPerM >= opus41.InputUSDPerM {
		t.Errorf("Opus 5 ($5) must be cheaper than retired Opus 4.1 ($15); got %v and %v", opus5.InputUSDPerM, opus41.InputUSDPerM)
	}
}

// Every published card prices a one-hour write above a five-minute one, and a
// cache read below base input. A row that violates either is a transcription
// error, not a pricing decision.
func TestEveryCardIsInternallyConsistent(t *testing.T) {
	for _, card := range DefaultRates().Cards {
		if card.CacheWrite1hUSDPerM <= card.CacheWrite5mUSDPerM {
			t.Errorf("%q: 1h write %v should exceed 5m write %v", card.Match, card.CacheWrite1hUSDPerM, card.CacheWrite5mUSDPerM)
		}
		if card.CacheWrite5mUSDPerM <= card.InputUSDPerM {
			t.Errorf("%q: 5m write %v should exceed base input %v", card.Match, card.CacheWrite5mUSDPerM, card.InputUSDPerM)
		}
		if card.CacheReadUSDPerM >= card.InputUSDPerM {
			t.Errorf("%q: cache read %v should be below base input %v", card.Match, card.CacheReadUSDPerM, card.InputUSDPerM)
		}
		if card.OutputUSDPerM <= card.InputUSDPerM {
			t.Errorf("%q: output %v should exceed input %v", card.Match, card.OutputUSDPerM, card.InputUSDPerM)
		}
	}
}
