// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package snapshot

// SchemaVersion is the shape of the JSON GetSnapshot returns.
//
// Helper and plasmoid are installed separately and versioned separately, so a
// field rename between them is otherwise a silent breakage.
const SchemaVersion = 1

// Degraded reasons say why a snapshot is incomplete. They are stable codes, not
// messages: the plasmoid owns the wording, and an error string could carry a
// filesystem path onto the bus.
const (
	// DegradedPending means nothing is cached yet and a refresh is running.
	DegradedPending = "pending"
	// DegradedUsage means the local logs could not be read.
	DegradedUsage = "usage_unavailable"
	// DegradedAllowance means the vendor Allowance could not be resolved.
	DegradedAllowance = "allowance_unavailable"
)

type Face string

const (
	FaceUnbound          Face = "unbound"
	FaceSignedOut        Face = "signed_out"
	FaceUnknownAllowance Face = "unknown_allowance"
	FaceReady            Face = "ready"
)

type AllowanceWindow struct {
	UsedPercent float64 `json:"used_percent"`
	ResetsAt    int64   `json:"resets_at"`
	Stale       bool    `json:"stale"`
}

type ConsumedPeriod struct {
	ListPriceUSD float64 `json:"list_price_usd"`
	Tokens       int64   `json:"tokens"`
}

type ConsumedUsage struct {
	Session ConsumedPeriod `json:"session"`
	Today   ConsumedPeriod `json:"today"`
	Week    ConsumedPeriod `json:"week"`
	Month   ConsumedPeriod `json:"month"`
}

type Snapshot struct {
	SchemaVersion    int             `json:"schema_version"`
	Face             Face            `json:"face"`
	AccountHome      string          `json:"account_home"`
	AccountLabel     string          `json:"account_label"`
	SessionAllowance AllowanceWindow `json:"session_allowance"`
	WeeklyAllowance  AllowanceWindow `json:"weekly_allowance"`
	UsageCredit      string          `json:"usage_credit"`
	ConsumedUsage    ConsumedUsage   `json:"consumed_usage"`
	FetchedAt        int64           `json:"fetched_at"`
	Degraded         []string        `json:"degraded,omitempty"`
}
