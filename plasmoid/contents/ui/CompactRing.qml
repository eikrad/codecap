// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

import QtQuick
import org.kde.kirigami as Kirigami
import "logic.js" as Logic

Item {
    id: root

    property string face: "unbound"
    property bool helperAvailable: false
    property bool isBound: false
    property real usedPercent: 0
    property bool stale: false
    property string usageCredit: "none"
    property bool showNumeral: true

    implicitWidth: Kirigami.Units.gridUnit * 2
    implicitHeight: implicitWidth

    readonly property bool showRing: face === "ready" && helperAvailable && isBound
    readonly property string iconName: Logic.compactIcon(face, helperAvailable, isBound)

    readonly property color ringColor: Logic.allowanceFillColor(
        usedPercent,
        stale,
        usageCredit,
        {
            highlight: Kirigami.Theme.highlightColor,
            neutral: Kirigami.Theme.neutralTextColor,
            negative: Kirigami.Theme.negativeTextColor,
            disabled: Kirigami.Theme.disabledTextColor
        }
    )

    Kirigami.Icon {
        anchors.centerIn: parent
        visible: !root.showRing && iconName !== ""
        width: parent.width * 0.75
        height: width
        source: iconName
        opacity: stale ? 0.65 : 1.0
    }

    Canvas {
        id: canvas
        anchors.fill: parent
        visible: root.showRing
        property real percent: root.usedPercent

        onPaint: {
            var ctx = getContext("2d")
            ctx.reset()
            var cx = width / 2
            var cy = height / 2
            var lineWidth = Math.max(2, width * 0.08)
            var radius = Math.min(width, height) / 2 - lineWidth

            ctx.lineWidth = lineWidth
            ctx.lineCap = "round"
            ctx.strokeStyle = Qt.rgba(
                Kirigami.Theme.disabledTextColor.r,
                Kirigami.Theme.disabledTextColor.g,
                Kirigami.Theme.disabledTextColor.b,
                0.35
            )
            ctx.beginPath()
            ctx.arc(cx, cy, radius, 0, 2 * Math.PI)
            ctx.stroke()

            ctx.strokeStyle = root.ringColor
            ctx.beginPath()
            var start = -Math.PI / 2
            var sweep = (Math.min(Math.max(percent, 0), 100) / 100.0) * 2 * Math.PI
            ctx.arc(cx, cy, radius, start, start + sweep)
            ctx.stroke()
        }

        onPercentChanged: requestPaint()
        onVisibleChanged: if (visible) requestPaint()
        Connections {
            target: root
            function onRingColorChanged() { canvas.requestPaint() }
        }
        Component.onCompleted: requestPaint()
    }

    Kirigami.Heading {
        anchors.centerIn: parent
        visible: root.showRing && root.showNumeral
        level: 5
        opacity: stale ? 0.65 : 1.0
        text: i18n("%1%", Math.round(root.usedPercent))
    }
}
