// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// BotRunner reads one chat bot's room and answers when to read next; zero for never.
type BotRunner interface {
	Run(ctx context.Context, tenantID, botID shared.ID) (time.Time, error)
}

// ChatBot is one chat bot's read of its room (UC-INT-18): one job per bot, rescheduling itself
// every minute and finishing when the bot is gone, off or refused its token.
//
// Detached: the room is on the homeserver, and no transaction stays open across it.
type ChatBot struct {
	Bots BotRunner
	// Now is the clock the wait is measured on; nil is the system's.
	Now func() time.Time
}

var (
	_ queue.Handler  = ChatBot{}
	_ queue.Detached = ChatBot{}
)

// OwnsItsTransactions marks the handler detached.
func (ChatBot) OwnsItsTransactions() {}

// Run reads the room of the bot the job names.
func (h ChatBot) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	raw, _ := job.Payload["bot_id"].(string)
	id, err := shared.ParseID(raw)
	if job.TenantID.IsZero() || err != nil {
		return queue.Result{}, shared.ErrInternal.WithDetail("chatbot.job_without_bot")
	}
	next, err := h.Bots.Run(ctx, job.TenantID, id)
	if err != nil {
		return queue.Result{}, err
	}
	if next.IsZero() {
		return queue.Result{}, nil
	}
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	return queue.Result{Repeat: true, RepeatAfter: waitUntil(now(), next, time.Hour)}, nil
}

// CardTeller posts one approval's card to one linked chat account.
type CardTeller interface {
	TellApproval(ctx context.Context, tenantID, linkID, approvalID shared.ID, txn string) error
}

// ChatApprovalCard posts an approval's card to a linked chat account (UC-SVC-09 check 6); a
// platform out of reach is retried by the queue.
//
// Detached: the platform is outside, and no transaction stays open across it.
type ChatApprovalCard struct {
	Bots CardTeller
}

var (
	_ queue.Handler  = ChatApprovalCard{}
	_ queue.Detached = ChatApprovalCard{}
)

// OwnsItsTransactions marks the handler detached.
func (ChatApprovalCard) OwnsItsTransactions() {}

// Run posts the card the job names.
func (h ChatApprovalCard) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	rawLink, _ := job.Payload["link_id"].(string)
	rawApproval, _ := job.Payload["approval_id"].(string)
	txn, _ := job.Payload["txn"].(string)
	link, err := shared.ParseID(rawLink)
	approval, aerr := shared.ParseID(rawApproval)
	if job.TenantID.IsZero() || err != nil || aerr != nil || txn == "" {
		return queue.Result{}, shared.ErrInternal.WithDetail("chatbot.link_job_without_link")
	}
	return queue.Result{}, h.Bots.TellApproval(ctx, job.TenantID, link, approval, txn)
}
