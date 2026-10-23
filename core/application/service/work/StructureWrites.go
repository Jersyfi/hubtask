// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// structureField is the change log's name for a container's structure in force.
const structureField = "structure"

// choosePreset is a layer copied from a preset the actor may choose, held to the installation and to the
// workspace's locks: a locked setting is left to the layer above, a locked capability set the way
// it is locked. It answers the locks that kept a value, which a structure write names
// (kept_by_lock).
func choosePreset(
	ctx context.Context, reading *layerReading, actor appshared.ActorContext, container domain.Container,
	key domain.PresetKey,
) (domain.StructureLayer, []domain.LockKey, error) {
	bound, err := reading.Bound(ctx)
	if err != nil {
		return domain.StructureLayer{}, nil, err
	}
	preset, err := reading.Preset(ctx, actor, key)
	if err != nil {
		return domain.StructureLayer{}, nil, err
	}
	above, err := reading.Above(ctx, container)
	if err != nil {
		return domain.StructureLayer{}, nil, err
	}
	values, kept := domain.KeepLocked(bound, above, preset.Values)
	return domain.StructureLayer{Preset: key, Values: values}, kept, nil
}

// recordStructure tells offline clients a container's structure in force (offline-sync.md §4.2).
// The resolved value travels whole, as one field the server decides: a client resolves nothing, and
// never pushes it - a structure is written online.
func recordStructure(
	ctx context.Context, changes changelog.ChangeLog, hlc clock.HLCSource, container domain.Container,
	actor appshared.ActorContext, effective domain.EffectiveStructure,
) error {
	return changes.Record(ctx, changelog.Change{
		TenantID:    container.TenantID,
		Entity:      containerTarget,
		EntityID:    container.ID,
		Op:          changelog.Upsert,
		ContainerID: container.ID,
		ActorID:     actor.AccountID,
		HLC:         hlc.Next(),
		Field:       structureField,
		Payload:     map[string]any{structureField: effectiveStructureOutput(effective)},
	})
}
