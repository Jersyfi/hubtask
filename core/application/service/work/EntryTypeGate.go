// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// chooseEntryType answers the type key a new entry of the level is written with, and the level's
// profile narrowed by that type's behaviours (UC-FLW-01 check 3, D-07). Without a registry every
// entry is its level's system type and the profile is unchanged.
func chooseEntryType(
	ctx context.Context, types repository.EntryTypes, level domain.ItemType, key string,
	parent *domain.WorkItem, profile domain.CapabilityProfile,
) (string, domain.CapabilityProfile, error) {
	if types == nil {
		return "", profile, nil
	}
	rows, err := types.List(ctx)
	if err != nil {
		return "", profile, err
	}
	registry := domain.NewEntryTypes(rows)
	var parentType *domain.EntryType
	if parent != nil {
		of := registry.Of(parent.Type, parent.EntryType)
		parentType = &of
	}
	chosen, err := registry.Choose(level, key, parentType)
	if err != nil {
		return "", profile, err
	}
	return chosen, registry.Of(level, chosen).Narrow(profile), nil
}

// narrowByEntryType takes from an entry's profile what its type switches off. Only a type of the
// workspace's own, or a shaped system type, can: an entry of an unshaped system type keeps its
// level's profile, so a workspace without a registry row pays no read.
func narrowByEntryType(
	ctx context.Context, types repository.EntryTypes, item domain.WorkItem, profile domain.CapabilityProfile,
) (domain.CapabilityProfile, error) {
	if types == nil {
		return profile, nil
	}
	rows, err := types.List(ctx)
	if err != nil {
		return profile, err
	}
	if len(rows) == 0 {
		return profile, nil
	}
	return domain.NewEntryTypes(rows).Of(item.Type, item.EntryType).Narrow(profile), nil
}
