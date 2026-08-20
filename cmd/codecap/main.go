// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"log"

	"github.com/eikrad/codecap/internal/dbusapi"
	"github.com/godbus/dbus/v5"
)

func main() {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Fatalf("connect session bus: %v", err)
	}
	defer conn.Close()

	reply, err := conn.RequestName(dbusapi.ServiceName, dbus.NameFlagDoNotQueue)
	if err != nil {
		log.Fatalf("request service name: %v", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		log.Fatalf("service name already in use: %s", dbusapi.ServiceName)
	}

	if err := dbusapi.Export(conn); err != nil {
		log.Fatalf("export dbus api: %v", err)
	}

	log.Printf("codecap helper running on %s", dbusapi.ServiceName)
	select {}
}
