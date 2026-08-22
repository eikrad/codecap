// SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
// SPDX-License-Identifier: GPL-2.0-or-later

package allowance

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// recorder counts resolve calls per Account Home.
type recorder struct {
	mu     sync.Mutex
	counts map[string]int
	fail   error
	calls  chan string
}

func newRecorder() *recorder {
	return &recorder{counts: map[string]int{}, calls: make(chan string, 64)}
}

func (r *recorder) resolve(_ context.Context, accountHome string) error {
	r.mu.Lock()
	r.counts[accountHome]++
	err := r.fail
	r.mu.Unlock()

	select {
	case r.calls <- accountHome:
	default:
	}
	return err
}

func (r *recorder) count(accountHome string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[accountHome]
}

func waitForCall(t *testing.T, rec *recorder) string {
	t.Helper()
	select {
	case home := <-rec.calls:
		return home
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a poll")
		return ""
	}
}

func TestPollerPollsRegisteredAccountHome(t *testing.T) {
	rec := newRecorder()
	poller := NewPoller(context.Background(), 10*time.Millisecond, rec.resolve)
	defer poller.Stop()

	poller.Ensure("/home/me/.claude")
	if got := waitForCall(t, rec); got != "/home/me/.claude" {
		t.Fatalf("polled %q", got)
	}
}

func TestPollerStopEndsTheLoop(t *testing.T) {
	rec := newRecorder()
	poller := NewPoller(context.Background(), 10*time.Millisecond, rec.resolve)
	poller.Ensure("/home/me/.claude")
	waitForCall(t, rec)

	poller.Stop()
	// Stop waits for the loop, so no further call can start after it returns.
	before := rec.count("/home/me/.claude")
	time.Sleep(60 * time.Millisecond)
	if after := rec.count("/home/me/.claude"); after != before {
		t.Fatalf("polled %d more times after Stop", after-before)
	}

	poller.Stop() // idempotent
}

func TestPollerEnsureAfterStopDoesNotRegister(t *testing.T) {
	rec := newRecorder()
	poller := NewPoller(context.Background(), 10*time.Millisecond, rec.resolve)
	poller.Stop()
	poller.Ensure("/home/me/.claude")

	time.Sleep(50 * time.Millisecond)
	if got := rec.count("/home/me/.claude"); got != 0 {
		t.Fatalf("expected no polling after Stop, got %d", got)
	}
}

func TestPollerRegistryIsBounded(t *testing.T) {
	rec := newRecorder()
	poller := NewPoller(context.Background(), time.Hour, rec.resolve)
	defer poller.Stop()

	// Account Homes arrive as D-Bus arguments; keeping them all meant any
	// caller could buy a permanent outbound request stream per path.
	for i := 0; i < MaxAccounts*3; i++ {
		poller.Ensure(fmt.Sprintf("/home/me/.claude-%d", i))
	}

	poller.mu.Lock()
	registered := len(poller.accounts)
	_, oldestStillThere := poller.accounts["/home/me/.claude-0"]
	_, newestThere := poller.accounts[fmt.Sprintf("/home/me/.claude-%d", MaxAccounts*3-1)]
	poller.mu.Unlock()

	if registered > MaxAccounts {
		t.Fatalf("registry grew to %d, cap is %d", registered, MaxAccounts)
	}
	if oldestStillThere {
		t.Fatal("expected the least recently requested entry to be evicted")
	}
	if !newestThere {
		t.Fatal("expected the most recent entry to be kept")
	}
}

func TestPollerForgetRemovesAnAccountHome(t *testing.T) {
	rec := newRecorder()
	poller := NewPoller(context.Background(), time.Hour, rec.resolve)
	defer poller.Stop()

	poller.Ensure("/home/me/.claude")
	poller.Forget("/home/me/.claude")

	poller.mu.Lock()
	_, ok := poller.accounts["/home/me/.claude"]
	poller.mu.Unlock()
	if ok {
		t.Fatal("Forget should drop the entry")
	}
}

func TestPollerBacksOffAfterFailure(t *testing.T) {
	rec := newRecorder()
	rec.fail = errors.New("vendor down")

	poller := NewPoller(context.Background(), 10*time.Millisecond, rec.resolve)
	defer poller.Stop()

	poller.Ensure("/home/me/.claude")
	waitForCall(t, rec)

	// One failure pushes the next attempt out by minBackoff, so a broken
	// endpoint is not hit once a minute for the whole session.
	time.Sleep(80 * time.Millisecond)
	if got := rec.count("/home/me/.claude"); got != 1 {
		t.Fatalf("expected backoff after a failure, got %d polls", got)
	}

	poller.mu.Lock()
	state := poller.accounts["/home/me/.claude"]
	wait := state.nextAttempt.Sub(state.lastSeen)
	poller.mu.Unlock()
	if wait < minBackoff {
		t.Fatalf("backoff %v is shorter than %v", wait, minBackoff)
	}
}

func TestPollerHonoursRetryAfter(t *testing.T) {
	rec := newRecorder()
	rec.fail = &TransientError{RetryAfter: time.Hour, err: errTransient}

	poller := NewPoller(context.Background(), 10*time.Millisecond, rec.resolve)
	defer poller.Stop()

	poller.Ensure("/home/me/.claude")
	waitForCall(t, rec)
	time.Sleep(40 * time.Millisecond)

	poller.mu.Lock()
	state := poller.accounts["/home/me/.claude"]
	var wait time.Duration
	if state != nil {
		wait = time.Until(state.nextAttempt)
	}
	poller.mu.Unlock()

	// A server that says how long to wait outranks our own backoff curve.
	if wait < 50*time.Minute {
		t.Fatalf("expected Retry-After to win, next attempt in %v", wait)
	}
}

func TestPollerSuccessResetsBackoff(t *testing.T) {
	rec := newRecorder()
	rec.fail = errors.New("vendor down")
	poller := NewPoller(context.Background(), 10*time.Millisecond, rec.resolve)
	defer poller.Stop()

	poller.Ensure("/home/me/.claude")
	waitForCall(t, rec)

	rec.mu.Lock()
	rec.fail = nil
	rec.mu.Unlock()

	poller.record("/home/me/.claude", nil)

	poller.mu.Lock()
	state := poller.accounts["/home/me/.claude"]
	failures := state.failures
	wait := time.Until(state.nextAttempt)
	poller.mu.Unlock()

	if failures != 0 {
		t.Fatalf("expected the failure count to reset, got %d", failures)
	}
	if wait > poller.interval+time.Second {
		t.Fatalf("expected the normal interval after success, got %v", wait)
	}
}
