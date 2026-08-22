// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"context"
	"sync"
	"time"
)

const (
	// PollInterval is the conservative Allowance refresh cadence from ADR 0006.
	PollInterval = time.Minute

	// MaxAccounts bounds the registry. Account Homes arrive as D-Bus arguments
	// and used to be kept forever, so rebinding a widget leaked the old one and
	// any caller could buy an unbounded outbound request stream.
	MaxAccounts = 8

	minBackoff = 2 * time.Minute
	maxBackoff = 30 * time.Minute
)

// ResolveFunc refreshes Allowance for one Account Home.
type ResolveFunc func(ctx context.Context, accountHome string) error

// Poller periodically refreshes Allowance for known Account Homes.
type Poller struct {
	mu       sync.Mutex
	accounts map[string]*pollState
	started  bool
	stopped  bool

	interval time.Duration
	resolve  ResolveFunc
	now      func() time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

type pollState struct {
	lastSeen    time.Time
	nextAttempt time.Time
	failures    int
}

func NewPoller(ctx context.Context, interval time.Duration, resolve ResolveFunc) *Poller {
	if interval <= 0 {
		interval = PollInterval
	}
	if ctx == nil {
		ctx = context.Background()
	}
	pollCtx, cancel := context.WithCancel(ctx)
	return &Poller{
		accounts: make(map[string]*pollState),
		interval: interval,
		resolve:  resolve,
		now:      func() time.Time { return time.Now() },
		ctx:      pollCtx,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
}

// Ensure registers an Account Home and starts the loop on first use.
func (p *Poller) Ensure(accountHome string) {
	if p == nil || accountHome == "" {
		return
	}

	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}

	now := p.now()
	if state, ok := p.accounts[accountHome]; ok {
		state.lastSeen = now
	} else {
		p.evictLocked(now)
		p.accounts[accountHome] = &pollState{lastSeen: now, nextAttempt: now.Add(p.interval)}
	}

	start := !p.started
	p.started = true
	p.mu.Unlock()

	if start {
		go p.loop()
	}
}

// Forget drops an Account Home from the registry.
func (p *Poller) Forget(accountHome string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	delete(p.accounts, accountHome)
	p.mu.Unlock()
}

// evictLocked makes room by dropping the least recently requested entry.
func (p *Poller) evictLocked(now time.Time) {
	if len(p.accounts) < MaxAccounts {
		return
	}
	oldestKey := ""
	oldestSeen := now
	for key, state := range p.accounts {
		if oldestKey == "" || state.lastSeen.Before(oldestSeen) {
			oldestKey = key
			oldestSeen = state.lastSeen
		}
	}
	if oldestKey != "" {
		delete(p.accounts, oldestKey)
	}
}

// Stop ends the loop and waits for the current tick to finish.
func (p *Poller) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	alreadyStopped := p.stopped
	started := p.started
	p.stopped = true
	p.mu.Unlock()

	if alreadyStopped {
		return
	}
	p.cancel()
	if started {
		<-p.done
	}
}

func (p *Poller) loop() {
	defer close(p.done)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.tick()
		}
	}
}

// due returns the Account Homes whose backoff has elapsed.
func (p *Poller) due(now time.Time) []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	homes := make([]string, 0, len(p.accounts))
	for home, state := range p.accounts {
		if !state.nextAttempt.After(now) {
			homes = append(homes, home)
		}
	}
	return homes
}

func (p *Poller) tick() {
	if p.resolve == nil {
		return
	}
	for _, home := range p.due(p.now()) {
		if p.ctx.Err() != nil {
			return
		}
		err := p.resolve(p.ctx, home)
		p.record(home, err)
	}
}

// record applies exponential backoff so a failing endpoint is not hammered once
// a minute for the life of the session.
func (p *Poller) record(accountHome string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok := p.accounts[accountHome]
	if !ok {
		return
	}
	now := p.now()
	if err == nil {
		state.failures = 0
		state.nextAttempt = now.Add(p.interval)
		return
	}

	state.failures++
	wait := minBackoff << (state.failures - 1)
	if wait > maxBackoff || wait <= 0 {
		wait = maxBackoff
	}
	// A server that told us how long to wait outranks our own guess.
	if suggested, ok := RetryAfter(err); ok && suggested > wait {
		wait = suggested
	}
	state.nextAttempt = now.Add(wait)
}
