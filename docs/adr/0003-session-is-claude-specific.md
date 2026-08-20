# Session is Claude-specific; Allowance Window is generic

Claude’s user-facing “session limit” is a five-hour rolling window, not a conversation. Other Agents do not share that name or duration (Codex has rate-limit windows of vendor-defined length; Cursor is closer to longer included usage). We keep Session as Claude’s primary short window for the compact view, and introduce Allowance Window as the optional per-Agent shape (percentage, reset, duration). Conversation-level session lists are not a widget view. Daily, weekly, and monthly remain Consumed Usage for every Agent.
