# Agent Usage

A Plasma 6 applet that shows coding-agent consumption and remaining subscription entitlement. One widget instance is bound to one Account. The same applet lives on the panel, the desktop, and the system tray.

Domain frozen 2026-08-19. Change terms only with an explicit glossary revision, not as a side effect of implementation.

## Language

### Identity

**Account**:
One authenticated identity at one vendor. Each widget instance is bound to exactly one Account Home, and therefore to exactly one Account. Allowance comes from that Account; Consumed Usage comes from the local logs that belong to it.
_Avoid_: login, profile, credential (a credential authenticates an Account; it is not the Account)

**Account Home**:
The local directory that belongs to one Account on this machine. It holds that Account's credentials and logs. A widget instance points at one Account Home; it does not create a second login.
_Avoid_: config dir, profile path, install prefix

**Agent**:
A coding product that can produce Consumed Usage and, for some products, Allowance. An Account always belongs to one Agent. v1 implements Claude only.
_Avoid_: model (a model is Sonnet/Opus, not the product), source, provider (when meaning the coding tool)

### Consumption and entitlement

**Consumed Usage**:
Tokens and List Price already incurred, read from local coding-agent logs. Aggregated as the current Session, then today, this week, and this month, in the desktop's local timezone, with week start from the desktop locale. Expanded view shows List Price in Display Currency, tokens second. Not aggregated by conversation.
_Avoid_: usage (unqualified), spend, cost (as the primary name)

**List Price**:
Estimated API cost in USD at the vendor's published token rates. Headline for expanded Consumed Usage. Not the subscription bill and not regional Pro/Max checkout prices.
_Avoid_: cost, bill, spend, invoice

**Display Currency**:
The currency a widget instance uses to show List Price, via exchange-rate conversion from USD. Default is the desktop locale. Does not change token accounting.
_Avoid_: local price, regional tariff, PPP

**Allowance**:
The remaining subscription entitlement, as reported by the vendor. Distinct from Consumed Usage: logs say what happened locally; Allowance says how much of the plan is left, including activity on other devices and claude.ai. Optional: an Agent may have Consumed Usage and no Allowance.
_Avoid_: quota, rate limit, remaining usage

**Last-Known Allowance**:
The most recently fetched Allowance, shown when a fresh vendor fetch fails. Staleness is visible. Time-to-reset still counts down locally from the last reset timestamp.
_Avoid_: cache, snapshot (as the primary name)

**Allowance Window**:
An Agent-defined rolling period of Allowance (percentage used, reset time, duration). Optional. Claude's primary short window is Session; Claude's longer window is Weekly Allowance. Other Agents may define different windows or none.
_Avoid_: session (when meaning a generic window), block, billing block

**Session**:
Claude's primary short Allowance Window, lasting five hours. The compact face shows percent used and time to reset (Last-Known marked stale; Usage Credit status if Session is exhausted). Session Consumed Usage is local consumption inside the current Session, shown only in the expanded view. List Price, tokens, and Weekly Allowance are not on the compact face.
_Avoid_: conversation, chat, thread, Five-Hour Allowance

**Weekly Allowance**:
Claude's seven-day Allowance Window. In the expanded view it sits under Session Allowance, above Usage Credit and Consumed Usage.
_Avoid_: weekly (unqualified), weekly limit, Seven-Day Allowance

**Usage Credit**:
Optional paid overage after Allowance is exhausted. A monthly ceiling in money that the Account holder sets: once any Allowance Window is exhausted, work continues against Usage Credit until the ceiling is reached. Shown on the widget as amount spent of ceiling, on one status line — still not a third bar. Amounts stay in USD as the vendor reports them; Display Currency is scoped to List Price.
_Avoid_: extra usage, overage (as the primary name)

> **Glossary revision 2026-08-26.** Was: "A status on the widget (enabled / available / exhausted), not a third bar." The status word was the whole of it, and it could not say how much was left — the widget said "Usage credit exhausted" where the vendor said "$4.04 of $4.00". Amounts were already fetched and thrown away in `mapUsageCredit`. The enabled / available / exhausted status is kept internally, because a ceiling of 0 with credit enabled and one with credit disabled have identical amounts, and only the status separates them.

### Faces

**Unbound**:
A widget instance with no Account Home chosen. Compact and expanded ask to pick a directory. Not shown as 0% Session.
_Avoid_: unconfigured, empty

**Signed Out**:
An Account Home exists but has no usable login. Compact and expanded ask to sign in with that Agent for that Account Home. Not shown as 0% Session.
_Avoid_: logged out, unauthenticated (as the primary name)

**Unknown Allowance**:
The Account is signed in, but there is no Allowance and no Last-Known Allowance yet. Compact shows an em dash; Consumed Usage from logs can still update. Not shown as 0% Session.
_Avoid_: error, loading (when meaning this face)
