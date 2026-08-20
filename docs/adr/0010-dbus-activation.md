# Helper is D-Bus activated, not exec’d by the plasmoid

The store KPackage cannot contain the helper and must not spawn processes. The helper package installs a session-bus `.service` so the first `org.kde.plasma.workspace.dbus` call starts the process. Several widget instances share that process. Missing package → Unknown Allowance. No download-or-exec from QML.
