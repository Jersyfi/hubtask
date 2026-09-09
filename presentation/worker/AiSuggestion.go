// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AiSuggestion is the queue's way into asking a provider what something should become (J-06): an
// inbound adapter, like every other handler, translating a job into a call on the application
// layer.
//
// Detached, and it has to be. The call reaches a provider between two reads, and a transaction
// held open across it is what observability-reliability.md §8 forbids - the notification
// delivery's reasoning, for the same kind of dependency. What is given up is smaller here than
// there: a process that dies after the provider answered loses the answer, and the retry asks
// again. A duplicate suggestion is a record somebody dismisses.
type AiSuggestion struct {
	Produce suggestion.Produce
}

var (
	_ queue.Handler  = AiSuggestion{}
	_ queue.Detached = AiSuggestion{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (h AiSuggestion) OwnsItsTransactions() {}

// Run asks the provider about the thing the job names.
func (h AiSuggestion) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("suggestions.job_without_tenant")
	}

	targetID, err := payloadID(job, "target_id")
	if err != nil {
		return queue.Result{}, err
	}
	askedBy, err := payloadID(job, "asked_by")
	if err != nil {
		return queue.Result{}, err
	}

	targetType := domain.TargetType(payloadString(job, "target_type"))
	if !targetType.Valid() {
		return queue.Result{}, shared.ErrInternal.
			WithDetail("suggestions.target_type_unknown").
			WithParams(map[string]string{"value": string(targetType)})
	}

	// The job acts for the person who asked, not for the system. That is what makes the read it
	// performs and the consent it is subject to *theirs*: a job with a system actor would read
	// past the permission the asking checked, and would keep sending a workspace's content after
	// the person who asked had lost access to it.
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: job.TenantID, AccountID: askedBy,
	}

	err = h.Produce.Execute(ctx, actor, targetType, targetID, domain.KindFields)
	if suggestion.IsUnavailable(err) {
		// The workspace switched AI off, withdrew consent, or its provider is out of reach
		// between the asking and the running. Finished rather than retried: a retry ladder
		// against a switch somebody turned off is a queue that fills with questions nobody wants
		// answered any more.
		return queue.Result{}, nil
	}
	if err != nil {
		return queue.Result{}, err
	}
	return queue.Result{}, nil
}

// payloadString reads one string out of a job's payload. Absent is empty, and the caller decides
// what that means - here, a target type nothing matches, which is refused by name.
func payloadString(job queue.Job, key string) string {
	value, _ := job.Payload[key].(string)
	return value
}
