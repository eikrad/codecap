// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.plasma.components as PlasmaComponents
import "logic.js" as Logic

Item {
    id: root

    property string title
    property real usedPercent: 0
    property int resetsAt: 0
    property bool stale: false
    property int nowUnix: Math.floor(Date.now() / 1000)

    implicitHeight: column.implicitHeight

    readonly property color fillColor: Logic.allowanceFillColor(
        usedPercent,
        stale,
        {
            highlight: Kirigami.Theme.highlightColor,
            neutral: Kirigami.Theme.neutralTextColor,
            negative: Kirigami.Theme.negativeTextColor,
            disabled: Kirigami.Theme.disabledTextColor
        }
    )

    ColumnLayout {
        id: column
        width: parent.width
        spacing: Kirigami.Units.smallSpacing

        PlasmaComponents.Label {
            Layout.fillWidth: true
            text: root.title
        }

        Rectangle {
            Layout.fillWidth: true
            Layout.preferredHeight: Kirigami.Units.gridUnit / 2
            radius: Kirigami.Units.smallSpacing / 2
            color: Kirigami.Theme.disabledTextColor
            opacity: 0.35

            Rectangle {
                anchors.left: parent.left
                anchors.top: parent.top
                anchors.bottom: parent.bottom
                width: parent.width * Math.min(Math.max(root.usedPercent, 0), 100) / 100.0
                radius: parent.radius
                color: root.fillColor
            }
        }

        RowLayout {
            Layout.fillWidth: true
            spacing: Kirigami.Units.largeSpacing

            PlasmaComponents.Label {
                text: i18n("%1%", Math.round(root.usedPercent))
            }

            PlasmaComponents.Label {
                Layout.fillWidth: true
                horizontalAlignment: Text.AlignRight
                opacity: 0.8
                text: {
                    var resetText = Logic.formatTimeToReset(root.resetsAt, root.nowUnix)
                    if (resetText === "") {
                        return stale ? i18n("Last-Known") : ""
                    }
                    return stale ? i18n("Last-Known · %1", resetText) : resetText
                }
            }
        }
    }
}
