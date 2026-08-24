// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SetCustomFieldName = "SetCustomField"

	// ItemCustomFieldSetAction is the audit code. Stable: an auditor filters on it and a SIEM rule
	// matches on it (audit.md §2).
	ItemCustomFieldSetAction audit.Action = "item.custom_field_set"
)

// SetCustomField writes one custom field on one entry.
//
// One key per call, and that is the merge rule made unavoidable rather than a convenience. The
// values live in one jsonb document on the entry, and merging that document as one scalar is
// exactly the loss the per-field rule exists to prevent: two devices setting two different keys
// would resolve to whichever wrote later, and one of the two values would quietly not be there
// (offline-sync.md §4.2). A call that could write the whole document would be writing keys it never
// read, so there is no such call.
//
// The value is judged against the definition in force for the entry's collection: the collection's
// own, or the workspace-wide one. Judged rather than coerced - a NUMBER arriving as a string is
// refused, because a client that sent "3" meant a string (domain-model.md §6).
type SetCustomField struct {
	Items      repository.Items
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Fields     repository.CustomFields
	Authorizer Authorizer
	// Visibility answers the one question a USER value asks: can the account named reach the
	// entry at all. The same question an assignment asks, and the same refusal.
	Visibility Visibility
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	Activity   ActivityJournal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// SetCustomFieldCommand is the input, typed.
type SetCustomFieldCommand struct {
	ItemID shared.ID
	Key    string
	// Value is what the field shall hold, shaped by the definition's kind. Nil clears the key.
	Value any
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read
	// none and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute writes the field and returns the entry as it now stands.
func (h SetCustomField) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SetCustomFieldCommand,
) (domain.WorkItem, error) {
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}
	if strings.TrimSpace(cmd.Key) == "" {
		return domain.WorkItem{}, shared.ErrValidation.
			WithDetail("fields.key_required").
			WithFields(shared.FieldError{Path: "/key", Code: "fields.key_required"})
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	subject, collection, err := readItemScope(
		ctx, h.UnitOfWork, h.Items, h.Containers, actor, cmd.ItemID)
	if err != nil {
		return domain.WorkItem{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     ItemCustomFieldSetAction,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   cmd.ItemID,
		On:         changing(subject),
	}); err != nil {
		return domain.WorkItem{}, err
	}

	var written domain.WorkItem
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Clock.Now()

		item, err := findItem(ctx, h.Items, cmd.ItemID)
		if err != nil {
			return err
		}
		collection, err := findCollection(ctx, h.Containers, item.CollectionID)
		if err != nil {
			return err
		}
		// I-C3: an archived or trashed collection is read-only, and its entries inherit that.
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}
		profile, err := profileOf(ctx, h.Profiles, item.Type)
		if err != nil {
			return err
		}
		if err := item.EnsureCustomisable(profile); err != nil {
			return err
		}

		value, err := h.judge(ctx, actor, item, collection, cmd)
		if err != nil {
			return err
		}

		wanted, moved := item.WithCustomField(cmd.Key, value, now)
		if !moved {
			// The entry already says what the caller asked it to say. Nothing is written, no
			// version is spent and nothing is announced - which is what makes a client that
			// echoes a form back harmless rather than merely accepted.
			//
			// The If-Match is still honoured: a caller writing against a version somebody else
			// has moved on is told so even when its own change would have been a no-op.
			if err := ensureExpectedVersion(item, cmd.ExpectedVersion); err != nil {
				return err
			}
			written = item
			return nil
		}
		if err := ensureFieldCount(wanted); err != nil {
			return err
		}

		stored, err := h.write(ctx, actor, item, wanted, cmd, profile, now)
		written = stored
		return err
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return written, nil
}

// judge resolves the definition in force and holds the value against it.
//
// The definition is the entry's collection's own, or the workspace-wide one under the same key -
// the collection's wins, so a team can narrow a workspace-wide default rather than having to avoid
// its key. A key nothing defines is refused rather than stored: `custom_fields` is a document, and
// a document that accepted any key would be a place for a typo to live forever.
func (h SetCustomField) judge(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem,
	collection domain.Container, cmd SetCustomFieldCommand,
) (any, error) {
	definition, err := h.Fields.FindInScope(ctx, item.CollectionID, cmd.Key)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrNotFound.
				WithDetail("fields.not_in_scope").
				WithParams(map[string]string{"key": cmd.Key}).
				WithFields(shared.FieldError{Path: "/key", Code: "fields.not_in_scope"})
		}
		return nil, err
	}
	if !definition.Carries(item.Type) {
		// The definition exists and this type is not one it applies to. Refused rather than
		// stored: a client that filled the field in on a work package a task-only definition
		// covers would believe the value is there.
		return nil, shared.ErrValidation.
			WithDetail("fields.not_for_this_type").
			WithParams(map[string]string{"key": cmd.Key, "type": string(item.Type)}).
			WithFields(shared.FieldError{Path: "/key", Code: "fields.not_for_this_type"})
	}

	value, err := definition.ValidateValue(cmd.Value)
	if err != nil {
		return nil, err
	}
	if definition.Kind == domain.CustomFieldUser && value != nil {
		if err := h.ensureAccountReachable(ctx, actor, value, collection); err != nil {
			return nil, err
		}
	}
	return value, nil
}

// ensureAccountReachable refuses a USER value naming somebody who cannot see the entry.
//
// The same question an assignment asks and the same refusal, for the same reason: a field pointing
// at an account that gets a 404 on the entry names a person who cannot act on it, and the three
// situations behind it - no membership, another workspace's account, no such account - come back
// as one answer so that the field cannot be used to discover which identifiers exist (T-04).
func (h SetCustomField) ensureAccountReachable(
	ctx context.Context, actor appshared.ActorContext, value any, collection domain.Container,
) error {
	text, isText := value.(string)
	if !isText {
		// The domain already judged the shape. Reaching this is a defect rather than input
		// (security.md §9).
		return shared.ErrInternal.WithDetail("fields.value_not_an_account")
	}
	named, err := shared.ParseID(text)
	if err != nil {
		return shared.ErrInternal.WithDetail("fields.value_not_an_account")
	}
	return ensureAccountCanSee(ctx, h.Visibility, actor, named, collection)
}

// ensureFieldCount bounds how many keys one entry carries. The column is one document that every
// read of the entry carries, so its size is the entry's size.
func ensureFieldCount(item domain.WorkItem) error {
	if len(item.CustomFields) <= domain.MaxCustomFieldsPerItem {
		return nil
	}
	return shared.ErrValidation.
		WithDetail("fields.too_many_on_one_item").
		WithParams(map[string]string{
			"maximum": strconv.Itoa(domain.MaxCustomFieldsPerItem),
		}).
		WithFields(shared.FieldError{Path: "/key", Code: "fields.too_many_on_one_item"})
}

// write stores the document and records what the change owes: the event outwards, the change log
// for offline clients, the audit entry, and the step of the entry's own history - all inside the
// caller's transaction (test AT-5).
func (h SetCustomField) write(
	ctx context.Context, actor appshared.ActorContext, before, after domain.WorkItem,
	cmd SetCustomFieldCommand, profile domain.CapabilityProfile, now time.Time,
) (domain.WorkItem, error) {
	expected := cmd.ExpectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still what the update matches on, so a concurrent write
		// between the read and here is still caught.
		expected = before.Version
	}
	if err := h.Items.SetCustomFields(ctx, after, expected); err != nil {
		return domain.WorkItem{}, err
	}
	after.Version = expected + 1

	change := domain.FieldChange{
		Field: domain.CustomFieldPath(cmd.Key),
		From:  customValueLabel(before.CustomFields[cmd.Key]),
		To:    customValueLabel(after.CustomFields[cmd.Key]),
	}

	// An ItemUpdated carrying the one key rather than an event of its own. A custom field is a
	// field of the entry - the catalogue's `item.updated` is defined as "changeSet (old/new per
	// field)" - and a rule written against "this field changed" is exactly the subscription it
	// offers (domain-model.md §4).
	announcement, err := event.NewItemUpdated(
		h.IDs.NewID(), after, []domain.FieldChange{change},
		event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := h.Events.Append(ctx, announcement); err != nil {
		return domain.WorkItem{}, err
	}
	if err := h.recordChange(ctx, after, actor, cmd.Key); err != nil {
		return domain.WorkItem{}, err
	}
	if err := h.recordAudit(ctx, after, actor, cmd.Key, change, now); err != nil {
		return domain.WorkItem{}, err
	}
	if err := h.recordActivity(ctx, after, actor, change, profile, now); err != nil {
		return domain.WorkItem{}, err
	}
	return after, nil
}

// recordChange writes what an offline client has to be told.
//
// One entry naming one key, with an HLC of its own. That is the merge rule for a map written down:
// last writer wins *per key*, so two devices setting two different keys converge to both
// (offline-sync.md §4.2). An entry carrying the whole document would give every key one HLC, and
// the later of two devices would erase the other's - which is the failure this shape exists to
// prevent.
//
// A cleared key travels as an explicit null rather than being left out: an absent key means "not
// touched", and a device that read it that way would keep a value somebody removed.
func (h SetCustomField) recordChange(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext, key string,
) error {
	value, held := item.CustomFields[key]
	if !held {
		value = nil
	}

	return h.Changes.Record(ctx, changelog.Change{
		TenantID:    item.TenantID,
		Entity:      itemTarget,
		EntityID:    item.ID,
		Op:          changelog.Upsert,
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         h.HLC.Next(),
		Payload:     map[string]any{domain.CustomFieldPath(key): value},
	})
}

// recordAudit writes the evidence: which key moved, never what it now says.
//
// The value is user content and is classified SENSITIVE, so the trail records `changed: true` with
// a hash of each side (audit.md §4). The entry outlives the item by design, so a value kept in
// clear text here would be a copy that no deletion of that item ever reaches - and "who filled
// this field in, and when" is answerable without it (rule 10, ADR-0017, ADR-0018). The key is not
// content: it is an identifier this workspace chose.
func (h SetCustomField) recordAudit(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	key string, change domain.FieldChange, now time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   item.TenantID,
		OccurredAt: now,
		Action:     ItemCustomFieldSetAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: itemTarget,
		TargetID:   item.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "key", Classification: audit.Open, To: key},
			audit.Change{
				Field: domain.FieldCustomFields, Classification: audit.Sensitive,
				From: change.From, To: change.To,
			},
			audit.Change{
				Field: "collection_id", Classification: audit.Open, To: item.CollectionID.String(),
			},
		),
	})
}

// recordActivity writes the step of the entry's own history.
//
// The key travels as the field name and both sides of the value as the change, so that a person
// reading the history sees what was filled in and what it replaced. The form is the type's, as
// every entry's is (domain-model.md §2).
func (h SetCustomField) recordActivity(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	change domain.FieldChange, profile domain.CapabilityProfile, now time.Time,
) error {
	field := activity.Field{
		Name: change.Field, Detail: activity.WithValues, From: change.From, To: change.To,
	}
	return h.Activity.record(ctx, actor, item, activity.ItemCustomFieldSet,
		activity.ChangeSet(historyForm(profile), field), now)
}

// customValueLabel renders a stored value for a change set. One spelling for the event, the trail
// and the history, so the three cannot describe the same change differently.
//
// A cleared key is the empty string, which is what every other field in this system uses for "not
// set" in a FieldChange - and the change set carries both sides, so "" on the right of a value is
// unambiguously a clearing.
func customValueLabel(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, element := range typed {
			parts = append(parts, customValueLabel(element))
		}
		return strings.Join(parts, ",")
	default:
		// A shape no kind produces. Rendered as its absence rather than as Go's own formatting,
		// which would put a type name into a payload that leaves the installation.
		return ""
	}
}

// Descriptor is the catalogue entry.
func (h SetCustomField) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetCustomFieldName,
		Summary: "Writes one custom field on an entry. The value is validated against the " +
			"definition in force for the entry's collection - its own, or the workspace-wide one " +
			"under the same key - and a SELECT value outside its options, a NUMBER arriving as a " +
			"string or a USER pointing at somebody who cannot reach the entry is refused rather " +
			"than stored. Null clears the key. One key per call, because the merge rule is per " +
			"key: two devices setting two different keys converge to both. Idempotent: the same " +
			"value again succeeds and announces nothing.",
		SideEffects: "Writes the entry's custom field document, announces " +
			string(event.ItemUpdated) + " carrying the one key, records the change for offline " +
			"clients, writes an audit entry and a step of the entry's history.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry to write the field on.",
			},
			{
				Name: "key", Kind: usecase.KindString, Required: true,
				Description: "The definition's key, as the collection sees it.",
			},
			{
				Name: "value", Kind: usecase.KindAny,
				Description: "What the field shall hold, shaped by the definition's kind. Null or " +
					"omitted clears the key, which a required field refuses.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read, from the If-Match header over REST. Omitted " +
					"means the caller read none and accepts whatever is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemCustomFieldSetAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemCustomFieldSet},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h SetCustomField) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}

	item, err := h.Execute(ctx, actor, SetCustomFieldCommand{
		ItemID:          itemID,
		Key:             in.String("key"),
		Value:           in["value"],
		ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}
