// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package dbusapi

import (
	"encoding/json"
	"fmt"

	"github.com/eikrad/codecap/internal/face"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const (
	ServiceName   = "dev.codecap.Helper"
	InterfaceName = "dev.codecap.Helper"
	ObjectPath    = dbus.ObjectPath("/dev/codecap/Helper")
)

type Server struct{}

func (s *Server) GetSnapshot(accountHome, timezone string, weekStart int32) (string, *dbus.Error) {
	_ = timezone
	_ = weekStart

	snap := face.Classify(accountHome)
	data, err := json.Marshal(snap)
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("marshal snapshot: %w", err))
	}
	return string(data), nil
}

func Export(conn *dbus.Conn) error {
	server := &Server{}
	if err := conn.Export(server, ObjectPath, InterfaceName); err != nil {
		return fmt.Errorf("export helper interface: %w", err)
	}

	node := &introspect.Node{
		Name: string(ObjectPath),
		Interfaces: []introspect.Interface{
			{
				Name: InterfaceName,
				Methods: []introspect.Method{
					{
						Name: "GetSnapshot",
						Args: []introspect.Arg{
							{Name: "accountHome", Type: "s", Direction: "in"},
							{Name: "timezone", Type: "s", Direction: "in"},
							{Name: "weekStart", Type: "i", Direction: "in"},
							{Name: "snapshotJSON", Type: "s", Direction: "out"},
						},
					},
				},
				Signals: []introspect.Signal{
					{
						Name: "Changed",
						Args: []introspect.Arg{
							{Name: "accountHome", Type: "s"},
						},
					},
				},
			},
			introspect.IntrospectData,
		},
	}

	if err := conn.Export(introspect.NewIntrospectable(node), ObjectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("export introspection: %w", err)
	}
	return nil
}
