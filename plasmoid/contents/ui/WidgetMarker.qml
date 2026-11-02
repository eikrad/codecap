// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import org.kde.kirigami as Kirigami

// The same identification mark appears in the compact Face and by the
// expanded Account label. Its border separates custom colors from the theme.
Rectangle {
    implicitWidth: Kirigami.Units.gridUnit * 0.4
    implicitHeight: implicitWidth
    radius: width / 2
    color: Kirigami.Theme.highlightColor
    border.width: Math.max(1, width * 0.15)
    border.color: Kirigami.Theme.backgroundColor
}
