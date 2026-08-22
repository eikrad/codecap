// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Dialogs
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import "logic.js" as Logic

// The FormLayout is the root item on purpose. It used to sit inside an Item
// that took its width from childrenRect while the FormLayout anchored itself to
// that same Item, which is a binding loop: Qt leaves the width unresolved, and
// a zero-width TextField cannot be focused or typed into.
Kirigami.FormLayout {
    id: page

    // The configuration loader reads and writes these by name. An alias is the
    // canonical form; `text: cfg_x` combined with `onTextChanged: cfg_x = text`
    // is a two-way write that fights the field's own input.
    property alias cfg_accountHome: accountHomeField.text
    property alias cfg_displayCurrency: displayCurrencyField.text

    RowLayout {
        Kirigami.FormData.label: i18n("Account Home:")
        Layout.fillWidth: true
        spacing: Kirigami.Units.smallSpacing

        QQC2.TextField {
            id: accountHomeField
            Layout.fillWidth: true
            placeholderText: i18n("~/.claude")
        }

        QQC2.Button {
            text: i18n("Browse…")
            icon.name: "folder-open-symbolic"
            onClicked: folderDialog.open()
        }
    }

    QQC2.TextField {
        id: displayCurrencyField
        Kirigami.FormData.label: i18n("Display Currency:")
        placeholderText: i18n("Locale default (for example DKK)")
    }

    // Not an Item, so it takes no part in the layout.
    FolderDialog {
        id: folderDialog
        title: i18n("Choose Account Home")
        // selectedFolder is a url value, and QML url values have no
        // toLocalFile(): calling it threw, the handler died, and choosing a
        // folder silently did nothing.
        onAccepted: accountHomeField.text = Logic.stripFileScheme(selectedFolder)
    }
}
