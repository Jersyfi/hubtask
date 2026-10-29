// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	PreviewCustomFieldReachName = "PreviewCustomFieldReach"

	// CustomFieldReachPreviewedAction is the audit code of a refused preview. Stable (audit.md §2).
	CustomFieldReachPreviewedAction audit.Action = "custom_field.reach_previewed"
)

// PreviewCustomFieldReach answers how many entries hold a value under a definition where it is in
// force: what removing it, or hiding it from readers, would stop showing - before it is done. A
// plan of the configuration document counts its reach so. Asked as a change of the definition
// is, so whoever may not change it learns nothing of the entries.
type PreviewCustomFieldReach struct {
	Writer CustomFieldWriter
	Counts repository.FieldValueCounts
}

// Execute counts.
func (h PreviewCustomFieldReach) Execute(ctx context.Context, actor appshared.ActorContext, fieldID shared.ID) (int, error) {
	if _, err := h.Writer.reachable(ctx, actor, fieldID, CustomFieldReachPreviewedAction); err != nil {
		return 0, err
	}
	if h.Counts == nil {
		return 0, shared.ErrInternal.WithDetail("fields.reach_unwired")
	}
	var entries int
	err := h.Writer.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		definition, err := findCustomField(ctx, h.Writer.Fields, fieldID)
		if err != nil {
			return err
		}
		if definition.IsDeleted() {
			// Out of use already: it shows nothing that could stop showing.
			return nil
		}
		entries, err = h.Counts.EntriesHolding(ctx, definition)
		return err
	})
	return entries, err
}

// Descriptor is the catalogue entry.
func (h PreviewCustomFieldReach) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       PreviewCustomFieldReachName,
		Permission: service.PermissionStructure,
		Summary: "Answers how many entries hold a value under a custom field where it is in force - " +
			"what removing the field, or hiding it from readers, would stop showing - before it is " +
			"done. A count, never a value. Needs what changing the field needs.",
		SideEffects: "None.",
		ReadOnly:    true,
		TokenScope:  customFieldsWrite,
		Input: []usecase.Field{
			{Name: "field_id", Kind: usecase.KindID, Required: true, Description: "The definition."},
		},
		Audit: usecase.AuditDeclaration{
			Action: CustomFieldReachPreviewedAction, TargetType: customFieldTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a preview writes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			fieldID, err := in.ID("field_id")
			if err != nil {
				return nil, err
			}
			entries, err := h.Execute(ctx, actor, fieldID)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"field_id": fieldID.String(), "entries": entries}, nil
		}),
	}
}
