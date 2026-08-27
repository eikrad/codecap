// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/eikrad/codecap/internal/dbusapi"
	"github.com/godbus/dbus/v5"
)

// readyLine is what scripts/run-qml-dbus-tests.sh waits for. Polling the bus
// for the name instead would need a D-Bus client in the script; printing it
// from the process that owns the name says it without one, and says it at the
// moment it becomes true rather than up to a poll interval later.
const readyLine = "ready"

func main() {
	name := flag.String("name", dbusapi.ServiceName, "well-known bus name to own")
	flag.Parse()

	if err := run(*name); err != nil {
		fmt.Fprintf(os.Stderr, "codecap helper stub: %v\n", err)
		os.Exit(1)
	}
}

func run(name string) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect session bus: %w", err)
	}
	defer func() { _ = conn.Close() }()

	stub := NewStub(conn)
	if err := stub.Export(conn); err != nil {
		return err
	}

	// DoNotQueue, so owning the name is either immediate or an error. Queuing
	// behind a real helper would leave the harness talking to the real one.
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("request name %s: %w", name, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("%s is already owned on this bus", name)
	}

	// os.Stdout is unbuffered in Go, so this reaches the reader as it is
	// written — the script may be blocking on exactly this line.
	fmt.Println(readyLine)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	return nil
}
