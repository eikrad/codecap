// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"sync"
	"time"
)

// PollInterval is the conservative Allowance refresh cadence from ADR 0006.
const PollInterval = time.Minute

// Poller periodically refreshes Allowance for known Account Homes.
type Poller struct {
	mu       sync.Mutex
	accounts map[string]struct{}
	interval time.Duration
	resolve  func(accountHome string)
	stop     chan struct{}
	once     sync.Once
}

func NewPoller(interval time.Duration, resolve func(accountHome string)) *Poller {
	if interval <= 0 {
		interval = PollInterval
	}
	return &Poller{
		accounts: make(map[string]struct{}),
		interval: interval,
		resolve:  resolve,
		stop:     make(chan struct{}),
	}
}

func (p *Poller) Ensure(accountHome string) {
	if accountHome == "" || p == nil {
		return
	}
	p.mu.Lock()
	p.accounts[accountHome] = struct{}{}
	p.mu.Unlock()
	p.once.Do(func() {
		go p.loop()
	})
}

func (p *Poller) Stop() {
	select {
	case <-p.stop:
	default:
		close(p.stop)
	}
}

func (p *Poller) loop() {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.tick()
		}
	}
}

func (p *Poller) tick() {
	p.mu.Lock()
	homes := make([]string, 0, len(p.accounts))
	for home := range p.accounts {
		homes = append(homes, home)
	}
	p.mu.Unlock()

	for _, home := range homes {
		if p.resolve != nil {
			p.resolve(home)
		}
	}
}
