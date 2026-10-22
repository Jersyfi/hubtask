// SPDX-License-Identifier: Apache-2.0
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

// AiSuggestionArrival is the queue's way into a proposal the workspace prepares about one jumble
// arrival (UC-JUM-07 check 10). Detached for AiSuggestion's reason: a provider sits between two
// reads.
//
// The actor is the system acting for the workspace - a tenant and no account - because nobody
// asked: the workspace switched it on (UC-JUM-07 check 10), and the embedding pass is the other work
// done that way.
type AiSuggestionArrival struct {
	Produce suggestion.Produce
}

var (
	_ queue.Handler  = AiSuggestionArrival{}
	_ queue.Detached = AiSuggestionArrival{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (h AiSuggestionArrival) OwnsItsTransactions() {}

// Run asks about the arrival the job names.
func (h AiSuggestionArrival) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("suggestions.job_without_tenant")
	}
	targetID, err := payloadID(job, "target_id")
	if err != nil {
		return queue.Result{}, err
	}
	request := suggestion.Request{
		TargetType: domain.TargetJumbleEntry, TargetID: targetID, Kind: domain.KindFields,
		OnArrival: true,
	}
	// Who made the arrival, where an account did: absent for a mail or a webhook.
	for key, into := range map[string]*shared.ID{"submitted_by": &request.SubmittedBy, "on_behalf_of": &request.OnBehalfOf} {
		if _, named := job.Payload[key]; !named {
			continue
		}
		if *into, err = payloadID(job, key); err != nil {
			return queue.Result{}, err
		}
	}

	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}
	outcome, err := h.Produce.Ask(ctx, actor, request)
	if suggestion.IsUnavailable(err) {
		// AI switched off, consent withdrawn or the budget spent since the arrival: ended refused
		// with the refusal's code and parameters, AiSuggestion's reasoning - the card that follows
		// this job says a spent budget once.
		refused := shared.AsError(err)
		return queue.Result{Refusal: &queue.Refusal{Code: refused.DetailCode, Params: refused.Params}}, nil
	}
	if err != nil {
		return queue.Result{}, err
	}
	return queue.Result{Outcome: outcome.Code}, nil
}
