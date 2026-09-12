// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The three AI actions `automation.md` §1.3 documents and `deferredActions` has refused by name
// since G-05, with a code that said "not built yet" and a comment naming the milestone that would
// build them (J-08).
//
// Three use cases rather than one with a mode, because an automation action *is* a use case: the
// kind a rule names is derived from the name (`usecase.Descriptor.AutomationAction`), so
// `AI_SUGGEST_FIELDS` exists exactly when `AiSuggestFields` does. That is the parity design working
// as intended - a rule cannot name something a person and an agent cannot also reach.
//
// What differs between them is the prompt and what the answer may set; what they share is `Ask`.
const (
	AiSuggestFieldsName = "AiSuggestFields"
	AiSummarizeName     = "AiSummarize"
	AiClassifyName      = "AiClassify"
	// The other two thirds of §2's Summarisation row (K-05): the same target and different
	// material, and a target of its own.
	AiSummarizeThreadName    = "AiSummarizeThread"
	AiSummarizeContainerName = "AiSummarizeContainer"
)

// The three actions' own audit codes. One each rather than one shared, because "what was sent to a
// provider, and what for" is the question ADR-0018 decision 7 asks, and one action covering three
// features answers it less well.
const (
	FieldsAskedAction   audit.Action = "ai.fields_asked"
	SummaryAskedAction  audit.Action = "ai.summary_asked"
	ClassifyAskedAction audit.Action = "ai.classification_asked"
)

// AiSuggestFields proposes a title, notes, a due date and labels for one entry.
type AiSuggestFields struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// AiSummarize proposes notes that say what an entry is about, more briefly.
type AiSummarize struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// AiClassify proposes labels for an entry.
type AiClassify struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// AiSummarizeThread proposes notes that say what an entry's discussion came to (K-05).
type AiSummarizeThread struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// AiSummarizeContainer proposes how a collection stands (K-05).
type AiSummarizeContainer struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// Execute asks for fields.
func (h AiSuggestFields) Execute(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID, apply bool,
) error {
	return Ask(h).queue(ctx, actor, domain.TargetWorkItem, itemID,
		FieldsAskedAction, domain.KindFields, itemFieldsPrompt, apply)
}

// Execute asks for a summary.
func (h AiSummarize) Execute(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID, apply bool,
) error {
	return Ask(h).queue(ctx, actor, domain.TargetWorkItem, itemID,
		SummaryAskedAction, domain.KindFields, "summarize", apply)
}

// Execute asks for labels.
func (h AiClassify) Execute(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID, apply bool,
) error {
	return Ask(h).queue(ctx, actor, domain.TargetWorkItem, itemID,
		ClassifyAskedAction, domain.KindFields, "classify", apply)
}

// Execute asks for a summary of the discussion.
func (h AiSummarizeThread) Execute(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID, apply bool,
) error {
	return Ask(h).queue(ctx, actor, domain.TargetWorkItem, itemID,
		SummaryAskedAction, domain.KindFields, threadPrompt, apply)
}

// Execute asks how the collection stands.
//
// `apply` is not offered and the descriptor says why: nothing accepts a collection summary, so a
// flag that applied one would be a flag with nothing behind it.
func (h AiSummarizeContainer) Execute(
	ctx context.Context, actor appshared.ActorContext, containerID shared.ID,
) error {
	return Ask(h).queue(ctx, actor, domain.TargetContainer, containerID,
		SummaryAskedAction, domain.KindFields, collectionPrompt, false)
}

// askInput is the input all three declare: which entry, and whether the answer is applied or
// proposed.
//
// `apply` defaults to false, which is the milestone's whole shape: a result is a suggestion unless
// a rule says otherwise, and `automation.md` §1.3's "or applied directly" is the exception that has
// to be written down rather than the behaviour that happens by not thinking about it.
func askInput(what string) []usecase.Field {
	return []usecase.Field{
		{Name: "item_id", Kind: usecase.KindID, Required: true,
			Description: "The entry to ask about. A rule leaves this out and the run supplies the " +
				"entry it is about."},
		{Name: "apply", Kind: usecase.KindBool,
			Description: "Apply the answer as soon as it arrives, instead of proposing it. False " +
				"unless it is said: " + what + " is a proposal, and applying one without a person " +
				"reading it is a decision somebody has to configure deliberately."},
	}
}

func (h AiSuggestFields) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AiSuggestFieldsName,
		Summary: "Asks the workspace's AI provider to propose a title, notes and a due date " +
			"for one entry that already exists. Not labels, which are the classifier's and are " +
			"chosen from the vocabulary the collection agreed on; and not subtasks, which are a " +
			"decomposition's. The answer is a suggestion somebody accepts, unless the caller " +
			"asked for it to be applied.",
		SideEffects: "Queues one question to the provider and writes an audit entry.",
		TokenScope:  suggestionsWrite,
		Input:       askInput("a set of fields"),
		Audit: usecase.AuditDeclaration{
			Action: FieldsAskedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Asking changes no entry; an applied answer writes the entry's history itself.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AiSuggestFields) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return askInvoke(ctx, actor, in, h.Execute)
}

func (h AiSummarize) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AiSummarizeName,
		Summary: "Asks the workspace's AI provider to summarise one entry into its notes. The " +
			"answer is a suggestion somebody accepts, unless the caller asked for it to be applied.",
		SideEffects: "Queues one question to the provider and writes an audit entry.",
		TokenScope:  suggestionsWrite,
		Input:       askInput("a summary"),
		Audit: usecase.AuditDeclaration{
			Action: SummaryAskedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Asking changes no entry; an applied answer writes the entry's history itself.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AiSummarize) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return askInvoke(ctx, actor, in, h.Execute)
}

func (h AiClassify) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AiClassifyName,
		Summary: "Asks the workspace's AI provider to propose labels for one entry. The answer is " +
			"a suggestion somebody accepts, unless the caller asked for it to be applied.",
		SideEffects: "Queues one question to the provider and writes an audit entry.",
		TokenScope:  suggestionsWrite,
		Input:       askInput("a classification"),
		Audit: usecase.AuditDeclaration{
			Action: ClassifyAskedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Asking changes no entry; an applied answer writes the entry's history itself.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AiClassify) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return askInvoke(ctx, actor, in, h.Execute)
}

func (h AiSummarizeThread) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AiSummarizeThreadName,
		Summary: "Asks the workspace's AI provider what an entry's discussion came to: what was " +
			"decided, what is still open, and what somebody is waiting for. The comments are read " +
			"oldest first and bounded - a thread of four hundred comments is a token problem " +
			"rather than a summary problem. The answer is a suggestion for the entry's notes, " +
			"which somebody accepts or dismisses. An entry nobody has commented on is asked " +
			"nothing at all.",
		SideEffects: "Queues one question to the provider and writes an audit entry.",
		TokenScope:  suggestionsWrite,
		Input:       askInput("a summary of a discussion"),
		Audit: usecase.AuditDeclaration{
			Action: SummaryAskedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Asking changes no entry; an applied answer writes the entry's history itself.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AiSummarizeThread) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return askInvoke(ctx, actor, in, h.Execute)
}

func (h AiSummarizeContainer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AiSummarizeContainerName,
		Summary: "Asks the workspace's AI provider how one collection stands: what is open, what " +
			"moved, and what is overdue, in the shape a person answers a colleague on a Monday. " +
			"It reads the entries directly in the collection, bounded, and changes nothing. " +
			"Nothing accepts the answer - a collection has nowhere to put a status summary - so " +
			"it is read under the suggestions and dismissed.",
		SideEffects: "Queues one question to the provider and writes an audit entry.",
		TokenScope:  suggestionsWrite,
		Input: []usecase.Field{
			{Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The collection to describe. A rule leaves this out and the run " +
					"supplies the container it is about."},
		},
		Audit: usecase.AuditDeclaration{
			Action: SummaryAskedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Asking changes nothing, and nothing accepts the answer.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AiSummarizeContainer) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, containerID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// askInvoke is the three invocations, which differ in nothing.
func askInvoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
	run func(context.Context, appshared.ActorContext, shared.ID, bool) error,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	apply := false
	if in.Present("apply") {
		apply = in.Bool("apply")
	}
	if err := run(ctx, actor, itemID, apply); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
