// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package dbusapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/eikrad/codecap/internal/accounthome"
	"github.com/eikrad/codecap/internal/allowance"
	"github.com/eikrad/codecap/internal/face"
	"github.com/eikrad/codecap/internal/httpx"
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

	// snapshotTimeout bounds one GetSnapshot. godbus dispatches every method
	// call in its own goroutine, so without a deadline a stuck call leaks one
	// goroutine per attempt.
	snapshotTimeout = httpx.Timeout + 10*time.Second

	// maxConcurrentSnapshots limits how much work unbounded incoming calls can
	// buy. Several widget instances share one helper, so a small number is
	// enough for legitimate use.
	maxConcurrentSnapshots = 4
)

type Server struct {
	conn       *dbus.Conn
	logWatches *usagewatch.Manager
	allowance  *allowance.Service
	poller     *allowance.Poller
	ctx        context.Context
	cancel     context.CancelFunc
	inFlight   chan struct{}
}

func (s *Server) GetSnapshot(rawAccountHome, timezone string, weekStart int32) (string, *dbus.Error) {
	// The Account Home is a plain string from any session-bus peer and is used
	// as a filesystem base throughout the helper.
	home, err := accounthome.Validate(rawAccountHome)
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}

	select {
	case s.inFlight <- struct{}{}:
		defer func() { <-s.inFlight }()
	case <-s.ctx.Done():
		return "", dbus.MakeFailedError(s.ctx.Err())
	}

	ctx, cancel := context.WithTimeout(s.ctx, snapshotTimeout)
	defer cancel()

	snap := face.Classify(home)
	s.addConsumedUsage(&snap, timezone, weekStart)
	s.addAllowance(ctx, &snap)
	s.ensureWatch(&snap)

	data, err := json.Marshal(snap)
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("marshal snapshot: %w", err))
	}
	return string(data), nil
}

// NewServer builds a helper server without publishing it on the bus.
//
// conn may be nil, in which case Changed signals are dropped. Export uses this,
// and so do tests that drive GetSnapshot directly; the previous version built
// every dependency inside Export, which is why Export itself had no test.
func NewServer(ctx context.Context, conn *dbus.Conn, allowanceService *allowance.Service) *Server {
	if ctx == nil {
		ctx = context.Background()
	}
	serverCtx, cancel := context.WithCancel(ctx)

	server := &Server{
		conn:      conn,
		allowance: allowanceService,
		ctx:       serverCtx,
		cancel:    cancel,
		inFlight:  make(chan struct{}, maxConcurrentSnapshots),
	}
	server.logWatches = usagewatch.NewManager(server.emitChanged)
	server.poller = allowance.NewPoller(serverCtx, allowance.PollInterval, server.pollAccountHome)
	return server
}

// Export publishes the helper interface and returns the server so the caller can
// shut it down.
func Export(ctx context.Context, conn *dbus.Conn) (*Server, error) {
	lastKnown := allowance.NewLastKnownStore(allowance.DefaultCacheRoot())
	server := NewServer(ctx, conn, allowance.NewService(lastKnown))

	if err := conn.Export(server, ObjectPath, InterfaceName); err != nil {
		_ = server.Close()
		return nil, fmt.Errorf("export helper interface: %w", err)
	}

	node := introspectNode()

	if err := conn.Export(introspect.NewIntrospectable(node), ObjectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		_ = server.Close()
		return nil, fmt.Errorf("export introspection: %w", err)
	}
	return server, nil
}

// IntrospectNode is the interface description published on the bus. It is the
// contract the plasmoid's asyncCall signature has to match.
func IntrospectNode() *introspect.Node {
	return introspectNode()
}

func introspectNode() *introspect.Node {
	return &introspect.Node{
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
}

// Close stops background work and releases the file watches.
func (s *Server) Close() error {
	s.cancel()
	s.poller.Stop()
	if s.logWatches == nil {
		return nil
	}
	return s.logWatches.Close()
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

func (s *Server) addAllowance(ctx context.Context, snap *snapshot.Snapshot) {
	if s.allowance == nil {
		return
	}
	if snap.Face != snapshot.FaceUnknownAllowance && snap.Face != snapshot.FaceReady {
		return
	}

	fields, err := s.allowance.Resolve(ctx, snap.AccountHome)
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

// ensureWatch starts a log watch only for an Account Home that classified as a
// real, signed-in one. It used to run for every call, before any face check, so
// any directory containing a projects/ subdirectory bought an inotify instance.
func (s *Server) ensureWatch(snap *snapshot.Snapshot) {
	if s.logWatches == nil || snap.AccountHome == "" {
		return
	}
	if snap.Face != snapshot.FaceUnknownAllowance && snap.Face != snapshot.FaceReady {
		return
	}
	s.logWatches.Ensure(snap.AccountHome)
}

func (s *Server) pollAccountHome(ctx context.Context, accountHome string) error {
	if s.allowance == nil {
		return nil
	}
	if _, err := s.allowance.Resolve(ctx, accountHome); err != nil {
		log.Printf("poll allowance failed for %s: %v", accountHome, err)
		return err
	}
	s.emitChanged(accountHome)
	return nil
}

func (s *Server) emitChanged(accountHome string) {
	if s.conn == nil {
		return
	}
	if err := s.conn.Emit(ObjectPath, InterfaceName+".Changed", accountHome); err != nil {
		log.Printf("emit changed signal failed for %s: %v", accountHome, err)
	}
}
