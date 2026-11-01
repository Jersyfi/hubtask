// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// CopyCustomFieldHereName is the catalogue name.
const CopyCustomFieldHereName = "CopyCustomFieldHere"

// CopyCustomFieldHereCommand is the input, typed.
type CopyCustomFieldHereCommand struct {
	SourceID shared.ID
	HubID    shared.ID
}

// CopyCustomFieldHere makes a hub's own copy of a field shared with it (UC-WRK-12 check 9): a
// workspace-wide field is read-only at the hub, and a hub that needs it different copies it under
// the same key, which then wins in the hub's collections. The values written under the source stay
// visible: each names the definition it was written under, and the source stays in force.
type CopyCustomFieldHere struct{ Define DefineCustomField }

// Execute makes the copy and returns it.
func (h CopyCustomFieldHere) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CopyCustomFieldHereCommand,
) (domain.CustomFieldDefinition, error) {
	if cmd.HubID.IsZero() {
		return domain.CustomFieldDefinition{}, shared.ErrValidation.WithDetail("fields.hub_required").
			WithFields(shared.FieldError{Path: "/hub_id", Code: "fields.hub_required"})
	}
	d := h.Define
	path, err := hubPath(ctx, d.UnitOfWork, actor, d.Containers, cmd.HubID)
	if err != nil {
		return domain.CustomFieldDefinition{}, err
	}
	if err := d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: path, Action: CustomFieldDefinedAction,
		TokenScope: customFieldsWrite, TargetType: customFieldTarget, TargetID: cmd.HubID,
	}); err != nil {
		return domain.CustomFieldDefinition{}, err
	}

	var copied domain.CustomFieldDefinition
	err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := liveHub(ctx, d.Containers, cmd.HubID); err != nil {
			return err
		}
		source, err := findCustomField(ctx, d.Fields, cmd.SourceID)
		if err != nil {
			return err
		}
		now := d.Clock.Now()
		copied, err = source.CopyAt(d.IDs.NewID(), cmd.HubID, now)
		if err != nil {
			return err
		}
		if err := d.Fields.Insert(ctx, copied); err != nil {
			return err
		}
		if err := d.seedFill(ctx, copied); err != nil {
			return err
		}
		if err := d.Changes.record(ctx, actor, copied); err != nil {
			return err
		}
		return d.Audit.Append(ctx, audit.Entry{
			TenantID: copied.TenantID, OccurredAt: now, Action: CustomFieldDefinedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: customFieldTarget, TargetID: copied.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "key", Classification: audit.Open, To: copied.Key},
				audit.Change{Field: "kind", Classification: audit.Open, To: string(copied.Kind)},
				audit.Change{Field: "collection_id", Classification: audit.Open, To: scopeLabel(copied.ScopeID())},
				audit.Change{Field: "copied_from", Classification: audit.Open, To: copied.CopiedFrom.String()},
			),
		})
	})
	if err != nil {
		return domain.CustomFieldDefinition{}, err
	}
	return copied, nil
}

// Descriptor is the catalogue entry.
func (h CopyCustomFieldHere) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       CopyCustomFieldHereName,
		Permission: service.PermissionStructure,
		Summary: "Makes a hub's own copy of a workspace-wide custom field under the same key: " +
			"everything of the source and its identifier in copied_from. In the hub's collections " +
			"the copy is the field in force from then on, the values written under the source stay " +
			"visible, and the source does not change. A collection's or a hub's field is not " +
			"shared and is not found from here.",
		SideEffects: "Writes the copy, records a change for offline clients, and writes an audit entry.",
		TokenScope:  customFieldsWrite,
		Input: []usecase.Field{
			{Name: "source_id", Kind: usecase.KindID, Required: true, Description: "The workspace-wide field to copy."},
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub the copy belongs to."},
		},
		Audit: usecase.AuditDeclaration{
			Action: CustomFieldDefinedAction, TargetType: customFieldTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A definition is the workspace's vocabulary rather than an entry; DefineCustomField says why.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			source, err := in.ID("source_id")
			if err != nil {
				return nil, err
			}
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			copied, err := h.Execute(ctx, actor, CopyCustomFieldHereCommand{SourceID: source, HubID: hub})
			if err != nil {
				return nil, err
			}
			return customFieldOutput(copied), nil
		}),
	}
}

// fieldScopeAmbiguous refuses a collection and a hub named together.
func fieldScopeAmbiguous() error {
	return shared.ErrValidation.WithDetail("fields.scope_ambiguous").
		WithFields(shared.FieldError{Path: "/hub_id", Code: "fields.scope_ambiguous"})
}

// liveHub reads a hub a field may belong to: a hub of the workspace, not in the trash. Anything
// else - a collection, another tenant's, none - is the same refusal (multi-tenancy.md §2).
func liveHub(ctx context.Context, containers repository.Containers, id shared.ID) (domain.Container, error) {
	hub, err := containers.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) || (err == nil && (hub.Type != domain.ContainerHub || hub.IsTrashed())) {
		return domain.Container{}, shared.ErrValidation.WithDetail("fields.hub_unknown").
			WithFields(shared.FieldError{Path: "/hub_id", Code: "fields.hub_unknown"})
	}
	return hub, err
}

// hubPath is the path a hub's field is authorised along, read before the permission question for
// the reason scopeOf gives.
func hubPath(
	ctx context.Context, uow persistence.UnitOfWork, actor appshared.ActorContext,
	containers repository.Containers, id shared.ID,
) ([]identity.Scope, error) {
	var hub domain.Container
	err := uow.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := liveHub(ctx, containers, id)
		hub = found
		return err
	})
	if err != nil {
		return nil, err
	}
	return containerPath(hub), nil
}
