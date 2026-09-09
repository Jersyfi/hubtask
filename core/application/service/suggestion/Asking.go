// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// SuggestDecompositionName is the catalogue name. It is added to domain-model.md §5 in the pull
// request that builds it, the way every use case name in this project arrives - the catalogue is
// the list a person, an agent and a rule all read, and a use case that is not in it is reachable
// through none of the three.
const SuggestDecompositionName = "SuggestDecomposition"

// DecompositionAskedAction records that somebody asked. Its own action rather than the jumble's,
// because "what was sent to a provider, and about what" is the question a data protection officer
// asks, and one action covering two features answers it less well.
const DecompositionAskedAction audit.Action = "ai.decomposition_asked"

// AiAvailability is the one question asked before anything is queued: may this workspace's content
// be sent at all (ai-first.md §2). The same seam the jumble's asking uses, declared here because
// the application layer may not import a sibling service.
type AiAvailability interface {
	CanSuggest(ctx context.Context, actor appshared.ActorContext) (bool, error)
}

// Jobs is the slice of the queue this use case needs.
type Jobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// SuggestDecomposition asks the workspace's provider what work sits under one entry (J-07).
//
// The second of the two suggestions the roadmap names, and the one that uses the level model for
// what it is for: a task the size of a project described in a sentence, and the work packages and
// activities that would carry it.
//
// It queues and answers, like the jumble's asking and for the same reasons - the AI call is
// somebody else's machine, and a proposal is a record rather than a change. What differs is only
// what is proposed.
type SuggestDecomposition struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// Execute checks, then queues.
func (h SuggestDecomposition) Execute(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) error {
	c := h.Cases
	if err := c.Authorizer.Authorize(ctx, actor, access.Request{
		// Asking spends the workspace's budget and sends its content somewhere, so it asks for
		// what writing asks for rather than for what reading does - the jumble's reasoning.
		Permission: service.PermissionWriteItems,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     DecompositionAskedAction,
		TokenScope: suggestionsWrite,
		TargetType: suggestionTarget,
		TargetID:   itemID,
	}); err != nil {
		return err
	}

	// After the permission check, so that it cannot become a way to learn what an installation has
	// configured; before the read, so that a workspace with AI switched off is told so whatever
	// the entry is.
	if h.AI == nil {
		return aiUnavailable
	}
	available, err := h.AI.CanSuggest(ctx, actor)
	if err != nil {
		return err
	}
	if !available {
		return aiUnavailable
	}

	return c.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// The entry is read through its own use case, which is the permission check and the
		// existence check at once - and it is read *before* the job, so that asking about
		// something that is not there fails now rather than in a worker.
		if _, err := c.Targets.Digest(ctx, actor, domain.TargetWorkItem, itemID); err != nil {
			return err
		}
		if _, err := h.Queue.Enqueue(ctx, queue.Request{
			Kind:     queue.KindAiSuggest,
			TenantID: actor.TenantID,
			Payload: map[string]any{
				"target_type": string(domain.TargetWorkItem),
				"target_id":   itemID.String(),
				"kind":        string(domain.KindDecomposition),
				"asked_by":    actor.AccountID.String(),
			},
		}); err != nil {
			return err
		}
		return c.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: c.Clock.Now(),
			Action:     DecompositionAskedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityNotice,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: suggestionTarget,
			TargetID:   itemID,
			// No changes: nothing changed. And no content - the title and the notes are the
			// entry's own and an audit entry is not where they go (rule 10).
		})
	})
}

// aiUnavailable is the port's one refusal, spelled here so that this package does not import the
// AI port for a sentinel: the caller's answer to "no provider", "not consented" and "out of reach"
// is the same one (arc42 QS-09).
var aiUnavailable = shared.ErrUnavailable.WithDetail("ai.unavailable")

// Descriptor is the catalogue entry.
func (h SuggestDecomposition) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SuggestDecompositionName,
		Summary: "Asks the workspace's AI provider what work sits under one entry: the work " +
			"packages and the activities that would carry it. It proposes and changes nothing - " +
			"the breakdown is read through the suggestions and is created only when somebody " +
			"accepts it, one ordinary create at a time with their own rights at each destination.",
		SideEffects: "Queues one question to the provider and writes an audit entry. Creates " +
			"nothing and changes no entry.",
		TokenScope: suggestionsWrite,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry to break down."},
		},
		Audit: usecase.AuditDeclaration{
			Action: DecompositionAskedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Asking changes no entry; the creates an acceptance performs write their own " +
				"history, which is where the breakdown appears.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SuggestDecomposition) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, itemID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
