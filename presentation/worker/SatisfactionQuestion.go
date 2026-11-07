// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// QuestionSender mails a resolved entry's satisfaction question to its requesters by mail.
type QuestionSender interface {
	Send(ctx context.Context, tenantID, itemID shared.ID) error
}

// SatisfactionQuestion mails the satisfaction question of a resolved entry (UC-SVC-07 check 6).
//
// Detached, RequesterReply's reason.
type SatisfactionQuestion struct{ Questions QuestionSender }

var (
	_ queue.Handler  = SatisfactionQuestion{}
	_ queue.Detached = SatisfactionQuestion{}
)

// OwnsItsTransactions marks the handler detached.
func (SatisfactionQuestion) OwnsItsTransactions() {}

// Run mails the question the job names.
func (h SatisfactionQuestion) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	raw, _ := job.Payload["item_id"].(string)
	id, err := shared.ParseID(raw)
	if job.TenantID.IsZero() || err != nil {
		return queue.Result{}, shared.ErrInternal.WithDetail("servicedesk.question_job_without_entry")
	}
	return queue.Result{}, h.Questions.Send(ctx, job.TenantID, id)
}
