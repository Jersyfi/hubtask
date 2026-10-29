// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

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
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ListLinkTypesName  = "ListLinkTypes"
	CreateLinkTypeName = "CreateLinkType"
	ChangeLinkTypeName = "ChangeLinkType"
	DeleteLinkTypeName = "DeleteLinkType"

	linkTypeTarget = "link_type"

	// Stable audit codes (audit.md §2).
	LinkTypeCreatedAction audit.Action = "link_type.created"
	LinkTypeChangedAction audit.Action = "link_type.changed"
	LinkTypeDeletedAction audit.Action = "link_type.deleted"
	LinkTypesReadAction   audit.Action = "link_type.read"

	// A link type is the workspace's shape, so it shares the workspace configuration's scope.
	linkTypesWrite = "workspace:manage"
)

// LinkTypeRegistry is what every link type use case shares (UC-PLN-01 check 5).
type LinkTypeRegistry struct {
	Types      repository.LinkTypes
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Text       text.Normalizer
}

// Read answers the workspace's registry: the six shipped types and its own.
func (r LinkTypeRegistry) Read(ctx context.Context) (domain.LinkTypes, error) {
	rows, err := r.Types.List(ctx)
	if err != nil {
		return nil, err
	}
	return domain.NewLinkTypes(rows), nil
}

func (r LinkTypeRegistry) authorizeWrite(ctx context.Context, actor appshared.ActorContext, action audit.Action) error {
	return r.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: linkTypesWrite, TargetType: linkTypeTarget, TargetID: actor.TenantID,
	})
}

// record writes the evidence. The names are a person's words and go as fingerprints (rule 10).
func (r LinkTypeRegistry) record(
	ctx context.Context, actor appshared.ActorContext, linkType domain.LinkType, action audit.Action,
	changes []domain.FieldChange, now time.Time,
) error {
	trail := []audit.Change{{Field: "key", Classification: audit.Open, To: linkType.Key}}
	for _, change := range changes {
		classification := audit.Open
		if change.Field == domain.FieldName || change.Field == "inverse_name" {
			classification = audit.Sensitive
		}
		entry := audit.Change{Field: change.Field, Classification: classification, To: change.To}
		if change.From != "" {
			entry.From = change.From
		}
		trail = append(trail, entry)
	}
	return r.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: linkTypeTarget, TargetID: linkType.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(trail...),
	})
}

// LinkTypeOutput is the shape every door returns (api/openapi.yaml, schema LinkType). A shipped
// type answers null names, which a client fills with its own words for the key (rule 8).
func LinkTypeOutput(t domain.LinkType) usecase.Output {
	out := usecase.Output{
		"key": t.Key, "system": t.System, "name": nil, "inverse_name": nil,
		"direction": string(t.Direction), "cardinality": string(t.Cardinality),
		"cycles": string(t.Cycles), "waits": string(t.Waits), "position": t.Position,
		"archived": t.IsArchived(), "version": t.Version,
	}
	if t.Name != "" {
		out["name"] = t.Name
	}
	if t.InverseName != "" {
		out["inverse_name"] = t.InverseName
	}
	return out
}

// ListLinkTypes reads the workspace's link types.
type ListLinkTypes struct{ Registry LinkTypeRegistry }

// Execute answers every type, archived ones included and marked. Any member may read it: every
// link drawn offers from it.
func (h ListLinkTypes) Execute(ctx context.Context, actor appshared.ActorContext) (domain.LinkTypes, error) {
	if err := actor.RequireScope(itemsRead); err != nil {
		return nil, err
	}
	var registry domain.LinkTypes
	err := h.Registry.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := h.Registry.Read(ctx)
		registry = read
		return err
	})
	return registry, err
}

// Descriptor is the catalogue entry.
func (h ListLinkTypes) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListLinkTypesName,
		Summary: "Lists the workspace's link types - the six every workspace has (blocks, depends_on, " +
			"duplicates, runs_on, affects, implements) and its own - with each type's direction, " +
			"cardinality, cycle rule and which end waits, archived ones included and marked.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: LinkTypesReadAction, TargetType: linkTypeTarget, Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			registry, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(registry))
			for _, t := range registry {
				data = append(data, LinkTypeOutput(t))
			}
			return usecase.Output{"data": data}, nil
		}),
	}
}

// CreateLinkTypeCommand is the input, typed.
type CreateLinkTypeCommand struct {
	Key         string
	Name        string
	InverseName string
	Direction   string
	Cardinality string
	Cycles      string
	Waits       string
	Position    int
}

// CreateLinkType adds one of the workspace's own link types.
type CreateLinkType struct{ Registry LinkTypeRegistry }

// Execute creates the type.
func (h CreateLinkType) Execute(ctx context.Context, actor appshared.ActorContext, cmd CreateLinkTypeCommand) (domain.LinkType, error) {
	if err := h.Registry.authorizeWrite(ctx, actor, LinkTypeCreatedAction); err != nil {
		return domain.LinkType{}, err
	}
	now := h.Registry.Clock.Now()
	linkType, err := domain.NewLinkType(domain.NewLinkTypeInput{
		ID: h.Registry.IDs.NewID(), TenantID: actor.TenantID, Key: cmd.Key,
		Name: cmd.Name, InverseName: cmd.InverseName,
		Direction: domain.LinkDirection(cmd.Direction), Cardinality: domain.LinkCardinality(cmd.Cardinality),
		Cycles: domain.LinkCycles(cmd.Cycles), Waits: domain.LinkWaits(cmd.Waits),
		Position: cmd.Position, Now: now, Text: h.Registry.Text,
	})
	if err != nil {
		return domain.LinkType{}, err
	}
	err = h.Registry.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Registry.Types.Insert(ctx, linkType); err != nil {
			return err
		}
		return h.Registry.record(ctx, actor, linkType, LinkTypeCreatedAction, []domain.FieldChange{
			{Field: domain.FieldName, To: linkType.Name}, {Field: "direction", To: string(linkType.Direction)},
			{Field: "cardinality", To: string(linkType.Cardinality)}, {Field: "cycles", To: string(linkType.Cycles)},
			{Field: "waits", To: string(linkType.Waits)},
		}, now)
	})
	if err != nil {
		return domain.LinkType{}, err
	}
	return linkType, nil
}

// Descriptor is the catalogue entry.
func (h CreateLinkType) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateLinkTypeName,
		Summary: "Adds one of the workspace's own link types with its two names (read from the source " +
			"and from the target), whether it is DIRECTED or SYMMETRIC, whether an entry may start MANY " +
			"or ONE of it, whether loops are FORBIDDEN or ALLOWED, and which end waits for the other " +
			"(NONE, SOURCE, TARGET; a waiting type forbids loops). These never change afterwards.",
		SideEffects: "Writes the type and an audit entry.",
		TokenScope:  linkTypesWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The stable key: lower case, digits and underscores."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "What the link is called from its source."},
			{Name: "inverse_name", Kind: usecase.KindString, Description: "What it is called from its target; empty for a symmetric type."},
			{Name: "direction", Kind: usecase.KindString, Enum: []string{"DIRECTED", "SYMMETRIC"}, Description: "DIRECTED by default."},
			{Name: "cardinality", Kind: usecase.KindString, Enum: []string{"MANY", "ONE"}, Description: "MANY by default."},
			{Name: "cycles", Kind: usecase.KindString, Enum: []string{"FORBIDDEN", "ALLOWED"}, Description: "ALLOWED by default."},
			{Name: "waits", Kind: usecase.KindString, Enum: []string{"NONE", "SOURCE", "TARGET"}, Description: "NONE by default."},
			{Name: "position", Kind: usecase.KindInt, Description: "The type's place in a picker."},
		},
		Audit: usecase.AuditDeclaration{
			Action: LinkTypeCreatedAction, TargetType: linkTypeTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "configuration of the workspace; its history is the audit trail."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			linkType, err := h.Execute(ctx, actor, CreateLinkTypeCommand{
				Key: in.String("key"), Name: in.String("name"), InverseName: in.String("inverse_name"),
				Direction: in.String("direction"), Cardinality: in.String("cardinality"),
				Cycles: in.String("cycles"), Waits: in.String("waits"), Position: in.Int("position"),
			})
			if err != nil {
				return nil, err
			}
			return LinkTypeOutput(linkType), nil
		}),
	}
}

// ChangeLinkTypeCommand is the input, typed. A nil field is left alone.
type ChangeLinkTypeCommand struct {
	Key             string
	Name            *string
	InverseName     *string
	Position        *int
	Archived        *bool
	ExpectedVersion int
}

// ChangeLinkType renames, reorders, archives or restores one of the workspace's own link types.
type ChangeLinkType struct{ Registry LinkTypeRegistry }

// Execute changes the type. A shipped type does not change.
func (h ChangeLinkType) Execute(ctx context.Context, actor appshared.ActorContext, cmd ChangeLinkTypeCommand) (domain.LinkType, error) {
	if err := h.Registry.authorizeWrite(ctx, actor, LinkTypeChangedAction); err != nil {
		return domain.LinkType{}, err
	}
	if domain.IsSystemLinkTypeKey(cmd.Key) {
		return domain.LinkType{}, shared.ErrConflict.WithDetail("link_types.system_fixed")
	}
	var changed domain.LinkType
	err := h.Registry.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Registry.Types.Find(ctx, cmd.Key)
		if err != nil {
			return err
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("link_types.version_conflict")
		}
		now := h.Registry.Clock.Now()
		next, changes, err := current.Changed(domain.LinkTypeChange{
			Name: cmd.Name, InverseName: cmd.InverseName, Position: cmd.Position, Archived: cmd.Archived,
		}, now, h.Registry.Text)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			changed = current
			return nil
		}
		if err := h.Registry.Types.Update(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		changed = next
		return h.Registry.record(ctx, actor, next, LinkTypeChangedAction, changes, now)
	})
	if err != nil {
		return domain.LinkType{}, err
	}
	return changed, nil
}

// Descriptor is the catalogue entry.
func (h ChangeLinkType) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ChangeLinkTypeName,
		Summary: "Renames, reorders, archives or restores one of the workspace's own link types. An " +
			"archived type draws no new link, and the links that stand stay readable. The six shipped " +
			"types do not change (link_types.system_fixed), nor does what a type means.",
		SideEffects: "Writes the type and an audit entry.",
		TokenScope:  linkTypesWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The type's key."},
			{Name: "name", Kind: usecase.KindString, Description: "What the link is called from its source."},
			{Name: "inverse_name", Kind: usecase.KindString, Description: "What it is called from its target."},
			{Name: "position", Kind: usecase.KindInt, Description: "The type's place in a picker."},
			{Name: "archived", Kind: usecase.KindBool, Description: "True archives the type, false restores it."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for optimistic locking."},
		},
		Audit: usecase.AuditDeclaration{
			Action: LinkTypeChangedAction, TargetType: linkTypeTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "configuration of the workspace; its history is the audit trail."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			cmd := ChangeLinkTypeCommand{
				Key: in.String("key"), Name: in.OptionalString("name"), InverseName: in.OptionalString("inverse_name"),
				ExpectedVersion: in.Int("expected_version"),
			}
			if in.Present("position") {
				position := in.Int("position")
				cmd.Position = &position
			}
			if in.Present("archived") {
				archived := in.Bool("archived")
				cmd.Archived = &archived
			}
			linkType, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return LinkTypeOutput(linkType), nil
		}),
	}
}

// DeleteLinkType removes one of the workspace's own link types that no link names.
type DeleteLinkType struct{ Registry LinkTypeRegistry }

// Execute deletes the type; one in use is refused (`link_types.in_use`) and can only be archived.
func (h DeleteLinkType) Execute(ctx context.Context, actor appshared.ActorContext, key string) error {
	if err := h.Registry.authorizeWrite(ctx, actor, LinkTypeDeletedAction); err != nil {
		return err
	}
	if domain.IsSystemLinkTypeKey(key) {
		return shared.ErrConflict.WithDetail("link_types.system_fixed")
	}
	return h.Registry.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Registry.Types.Find(ctx, key)
		if err != nil {
			return err
		}
		inUse, err := h.Registry.Types.InUse(ctx, key)
		if err != nil {
			return err
		}
		if inUse {
			return shared.ErrConflict.WithDetail("link_types.in_use")
		}
		if err := h.Registry.Types.Delete(ctx, key); err != nil {
			return err
		}
		return h.Registry.record(ctx, actor, current, LinkTypeDeletedAction, nil, h.Registry.Clock.Now())
	})
}

// Descriptor is the catalogue entry.
func (h DeleteLinkType) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteLinkTypeName,
		Summary: "Deletes one of the workspace's own link types that no link names. A type in use is " +
			"refused with link_types.in_use - archive it instead; the shipped types are never deleted.",
		SideEffects: "Deletes the type and writes an audit entry.",
		TokenScope:  linkTypesWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The type's key."},
		},
		Audit: usecase.AuditDeclaration{
			Action: LinkTypeDeletedAction, TargetType: linkTypeTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "configuration of the workspace; its history is the audit trail."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Execute(ctx, actor, in.String("key")); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}
}
