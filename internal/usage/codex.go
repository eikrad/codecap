// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package usage

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type codexTokens struct {
	Input  int64 `json:"input_tokens"`
	Cached int64 `json:"cached_input_tokens"`
	Write  int64 `json:"cache_write_input_tokens"`
	Output int64 `json:"output_tokens"`
	Total  int64 `json:"total_tokens"`
}

type codexLog struct {
	model    string
	tier     string
	previous codexTokens
}

func (t codexTokens) valid() bool {
	return t.Input >= 0 && t.Cached >= 0 && t.Write >= 0 && t.Output >= 0 &&
		t.Cached <= t.Input && t.Write <= t.Input-t.Cached &&
		t.Total >= t.Output && t.Input == t.Total-t.Output
}

func (t codexTokens) subtract(previous codexTokens) codexTokens {
	return codexTokens{Input: t.Input - previous.Input, Cached: t.Cached - previous.Cached,
		Write: t.Write - previous.Write, Output: t.Output - previous.Output, Total: t.Total - previous.Total}
}

func (c *codexLog) consume(decoded usageEvent) (event, bool, error) {
	if decoded.Type == "turn_context" {
		var context struct {
			Model string `json:"model"`
			Tier  string `json:"service_tier"`
		}
		if err := json.Unmarshal(decoded.Payload, &context); err != nil {
			return event{}, false, errors.New("invalid Codex turn context")
		}
		c.model, c.tier = context.Model, context.Tier
		return event{}, false, nil
	}
	var payload struct {
		Type string `json:"type"`
		Info *struct {
			Total *codexTokens `json:"total_token_usage"`
			Last  *codexTokens `json:"last_token_usage"`
		} `json:"info"`
	}
	if err := json.Unmarshal(decoded.Payload, &payload); err != nil {
		return event{}, false, errors.New("invalid Codex token event")
	}
	if payload.Type != "token_count" || payload.Info == nil {
		return event{}, false, nil
	}
	if payload.Info.Total == nil || !payload.Info.Total.valid() {
		return event{}, false, errors.New("missing or inconsistent Codex cumulative counters")
	}
	total := *payload.Info.Total
	if total == c.previous {
		return event{}, false, nil
	}
	delta := total.subtract(c.previous)
	if total.Total < c.previous.Total {
		// A compaction/counter reset is not a negative purchase. The log's
		// per-request counters are authoritative at that boundary.
		if payload.Info.Last == nil {
			return event{}, false, errors.New("missing Codex request counters after reset")
		}
		delta = *payload.Info.Last
	}
	if !delta.valid() {
		return event{}, false, errors.New("inconsistent Codex request counters")
	}
	at, err := time.Parse(time.RFC3339Nano, decoded.Timestamp)
	if err != nil {
		return event{}, false, errors.New("invalid Codex token event timestamp")
	}
	c.previous = total
	if delta.Total == 0 {
		return event{}, false, nil
	}
	price, ok := codexListPrice(c.model, c.tier, delta)
	// Forks and archives can copy an event byte for byte. An event's time,
	// model and cumulative counters identify that usage across those copies.
	identity := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%v", decoded.Timestamp, c.model, total)))
	return event{at: at, identity: fmt.Sprintf("codex:%x", identity), tokens: delta.Total, priceUSD: price, priceUnavailable: !ok}, true, nil
}

type codexRateCard struct{ input, cached, output float64 }

var codexRateCards = map[string]codexRateCard{
	"gpt-6-astra":   {10, 1, 50},
	"gpt-6.1-sol":   {2, .1, 10},
	"gpt-6.1-terra": {1, .05, 5},
	"gpt-6-luna":    {.1, .01, .5},
	"gpt-5.6-sol":   {4, .4, 20},
	"gpt-5.3-codex": {1.75, .175, 14},
}

// Standard, short-context USD list rates checked 2026-10-28:
// https://developers.openai.com/api/docs/pricing
// https://developers.openai.com/api/docs/models/gpt-6.1-sol
// Cache reads and reasoning are subsets of input and output respectively.
// Unknown models/write lifetimes are visibly unavailable, never Sonnet-priced.
func codexListPrice(model, tier string, u codexTokens) (float64, bool) {
	rates, ok := codexRateCards[model]
	if !ok || u.Write != 0 || !u.valid() {
		return 0, false
	}
	inputFactor, outputFactor := 1.0, 1.0
	if u.Input > 272000 {
		inputFactor, outputFactor = 2, 1.5
	}
	tierFactor := 1.0
	switch tier {
	case "", "default", "standard":
	case "fast", "priority":
		tierFactor = 2
	case "flex", "batch":
		tierFactor = .5
	case "ultrafast":
		tierFactor = 6
	default:
		return 0, false
	}
	return (float64(u.Input-u.Cached)*rates.input*inputFactor + float64(u.Cached)*rates.cached*inputFactor + float64(u.Output)*rates.output*outputFactor) / 1e6 * tierFactor, true
}
