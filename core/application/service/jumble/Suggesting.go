// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// SuggestFromJumbleEntryName is the catalogue name, as domain-model.md §5 has written it under
// Jumble since G-10 with "(AI, optional)" beside it.
const SuggestFromJumbleEntryName = "SuggestFromJumbleEntry"

// EntrySuggestionAskedAction records that somebody asked. Worth a trail entry on its own: it is
// the moment a workspace's content leaves for a provider, and "who sent this there" is a question
// a data protection officer asks (ADR-0018 decision 7).
const EntrySuggestionAskedAction audit.Action = "jumble.suggestion_asked"

// AiAvailability is the one question this use case asks about AI before queuing anything: may this
// workspace's content be sent at all.
//
// A port rather than the provider itself, and narrow on purpose. The use case does not call a
// model - the job does - so what it needs is not a provider but a yes or a no, asked before a job
// exists rather than discovered by a worker three retries later. It is also where ai-first.md §2's
// "checked before every call" is honoured for the *first* time in a request: the second is in the
// resolver, when the job runs, because consent can be withdrawn in between.
type AiAvailability interface {
	// CanSuggest reports whether this workspace has a provider that can complete and has
	// consented to being asked.
	CanSuggest(ctx context.Context, actor appshared.ActorContext) (bool, error)
}

// Jobs is the slice of the queue this use case needs.
type Jobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// SuggestFromJumbleEntry asks the workspace's provider what one entry should become (J-06).
//
// It queues and answers; it does not wait. An AI call reaches somebody else's machine, and
// ai-first.md §2 puts every one of them on a job for that reason - what a caller gets back is
// "asked", and the proposal appears under /suggestions when the provider has answered.
//
// Nothing is created and nothing is changed. That is not a limitation of this task: it is what
// ADR-0012 means by "AI results are always suggestions", and it is why asking twice is allowed -
// two proposals are two records somebody dismisses one of, where two *conversions* would be two
// items somebody has to delete.
type SuggestFromJumbleEntry struct {
	Writer Writer
	// AI is the availability question, and it is optional: an installation built without it is one
	// where nothing can be asked, which answers the same refusal as a workspace that configured
	// no provider.
	AI AiAvailability
	// Queue is where the question goes.
	Queue Jobs
}

// Execute checks, then queues.
func (h SuggestFromJumbleEntry) Execute(
	ctx context.Context, actor appshared.ActorContext, entryID shared.ID,
) error {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		// Asking costs the workspace's own budget and sends its content somewhere, so it asks for
		// what writing an entry asks for rather than for what reading one does.
		Permission: service.PermissionWriteItems,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     EntrySuggestionAskedAction,
		TokenScope: itemsWriteScope,
		TargetType: entryTarget,
		TargetID:   entryID,
	}); err != nil {
		return err
	}

	// The availability question comes before the read, so that a workspace with AI switched off is
	// told so whatever the entry is - and after the permission check, so that it does not become a
	// way to learn what an installation has configured.
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

	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		entry, err := w.Entries.Find(ctx, entryID)
		if err != nil {
			return err
		}
		if entry.Status != domain.StatusNew {
			// A settled entry is one somebody has already decided about. A proposal for it would
			// be a proposal nobody can take, which is an inbox of noise rather than a feature.
			return shared.ErrConflict.WithDetail("jumble.entry_already_settled")
		}

		// The job carries the entry's identity and nothing else. A payload holding the subject and
		// the body would be a second copy of the least trusted text in the system, sitting in a
		// table with no row level security (core/port/queue), and it would be a snapshot the
		// worker might act on after the entry had changed.
		if _, err := h.Queue.Enqueue(ctx, queue.Request{
			Kind:     queue.KindAiSuggest,
			TenantID: actor.TenantID,
			Payload: map[string]any{
				"target_type": "JUMBLE_ENTRY",
				"target_id":   entryID.String(),
				"asked_by":    actor.AccountID.String(),
			},
		}); err != nil {
			return err
		}
		return h.record(ctx, actor, entry.ID)
	})
}

// aiUnavailable is what a workspace without AI is told, and it is the port's own one refusal: the
// caller's answer to "no provider", "not consented" and "the provider is down" is the same one
// (core/port/ai, arc42 QS-09).
var aiUnavailable = shared.ErrUnavailable.WithDetail("ai.unavailable")

func (h SuggestFromJumbleEntry) record(
	ctx context.Context, actor appshared.ActorContext, entryID shared.ID,
) error {
	w := h.Writer
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: w.Clock.Now(),
		Action:     EntrySuggestionAskedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: entryTarget,
		TargetID:   entryID,
		// No changes: nothing changed. What the entry is about does not travel either - the
		// subject and the body are PERSONAL_CONTENT and an audit entry is not where they go
		// (rule 10).
	})
}

// Descriptor is the catalogue entry - and, through it, the MCP tool and the automation action.
func (h SuggestFromJumbleEntry) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SuggestFromJumbleEntryName,
		Summary: "Asks the workspace's AI provider to propose a title, notes, a due date and " +
			"labels for one jumble entry. It proposes and changes nothing: the proposal is read " +
			"through the suggestions, and the entry becomes work only when somebody converts it. " +
			"Asking twice produces two proposals rather than a refusal.",
		SideEffects: "Queues one question to the provider and writes an audit entry. Creates " +
			"nothing and changes no entry.",
		TokenScope: itemsWriteScope,
		Input: []usecase.Field{
			{
				Name: "entry_id", Kind: usecase.KindID, Required: true,
				Description: "Which entry. A rule leaves this out and the run supplies the " +
					"entry it is about.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: EntrySuggestionAskedAction, TargetType: entryTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An entry is not an item, and the item history is keyed on an entry that " +
				"does not exist until somebody converts this one.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SuggestFromJumbleEntry) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	entryID, err := in.ID("entry_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, entryID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
