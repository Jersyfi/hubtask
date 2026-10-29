// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const EvaluateEntryFormName = "EvaluateEntryForm"

// EvaluateEntryForm answers how a type's form stands for a draft - which fields are required,
// read-only or hidden, which validations do not hold, whether the channel may create it - so that
// an editor unfolds what the form asks before it sends anything (UC-FLW-01 check 2, D-13). The
// server answers rather than the browser: the conditions are CEL, and one evaluator decides both
// what an editor shows and what a write is refused for (P-05).
type EvaluateEntryForm struct {
	Items      repository.Items
	Containers repository.Containers
	Gate       EntryGate
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// EvaluateEntryFormCommand is a draft: an existing entry with the values an editor changed, or a
// new one in a collection.
type EvaluateEntryFormCommand struct {
	ItemID       shared.ID
	CollectionID shared.ID
	Type         domain.ItemType
	EntryType    string
	Title        *string
	Notes        *string
	CustomFields map[string]any
}

// EntryFormAnswer is the form's state for the draft; State is nil where the type has no form.
type EntryFormAnswer struct {
	EntryType string
	State     *domain.FormState
	Channel   domain.FormChannel
}

// Execute answers the form for the draft, after the question whether the actor may read where it
// would be written.
func (h EvaluateEntryForm) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd EvaluateEntryFormCommand,
) (EntryFormAnswer, error) {
	draft := domain.WorkItem{Type: cmd.Type, EntryType: cmd.EntryType, CollectionID: cmd.CollectionID}
	var collection domain.Container
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if !cmd.ItemID.IsZero() {
			item, err := findItem(ctx, h.Items, cmd.ItemID)
			if err != nil {
				return err
			}
			draft = item
			if cmd.EntryType != "" {
				draft.EntryType = cmd.EntryType
			}
		}
		if draft.CollectionID.IsZero() {
			return shared.ErrValidation.WithDetail("items.collection_or_parent_required").
				WithFields(shared.FieldError{Path: "/collection_id", Code: "items.collection_or_parent_required"})
		}
		var err error
		collection, err = findCollection(ctx, h.Containers, draft.CollectionID)
		return err
	})
	if err != nil {
		return EntryFormAnswer{}, err
	}
	if !draft.Type.Valid() {
		return EntryFormAnswer{}, shared.ErrValidation.WithDetail("items.type_unknown").
			WithParams(map[string]string{"value": string(draft.Type)}).
			WithFields(shared.FieldError{Path: "/type", Code: "items.type_unknown"})
	}
	// Outside the transaction, so that a refusal's audit entry is not rolled back with it (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: containerPath(collection), Action: ItemReadAction,
		TokenScope: itemsRead, TargetType: itemTarget, TargetID: collection.ID,
	}); err != nil {
		return EntryFormAnswer{}, err
	}
	if cmd.Title != nil {
		draft.Title = *cmd.Title
	}
	if cmd.Notes != nil {
		draft.Notes = *cmd.Notes
	}
	if cmd.CustomFields != nil {
		merged := make(map[string]any, len(draft.CustomFields)+len(cmd.CustomFields))
		for key, value := range draft.CustomFields {
			merged[key] = value
		}
		for key, value := range cmd.CustomFields {
			merged[key] = value
		}
		draft.CustomFields = merged
	}
	answer := EntryFormAnswer{Channel: FormChannelOf(actor)}
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		state, entryType, err := h.Gate.Forms.Evaluate(ctx, actor, collection, draft)
		answer.EntryType, answer.State = entryType.Key, state
		return err
	})
	return answer, err
}

// Descriptor is the catalogue entry.
func (h EvaluateEntryForm) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: EvaluateEntryFormName,
		Summary: "Answers how an entry type's form stands for a draft: which fields are required, " +
			"read-only or hidden, which validations do not hold, and whether this door may create " +
			"the type. Nothing is written. A type without a form answers no fields.",
		SideEffects: "None.",
		TokenScope:  itemsRead,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Description: "The entry being edited; empty for a new one."},
			{Name: "collection_id", Kind: usecase.KindID, Description: "Where a new entry would be created."},
			{Name: "type", Kind: usecase.KindString, Description: "The level of a new entry: TASK, WORK_PACKAGE or ACTIVITY."},
			{Name: "entry_type", Kind: usecase.KindString, Description: "The entry type's key; empty for the level's own."},
			{Name: "title", Kind: usecase.KindString, Description: "The draft's title."},
			{Name: "notes", Kind: usecase.KindString, Description: "The draft's notes."},
			{Name: "custom_fields", Kind: usecase.KindObject, Description: "The draft's custom field values by key."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemReadAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "nothing is written."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h EvaluateEntryForm) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	cmd := EvaluateEntryFormCommand{
		Type: domain.ItemType(in.String("type")), EntryType: in.String("entry_type"),
		Title: in.OptionalString("title"), Notes: in.OptionalString("notes"),
	}
	var err error
	if cmd.ItemID, err = in.ID("item_id"); err != nil {
		return nil, err
	}
	if cmd.CollectionID, err = in.ID("collection_id"); err != nil {
		return nil, err
	}
	if cmd.CustomFields, err = customFieldsOf(in); err != nil {
		return nil, err
	}
	answer, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return EntryFormAnswerOutput(answer), nil
}

// EntryFormAnswerOutput is the answer as every door returns it (schema EntryFormState).
func EntryFormAnswerOutput(answer EntryFormAnswer) usecase.Output {
	fields := map[string]any{}
	failed := []string{}
	closed := false
	if answer.State != nil {
		for key, state := range answer.State.Fields {
			fields[key] = map[string]any{"required": state.Required, "read_only": state.ReadOnly, "hidden": state.Hidden}
		}
		if answer.State.Failed != nil {
			failed = answer.State.Failed
		}
		closed = answer.State.Closed
	}
	return usecase.Output{
		"entry_type": answer.EntryType, "channel": string(answer.Channel),
		"fields": fields, "failed": failed, "closed": closed,
	}
}
