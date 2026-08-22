// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package dbusapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eikrad/codecap/internal/snapshot"
	"github.com/godbus/dbus/v5"
)

// These exercise the real bus surface: the exported name, the object path, the
// argument signature and the Changed signal. Everything else in this package
// calls GetSnapshot as a Go method, which cannot catch a wrong path, a
// mismatched signature, or a signal the plasmoid subscribes to but never sees.
//
// Run under `dbus-run-session`; `make test` does that when it is available.
func sessionBus(t *testing.T) *dbus.Conn {
	t.Helper()
	// Skipping locally is a convenience; skipping in CI would make this one of
	// the tests that quietly never runs, which is the failure mode the whole
	// branch has been chasing.
	requireBus := os.Getenv("CI") != ""

	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		if requireBus {
			t.Fatal("no session bus in CI; the test step must run under dbus-run-session")
		}
		t.Skip("no session bus; run under dbus-run-session")
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		if requireBus {
			t.Fatalf("session bus unreachable in CI: %v", err)
		}
		t.Skipf("session bus unreachable: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func exportForTest(t *testing.T) (*dbus.Conn, *Server) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	conn := sessionBus(t)
	reply, err := conn.RequestName(ServiceName, dbus.NameFlagDoNotQueue)
	if err != nil {
		t.Fatalf("request name: %v", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		t.Skipf("%s is already owned on this bus", ServiceName)
	}

	server, err := Export(context.Background(), conn)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return conn, server
}

func TestGetSnapshotIsCallableOverTheBus(t *testing.T) {
	_, _ = exportForTest(t)

	accountHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(accountHome, "projects"), 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}

	client := sessionBus(t)
	object := client.Object(ServiceName, ObjectPath)

	// These are exactly the arguments the plasmoid sends: the Account Home, an
	// empty timezone meaning "the helper's own zone", and the locale week start.
	var payload string
	call := object.Call(InterfaceName+".GetSnapshot", 0, accountHome, "", int32(1))
	if call.Err != nil {
		t.Fatalf("GetSnapshot over the bus: %v", call.Err)
	}
	if err := call.Store(&payload); err != nil {
		t.Fatalf("store reply: %v", err)
	}

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if snap.SchemaVersion != snapshot.SchemaVersion {
		t.Fatalf("schema_version %d, want %d", snap.SchemaVersion, snapshot.SchemaVersion)
	}
	if snap.AccountHome != accountHome {
		t.Fatalf("account_home %q, want %q", snap.AccountHome, accountHome)
	}
}

func TestIntrospectionOverTheBusDeclaresWhatCallersMustSend(t *testing.T) {
	_, _ = exportForTest(t)

	client := sessionBus(t)
	var xml string
	call := client.Object(ServiceName, ObjectPath).Call("org.freedesktop.DBus.Introspectable.Introspect", 0)
	if call.Err != nil {
		t.Fatalf("Introspect: %v", call.Err)
	}
	if err := call.Store(&xml); err != nil {
		t.Fatalf("store introspection: %v", err)
	}

	// The plasmoid's D-Bus binding derives its argument signature from this
	// document. If it stops describing three in-arguments of s, s and i, the
	// call is encoded wrongly and fails at runtime with nothing to read.
	for _, want := range []string{
		`name="GetSnapshot"`,
		`name="accountHome" type="s" direction="in"`,
		`name="timezone" type="s" direction="in"`,
		`name="weekStart" type="i" direction="in"`,
		`name="snapshotJSON" type="s" direction="out"`,
		`name="Changed"`,
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("introspection is missing %s\n---\n%s", want, xml)
		}
	}
}

func TestChangedSignalReachesASubscriber(t *testing.T) {
	_, server := exportForTest(t)

	client := sessionBus(t)
	if err := client.AddMatchSignal(
		dbus.WithMatchObjectPath(ObjectPath),
		dbus.WithMatchInterface(InterfaceName),
		dbus.WithMatchMember("Changed"),
	); err != nil {
		t.Fatalf("add match: %v", err)
	}

	signals := make(chan *dbus.Signal, 4)
	client.Signal(signals)

	server.emitChanged("/home/me/.claude")

	select {
	case received := <-signals:
		// The plasmoid listens for exactly this member on this interface; a
		// rename on either side is silent until a widget stops updating.
		if received.Name != InterfaceName+".Changed" {
			t.Fatalf("signal name %q", received.Name)
		}
		if len(received.Body) != 1 {
			t.Fatalf("signal body %v, want one Account Home", received.Body)
		}
		if home, ok := received.Body[0].(string); !ok || home != "/home/me/.claude" {
			t.Fatalf("signal argument %v", received.Body[0])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Changed never arrived at a subscriber")
	}
}
