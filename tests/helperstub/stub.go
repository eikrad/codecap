// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

// Package main is the scriptable stand-in for the codecap helper that the QML
// D-Bus harness drives (docs/plan-hardening.md, phase 5.3).
//
// It exists because four of the eight critical findings of 2026-08-22 were
// non-existent QML names — `onReceivedSignal` for what is really `dbusChanged`,
// a `MouseArea` that was never written — and no gate in this repo can see that
// class of bug. A stub on a private session bus can: the plasmoid's own
// SnapshotSource talks to it exactly as it talks to the helper, so a handler
// that is never called shows up as a snapshot that never changes.
//
// The interface it publishes is not written here. It comes from
// dbusapi.IntrospectNode(), the same document the real helper exports, so the
// harness cannot drift into testing a contract the helper does not have.
package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/eikrad/codecap/internal/dbusapi"
	"github.com/eikrad/codecap/internal/snapshot"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

// The control surface the harness scripts the stub through. It is a second
// object on the same connection rather than extra methods on the helper
// interface, so that /dev/codecap/Helper introspects byte for byte like the
// real helper — which is the whole point of the exercise, because the QML
// binding derives its argument signature from that document.
const (
	ControlInterface = "dev.codecap.Stub"
	ControlPath      = dbus.ObjectPath("/dev/codecap/Stub")
)

// Call is one GetSnapshot as the stub received it. The harness asserts on these:
// an empty timezone is the ADR 0011 contract for "the helper's own zone", and a
// call that never arrives is how a dead signal handler looks from this side.
type Call struct {
	AccountHome string `json:"account_home"`
	Timezone    string `json:"timezone"`
	WeekStart   int32  `json:"week_start"`
}

// Stub holds the scripted behaviour. Every field is guarded: godbus dispatches
// each incoming method call in its own goroutine.
type Stub struct {
	conn *dbus.Conn

	mu      sync.Mutex
	snap    snapshot.Snapshot
	raw     string
	rawSet  bool
	failure string
	delay   time.Duration
	calls   []Call
}

// NewStub builds a stub serving the "ready" fixture.
func NewStub(conn *dbus.Conn) *Stub {
	return &Stub{conn: conn, snap: Fixture(string(snapshot.FaceReady))}
}

// Export publishes the helper interface, its introspection, and the control
// interface. Both objects get an Introspectable: the QML binding introspects
// before every call it has not seen before, and a missing document fails the
// call rather than degrading.
func (s *Stub) Export(conn *dbus.Conn) error {
	if err := conn.Export(helperFace{s}, dbusapi.ObjectPath, dbusapi.InterfaceName); err != nil {
		return fmt.Errorf("export helper interface: %w", err)
	}
	helperNode := dbusapi.IntrospectNode()
	if err := conn.Export(introspect.NewIntrospectable(helperNode), dbusapi.ObjectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("export helper introspection: %w", err)
	}
	if err := conn.Export(controlFace{s}, ControlPath, ControlInterface); err != nil {
		return fmt.Errorf("export control interface: %w", err)
	}
	if err := conn.Export(introspect.NewIntrospectable(controlNode()), ControlPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("export control introspection: %w", err)
	}
	return nil
}

// helperFace carries only the methods of dev.codecap.Helper. godbus exports
// every exported method of the value it is given, so the two interfaces need
// two values or each would answer for the other's members.
type helperFace struct{ s *Stub }

func (h helperFace) GetSnapshot(accountHome, timezone string, weekStart int32) (string, *dbus.Error) {
	return h.s.getSnapshot(accountHome, timezone, weekStart)
}

type controlFace struct{ s *Stub }

// SetFace serves the fixture for a Face, clearing any raw override.
func (c controlFace) SetFace(face string) *dbus.Error { return c.s.setFace(face) }

// SetSessionPercent moves the Session Allowance of the served fixture. The
// harness uses it as a watermark: a value that arrives without the harness
// having asked for a refresh is proof the push signal was delivered.
func (c controlFace) SetSessionPercent(percent float64) *dbus.Error {
	return c.s.setSessionPercent(percent)
}

// SetRaw serves a byte string verbatim, valid JSON or not. Unreadable helper
// output has to reach Unknown Allowance rather than Unbound (finding P-M6), and
// that path cannot be reached through a well-formed snapshot.
func (c controlFace) SetRaw(payload string) *dbus.Error { return c.s.setRaw(payload) }

// SetFailure makes GetSnapshot answer with a D-Bus error. Empty clears it.
func (c controlFace) SetFailure(message string) *dbus.Error { return c.s.setFailure(message) }

// SetDelayMs holds each reply back, so the harness can start a second request
// while the first is in flight (finding P-M3).
func (c controlFace) SetDelayMs(ms int32) *dbus.Error { return c.s.setDelay(ms) }

// EmitChanged sends the push signal the helper sends after a recompute.
func (c controlFace) EmitChanged(accountHome string) *dbus.Error {
	return c.s.emitChanged(accountHome)
}

// CallLog returns every GetSnapshot the stub has received, as JSON. A JSON
// string rather than a struct array keeps the control signature to "s" and
// spares the QML side a decoder for a D-Bus type it would have to guess at.
func (c controlFace) CallLog() (string, *dbus.Error) { return c.s.callLog() }

// Reset restores the ready fixture and empties the call log.
func (c controlFace) Reset() *dbus.Error { return c.s.reset() }

func (s *Stub) getSnapshot(accountHome, timezone string, weekStart int32) (string, *dbus.Error) {
	s.mu.Lock()
	s.calls = append(s.calls, Call{AccountHome: accountHome, Timezone: timezone, WeekStart: weekStart})
	failure, delay, raw, rawSet := s.failure, s.delay, s.raw, s.rawSet
	snap := s.snap
	s.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}
	if failure != "" {
		return "", dbus.MakeFailedError(fmt.Errorf("%s", failure))
	}
	if rawSet {
		return raw, nil
	}

	// The real helper classifies the Account Home it was handed and reports it
	// back, so a reply identifies which request it answers. The in-flight test
	// depends on that: it is the only way to tell a superseded reply apart.
	snap.AccountHome = accountHome
	data, err := json.Marshal(snap)
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return string(data), nil
}

func (s *Stub) setFace(face string) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap = Fixture(face)
	s.raw, s.rawSet = "", false
	return nil
}

func (s *Stub) setSessionPercent(percent float64) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.SessionAllowance.UsedPercent = percent
	s.raw, s.rawSet = "", false
	return nil
}

func (s *Stub) setRaw(payload string) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw, s.rawSet = payload, true
	return nil
}

func (s *Stub) setFailure(message string) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure = message
	return nil
}

func (s *Stub) setDelay(ms int32) *dbus.Error {
	if ms < 0 {
		return dbus.MakeFailedError(fmt.Errorf("negative delay %d", ms))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delay = time.Duration(ms) * time.Millisecond
	return nil
}

func (s *Stub) emitChanged(accountHome string) *dbus.Error {
	if s.conn == nil {
		return dbus.MakeFailedError(fmt.Errorf("stub is not on a bus"))
	}
	// Emitted with the member name the plasmoid subscribes to. dbusapi builds
	// it the same way; a rename on either side is silent until a widget stops
	// updating, which is finding P-C2.
	if err := s.conn.Emit(dbusapi.ObjectPath, dbusapi.InterfaceName+".Changed", accountHome); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

func (s *Stub) callLog() (string, *dbus.Error) {
	s.mu.Lock()
	calls := make([]Call, len(s.calls))
	copy(calls, s.calls)
	s.mu.Unlock()

	data, err := json.Marshal(calls)
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return string(data), nil
}

func (s *Stub) reset() *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap = Fixture(string(snapshot.FaceReady))
	s.raw, s.rawSet = "", false
	s.failure = ""
	s.delay = 0
	s.calls = nil
	return nil
}

// controlNode describes the control interface for introspection. The QML
// binding needs it to encode a call it has not made before.
func controlNode() *introspect.Node {
	str := func(name, dir string) introspect.Arg {
		return introspect.Arg{Name: name, Type: "s", Direction: dir}
	}
	return &introspect.Node{
		Name: string(ControlPath),
		Interfaces: []introspect.Interface{
			{
				Name: ControlInterface,
				Methods: []introspect.Method{
					{Name: "SetFace", Args: []introspect.Arg{str("face", "in")}},
					{Name: "SetSessionPercent", Args: []introspect.Arg{{Name: "percent", Type: "d", Direction: "in"}}},
					{Name: "SetRaw", Args: []introspect.Arg{str("payload", "in")}},
					{Name: "SetFailure", Args: []introspect.Arg{str("message", "in")}},
					{Name: "SetDelayMs", Args: []introspect.Arg{{Name: "ms", Type: "i", Direction: "in"}}},
					{Name: "EmitChanged", Args: []introspect.Arg{str("accountHome", "in")}},
					{Name: "CallLog", Args: []introspect.Arg{str("callsJSON", "out")}},
					{Name: "Reset"},
				},
			},
			introspect.IntrospectData,
		},
	}
}
