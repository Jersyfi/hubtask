// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	UpdateCustomFieldName = "UpdateCustomField"
	DeleteCustomFieldName = "DeleteCustomField"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	CustomFieldUpdatedAction audit.Action = "custom_field.updated"
	CustomFieldDeletedAction audit.Action = "custom_field.deleted"
)

// CustomFieldWriter is what an edit and a deletion share.
//
// One dependency set held by both, for the reason the label pair holds one: the two are the same
// walk - read the definition, resolve the scope it lives in, ask the permission there, write under
// the optimistic lock - and the only thing that differs is what is written at the end.
type CustomFieldWriter struct {
	Fields     repository.CustomFields
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// UpdateCustomField changes what a definition permits.
type UpdateCustomField struct {
	Writer CustomFieldWriter
}

// DeleteCustomField takes a definition out of use.
type DeleteCustomField struct {
	Writer CustomFieldWriter
}

// UpdateCustomFieldCommand is the input, typed. A pointer per field, so that "set it to nothing"
// and "do not touch it" stay two different requests all the way down from the merge patch that
// expressed them.
type UpdateCustomFieldCommand struct {
	FieldID    shared.ID
	Attributes domain.CustomFieldAttributes
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read
	// none and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// DeleteCustomFieldCommand is the input, typed.
type DeleteCustomFieldCommand struct {
	FieldID         shared.ID
	ExpectedVersion int
}

// Execute applies the change and returns the definition as it now stands.
func (h UpdateCustomField) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateCustomFieldCommand,
) (domain.CustomFieldDefinition, error) {
	if _, err := h.Writer.reachable(
		ctx, actor, cmd.FieldID, CustomFieldUpdatedAction,
	); err != nil {
		return domain.CustomFieldDefinition{}, err
	}

	var updated domain.CustomFieldDefinition
	err := h.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := findCustomField(ctx, h.Writer.Fields, cmd.FieldID)
		if err != nil {
			return err
		}
		if cmd.Attributes.AppliesTo != nil {
			if err := ensureTypesCarryFields(
				ctx, h.Writer.Profiles, *cmd.Attributes.AppliesTo,
			); err != nil {
				return err
			}
		}

		now := h.Writer.Clock.Now()
		wanted, changes, err := stored.Updated(cmd.Attributes, now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// The definition already says what the caller asked it to say. Nothing is written and
			// no version is spent, which is what makes a client that echoes the whole object back
			// harmless rather than merely accepted.
			//
			// The If-Match is still honoured: a caller writing against a version somebody else has
			// moved on is told so even when its own change would have been a no-op, because the
			// state it was reasoning about is not the state that is there.
			if err := ensureFieldVersion(stored, cmd.ExpectedVersion); err != nil {
				return err
			}
			updated = stored
			return nil
		}

		expected := cmd.ExpectedVersion
		if expected == 0 {
			expected = stored.Version
		}
		if err := h.Writer.Fields.SetAttributes(ctx, wanted, expected); err != nil {
			return err
		}
		wanted.Version = expected + 1
		updated = wanted

		return h.Writer.recordAudit(ctx, wanted, actor, CustomFieldUpdatedAction, changes, now)
	})
	if err != nil {
		return domain.CustomFieldDefinition{}, err
	}
	return updated, nil
}

// Execute takes the definition out of use.
//
// A soft delete, and what makes it one is the size of the alternative: rewriting `custom_fields`
// across every entry in a collection would be an unbounded write from one request. The values stay
// and stop being visible, and a definition recreated under the same key is a new definition that
// shows none of what the old one held - which the partial unique index makes true rather than
// promised.
func (h DeleteCustomField) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DeleteCustomFieldCommand,
) error {
	if _, err := h.Writer.reachable(
		ctx, actor, cmd.FieldID, CustomFieldDeletedAction,
	); err != nil {
		return err
	}

	return h.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := findCustomField(ctx, h.Writer.Fields, cmd.FieldID)
		if err != nil {
			return err
		}
		if stored.IsDeleted() {
			// Already out of use. Idempotent rather than a conflict: the caller asked for a state
			// that is the state, and a second DELETE of the same definition is a client retrying.
			return ensureFieldVersion(stored, cmd.ExpectedVersion)
		}

		now := h.Writer.Clock.Now()
		deleted, changes, err := stored.Deleted(now)
		if err != nil {
			return err
		}

		expected := cmd.ExpectedVersion
		if expected == 0 {
			expected = stored.Version
		}
		if err := h.Writer.Fields.SetDeleted(ctx, deleted, expected); err != nil {
			return err
		}
		return h.Writer.recordAudit(ctx, deleted, actor, CustomFieldDeletedAction, changes, now)
	})
}

// reachable reads the definition and asks the permission in the scope it lives in.
//
// Outside the write transaction, deliberately: a refusal writes an audit entry, and an entry
// written inside that transaction would be rolled back together with the refusal (audit.md §7).
// Nothing read here is trusted afterwards - the state that decides the write is read again inside
// the transaction.
func (w CustomFieldWriter) reachable(
	ctx context.Context, actor appshared.ActorContext, fieldID shared.ID, action audit.Action,
) (domain.CustomFieldDefinition, error) {
	if fieldID.IsZero() {
		return domain.CustomFieldDefinition{}, shared.ErrValidation.
			WithDetail("fields.field_id_required").
			WithFields(shared.FieldError{Path: "/field_id", Code: "fields.field_id_required"})
	}

	var (
		definition domain.CustomFieldDefinition
		path       []identity.Scope
	)
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := findCustomField(ctx, w.Fields, fieldID)
		if err != nil {
			return err
		}
		definition = found

		path = []identity.Scope{identity.TenantScope()}
		if !found.IsTenantWide() {
			collection, err := findCollection(ctx, w.Containers, found.CollectionID)
			if err != nil {
				return err
			}
			path = containerPath(collection)
		}
		return nil
	})
	if err != nil {
		return domain.CustomFieldDefinition{}, err
	}

	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       path,
		Action:     action,
		TokenScope: customFieldsWrite,
		TargetType: customFieldTarget,
		TargetID:   fieldID,
	}); err != nil {
		return domain.CustomFieldDefinition{}, err
	}
	return definition, nil
}

// recordAudit writes the evidence: which fields of the definition moved, and never the options.
//
// The options are user content - "which words a team picked to choose between" - and rule 10 keeps
// them out of a trail that outlives the definition. That the list changed is recorded; what it
// changed to is read from the definition while it exists.
func (w CustomFieldWriter) recordAudit(
	ctx context.Context, definition domain.CustomFieldDefinition, actor appshared.ActorContext,
	action audit.Action, changes []domain.FieldChange, now time.Time,
) error {
	recorded := make([]audit.Change, 0, len(changes)+2)
	for _, change := range changes {
		recorded = append(recorded, auditChangeOf(change))
	}
	recorded = append(recorded,
		audit.Change{Field: "key", Classification: audit.Open, To: definition.Key},
		audit.Change{
			Field: "collection_id", Classification: audit.Open,
			To: scopeLabel(definition.CollectionID),
		},
	)

	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   definition.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: customFieldTarget,
		TargetID:   definition.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(recorded...),
	})
}

// auditChangeOf classifies one changed field. The options are the only user content among them, so
// they are recorded as a fingerprint: enough to see that two entries concern the same list, not
// enough to read it (audit.md §4).
func auditChangeOf(change domain.FieldChange) audit.Change {
	classification := audit.Open
	if change.Field == domain.FieldOptions {
		classification = audit.Sensitive
	}
	return audit.Change{
		Field: change.Field, Classification: classification,
		From: change.From, To: change.To,
	}
}

// ensureFieldVersion refuses a caller writing against a version that has moved on, even when the
// change it asked for would have been a no-op.
//
// Zero means the caller read no version and accepts whatever is there (api-guidelines.md §5).
func ensureFieldVersion(definition domain.CustomFieldDefinition, expected int) error {
	if expected == 0 || expected == definition.Version {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("items.version_conflict").
		WithParams(map[string]string{
			"field_id":        definition.ID.String(),
			"current_version": strconv.Itoa(definition.Version),
		})
}

// findCustomField reads a definition, or says it does not exist in the words a client can act on.
func findCustomField(
	ctx context.Context, fields repository.CustomFields, id shared.ID,
) (domain.CustomFieldDefinition, error) {
	definition, err := fields.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The same answer whether it does not exist or belongs to another tenant. Anything
			// else would confirm the existence of another tenant's data (multi-tenancy.md §2).
			return domain.CustomFieldDefinition{}, shared.ErrNotFound.
				WithDetail("fields.not_found").
				WithParams(map[string]string{"field_id": id.String()})
		}
		return domain.CustomFieldDefinition{}, err
	}
	return definition, nil
}

// Descriptor is the catalogue entry.
func (h UpdateCustomField) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateCustomFieldName,
		Summary: "Changes what a definition permits: its options, whether it is required, and " +
			"which types carry it. The key and the kind are not among them - a key that moved " +
			"would orphan every value stored under it, and a kind that changed would reinterpret " +
			"them. Narrowing the options of a SELECT does not rewrite the entries holding a value " +
			"that is no longer offered; they keep it until somebody sets the field again.",
		SideEffects: "Writes the definition and an audit entry.",
		TokenScope:  customFieldsWrite,
		Input: []usecase.Field{
			{
				Name: "field_id", Kind: usecase.KindID, Required: true,
				Description: "The definition to change.",
			},
			{
				Name: "options", Kind: usecase.KindList,
				Description: "The values a SELECT or a MULTI_SELECT may hold, replacing the list.",
			},
			{
				Name: "is_required", Kind: usecase.KindBool,
				Description: "Whether the field has to hold a value from now on.",
			},
			{
				Name: "applies_to", Kind: usecase.KindList,
				Description: "The item types that carry the field, replacing the list.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read, from the If-Match header over REST.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: CustomFieldUpdatedAction, TargetType: customFieldTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A definition is the workspace's vocabulary rather than an entry, and the item " +
				"history is keyed on an entry. What a person reads in an entry's history is the " +
				"value that was written on it, which SetCustomField records there.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateCustomField) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	fieldID, err := in.ID("field_id")
	if err != nil {
		return nil, err
	}

	attributes := domain.CustomFieldAttributes{}
	if in.Present("options") {
		options, err := in.StringList("options")
		if err != nil {
			return nil, err
		}
		attributes.Options = &options
	}
	if in.Present("is_required") {
		required := in.Bool("is_required")
		attributes.IsRequired = &required
	}
	if in.Present("applies_to") {
		raw, err := in.StringList("applies_to")
		if err != nil {
			return nil, err
		}
		types, err := itemTypesFrom(raw)
		if err != nil {
			return nil, err
		}
		attributes.AppliesTo = &types
	}
	if attributes.IsEmpty() {
		return nil, shared.ErrValidation.
			WithDetail("fields.update_empty").
			WithFields(shared.FieldError{Path: "/", Code: "fields.update_empty"})
	}

	definition, err := h.Execute(ctx, actor, UpdateCustomFieldCommand{
		FieldID: fieldID, Attributes: attributes, ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return customFieldOutput(definition), nil
}

// Descriptor is the catalogue entry.
func (h DeleteCustomField) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteCustomFieldName,
		Summary: "Takes a definition out of use. A soft delete: the values stay in the entries and " +
			"stop being visible, because rewriting the custom fields of every entry in a " +
			"collection would be an unbounded write from one request. The key is free again at " +
			"once, and a definition recreated under it is a new definition that shows none of " +
			"what the old one held. Idempotent: a definition already out of use succeeds.",
		SideEffects: "Marks the definition out of use and writes an audit entry.",
		TokenScope:  customFieldsWrite,
		Input: []usecase.Field{
			{
				Name: "field_id", Kind: usecase.KindID, Required: true,
				Description: "The definition to take out of use.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read, from the If-Match header over REST.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: CustomFieldDeletedAction, TargetType: customFieldTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A definition is the workspace's vocabulary rather than an entry, and the item " +
				"history is keyed on an entry. What a person reads in an entry's history is the " +
				"value that was written on it, which SetCustomField records there.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteCustomField) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	fieldID, err := in.ID("field_id")
	if err != nil {
		return nil, err
	}

	if err := h.Execute(ctx, actor, DeleteCustomFieldCommand{
		FieldID: fieldID, ExpectedVersion: in.Int("expected_version"),
	}); err != nil {
		return nil, err
	}
	// Nothing to project: the definition is out of use, and a body describing it would invite a
	// client to keep rendering it. The contract answers 204.
	return usecase.Output{}, nil
}
