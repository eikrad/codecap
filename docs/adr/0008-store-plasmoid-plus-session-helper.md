# Store plasmoid plus session helper

Get New Widgets only installs QML KPackages. Session Allowance and local JSONL cannot be done with a documented store-only API without either Plasma5Support (compat, to be dropped) or putting `/login` in plasmashell via XMLHttpRequest. We ship two artifacts: a QML plasmoid on the store, and a session helper (D-Bus or localhost HTTP) that owns Account Home, tokens, and log watch. The helper is not in the zip. Missing helper → Unknown Allowance, not a token fallback in QML.
