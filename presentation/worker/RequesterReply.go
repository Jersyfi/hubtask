// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// ReplySender mails one reply to the requesters an entry was filed for by mail.
type ReplySender interface {
	Send(ctx context.Context, tenantID, commentID shared.ID) error
}

// RequesterReply mails a public reply to the requesters of an entry filed by mail (UC-SVC-12).
//
// Detached: the mail goes to somebody else's server, and no transaction stays open across it.
type RequesterReply struct{ Replies ReplySender }

var (
	_ queue.Handler  = RequesterReply{}
	_ queue.Detached = RequesterReply{}
)

// OwnsItsTransactions marks the handler detached.
func (RequesterReply) OwnsItsTransactions() {}

// Run mails the reply the job names.
func (h RequesterReply) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	raw, _ := job.Payload["comment_id"].(string)
	id, err := shared.ParseID(raw)
	if job.TenantID.IsZero() || err != nil {
		return queue.Result{}, shared.ErrInternal.WithDetail("servicedesk.reply_job_without_comment")
	}
	return queue.Result{}, h.Replies.Send(ctx, job.TenantID, id)
}
