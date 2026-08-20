# Store widgets without Plasma5Support

Question: can a Plasma 6 applet ship via Get New Widgets without Plasma5Support and without a custom C++ plugin?

**Split answer:** QML-only on the store is the intended model. HTTP with `XMLHttpRequest` works without Plasma5Support. Reading `Account Home` JSONL and spawning a helper have **no first-class store API**. KDE’s shaped alternative is a **QML plasmoid on the store** plus a **session helper** reached over D-Bus or localhost HTTP — not `engine: "executable"`.

## What the store actually installs

KPackage / Get Hot New Stuff installs **non-binary** add-ons (QML, JS, images). KDE: write plasmoids in QML with imports found on most systems so they install without compilation, including via Get Hot New Stuff. A custom C++ plugin means the plasmoid **cannot** go through GHNS and must use the distro packaging system.

- https://community.kde.org/Plasma/DeveloperGuide (KPackage, “When possible write a plasmoid using only QML…”, “Advanced: Plasmoids using C++”)
- https://develop.kde.org/docs/plasma/widget/c-api (“You cannot ship a widget that needs to be compiled on the KDE Store.”)

Pling hosting terms do not ban binaries; **kpackagetool / Get New Widgets will not load a custom `.so`**. Plasma 6 listings need `"X-Plasma-API-Minimum-Version": "6.0"` and `KPackageStructure: Plasma/Applet`.

## KDE’s intended data paths (not DataEngines)

David Edmundson (Plasma) on getting data into a plasmoid:

1. **In-built JavaScript `XMLHttpRequest`** — first option for HTTP/RSS-style data. Same AJAX model as the web. No C++, no DataEngine. There is **no QML `fetch()`**.
2. **C++ QML plugins** — for existing libraries, fast processing, hardware, **file I/O**. Maximum power; **distribution is harder because it must be compiled**.
3. **DataEngines** — Plasma 4-era abstraction. Do not write new ones. Marco Martin: plasma5support is available for a time, **plan to drop it**.

- https://blog.davidedmundson.co.uk/blog/plasmoid-tutorial-2-getting-data/
- https://doc.qt.io/qt-6/qml-qtqml-xmlhttprequest.html
- https://develop.kde.org/docs/plasma/widget/c-api
- https://notmart.org/blog/2023/07/akademy-2023-plasma-6-and-plasmoids/

Qt `XMLHttpRequest` does **not** enforce same-origin, so `https://` from a plasmoid works. **`file://` is off** unless `QML_XHR_ALLOW_FILE_READ=1`. Plasmashell does not set that (weather-widget-2, Plasma 6). A store widget must not depend on it.

`org.kde.kio` is not a Plasma 6 store HTTP/file-read module (KF5 leftover). `Qt.labs.folderlistmodel` can **list** `~/.claude` names, not read JSONL bytes. Qt 6 QML has **no Process type**.

## Plasma5Support

DataEngine support is **not** in KF6 Plasma. Remaining engines live in Plasma5Support “in the meantime”; they should migrate to QML imports.

- README: https://invent.kde.org/plasma/plasma5support/-/blob/master/README.md
- Plasma 6.5/6.6: demote Plasma5Support to a **runtime** dependency (kdeplasma-addons no longer REQUIRED-link it).
- Dec 2025: `executable` dataengine **moved** from plasma-workspace into plasma5support (`KProcess` + shell command = source name).
- No official QML replacement for “run command, get stdout” (discuss.kde.org, unanswered). Executable-from-GHNS is also a known file-exfil vector (Bug 480112).

## How 2026 store widgets actually behave

- **Weather:** QML + `XMLHttpRequest` + API keys in `plasmoid.configuration`. No C++, often no Plasma5Support.
- **Command-output (Zren):** `Plasma5Support.DataSource { engine: "executable" }`. Ecosystem pattern, not KDE’s future.
- **In-tree monitors:** C++ / private imports — not a store template. Do not import `org.kde.plasma.private.*` from a store applet.

## D-Bus / localhost: store UI + helper, no Plasma5Support

Plasma Workspace exposes **`org.kde.plasma.workspace.dbus`**: session-bus calls from QML without spawning a process (used in-tree; third-party notes 2025). XHR to `http://127.0.0.1` also works (file-XHR workaround used by wallpaper authors).

That is the architecture that is **both store-legal and not Plasma5Support**:

```
Get New Widgets KPackage (QML only)
  → XMLHttpRequest for public HTTPS, and/or
  → org.kde.plasma.workspace.dbus / XHR to 127.0.0.1
       → session helper (separate install: AUR/pip/systemd --user)
            reads Account Home, holds login, returns Allowance + Consumed Usage
```

The helper is **not** inside the `.plasmoid` zip. The store artifact stays QML-only.

## Is this widget unusual?

**HTTP dashboards are not unusual.** **Private JSONL + `/login` + spawning a CLI from the zip** is unusual relative to KDE’s architecture, common only in third-party executable-engine widgets.

## Implication

| Job | Store + no P5S + no custom C++ |
|---|---|
| Plasmoid on Get New Widgets | Yes — QML KPackage |
| Session / Weekly Allowance over HTTPS | Yes — `XMLHttpRequest` (tokens then live in plasmashell JS) |
| Consumed Usage from local logs | No first-class API; yes if a **session helper** owns the files |
| Keep tokens out of plasmashell | Session helper (D-Bus or localhost), not a helper exec’d from the zip |
| Run helper via executable DataSource | Works today; not KDE’s intended future |

KDE did not forget a Process API for store widgets. Scripted GHNS applets stay in QML. Native file I/O belongs in a compiled plugin **or** an already-running session service the QML talks to.
