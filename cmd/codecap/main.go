// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/eikrad/codecap/internal/dbusapi"
	"github.com/godbus/dbus/v5"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Fatalf("connect session bus: %v", err)
	}
	defer func() { _ = conn.Close() }()

	reply, err := conn.RequestName(dbusapi.ServiceName, dbus.NameFlagDoNotQueue)
	if err != nil {
		log.Fatalf("request service name: %v", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		// Another helper already owns the name and is serving the session.
		// Exiting non-zero here made systemd's Restart=on-failure spin.
		log.Printf("service name already owned, nothing to do: %s", dbusapi.ServiceName)
		os.Exit(0)
	}

	server, err := dbusapi.Export(ctx, conn)
	if err != nil {
		log.Fatalf("export dbus api: %v", err)
	}

	log.Printf("codecap helper running on %s", dbusapi.ServiceName)
	<-ctx.Done()

	log.Printf("codecap helper shutting down")
	if err := server.Close(); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
