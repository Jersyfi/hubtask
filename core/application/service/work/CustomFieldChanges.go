// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// CustomFieldEntity is what the change log and the replica file a custom field definition under:
// its collection, none for a workspace-wide one, so a device draws an entry's form offline where it
// reads the collection (offline-sync.md §3.1). A definition is read by whoever reads its scope;
// a read-restricted field narrows its values, never its definition (UC-ID-27).
const CustomFieldEntity = customFieldTarget

// CustomFieldRecord is a definition as a device keeps it: the shape every door returns.
func CustomFieldRecord(definition domain.CustomFieldDefinition) usecase.Output {
	return customFieldOutput(definition)
}

// FieldChanges records a definition's change in the caller's transaction; a nil change log or
// clock records nothing, a test's convenience.
type FieldChanges struct {
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
}

func (c FieldChanges) record(ctx context.Context, actor appshared.ActorContext, definition domain.CustomFieldDefinition) error {
	if c.Changes == nil || c.HLC == nil {
		return nil
	}
	change := changelog.Change{
		TenantID: actor.TenantID, Entity: CustomFieldEntity, EntityID: definition.ID, Op: changelog.Upsert,
		ContainerID: definition.CollectionID, ActorID: actor.AccountID, HLC: c.HLC.Next(),
		Payload: CustomFieldRecord(definition),
	}
	if definition.DeletedAt != nil {
		change.Op, change.Payload = changelog.Delete, nil
	}
	return c.Changes.Record(ctx, change)
}
