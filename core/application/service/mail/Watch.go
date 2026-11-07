// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"context"
	"errors"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// What one hold of a mailbox came to.
const (
	// WatchArrived: mail arrived and the mailbox's read is due now.
	WatchArrived = "arrived"
	// WatchQuiet: the hold passed and nothing came; the next hold follows.
	WatchQuiet = "quiet"
	// WatchGone: the mailbox is removed, switched off or not IMAP; nothing holds it.
	WatchGone = "gone"
	// WatchUnsupported: the server cannot announce mail; the minute's read alone serves it.
	WatchUnsupported = "unsupported"
	// WatchFailed: the hold could not start or broke off; the caller waits and tries again.
	WatchFailed = "failed"
)

// WatchMailbox holds a connected IMAP mailbox open (IDLE) and pulls its read forward when mail
// arrives, so the desk sees it in seconds rather than at the next minute (UC-JUM-11 check 10). It
// reads nothing itself: the read stays PollMailbox's, which also stays the fallback for a mailbox
// nobody holds.
type WatchMailbox struct {
	Boxes   Boxes
	Watcher mailboxport.Watcher
	// Keys lists the mailboxes whose read lives: one open mail.mailbox_poll job each, keyed by the
	// mailbox, so the list crosses no table with a tenant boundary.
	Keys queue.OpenKeys
}

// WatchTarget is a mailbox to hold, by its workspace.
type WatchTarget struct {
	TenantID  shared.ID
	MailboxID shared.ID
}

// Targets answers up to limit mailboxes of the shard's workspaces whose read lives.
func (w WatchMailbox) Targets(ctx context.Context, shard queue.Shard, limit int) ([]WatchTarget, error) {
	var keys []queue.OpenKey
	err := w.Boxes.Writer.UnitOfWork.WithinReadOnly(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		var err error
		keys, err = w.Keys.OpenKeys(ctx, queue.KindMailboxPoll, shard, limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]WatchTarget, 0, len(keys))
	for _, key := range keys {
		id, err := shared.ParseID(key.DedupeKey)
		if err != nil || key.TenantID.IsZero() {
			continue
		}
		out = append(out, WatchTarget{TenantID: key.TenantID, MailboxID: id})
	}
	return out, nil
}

// Wanted is how many mailboxes of the shard's workspaces have a living read, beyond any limit:
// what a watch holding fewer leaves to the minute's read.
func (w WatchMailbox) Wanted(ctx context.Context, shard queue.Shard) (int64, error) {
	var wanted int64
	err := w.Boxes.Writer.UnitOfWork.WithinReadOnly(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		var err error
		wanted, err = w.Keys.OpenKeyCount(ctx, queue.KindMailboxPoll, shard)
		return err
	})
	return wanted, err
}

// Hold holds one mailbox for up to hold and answers what it came to; an error comes with
// WatchFailed and is coded, never the server's words.
func (w WatchMailbox) Hold(ctx context.Context, target WatchTarget, hold time.Duration) (string, error) {
	b := w.Boxes
	scope := persistence.Scope{TenantID: target.TenantID}
	box, opened, err := b.load(ctx, scope, target.MailboxID)
	if errors.Is(err, shared.ErrNotFound) {
		return WatchGone, nil
	}
	if err != nil {
		return WatchFailed, err
	}
	if !box.Enabled || box.Protocol != domain.ProtocolIMAP {
		return WatchGone, nil
	}
	password, err := b.credential(ctx, scope, box, opened)
	if err != nil {
		return WatchFailed, err
	}
	arrived, err := w.Watcher.Watch(ctx, mailboxport.Connection{Mailbox: box, Secret: password}, hold)
	if err != nil {
		if codeOf(err) == "mail.mailbox_idle_unsupported" {
			return WatchUnsupported, nil
		}
		return WatchFailed, err
	}
	if !arrived {
		return WatchQuiet, nil
	}
	if err := b.seed(ctx, box, b.Writer.Clock.Now()); err != nil {
		return WatchFailed, err
	}
	return WatchArrived, nil
}
