// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package dbusapi

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eikrad/codecap/internal/allowance"
	"github.com/eikrad/codecap/internal/face"
	"github.com/eikrad/codecap/internal/snapshot"
	"github.com/eikrad/codecap/internal/usage"
	"github.com/eikrad/codecap/internal/usagewatch"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const (
	ServiceName   = "dev.codecap.Helper"
	InterfaceName = "dev.codecap.Helper"
	ObjectPath    = dbus.ObjectPath("/dev/codecap/Helper")
)

type Server struct {
	conn       *dbus.Conn
	logWatches *usagewatch.Manager
	allowance  *allowance.Service
	poller     *allowance.Poller
}

func (s *Server) GetSnapshot(accountHome, timezone string, weekStart int32) (string, *dbus.Error) {
	snap := face.Classify(accountHome)
	s.addConsumedUsage(&snap, timezone, weekStart)
	s.addAllowance(&snap)
	s.ensureWatch(accountHome)

	data, err := json.Marshal(snap)
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("marshal snapshot: %w", err))
	}
	return string(data), nil
}

func Export(conn *dbus.Conn) error {
	lastKnown := allowance.NewLastKnownStore(allowance.DefaultCacheRoot())
	server := &Server{
		conn:      conn,
		allowance: allowance.NewService(lastKnown),
	}
	server.logWatches = usagewatch.NewManager(server.emitChanged)
	server.poller = allowance.NewPoller(allowance.PollInterval, server.pollAccountHome)

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

func (s *Server) addConsumedUsage(snap *snapshot.Snapshot, timezone string, weekStart int32) {
	if snap.Face != snapshot.FaceUnknownAllowance && snap.Face != snapshot.FaceReady {
		return
	}

	usageSnapshot, err := usage.Compute(snap.AccountHome, timezone, weekStart)
	if err != nil {
		log.Printf("compute consumed usage failed for %s: %v", snap.AccountHome, err)
		return
	}
	snap.ConsumedUsage = usageSnapshot
	if snap.FetchedAt == 0 {
		snap.FetchedAt = time.Now().Unix()
	}
}

func (s *Server) addAllowance(snap *snapshot.Snapshot) {
	if s.allowance == nil {
		return
	}
	if snap.Face != snapshot.FaceUnknownAllowance && snap.Face != snapshot.FaceReady {
		return
	}

	fields, err := s.allowance.Resolve(snap.AccountHome)
	if err != nil {
		log.Printf("resolve allowance failed for %s: %v", snap.AccountHome, err)
	}
	if fields.Face != "" {
		snap.Face = fields.Face
	}
	snap.SessionAllowance = fields.SessionAllowance
	snap.WeeklyAllowance = fields.WeeklyAllowance
	if fields.UsageCredit != "" {
		snap.UsageCredit = fields.UsageCredit
	}
	if fields.FetchedAt != 0 {
		snap.FetchedAt = fields.FetchedAt
	}

	if s.poller != nil {
		s.poller.Ensure(snap.AccountHome)
	}
}

func (s *Server) ensureWatch(accountHome string) {
	if s.logWatches == nil {
		return
	}
	if strings.TrimSpace(accountHome) == "" {
		return
	}
	s.logWatches.Ensure(accountHome)
}

func (s *Server) pollAccountHome(accountHome string) {
	if s.allowance == nil {
		return
	}
	if _, err := s.allowance.Resolve(accountHome); err != nil {
		log.Printf("poll allowance failed for %s: %v", accountHome, err)
	}
	s.emitChanged(accountHome)
}

func (s *Server) emitChanged(accountHome string) {
	if s.conn == nil {
		return
	}
	if err := s.conn.Emit(ObjectPath, InterfaceName+".Changed", accountHome); err != nil {
		log.Printf("emit changed signal failed for %s: %v", accountHome, err)
	}
}
