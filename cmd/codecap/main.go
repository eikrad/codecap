// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/eikrad/codecap/internal/dbusapi"
	"github.com/godbus/dbus/v5"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// No flag package: the helper takes no options, and a subcommand that only
	// exists for inspecting the vendor payload should not turn the daemon's
	// entry point into a CLI. Bare `codecap` stays the helper, exactly as the
	// systemd unit and the D-Bus service file invoke it.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "dump-usage":
			if err := dumpUsage(ctx, os.Stdout, strings.Join(os.Args[2:], " ")); err != nil {
				log.Fatalf("dump-usage: %v", err)
			}
			return
		default:
			log.Fatalf("unknown subcommand %q (try: dump-usage <account-home>)", os.Args[1])
		}
	}

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
