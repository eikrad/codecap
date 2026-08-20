# QML plasmoid plus helper for Allowance

Session Allowance is not in local logs, not in the org Admin API, and not in `claude setup-token`. Claude Code only exposes `rate_limits` inside a running session. We split the applet: QML renders; a helper is the only process that uses the Account Home `/login` and returns Allowance. QML never reads credentials, keeping secrets out of plasmashell and out of plaintext plasmoid config.
