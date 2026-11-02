# Optional per-widget color marker

Decision: 2026-11-02, explicitly requested and selected by the user. This is a
visual revision to the frozen v1 configuration scope, recorded in design.md.

Add a small, optional colored dot at the upper-right edge of the compact Face.
Repeat the same dot by the expanded Account label, including on the desktop.
Each widget independently stores `widgetMarkerEnabled` (false by default) and
`widgetMarkerColor` (an empty string means the desktop accent). The standard
color dialog chooses a custom color. Disabling the marker retains its color.

The marker identifies the widget instance. Session percentage, Allowance ring
colors, stale presentation, sizing and click-to-expand keep their existing
meaning and behavior. The marker also remains visible on empty Faces so an
Account Home needing attention can still be distinguished. Existing Account
labels in the popup/tooltip provide the textual identity. No Agent logo or
short name is added to the panel. A desktop-specific short name is outside
this change.

Configuration uses Qt Quick Controls, the existing Qt Quick Dialogs import and
Kirigami spacing/theme colors. Custom colors come from the user's choice;
the implementation adds no hard-coded palette or bundled vendor assets.
The setting is local to the plasmoid and never enters the helper or snapshot.

Verification: instantiate the applet's real CompactRing and marker settings
with qmltestrunner on a Plasma-capable machine. Assert disabled defaults,
independent colors, retained Allowance colors, compact-size geometry, and
accepted/cancelled color selection. Platform identifiers are checked against
the installed Qt .qmltypes and in-tree KDE ColorButton implementation.
The final configuration-loader and panel rendering still require a desktop
check after installing the updated plasmoid: configure two colors, disable
and re-enable one marker, and verify both widgets retain distinct markers.

Installed-desktop follow-up, 2026-11-02: the controls were visible but Apply stayed
disabled because plasmashell had been running since 2026-10-30 and retained the
old configuration schema. The installed AppletConfiguration.qml observes and
saves only keys from Plasmoid.configuration.keys(); newly installed QML alone
does not register new keys in an already-running applet. Reload plasmashell once
after a schema upgrade. Both installers now explain this requirement explicitly.
The additional ConfigurationPage suite executes the actual configGeneral.qml,
checks its cfg_*Changed signals as Plasma observes them, and checks restoration
of saved marker values. It cannot reproduce an existing desktop applet's cached
schema; desktop persistence remains the acceptance check for that lifecycle.

Desktop acceptance, 2026-11-02: after reloading plasmashell, the user confirmed
that enabling the marker, choosing a color, saving and reopening the settings
retains both values. The user also accepted the marker's small, simple appearance;
its size is intentionally unchanged.
