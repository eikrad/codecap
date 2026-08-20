# Dual sources: Consumed Usage and Allowance

A glance at the widget must answer both “what have I used?” and “how much plan is left?”. Local logs cannot see other devices or claude.ai; Anthropic Admin APIs cannot serve individual Pro/Max accounts. We keep both: Consumed Usage from local agent logs, Allowance from the vendor’s remaining-entitlement signal (Claude Code `rate_limits` / `/usage` for Pro and Max).
