// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/eikrad/codecap/internal/dbusapi"
	"github.com/eikrad/codecap/internal/snapshot"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

// The stub is the half of the QML harness that can be executed without Plasma,
// so it is tested like production code. If the stub is wrong, every QML
// assertion above it is measuring the stub.
//
// Run under `dbus-run-session`; `make test` does that when it is available.

func sessionBus(t *testing.T) *dbus.Conn {
	t.Helper()
	// Skipping locally is a convenience; skipping in CI would make this one of
	// the tests that quietly never runs.
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

// exportStub publishes a stub under a name unique to this test. The harness
// itself takes dev.codecap.Helper, but `go test ./...` runs packages in
// parallel on one bus, and internal/dbusapi's own bus tests want that name —
// two suites racing for it would show up as one of them skipping at random.
func exportStub(t *testing.T) (*Stub, dbus.BusObject, dbus.BusObject) {
	t.Helper()

	conn := sessionBus(t)
	stub := NewStub(conn)
	if err := stub.Export(conn); err != nil {
		t.Fatalf("export: %v", err)
	}

	name := fmt.Sprintf("dev.codecap.StubTest%d", time.Now().UnixNano())
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil {
		t.Fatalf("request name: %v", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("%s is already owned on this bus", name)
	}
	t.Cleanup(func() { _, _ = conn.ReleaseName(name) })

	client := sessionBus(t)
	return stub, client.Object(name, dbusapi.ObjectPath), client.Object(name, ControlPath)
}

func getSnapshot(t *testing.T, helper dbus.BusObject, accountHome string) string {
	t.Helper()
	var payload string
	call := helper.Call(dbusapi.InterfaceName+".GetSnapshot", 0, accountHome, "", int32(1))
	if call.Err != nil {
		t.Fatalf("GetSnapshot: %v", call.Err)
	}
	if err := call.Store(&payload); err != nil {
		t.Fatalf("store reply: %v", err)
	}
	return payload
}

func control(t *testing.T, ctl dbus.BusObject, member string, args ...any) {
	t.Helper()
	if call := ctl.Call(ControlInterface+"."+member, 0, args...); call.Err != nil {
		t.Fatalf("%s: %v", member, call.Err)
	}
}

// The document the plasmoid derives its argument encoding from. If the stub
// publishes anything but the helper's own, the harness proves nothing about the
// helper — it tests a contract only the harness has.
func TestHelperIntrospectionIsTheRealHelpersByteForByte(t *testing.T) {
	_, helper, _ := exportStub(t)

	var served string
	call := helper.Call("org.freedesktop.DBus.Introspectable.Introspect", 0)
	if call.Err != nil {
		t.Fatalf("Introspect: %v", call.Err)
	}
	if err := call.Store(&served); err != nil {
		t.Fatalf("store introspection: %v", err)
	}

	want, dbusErr := introspect.NewIntrospectable(dbusapi.IntrospectNode()).Introspect()
	if dbusErr != nil {
		t.Fatalf("build expected introspection: %v", dbusErr)
	}
	if served != want {
		t.Fatalf("stub introspection differs from the helper's\n--- stub ---\n%s\n--- helper ---\n%s", served, want)
	}
}

// The control interface is introspected too: the QML side encodes a call it has
// not made before from this document, so a missing one fails the call outright
// rather than degrading.
func TestControlIntrospectionDescribesEveryScriptingMethod(t *testing.T) {
	_, _, ctl := exportStub(t)

	var doc string
	call := ctl.Call("org.freedesktop.DBus.Introspectable.Introspect", 0)
	if call.Err != nil {
		t.Fatalf("Introspect: %v", call.Err)
	}
	if err := call.Store(&doc); err != nil {
		t.Fatalf("store introspection: %v", err)
	}

	var node introspect.Node
	if err := xml.Unmarshal([]byte(doc), &node); err != nil {
		t.Fatalf("parse introspection: %v", err)
	}

	declared := map[string]bool{}
	for _, iface := range node.Interfaces {
		if iface.Name != ControlInterface {
			continue
		}
		for _, method := range iface.Methods {
			declared[method.Name] = true
		}
	}
	for _, want := range []string{
		"SetFace", "SetSessionPercent", "SetRaw", "SetFailure",
		"SetDelayMs", "EmitChanged", "CallLog", "Reset",
	} {
		if !declared[want] {
			t.Fatalf("%s is exported but not introspectable\n%s", want, doc)
		}
	}
}

func TestReadyFixtureIsAReadableSnapshotOfTheCurrentSchema(t *testing.T) {
	_, helper, _ := exportStub(t)

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(getSnapshot(t, helper, "/home/me/.claude")), &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if snap.SchemaVersion != snapshot.SchemaVersion {
		t.Fatalf("schema_version %d, want %d", snap.SchemaVersion, snapshot.SchemaVersion)
	}
	if snap.Face != snapshot.FaceReady {
		t.Fatalf("face %q, want %q", snap.Face, snapshot.FaceReady)
	}
	if snap.SessionAllowance.UsedPercent != 37 {
		t.Fatalf("session used_percent %v, want 37", snap.SessionAllowance.UsedPercent)
	}
	// Distinct per window, so a QML assertion cannot pass by reading the wrong
	// one.
	for _, seen := range []struct {
		name   string
		tokens int64
	}{
		{"session", snap.ConsumedUsage.Session.Tokens},
		{"today", snap.ConsumedUsage.Today.Tokens},
		{"week", snap.ConsumedUsage.Week.Tokens},
		{"month", snap.ConsumedUsage.Month.Tokens},
	} {
		if seen.tokens == 0 {
			t.Fatalf("%s tokens is 0; the fixture must distinguish the windows", seen.name)
		}
	}
}

// The reply has to say which request it answers, or the in-flight test in the
// QML harness cannot tell a superseded reply from a current one (finding P-M3).
func TestReplyEchoesTheRequestedAccountHome(t *testing.T) {
	_, helper, _ := exportStub(t)

	for _, home := range []string{"/home/me/.claude", "/home/me/.claude-work"} {
		var snap snapshot.Snapshot
		if err := json.Unmarshal([]byte(getSnapshot(t, helper, home)), &snap); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if snap.AccountHome != home {
			t.Fatalf("account_home %q, want %q", snap.AccountHome, home)
		}
	}
}

func TestSetFaceServesThatFace(t *testing.T) {
	_, helper, ctl := exportStub(t)

	for _, face := range []snapshot.Face{
		snapshot.FaceSignedOut, snapshot.FaceUnknownAllowance, snapshot.FaceReady,
	} {
		control(t, ctl, "SetFace", string(face))

		var snap snapshot.Snapshot
		if err := json.Unmarshal([]byte(getSnapshot(t, helper, "/home/me/.claude")), &snap); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if snap.Face != face {
			t.Fatalf("face %q, want %q", snap.Face, face)
		}
	}
}

func TestSetSessionPercentIsTheWatermarkTheHarnessWatchesFor(t *testing.T) {
	_, helper, ctl := exportStub(t)

	control(t, ctl, "SetSessionPercent", float64(88))

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(getSnapshot(t, helper, "/home/me/.claude")), &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if snap.SessionAllowance.UsedPercent != 88 {
		t.Fatalf("session used_percent %v, want 88", snap.SessionAllowance.UsedPercent)
	}
}

// Unreadable helper output must reach the plasmoid as bytes, not as an error:
// P-M6 is about what the plasmoid does with a payload it cannot parse, and that
// path does not exist if the stub refuses to send one.
func TestSetRawIsServedVerbatim(t *testing.T) {
	_, helper, ctl := exportStub(t)

	for _, payload := range []string{"not-json", "", "[]", `{"face":`} {
		control(t, ctl, "SetRaw", payload)
		if got := getSnapshot(t, helper, "/home/me/.claude"); got != payload {
			t.Fatalf("served %q, want %q", got, payload)
		}
	}
}

func TestSetFailureMakesTheCallFail(t *testing.T) {
	_, helper, ctl := exportStub(t)

	control(t, ctl, "SetFailure", "helper is not there")
	call := helper.Call(dbusapi.InterfaceName+".GetSnapshot", 0, "/home/me/.claude", "", int32(1))
	if call.Err == nil {
		t.Fatal("GetSnapshot succeeded while a failure was scripted")
	}

	control(t, ctl, "SetFailure", "")
	getSnapshot(t, helper, "/home/me/.claude")
}

func TestSetDelayHoldsTheReplyBack(t *testing.T) {
	_, helper, ctl := exportStub(t)

	control(t, ctl, "SetDelayMs", int32(300))
	started := time.Now()
	getSnapshot(t, helper, "/home/me/.claude")
	if elapsed := time.Since(started); elapsed < 250*time.Millisecond {
		t.Fatalf("reply took %v; the scripted delay did not hold it back", elapsed)
	}
}

// The signal the plasmoid subscribes to. The stub emits it on the same path,
// interface and member as the helper, because the harness above this is trying
// to prove the QML handler for it is named correctly (finding P-C2).
func TestEmitChangedReachesASubscriber(t *testing.T) {
	_, _, ctl := exportStub(t)

	client := sessionBus(t)
	if err := client.AddMatchSignal(
		dbus.WithMatchObjectPath(dbusapi.ObjectPath),
		dbus.WithMatchInterface(dbusapi.InterfaceName),
		dbus.WithMatchMember("Changed"),
	); err != nil {
		t.Fatalf("add match: %v", err)
	}
	signals := make(chan *dbus.Signal, 4)
	client.Signal(signals)

	control(t, ctl, "EmitChanged", "/home/me/.claude")

	select {
	case received := <-signals:
		if received.Name != dbusapi.InterfaceName+".Changed" {
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

// What the QML harness asserts a refresh with: the count says a call happened,
// the arguments say the plasmoid still sends what ADR 0011 says it sends.
func TestCallLogRecordsWhatThePlasmoidSends(t *testing.T) {
	_, helper, ctl := exportStub(t)

	getSnapshot(t, helper, "/home/me/.claude")

	var payload string
	call := ctl.Call(ControlInterface+".CallLog", 0)
	if call.Err != nil {
		t.Fatalf("CallLog: %v", call.Err)
	}
	if err := call.Store(&payload); err != nil {
		t.Fatalf("store call log: %v", err)
	}

	var calls []Call
	if err := json.Unmarshal([]byte(payload), &calls); err != nil {
		t.Fatalf("unmarshal call log: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("call log has %d entries, want 1: %s", len(calls), payload)
	}
	if calls[0].AccountHome != "/home/me/.claude" {
		t.Fatalf("account_home %q", calls[0].AccountHome)
	}
	// Empty means "the helper's own zone". Sending a display name here resolved
	// to UTC and moved every day boundary (finding C-1).
	if calls[0].Timezone != "" {
		t.Fatalf("timezone %q, want empty", calls[0].Timezone)
	}
	if calls[0].WeekStart != 1 {
		t.Fatalf("week_start %d, want 1", calls[0].WeekStart)
	}
}

func TestResetClearsTheScriptAndTheLog(t *testing.T) {
	stub, helper, ctl := exportStub(t)

	control(t, ctl, "SetRaw", "not-json")
	control(t, ctl, "SetFailure", "boom")
	control(t, ctl, "SetDelayMs", int32(200))
	getSnapshotFailure(t, helper)

	control(t, ctl, "Reset")

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(getSnapshot(t, helper, "/home/me/.claude")), &snap); err != nil {
		t.Fatalf("unmarshal after reset: %v", err)
	}
	if snap.Face != snapshot.FaceReady {
		t.Fatalf("face after reset %q", snap.Face)
	}

	stub.mu.Lock()
	calls := len(stub.calls)
	stub.mu.Unlock()
	// Reset emptied the log, then the call above added one.
	if calls != 1 {
		t.Fatalf("call log has %d entries after reset, want 1", calls)
	}
}

func getSnapshotFailure(t *testing.T, helper dbus.BusObject) {
	t.Helper()
	if call := helper.Call(dbusapi.InterfaceName+".GetSnapshot", 0, "/home/me/.claude", "", int32(1)); call.Err == nil {
		t.Fatal("GetSnapshot succeeded while a failure was scripted")
	}
}
