// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// SuggestDecompositionName is the catalogue name, registered in core/application/catalogue the way
// every use case name is - the catalogue is the list a person, an agent and a rule all read, and a
// use case that is not in it is reachable through none of the three.
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

// Ask is what every "ask the provider about this entry" use case is made of.
//
// One shape rather than four near-copies, because they differ in three values - the name, the
// prompt and what the answer may set - and in nothing else. What they share is the part worth
// having once: the permission, the availability question in the right order, the read that proves
// the entry exists, the job, and the audit entry.
type Ask struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// asking is one question to queue: what it is about, what it asks, and what it carries.
type asking struct {
	TargetType domain.TargetType
	TargetID   shared.ID
	Action     audit.Action
	Kind       domain.Kind
	PromptID   string
	Apply      bool
	// Text is what a question is asked *with*, where it has no material of its own to read: a
	// template is drafted from a description that exists in no row the workspace holds. It is held
	// under row level security for the job that will read it, and the job's payload names the row -
	// the queue carries references and nothing else. Empty for every question that reads its
	// material from the target.
	Text string
	// Scope is where a rule drafted from the question would apply; it travels as two references.
	// The zero value for every other question.
	Scope automation.Scope
	// Access is what asking needs where it is not writing entries at the workspace - drafting a
	// rule asks what writing one asks. Nil takes the default.
	Access *access.Request
	// JobKind is the queue kind the question runs as, and Attempts its budget. Empty and zero are
	// the ordinary AI question.
	JobKind  queue.Kind
	Attempts int
	// Reads marks a reading aid (UC-AI-04 check 8): asked by everybody who may read the entry,
	// unless the answer is to be applied, which changes it.
	Reads bool
	// Fields are the fields of a field set the person asked about, travelling on the job; empty
	// asks for every field the prompt keeps.
	Fields []string
}

// queue checks and queues one question.
//
// The target type travels because the shape has more than one: a collection's status is a
// question about a container, and everything else about asking it - the permission, the
// availability, the read that proves it exists, the job, the audit entry - is the same.
func (a Ask) queue(ctx context.Context, actor appshared.ActorContext, q asking) (shared.ID, error) {
	if _, asked := AsksAbout(q.PromptID, q.TargetType, q.Kind); !asked {
		// Before the permission, the consent and the audit entry, because those are what a
		// refusal in the worker would already have spent. A pair nothing declares is a defect in
		// this build rather than anything about the caller.
		return shared.ID(""), shared.ErrInternal.WithDetail("ai.prompt_target_unknown").
			WithParams(map[string]string{
				"prompt": q.PromptID, "target_type": string(q.TargetType),
			})
	}

	c := a.Cases
	permission := access.Request{
		// Asking spends the workspace's budget and sends its content somewhere, so it asks for
		// what writing asks for rather than for what reading does.
		Permission: service.PermissionWriteItems,
		Path:       []identity.Scope{identity.TenantScope()},
		TokenScope: suggestionsWrite,
		TargetType: suggestionTarget,
		TargetID:   q.TargetID,
	}
	// A question about an entry reads the entry first and asks its permission along the entry's
	// own path, so that a role held on its hub, its collection or the entry itself counts
	// (domain-model.md §3.2, P-05). The read is the existence and visibility question at once: an
	// entry the caller cannot read answers not-found exactly as reading it does, with the refusal
	// that read records - so nothing is learned from asking first.
	read := false
	switch {
	case q.Access != nil:
		permission = *q.Access
	case q.TargetType == domain.TargetWorkItem:
		var subject Subject
		if err := c.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			subject, err = c.Targets.Subject(ctx, actor, q.TargetType, q.TargetID)
			return err
		}); err != nil {
			return shared.ID(""), err
		}
		read = true
		if q.Reads && !q.Apply {
			// A reading aid admits a token that reads entries, and one that asks for suggestions
			// as it did before the aid was offered to readers: no token that worked stops working.
			permission = readsEntry(subject, q.TargetID, q.Action, q.TargetID)
			permission.TokenScope = suggestionsRead
			if actor.HasScope(suggestionsWrite) && !actor.HasScope(suggestionsRead) {
				permission.TokenScope = suggestionsWrite
			}
		} else {
			permission = writesEntry(subject, q.TargetID, q.Action, q.TargetID)
			permission.TokenScope = suggestionsWrite
		}
	case q.TargetType == domain.TargetContainer:
		// About a collection: along its own path, so that a role on its hub counts; the
		// authoriser fills in the hub, and answers an outsider of a private hub that there is no
		// such collection.
		permission.Path = collectionPath(q.TargetID)
	}
	permission.Action = q.Action
	if err := c.Authorizer.Authorize(ctx, actor, permission); err != nil {
		return shared.ID(""), err
	}

	// After the permission check, so that it cannot become a way to learn what an installation has
	// configured; before the read, so that a workspace with AI switched off is told so whatever
	// the entry is.
	if a.AI == nil {
		return shared.ID(""), aiUnavailable
	}
	available, err := a.AI.CanSuggest(ctx, actor)
	if err != nil {
		return shared.ID(""), err
	}
	if !available {
		return shared.ID(""), aiUnavailable
	}

	question := Question{
		TargetType: q.TargetType, TargetID: q.TargetID, PromptID: q.PromptID,
		Scope: q.Scope, Words: q.Text,
	}
	// The entry is read through its own use case, which is the permission check and the existence
	// check at once - and it is read *before* the job, so that asking about something that is not
	// there fails now rather than in a worker. In a read of its own rather than inside the write
	// below: the target's reader records its refusal where it is asked from, and inside a
	// read-write transaction that this refusal then rolls back, the DENIED entry went with it.
	if err := c.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if !read {
			if _, err := c.Targets.Digest(ctx, actor, q.TargetType, q.TargetID); err != nil {
				return err
			}
		}
		return c.workable(ctx, actor, question)
	}); err != nil {
		return shared.ID(""), err
	}

	jobKind := q.JobKind
	if jobKind == "" {
		jobKind = queue.KindAiSuggest
	}
	var jobID shared.ID
	err = c.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		payload := map[string]any{
			"target_type": string(q.TargetType),
			"target_id":   q.TargetID.String(),
			"kind":        string(q.Kind),
			"prompt":      q.PromptID,
			"asked_by":    actor.AccountID.String(),
		}
		if q.Apply {
			// `automation.md` §1.3's "or applied directly, configured explicitly". The flag is on
			// the job rather than a second job kind, because what differs is one step at the end;
			// and it is written only when it is true, so a job without it is a proposal - an AI
			// result is a suggestion unless somebody said otherwise (ai-first.md §2).
			payload["apply"] = true
		}
		if len(q.Fields) > 0 {
			named := make([]any, 0, len(q.Fields))
			for _, field := range q.Fields {
				named = append(named, field)
			}
			payload["fields"] = named
		}
		if q.Scope.Type != "" {
			// References, as everything a job carries: the scope's kind and its container.
			payload["scope_type"] = string(q.Scope.Type)
			if !q.Scope.ID.IsZero() {
				payload["scope_id"] = q.Scope.ID.String()
			}
		}
		if q.Text != "" {
			if c.Requests == nil || c.IDs == nil {
				return shared.ErrInternal.WithDetail("suggestions.requests_not_wired")
			}
			requestID := c.IDs.NewID()
			if err := c.Requests.Put(ctx, repository.Request{
				ID: requestID, AskedBy: actor.AccountID, Text: q.Text, CreatedAt: c.Clock.Now(),
			}); err != nil {
				return err
			}
			payload["request_id"] = requestID.String()
		}
		queued, err := a.Queue.Enqueue(ctx, queue.Request{
			Kind: jobKind, TenantID: actor.TenantID, Payload: payload, MaxAttempts: q.Attempts,
		})
		if err != nil {
			return err
		}
		jobID = queued
		return c.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: c.Clock.Now(),
			Action:     q.Action,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityNotice,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: permission.TargetType,
			TargetID:   permission.TargetID,
			Changes: audit.Changes(
				audit.Change{Field: "prompt", Classification: audit.Open, To: q.PromptID},
				audit.Change{Field: "applied_directly", Classification: audit.Open,
					To: boolText(q.Apply)},
			),
		})
	})
	if err != nil {
		return shared.ID(""), err
	}
	return jobID, nil
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// SuggestDecomposition asks the workspace's provider what work sits under one entry.
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
) (shared.ID, error) {
	return Ask(h).queue(ctx, actor, asking{
		TargetType: domain.TargetWorkItem, TargetID: itemID,
		Action: DecompositionAskedAction, Kind: domain.KindDecomposition, PromptID: "decompose",
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
	jobID, err := h.Execute(ctx, actor, itemID)
	if err != nil {
		return nil, err
	}
	return queued(jobID), nil
}

// queued is what every ask answers: the job that will put the question, which is what a client
// follows to learn that the job met a refusal rather than an answer.
func queued(jobID shared.ID) usecase.Output {
	return usecase.Output{"job_id": jobID.String()}
}

// workable reads the material the producer will read, with the same exclusion: a person who keeps
// their own content out is told no AI action is theirs (ai.kept_out), and a question with nothing
// left to work with is refused in the words an empty entry gets (ai.nothing_to_work_with) - the
// one answer for both, so that it says nothing about who wrote what. A question asked with words
// of its own (a template's description) always has something to work with.
func (c Cases) workable(
	ctx context.Context, actor appshared.ActorContext, question Question,
) error {
	if c.Material == nil {
		return nil
	}
	promptID, text := question.PromptID, question.Words
	material, err := c.Material.Material(ctx, actor, question)
	if err != nil {
		return err
	}
	if refused := material.Kept.Refusal(actor.AccountID); refused != nil {
		return refused
	}
	if text == "" && strings.TrimSpace(material.Content) == "" {
		return integration.NothingToWorkWith()
	}
	if len(choiceSets[promptID]) > 0 && len(material.Choices) == 0 && len(material.Declared) == 0 {
		// A question that may only choose - labels the collection agreed on, the board's columns,
		// the closed fields it declared - with nothing to choose from can only answer nothing, and
		// would spend the workspace's budget to learn it. Refused rather than queued, so that the
		// client, which reads the same three lists, offers it exactly where it can succeed (P-05).
		return nothingToChooseFrom
	}
	return nil
}

// nothingToChooseFrom refuses a classification of an entry whose collection offers no label, no
// column it could move to and no closed field: not "empty" - the entry has words - but nothing to
// pick, which is a different sentence.
var nothingToChooseFrom = shared.ErrValidation.WithDetail("ai.nothing_to_choose_from")
