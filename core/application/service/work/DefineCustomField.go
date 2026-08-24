// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strings"
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
	DefineCustomFieldName = "DefineCustomField"
	ListCustomFieldsName  = "ListCustomFields"

	customFieldTarget = "custom_field"

	// The token scopes. A definition is structure rather than content, so it shares the
	// container's scopes for the reason a label's do: a token that may reorganise a workspace may
	// define its vocabulary (security.md §5).
	customFieldsWrite = containersWrite
	customFieldsRead  = containersRead

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	CustomFieldDefinedAction audit.Action = "custom_field.defined"
	// CustomFieldReadAction is declared even though an ordinary read writes no entry: a *refused*
	// read does, and it is recorded against the action that was refused rather than against a
	// generic "denied" (audit.md §4).
	CustomFieldReadAction audit.Action = "custom_field.read"
)

// A definition announces nothing and syncs nothing, unlike a label.
//
// No event: the catalogue in domain-model.md §4 names none, and a rule that fired on a field being
// defined would be reacting to configuration rather than to work. No change log entry either: a
// definition is what a client needs in order to *render* a form, which it reads when it opens one,
// and a device that never learned of a key could not display it whether it had merged the
// definition or not. What does merge is the value on the entry, per key (SetCustomField).

// DefineCustomField adds a field to a workspace's or a collection's vocabulary.
//
// Two scopes and no more. A field every collection defines separately is a field nobody can filter
// across; a field only ever defined workspace-wide is one team's vocabulary imposed on every other
// (domain-model.md §3.5). Which of the two applies is the presence of a collection in the command.
type DefineCustomField struct {
	Fields     repository.CustomFields
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// DefineCustomFieldCommand is the input, typed.
type DefineCustomFieldCommand struct {
	// CollectionID is the collection the field belongs to, zero for the whole workspace.
	CollectionID shared.ID
	Key          string
	Kind         domain.CustomFieldKind
	Options      []string
	IsRequired   bool
	// AppliesTo are the item types that carry the field. Empty means the column's default, which
	// is a task alone.
	AppliesTo []domain.ItemType
}

// Execute defines the field and returns it.
func (h DefineCustomField) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DefineCustomFieldCommand,
) (domain.CustomFieldDefinition, error) {
	path, err := h.scopeOf(ctx, actor, cmd.CollectionID)
	if err != nil {
		return domain.CustomFieldDefinition{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       path,
		Action:     CustomFieldDefinedAction,
		TokenScope: customFieldsWrite,
		TargetType: customFieldTarget,
		// The definition does not exist yet, so the refusal names the scope it would have joined.
		// Zero for a workspace-wide one, which is the scope itself.
		TargetID: cmd.CollectionID,
	}); err != nil {
		return domain.CustomFieldDefinition{}, err
	}

	appliesTo := cmd.AppliesTo
	if len(appliesTo) == 0 {
		// The column's default. Written out rather than left to the database, so that the answer
		// says what was stored rather than the caller having to read it back.
		appliesTo = []domain.ItemType{domain.ItemTask}
	}

	var defined domain.CustomFieldDefinition
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if !cmd.CollectionID.IsZero() {
			collection, err := findNamedCollection(ctx, h.Containers, cmd.CollectionID)
			if err != nil {
				return err
			}
			// The same gate a new entry passes: a hub has no vocabulary, and a trashed or archived
			// collection is read-only (I-C2, I-C3).
			if err := collection.EnsureAcceptsItems(); err != nil {
				return err
			}
		}
		if err := ensureTypesCarryFields(ctx, h.Profiles, appliesTo); err != nil {
			return err
		}

		now := h.Clock.Now()
		definition, err := domain.NewCustomFieldDefinition(domain.NewCustomFieldInput{
			ID: h.IDs.NewID(), TenantID: actor.TenantID, CollectionID: cmd.CollectionID,
			Key: cmd.Key, Kind: cmd.Kind, Options: cmd.Options,
			IsRequired: cmd.IsRequired, AppliesTo: appliesTo, Now: now,
		})
		if err != nil {
			return err
		}
		if err := h.Fields.Insert(ctx, definition); err != nil {
			return err
		}

		defined = definition
		return h.recordAudit(ctx, definition, actor, now)
	})
	if err != nil {
		return domain.CustomFieldDefinition{}, err
	}
	return defined, nil
}

// scopeOf is the path the permission is resolved along: the collection's when the field belongs to
// one, and the workspace's when it does not.
//
// The workspace scope refuses everybody whose membership sits on a hub or a collection, and that is
// the intended answer: a field every entry in the workspace carries is a decision about the
// workspace, and somebody who administers one hub does not get to make it.
func (h DefineCustomField) scopeOf(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID,
) ([]identity.Scope, error) {
	if collectionID.IsZero() {
		return []identity.Scope{identity.TenantScope()}, nil
	}

	// Read before the permission question, because the answer depends on the path: a membership
	// held at the hub applies downwards (domain-model.md §3.2). Nothing read here is trusted
	// afterwards - the state that decides the write is read again inside the transaction.
	var collection domain.Container
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := findNamedCollection(ctx, h.Containers, collectionID)
		collection = found
		return err
	})
	if err != nil {
		return nil, err
	}
	return containerPath(collection), nil
}

// ensureTypesCarryFields refuses a type whose profile does not carry CUSTOM_FIELDS.
//
// An activity does not (domain-model.md §2). Refused rather than silently dropped from the list,
// which is the rule the capability matrix states: a client that defined a field for an activity
// and received a 201 would believe an activity can hold it.
func ensureTypesCarryFields(
	ctx context.Context, profiles metarepo.CapabilityProfiles, types []domain.ItemType,
) error {
	for _, itemType := range types {
		profile, err := profileOf(ctx, profiles, itemType)
		if err != nil {
			return err
		}
		if !profile.Allows(domain.CapabilityCustomFields) {
			return shared.ErrValidation.
				WithDetail("fields.applies_to_unsupported").
				WithParams(map[string]string{"type": string(itemType)}).
				WithFields(shared.FieldError{
					Path: "/applies_to", Code: "fields.applies_to_unsupported",
					Params: map[string]string{"type": string(itemType)},
				})
		}
	}
	return nil
}

// recordAudit writes the evidence.
//
// The key, the kind and the scope in clear text: none of them is anybody's content - a key is an
// identifier this workspace chose and a kind is one of eight names. The options are not recorded.
// They are user content, and "which words a team picked to choose between" is exactly what rule 10
// keeps out of a trail that outlives the definition.
func (h DefineCustomField) recordAudit(
	ctx context.Context, definition domain.CustomFieldDefinition,
	actor appshared.ActorContext, now time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   definition.TenantID,
		OccurredAt: now,
		Action:     CustomFieldDefinedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: customFieldTarget,
		TargetID:   definition.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "key", Classification: audit.Open, To: definition.Key},
			audit.Change{Field: "kind", Classification: audit.Open, To: string(definition.Kind)},
			audit.Change{
				Field: "collection_id", Classification: audit.Open,
				To: scopeLabel(definition.CollectionID),
			},
			audit.Change{
				Field: domain.FieldAppliesTo, Classification: audit.Open,
				To: joinItemTypes(definition.AppliesTo),
			},
		),
	})
}

// scopeLabel is how the scope reaches a trail: the collection, or the word for the whole
// workspace. An empty string would read as "not recorded" rather than as "everywhere".
func scopeLabel(collectionID shared.ID) string {
	if collectionID.IsZero() {
		return "TENANT"
	}
	return collectionID.String()
}

func joinItemTypes(types []domain.ItemType) string {
	names := make([]string, 0, len(types))
	for _, itemType := range types {
		names = append(names, string(itemType))
	}
	return strings.Join(names, ",")
}

// customFieldOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema CustomFieldDefinition).
func customFieldOutput(definition domain.CustomFieldDefinition) usecase.Output {
	options := make([]string, 0, len(definition.Options))
	options = append(options, definition.Options...)

	appliesTo := make([]string, 0, len(definition.AppliesTo))
	for _, itemType := range definition.AppliesTo {
		appliesTo = append(appliesTo, string(itemType))
	}

	return usecase.Output{
		"id": definition.ID.String(),
		// Always present, as null for a workspace-wide field: absent would say this server does
		// not know about scopes, which is a different statement from "this one is everywhere".
		"collection_id": idOrNil(definition.CollectionID),
		"key":           definition.Key,
		"kind":          string(definition.Kind),
		// An empty array rather than null for a kind with no choices: a client renders the option
		// editor from this, and null would make it special-case the eight kinds.
		"options":     options,
		"is_required": definition.IsRequired,
		"applies_to":  appliesTo,
		"created_at":  definition.CreatedAt,
		"updated_at":  definition.UpdatedAt,
		"version":     definition.Version,
	}
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h DefineCustomField) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DefineCustomFieldName,
		Summary: "Defines a custom field, for one collection or for the whole workspace. The key " +
			"has to be free in its scope and is an identifier rather than a label - lower case, " +
			"digits and underscores - and it never changes afterwards: it appears in a " +
			"custom_fields.<key> filter, and a key that could be renamed would orphan every value " +
			"stored under it. Which types carry the field is bounded by the CUSTOM_FIELDS " +
			"capability: an activity has none, so naming one is refused rather than dropped.",
		SideEffects: "Writes the definition and an audit entry. Announces nothing: a definition is " +
			"configuration, and no event in the catalogue is about one.",
		TokenScope: customFieldsWrite,
		Input: []usecase.Field{
			{
				Name: "key", Kind: usecase.KindString, Required: true,
				Description: "The identifier the value is stored under, matching " +
					"^[a-z][a-z0-9_]{0,49}$ and free in its scope.",
			},
			{
				Name: "kind", Kind: usecase.KindString, Required: true,
				Description: "TEXT, NUMBER, DATE, SELECT, MULTI_SELECT, BOOL, USER or URL. Fixed " +
					"once defined: a kind that changed would reinterpret every value already " +
					"stored under the key.",
			},
			{
				Name: "collection_id", Kind: usecase.KindID,
				Description: "The collection the field belongs to. Omitted defines it for the " +
					"whole workspace, which is a decision about the workspace and asks for a " +
					"permission held there.",
			},
			{
				Name: "options", Kind: usecase.KindList,
				Description: "The values a SELECT or a MULTI_SELECT may hold. Required for those " +
					"two and refused for every other kind.",
			},
			{
				Name: "is_required", Kind: usecase.KindBool,
				Description: "Whether the field has to hold a value. Enforced when the field is " +
					"written and never retroactively: the entries that predate it stay valid.",
			},
			{
				Name: "applies_to", Kind: usecase.KindList,
				Description: "The item types that carry the field. Omitted means TASK alone.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: CustomFieldDefinedAction, TargetType: customFieldTarget,
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

func (h DefineCustomField) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	kind, err := domain.ParseCustomFieldKind(in.String("kind"))
	if err != nil {
		return nil, err
	}
	collectionID, err := optionalIDField(in, "collection_id")
	if err != nil {
		return nil, err
	}
	rawTypes, err := in.StringList("applies_to")
	if err != nil {
		return nil, err
	}
	appliesTo, err := itemTypesFrom(rawTypes)
	if err != nil {
		return nil, err
	}
	options, err := in.StringList("options")
	if err != nil {
		return nil, err
	}

	definition, err := h.Execute(ctx, actor, DefineCustomFieldCommand{
		CollectionID: collectionID,
		Key:          in.String("key"),
		Kind:         kind,
		Options:      options,
		IsRequired:   in.Bool("is_required"),
		AppliesTo:    appliesTo,
	})
	if err != nil {
		return nil, err
	}
	return customFieldOutput(definition), nil
}

// itemTypesFrom parses the type list. The names are the contract's, and one that is not a type is
// refused by name rather than dropped.
func itemTypesFrom(values []string) ([]domain.ItemType, error) {
	types := make([]domain.ItemType, 0, len(values))
	for _, value := range values {
		itemType := domain.ItemType(value)
		if !itemType.Valid() {
			return nil, shared.ErrValidation.
				WithDetail("items.type_unknown").
				WithParams(map[string]string{"value": value}).
				WithFields(shared.FieldError{Path: "/applies_to", Code: "items.type_unknown"})
		}
		types = append(types, itemType)
	}
	return types, nil
}

// optionalIDField reads an identifier that may be absent. An empty entry is "not named" rather
// than a malformed identifier, which is what lets one command serve both scopes.
func optionalIDField(in usecase.Input, name string) (shared.ID, error) {
	if in.String(name) == "" {
		return "", nil
	}
	return in.ID(name)
}

// ListCustomFields reads the definitions in force.
type ListCustomFields struct {
	Fields     repository.CustomFields
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// ListCustomFieldsQuery is the input, typed.
type ListCustomFieldsQuery struct {
	// CollectionID is the collection whose definitions are wanted, together with the
	// workspace-wide ones above it. Zero answers the workspace-wide ones alone.
	CollectionID shared.ID
}

// Execute returns the definitions.
//
// Unpaged, deliberately. A workspace's vocabulary is small and bounded by what a person can fill in
// on one form; a client renders the whole of it or none of it, and a cursor over a list nobody
// scrolls would be machinery for its own sake.
func (h ListCustomFields) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListCustomFieldsQuery,
) ([]domain.CustomFieldDefinition, error) {
	path := []identity.Scope{identity.TenantScope()}
	if !query.CollectionID.IsZero() {
		var collection domain.Container
		err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			found, err := findNamedCollection(ctx, h.Containers, query.CollectionID)
			collection = found
			return err
		})
		if err != nil {
			return nil, err
		}
		path = containerPath(collection)
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       path,
		Action:     CustomFieldReadAction,
		TokenScope: customFieldsRead,
		TargetType: customFieldTarget,
		TargetID:   query.CollectionID,
	}); err != nil {
		return nil, err
	}

	var definitions []domain.CustomFieldDefinition
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		definitions, err = h.Fields.ListInScope(ctx, query.CollectionID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return definitions, nil
}

// Descriptor is the catalogue entry.
func (h ListCustomFields) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListCustomFieldsName,
		Summary: "The custom field definitions in force, by key. Naming a collection answers the " +
			"ones that apply inside it: its own, and the workspace-wide ones above it. Naming none " +
			"answers the workspace-wide ones alone. Deleted definitions are not in the answer - " +
			"their values stay in the entries and stop being visible.",
		SideEffects: "None. Reads only.",
		TokenScope:  customFieldsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "collection_id", Kind: usecase.KindID,
				Description: "The collection whose definitions are wanted, together with the " +
					"workspace-wide ones. Omitted answers the workspace-wide ones alone.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: CustomFieldReadAction, TargetType: customFieldTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListCustomFields) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	collectionID, err := optionalIDField(in, "collection_id")
	if err != nil {
		return nil, err
	}

	definitions, err := h.Execute(ctx, actor, ListCustomFieldsQuery{CollectionID: collectionID})
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(definitions))
	for _, definition := range definitions {
		rows = append(rows, customFieldOutput(definition))
	}
	// A bare list rather than a page: the contract answers an array here, because a workspace's
	// vocabulary is not something a client walks.
	return usecase.Output{"data": rows}, nil
}
