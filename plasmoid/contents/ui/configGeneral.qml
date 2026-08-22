// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Dialogs
import QtQuick.Layouts
import org.kde.kirigami as Kirigami

Item {
    id: page
    width: childrenRect.width
    height: childrenRect.height

    property string cfg_accountHome: ""
    property string cfg_displayCurrency: ""

    Kirigami.FormLayout {
        anchors.left: parent.left
        anchors.right: parent.right

        RowLayout {
            Kirigami.FormData.label: i18n("Account Home:")
            Layout.fillWidth: true
            spacing: Kirigami.Units.smallSpacing

            QQC2.TextField {
                id: accountHomeField
                Layout.fillWidth: true
                placeholderText: i18n("Full path, for example /home/you/.claude")
                text: page.cfg_accountHome
                onTextChanged: page.cfg_accountHome = text
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
            text: page.cfg_displayCurrency
            onTextChanged: page.cfg_displayCurrency = text
        }
    }

    FolderDialog {
        id: folderDialog
        title: i18n("Choose Account Home")
        onAccepted: page.cfg_accountHome = selectedFolder.toLocalFile()
    }
}
