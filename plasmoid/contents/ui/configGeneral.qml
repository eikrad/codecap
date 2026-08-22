// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.kquickcontrolsaddons as KQuickControlsAddons

Kirigami.FormLayout {
    id: page

    property alias cfg_accountHome: accountHomeField.text
    property alias cfg_displayCurrency: displayCurrencyField.text

    RowLayout {
        Kirigami.FormData.label: i18n("Account Home:")
        Layout.fillWidth: true
        spacing: Kirigami.Units.smallSpacing

        QQC2.TextField {
            id: accountHomeField
            Layout.fillWidth: true
            placeholderText: "~/.claude"
        }

        QQC2.Button {
            text: i18n("Browse…")
            onClicked: folderDialog.open()
        }
    }

    QQC2.TextField {
        id: displayCurrencyField
        Kirigami.FormData.label: i18n("Display Currency:")
        placeholderText: i18n("Locale default (for example DKK)")
    }

    KQuickControlsAddons.FileDialog {
        id: folderDialog
        selectFolder: true
        title: i18n("Choose Account Home")
        onAccepted: accountHomeField.text = selectedFile.toLocalFile()
    }
}
