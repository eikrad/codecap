// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Dialogs
import QtQuick.Layouts
import org.kde.kirigami as Kirigami

RowLayout {
    id: root

    property alias markerEnabled: markerCheckBox.checked
    // Empty is the defined preference for the desktop accent. Store a string
    // so KConfig and ColorDialog use the same representation across reloads.
    property string markerColor: ""
    readonly property color effectiveColor: markerColor !== "" ? markerColor : Kirigami.Theme.highlightColor

    spacing: Kirigami.Units.smallSpacing

    QQC2.CheckBox {
        id: markerCheckBox
        text: i18n("Show color marker")
    }

    WidgetMarker {
        objectName: "markerPreview"
        color: root.effectiveColor
        opacity: root.markerEnabled ? 1.0 : 0.4
    }

    QQC2.Button {
        objectName: "markerColorButton"
        text: i18n("Choose color…")
        enabled: root.markerEnabled
        onClicked: {
            colorDialog.selectedColor = root.effectiveColor
            colorDialog.open()
        }
    }

    ColorDialog {
        id: colorDialog
        objectName: "markerColorDialog"
        title: i18n("Choose widget marker color")
        // Keep the dialog's selection separate until accepted, so cancelling
        // cannot change a saved color through a live two-way binding.
        onAccepted: root.markerColor = String(selectedColor)
    }
}
