# Codex Account Home and the existing Session/Weekly presentation

Decision: 2026-10-28. Supersedes the Claude-only scope of ADR 0002 and extends
ADR 0003 following the user's explicit request to transfer Codex's Session and
Weekly information into the existing widget. Glossary and visual revisions are
recorded explicitly in CONTEXT.md and design.md.

One widget still binds one Account Home. Identify Codex by `auth.json` or its
`sessions` / `archived_sessions` log layout, rather than the directory basename;
Claude retains `.credentials.json` / `projects`. Ambiguous credential layouts
are errors. No additional configuration field or D-Bus method is needed.

The helper reads Codex Consumed Usage from local JSONL token_count events.
Cumulative totals become deltas; repeated totals add nothing. Cached input and
reasoning tokens are subsets, not extra consumption. Copies in forks/archives
are deduplicated. Parse historical counters before applying period boundaries.
Session Consumed Usage remains the existing rolling five-hour aggregation.

For live Allowance the helper invokes the installed `codex app-server` over
stdio, with `CODEX_HOME` set to the validated Account Home. Only initialize,
initialized, account/read (refreshToken=false), and account/rateLimits/read are
sent. No thread, prompt, login, or forced refresh is requested. The child has a
15-second deadline, bounded output and guaranteed termination/reaping. Codex
owns its authentication, including keyring access. The helper rejects unsafe
filesystem auth stores before invoking it; actual Codex credential I/O remains
owned by the installed CLI. Its upstream credential-open race behaviour is
not verified by this repository. Raw protocol errors/stderr never reach logs
or the bus. Missing CLI/network uses the existing visible Last-Known fallback;
no known data means Unknown Allowance. Explicit signed-out data is Signed Out.

Prefer rateLimitsByLimitId.codex when provided. Only a complete 300-minute
primary and 10080-minute secondary map to Session/Weekly. Missing percentages,
missing windows and different durations are unavailable, never plausible zero
bars. Codex credits are not Claude Usage Credit and are not mapped to it.

List Price uses explicit OpenAI USD model cards. Logs without a service tier
are estimated at Standard list rates, not subscription bill amounts. Unknown
models, unrecognised tiers and unpriceable cache writes retain tokens and mark
the whole affected period's List Price unavailable. Snapshot schema 2 adds
`list_price_unavailable` to every Consumed Usage period. Upgrade helper and
plasmoid together; old plasmoids do not understand unavailable pricing. New
plasmoids still accept schema-1 snapshots from old helpers.

Verification: real Claude and Codex log excerpts in internal/usage/testdata;
real app-server Allowance response in internal/codex/testdata, captured with
Codex 0.153.4 on 2026-10-28. Parser, process-deadline, service fallback, file
watch and GetSnapshot integration tests execute these claims. Existing QML
tests cover the unchanged bus path and price-unavailability propagation.
The installed applet still needs a human desktop check: bind ~/.codex, open
the popup, compare both Allowance windows against Codex, send a turn and watch
Consumed Usage update. Repeat for a Claude Account Home after upgrading.

Refinement after the requested reliability refactor, 2026-10-28:

- `accounthome.OpenCredentials` owns the helper's credential-opening checks for
  both Agents. Face classification reuses the protected Claude loader; it no
  longer has its own unbounded file read or package-level filesystem stubs.
  This does not change the upstream Codex credential-I/O limitation above.
- Parsers emit opaque identities and explicitly mark replaceable request totals.
  One stable aggregation pass selects each request's final totals; identity text
  is never interpreted by the summation code. Cache refresh and aggregation share
  one lock so retention pruning cannot modify slices being read concurrently.
- Missing/inconsistent Codex counters, oversized/unreadable logs, failed walks
  and file-count overflow return errors instead of partial successful sums.
  GetSnapshot retains previously known Consumed Usage with `usage_unavailable`.
  Failed reads have a bounded retry cadence and emit a change on the transition
  into failure, avoiding a Changed/GetSnapshot retry loop.
- One Allowance error policy handles both Agents. Serving usable Last-Known
  Allowance still returns the original vendor error so the existing poller can
  back off. Regression tests exercise these paths through the affected seams.

Installed-desktop follow-up, 2026-11-02: a real Claude `attachment` record is
5,353,352 bytes and exceeds the old 4 MiB record limit. Raise the bounded record
size to 64 MiB while retaining the 512 MiB file limit and visible error policy.
A metadata-only excerpt plus synthetic padding reproduces the real failure
without retaining attachment content; an additional test executes the new bound.

Sources checked 2026-10-28:

- https://learn.chatgpt.com/docs/app-server (protocol and Allowance fields)
- https://developers.openai.com/api/docs/pricing (USD list rates)
- https://developers.openai.com/api/docs/models/gpt-6.1-sol (cached tokens,
  long-context and service-tier pricing)
- `codex app-server generate-json-schema` from installed Codex 0.153.4
