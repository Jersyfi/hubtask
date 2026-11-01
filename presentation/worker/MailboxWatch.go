// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"
	"time"

	mailservice "github.com/Jersyfi/hubtask/core/application/service/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// MailboxHolds is the application's side of the watch: which mailboxes live, and one hold of one.
type MailboxHolds interface {
	Targets(ctx context.Context, limit int) ([]mailservice.WatchTarget, error)
	// Wanted is how many mailboxes' reads live, beyond the limit.
	Wanted(ctx context.Context) (int64, error)
	Hold(ctx context.Context, target mailservice.WatchTarget, hold time.Duration) (string, error)
}

// MailboxWatchSignals is the slice of the metrics adapter the watch uses.
type MailboxWatchSignals interface {
	// MailboxWatchesHeld is how many mailboxes this process holds open now.
	MailboxWatchesHeld(ctx context.Context, held int64)
	// MailboxWatchesUnheld is how many mailboxes the limit leaves to the minute's read - A-38's
	// number: those workspaces lose the arrival in seconds, and nothing else says so.
	MailboxWatchesUnheld(ctx context.Context, unheld int64)
	// MailboxWatchEnded counts one hold by what it came to (mailservice.Watch*).
	MailboxWatchEnded(ctx context.Context, outcome string)
}

const (
	mailboxWatchTick  = time.Minute
	mailboxWatchHold  = 25 * time.Minute
	mailboxWatchLimit = 1000
	// mailboxWatchRest bounds how often an arrival wakes a read: a server that announces mail on
	// every hold costs a read every few seconds, not a loop.
	mailboxWatchRest = 5 * time.Second
	// mailboxWatchRetry is when a mailbox that cannot be held (no IDLE, not IMAP) is tried again.
	mailboxWatchRetry   = time.Hour
	mailboxWatchBackoff = 5 * time.Second
	mailboxWatchCeiling = 5 * time.Minute
)

// MailboxWatch holds the connected IMAP mailboxes open (IDLE) so mail reaches the desk in seconds
// (UC-JUM-11 check 10). One process of the installation holds them all, by an advisory lock of
// its own - two would sign every mailbox in twice and providers count connections. Everything it
// does is an acceleration: a hold that fails, a lost lock or a stopped process leave the minute's
// read, which never depends on it. Between a lost lock and the former holder's next tick both may
// hold a mailbox; that costs a connection, never a message, since a wake is the read's dedupe key.
type MailboxWatch struct {
	Holds      MailboxHolds
	Leadership queue.Leadership
	Signals    MailboxWatchSignals
	// SchemaBehind stands the watch by while the database lacks migrations this build needs.
	SchemaBehind func(ctx context.Context) bool
	// Tick, Hold, Limit, Rest, Retry, Backoff and Ceiling default to the constants above; tests
	// shorten them.
	Tick, Hold, Rest, Retry, Backoff, Ceiling time.Duration
	Limit                                     int
}

type mailboxHolder struct {
	cancel context.CancelFunc
	done   chan struct{}
	// endedAt is written before done closes, so it is read only after.
	endedAt time.Time
}

// Run is the loop. It returns when the context ends, letting every mailbox and the lock go.
func (m MailboxWatch) Run(ctx context.Context) {
	if m.Holds == nil || m.Leadership == nil {
		slog.ErrorContext(ctx, "the mailbox watch did not start", slog.String("error", "queue.mailbox_watch_incomplete"))
		return
	}
	m = m.withDefaults()
	holders := map[mailservice.WatchTarget]*mailboxHolder{}
	defer func() {
		m.stopAll(holders)
		released, cancel := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
		defer cancel()
		_ = m.Leadership.Release(released)
	}()
	for {
		m.tick(ctx, holders)
		if !wait(ctx, m.Tick) {
			return
		}
	}
}

func (m MailboxWatch) withDefaults() MailboxWatch {
	defaults := []struct {
		value    *time.Duration
		fallback time.Duration
	}{
		{&m.Tick, mailboxWatchTick}, {&m.Hold, mailboxWatchHold}, {&m.Rest, mailboxWatchRest},
		{&m.Retry, mailboxWatchRetry}, {&m.Backoff, mailboxWatchBackoff}, {&m.Ceiling, mailboxWatchCeiling},
	}
	for _, d := range defaults {
		if *d.value <= 0 {
			*d.value = d.fallback
		}
	}
	if m.Limit <= 0 {
		m.Limit = mailboxWatchLimit
	}
	return m
}

// tick takes or confirms the lock, then holds what lives and lets go of what does not.
func (m MailboxWatch) tick(ctx context.Context, holders map[mailservice.WatchTarget]*mailboxHolder) {
	bounded, cancel := context.WithTimeout(ctx, bookkeepingTimeout)
	defer cancel()
	if !m.leading(bounded) {
		m.stopAll(holders)
		m.held(ctx, holders)
		return
	}
	targets, err := m.Holds.Targets(bounded, m.Limit)
	if err != nil {
		// The holds that run keep running: a database that does not answer a list for a minute
		// is no reason to drop mailboxes that are held fine.
		slog.WarnContext(ctx, "listing the mailboxes to hold failed", slog.String("error", shared.AsError(err).Code))
		return
	}
	if m.Signals != nil {
		if total, err := m.Holds.Wanted(bounded); err == nil {
			m.Signals.MailboxWatchesUnheld(ctx, max(total-int64(len(targets)), 0))
		}
	}
	wanted := make(map[mailservice.WatchTarget]bool, len(targets))
	for _, target := range targets {
		wanted[target] = true
	}
	for target, holder := range holders {
		if !wanted[target] {
			holder.cancel()
			<-holder.done
			delete(holders, target)
		}
	}
	now := time.Now()
	for _, target := range targets {
		if holder, ok := holders[target]; ok {
			select {
			case <-holder.done:
				if now.Sub(holder.endedAt) < m.Retry {
					continue
				}
			default:
				continue
			}
		}
		holders[target] = m.start(ctx, target)
	}
	m.held(ctx, holders)
}

func (m MailboxWatch) leading(ctx context.Context) bool {
	if m.SchemaBehind != nil && m.SchemaBehind(ctx) {
		_ = m.Leadership.Release(ctx)
		return false
	}
	leading, err := m.Leadership.Confirm(ctx)
	if err == nil && !leading {
		leading, err = m.Leadership.Acquire(ctx)
	}
	if err != nil {
		slog.WarnContext(ctx, "the mailbox watch's lock could not be asked", slog.String("error", shared.AsError(err).Code))
		return false
	}
	return leading
}

// start holds one mailbox until its context ends or it cannot be held.
func (m MailboxWatch) start(ctx context.Context, target mailservice.WatchTarget) *mailboxHolder {
	holdCtx, cancel := context.WithCancel(ctx)
	holder := &mailboxHolder{cancel: cancel, done: make(chan struct{})}
	concurrency.Go(holdCtx, "worker.mailbox_watch", func(ctx context.Context) {
		defer close(holder.done)
		defer func() { holder.endedAt = time.Now() }()
		failures := 0
		for ctx.Err() == nil {
			outcome, err := m.Holds.Hold(ctx, target, m.Hold)
			if ctx.Err() != nil {
				return
			}
			if m.Signals != nil {
				m.Signals.MailboxWatchEnded(ctx, outcome)
			}
			pause := time.Duration(0)
			switch outcome {
			case mailservice.WatchGone, mailservice.WatchUnsupported:
				return
			case mailservice.WatchArrived:
				failures, pause = 0, m.Rest
			case mailservice.WatchQuiet:
				failures = 0
			default:
				failures++
				pause = m.backoff(failures)
				slog.WarnContext(ctx, "holding a mailbox failed", slog.String("error", shared.AsError(err).DetailCode),
					slog.Int("failures", failures))
			}
			if !wait(ctx, pause) {
				return
			}
		}
	})
	return holder
}

// backoff doubles from Backoff up to Ceiling, so a server that is down is asked a few times an
// hour rather than every few seconds.
func (m MailboxWatch) backoff(failures int) time.Duration {
	pause := m.Backoff
	for range failures - 1 {
		pause *= 2
		if pause >= m.Ceiling {
			return m.Ceiling
		}
	}
	return pause
}

func (m MailboxWatch) stopAll(holders map[mailservice.WatchTarget]*mailboxHolder) {
	for target, holder := range holders {
		holder.cancel()
		<-holder.done
		delete(holders, target)
	}
}

func (m MailboxWatch) held(ctx context.Context, holders map[mailservice.WatchTarget]*mailboxHolder) {
	if m.Signals == nil {
		return
	}
	live := 0
	for _, holder := range holders {
		select {
		case <-holder.done:
		default:
			live++
		}
	}
	m.Signals.MailboxWatchesHeld(ctx, int64(live))
}
