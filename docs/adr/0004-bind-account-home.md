# Bind each widget to an existing Account Home

Multiple widget instances need distinct Accounts without a second authentication system. Claude Code already stores credentials and logs in a per-Account directory. Each plasmoid instance stores only a path to that Account Home. Secrets stay where the CLI put them — Plasma’s per-widget config is plaintext and must not hold tokens or API keys.
