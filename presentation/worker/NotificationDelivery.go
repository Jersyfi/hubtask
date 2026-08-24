// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// NotificationDelivery is the queue's way into sending one notification (C-09): an inbound
// adapter, like every other handler, translating a job into a call on the application layer.
//
// Detached, and it has to be. The delivery reaches an SMTP server between two writes, and a
// transaction held open across that call is what observability-reliability.md §8 forbids - the
// same reason the media reconciliation is detached, for the same kind of dependency. What is given
// up is the atomicity everybody else gets for free: a process that dies between the send and the
// write-back leaves a message sent and a record that still says pending, and the retry sends it
// again. That is the right side of the trade for email - a duplicate is a nuisance, a lost one is
// the thing somebody needed to know.
type NotificationDelivery struct {
	Delivery notification.DeliverNotification
}

var (
	_ queue.Handler  = NotificationDelivery{}
	_ queue.Detached = NotificationDelivery{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for. Its value is never read;
// implementing the interface is the declaration.
func (h NotificationDelivery) OwnsItsTransactions() {}

// Run sends the notification the job names.
func (h NotificationDelivery) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// Every notification belongs to somebody's workspace, and the transactions this handler
		// opens are bound to it. A job without one is a programming error, not an empty send.
		return queue.Result{}, shared.ErrInternal.WithDetail("notifications.job_without_tenant")
	}

	notificationID, err := payloadID(job, "notification_id")
	if err != nil {
		return queue.Result{}, err
	}

	// The attempt budget is the queue's, and it is passed rather than recomputed: a handler that
	// counted its own would be a second budget disagreeing with the first (queue.Job.LastAttempt).
	if err := h.Delivery.Execute(ctx, job.TenantID, notificationID, job.LastAttempt()); err != nil {
		return queue.Result{}, err
	}
	// Finished rather than repeated: one job is one message, so that a refused address holds up
	// nobody else's mail.
	return queue.Result{}, nil
}

// payloadID reads one identifier out of a job's payload, and refuses what is not one.
//
// A payload is data that outlived the process that wrote it (core/port/queue), so a key that is
// missing or is not an identifier is a defect rather than something to work around - and it is
// parsed rather than trusted, because an identifier reaching a query unparsed is the shape of T-06
// even where the query is parameterised.
func payloadID(job queue.Job, key string) (shared.ID, error) {
	malformed := shared.ErrInternal.
		WithDetail("notifications.payload_malformed").
		WithParams(map[string]string{"field": key})

	raw, present := job.Payload[key]
	if !present {
		return "", malformed
	}
	text, ok := raw.(string)
	if !ok {
		return "", malformed
	}
	id, err := shared.ParseID(text)
	if err != nil {
		return "", malformed
	}
	return id, nil
}
