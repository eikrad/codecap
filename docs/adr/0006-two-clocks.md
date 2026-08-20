# Two clocks: watch logs, poll Allowance calmly

Consumed Usage lives in Account Home files and can update when those files change. Allowance comes from a vendor endpoint that Claude Code itself rate-limits, falling back to last-known bars for up to an hour. We watch logs for Consumed Usage, fetch Allowance about once a minute, keep Last-Known Allowance on failure, and tick time-to-reset locally from `resetsAt` so the compact view stays alive without extra network calls.
