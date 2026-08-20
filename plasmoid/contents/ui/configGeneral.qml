// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtQuick.Controls as QQC2
import org.kde.kirigami as Kirigami

Kirigami.FormLayout {
    id: page

    property alias cfg_accountHome: accountHomeField.text

    QQC2.TextField {
        id: accountHomeField
        Kirigami.FormData.label: i18n("Account Home:")
        placeholderText: "~/.claude"
    }
}
