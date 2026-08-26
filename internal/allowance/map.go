// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/eikrad/codecap/internal/snapshot"
)

// Result is vendor Allowance mapped into snapshot fields.
type Result struct {
	Session          snapshot.AllowanceWindow
	Weekly           snapshot.AllowanceWindow
	UsageCredit      string
	UsageCreditSpend snapshot.UsageCreditSpend
}

type usageBucket struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *string  `json:"resets_at"`
}

type extraUsage struct {
	IsEnabled    *bool    `json:"is_enabled"`
	MonthlyLimit *float64 `json:"monthly_limit"`
	UsedCredits  *float64 `json:"used_credits"`
	Utilization  *float64 `json:"utilization"`
}

type usagePayload struct {
	FiveHour   *usageBucket `json:"five_hour"`
	SevenDay   *usageBucket `json:"seven_day"`
	ExtraUsage *extraUsage  `json:"extra_usage"`
}

// FromUsagePayload maps Claude Code's OAuth /api/oauth/usage JSON into Allowance windows.
func FromUsagePayload(data []byte) (Result, error) {
	var payload usagePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return Result{}, fmt.Errorf("decode usage payload: %w", err)
	}

	return Result{
		Session:          mapBucket(payload.FiveHour),
		Weekly:           mapBucket(payload.SevenDay),
		UsageCredit:      mapUsageCredit(payload.ExtraUsage),
		UsageCreditSpend: mapUsageCreditSpend(payload.ExtraUsage),
	}, nil
}

// mapUsageCreditSpend keeps the amounts the status is derived from. The widget
// showed "Usage credit exhausted" where the vendor shows "$4.04 of $4.00" — the
// same fact, but the word alone cannot say how much is left or how far over the
// ceiling the Account is.
//
// Disabled credit reports zeroes rather than the vendor's ceiling: a limit that
// is not in force is not a limit, and printing one would suggest headroom that
// does not exist.
func mapUsageCreditSpend(extra *extraUsage) snapshot.UsageCreditSpend {
	if extra == nil || extra.IsEnabled == nil || !*extra.IsEnabled {
		return snapshot.UsageCreditSpend{}
	}
	spend := snapshot.UsageCreditSpend{}
	if extra.UsedCredits != nil {
		spend.UsedUSD = *extra.UsedCredits
	}
	if extra.MonthlyLimit != nil {
		spend.LimitUSD = *extra.MonthlyLimit
	}
	return spend
}

func mapBucket(bucket *usageBucket) snapshot.AllowanceWindow {
	if bucket == nil {
		return snapshot.AllowanceWindow{}
	}

	window := snapshot.AllowanceWindow{}
	if bucket.Utilization != nil {
		window.UsedPercent = *bucket.Utilization
	}
	if bucket.ResetsAt != nil && *bucket.ResetsAt != "" {
		if ts, err := time.Parse(time.RFC3339Nano, *bucket.ResetsAt); err == nil {
			window.ResetsAt = ts.Unix()
		} else if ts, err := time.Parse(time.RFC3339, *bucket.ResetsAt); err == nil {
			window.ResetsAt = ts.Unix()
		}
	}
	return window
}

func mapUsageCredit(extra *extraUsage) string {
	if extra == nil || extra.IsEnabled == nil || !*extra.IsEnabled {
		return "none"
	}

	utilization := 0.0
	if extra.Utilization != nil {
		utilization = *extra.Utilization
	} else if extra.MonthlyLimit != nil && *extra.MonthlyLimit > 0 && extra.UsedCredits != nil {
		utilization = (*extra.UsedCredits / *extra.MonthlyLimit) * 100.0
	}

	switch {
	case utilization >= 100.0:
		return "exhausted"
	case utilization <= 0.0:
		return "available"
	default:
		return "enabled"
	}
}
