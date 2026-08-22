// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package dbusapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
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

	// consumedTTL is how long a computed Consumed Usage is served before a
	// background recompute is triggered. The reported windows move even when no
	// log changes — the Session window is rolling and the day rolls over at
	// midnight — so this bounds how late a boundary can be, not how stale the
	// data is.
	consumedTTL = 90 * time.Second

	// allowanceTTL is the same for Allowance, against the ADR 0006 cadence.
	allowanceTTL = 2 * allowance.PollInterval
)

// usageKey is what a Consumed Usage aggregation depends on besides the logs.
type usageKey struct {
	timezone  string
	weekStart int32
}

// accountState is the cached face of one Account Home.
//
// ADR 0011 puts polling and file watch in the helper, so GetSnapshot reads this
// and returns. It used to do the vendor fetch and a full log re-read inline,
// which cost seconds per call.
type accountState struct {
	mu sync.Mutex

	key        usageKey
	consumed   snapshot.ConsumedUsage
	consumedAt time.Time
	consumedOK bool

	allowanceFields allowance.SnapshotFields
	allowanceAt     time.Time
	allowanceOK     bool

	refreshing bool
}

type Server struct {
	conn       *dbus.Conn
	logWatches *usagewatch.Manager
	allowance  *allowance.Service
	poller     *allowance.Poller
	usage      *usage.Cache
	ctx        context.Context
	cancel     context.CancelFunc
	inFlight   chan struct{}
	refreshes  sync.WaitGroup

	statesMu sync.Mutex
	states   map[string]*accountState
	now      func() time.Time
}

func (s *Server) state(accountHome string) *accountState {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	if existing, ok := s.states[accountHome]; ok {
		return existing
	}
	created := &accountState{}
	s.states[accountHome] = created
	return created
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

	snap := face.Classify(home)
	s.fillFromCache(&snap, usageKey{timezone: timezone, weekStart: weekStart})

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
		usage:     usage.NewCache(usage.DefaultRates()),
		ctx:       serverCtx,
		cancel:    cancel,
		inFlight:  make(chan struct{}, maxConcurrentSnapshots),
		states:    make(map[string]*accountState),
		now:       func() time.Time { return time.Now() },
	}
	server.logWatches = usagewatch.NewManager(server.onUsageChanged, usagewatch.DefaultDebounce)
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
	s.refreshes.Wait()
	if s.logWatches == nil {
		return nil
	}
	return s.logWatches.Close()
}

// fillFromCache serves whatever is already known and triggers a refresh for
// whatever is missing or has aged out. It never blocks on the network or on the
// filesystem.
func (s *Server) fillFromCache(snap *snapshot.Snapshot, key usageKey) {
	if snap.Face != snapshot.FaceUnknownAllowance && snap.Face != snapshot.FaceReady {
		return
	}

	state := s.state(snap.AccountHome)
	now := s.now()

	state.mu.Lock()
	if state.key != key {
		// A different timezone or week start needs its own aggregation.
		state.consumedOK = false
		state.key = key
	}
	consumedOK, allowanceOK := state.consumedOK, state.allowanceOK
	consumedFresh := consumedOK && now.Sub(state.consumedAt) < consumedTTL
	allowanceFresh := allowanceOK && now.Sub(state.allowanceAt) < allowanceTTL
	if consumedOK {
		snap.ConsumedUsage = state.consumed
		snap.FetchedAt = state.consumedAt.Unix()
	}
	fields := state.allowanceFields
	state.mu.Unlock()

	if allowanceOK {
		applyAllowance(snap, fields)
	}

	switch {
	case !consumedOK && !allowanceOK:
		snap.Degraded = append(snap.Degraded, snapshot.DegradedPending)
	default:
		if !consumedOK {
			snap.Degraded = append(snap.Degraded, snapshot.DegradedUsage)
		}
		if !allowanceOK {
			snap.Degraded = append(snap.Degraded, snapshot.DegradedAllowance)
		}
	}

	s.logWatches.Ensure(snap.AccountHome)
	if s.poller != nil {
		s.poller.Ensure(snap.AccountHome)
	}
	if !consumedFresh || !allowanceFresh {
		s.scheduleRefresh(snap.AccountHome, key)
	}
}

func applyAllowance(snap *snapshot.Snapshot, fields allowance.SnapshotFields) {
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
}

// scheduleRefresh runs at most one refresh per Account Home at a time.
func (s *Server) scheduleRefresh(accountHome string, key usageKey) {
	state := s.state(accountHome)
	state.mu.Lock()
	if state.refreshing {
		state.mu.Unlock()
		return
	}
	state.refreshing = true
	state.mu.Unlock()

	if s.ctx.Err() != nil {
		state.mu.Lock()
		state.refreshing = false
		state.mu.Unlock()
		return
	}

	s.refreshes.Add(1)
	go func() {
		defer s.refreshes.Done()
		defer func() {
			state.mu.Lock()
			state.refreshing = false
			state.mu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(s.ctx, snapshotTimeout)
		defer cancel()

		usageChanged := s.refreshUsage(accountHome, key)
		allowanceChanged := s.refreshAllowance(ctx, accountHome)
		if usageChanged || allowanceChanged {
			s.emitChanged(accountHome)
		}
	}()
}

// refreshUsage recomputes Consumed Usage from the logs.
func (s *Server) refreshUsage(accountHome string, key usageKey) bool {
	if s.usage == nil {
		return false
	}
	computed, err := s.usage.Compute(accountHome, key.timezone, key.weekStart)
	if err != nil {
		log.Printf("compute consumed usage failed for %s: %v", accountHome, err)
		return false
	}

	state := s.state(accountHome)
	state.mu.Lock()
	state.consumed = computed
	state.consumedAt = s.now()
	state.consumedOK = true
	state.key = key
	state.mu.Unlock()
	return true
}

// refreshAllowance resolves live or Last-Known Allowance.
func (s *Server) refreshAllowance(ctx context.Context, accountHome string) bool {
	if s.allowance == nil {
		return false
	}
	fields, err := s.allowance.Resolve(ctx, accountHome)
	if err != nil {
		log.Printf("resolve allowance failed for %s: %v", accountHome, err)
	}
	// Even a failed resolve yields a usable face, from Last-Known or Signed Out.
	if fields.Face == "" {
		return false
	}

	state := s.state(accountHome)
	state.mu.Lock()
	state.allowanceFields = fields
	state.allowanceAt = s.now()
	state.allowanceOK = true
	state.mu.Unlock()
	return true
}

// currentKey is the aggregation a watcher-driven recompute should use.
func (s *Server) currentKey(accountHome string) usageKey {
	state := s.state(accountHome)
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.key
}

// onUsageChanged is the debounced file-watch clock from ADR 0006.
func (s *Server) onUsageChanged(accountHome string) {
	if s.refreshUsage(accountHome, s.currentKey(accountHome)) {
		s.emitChanged(accountHome)
	}
}

// pollAccountHome is the Allowance clock from ADR 0006. It also recomputes
// Consumed Usage, because the reported windows roll over on their own.
func (s *Server) pollAccountHome(ctx context.Context, accountHome string) error {
	allowanceChanged := s.refreshAllowance(ctx, accountHome)
	usageChanged := s.refreshUsage(accountHome, s.currentKey(accountHome))
	if allowanceChanged || usageChanged {
		s.emitChanged(accountHome)
	}
	if !allowanceChanged && s.allowance != nil {
		return fmt.Errorf("allowance unavailable for %s", accountHome)
	}
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
