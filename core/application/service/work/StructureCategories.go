// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"slices"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// ensureCategories holds the categories a write names to what they can be: labels of the
// collection whose own layer names them (domain-model.md §2, UC-WRK-25 check 5). A hub's or the
// workspace's layer names none, since labels belong to a collection. An id this layer already
// names is kept even when its label was deleted since, so that bringing the label back brings its
// place in the order back too; a new one has to be a live label of this collection.
//
// The label is read under the request's row level security: another workspace's label is not
// found, and the refusal is the same as for a label of another collection or none at all, so it
// tells the caller nothing about what exists elsewhere (security.md T-04).
func ensureCategories(
	ctx context.Context, labels repository.Labels, container domain.Container, values domain.StructureValues,
) error {
	for itemType, fields := range values.Fields {
		if len(fields.Categories) == 0 {
			continue
		}
		if container.Type != domain.ContainerCollection {
			return categoriesCollectionOnly(itemType)
		}
		stored := container.Structure.Values.Fields[itemType].Categories
		for _, id := range fields.Categories {
			if slices.Contains(stored, id) {
				continue
			}
			if labels == nil {
				return categoryUnknown(itemType)
			}
			label, err := labels.Find(ctx, id)
			if errors.Is(err, shared.ErrNotFound) {
				return categoryUnknown(itemType)
			}
			if err != nil {
				return err
			}
			if label.CollectionID != container.ID || label.DeletedAt != nil {
				return categoryUnknown(itemType)
			}
		}
	}
	return nil
}

// refuseWorkspaceCategories refuses categories on the workspace's layer, which has no labels.
func refuseWorkspaceCategories(values domain.StructureValues) error {
	for itemType, fields := range values.Fields {
		if len(fields.Categories) > 0 {
			return categoriesCollectionOnly(itemType)
		}
	}
	return nil
}

func categoriesCollectionOnly(itemType domain.ItemType) error {
	params := map[string]string{"item_type": string(itemType)}
	return shared.ErrValidation.
		WithDetail("structure.categories_collection_only").
		WithParams(params).
		WithFields(shared.FieldError{
			Path: "/settings/fields", Code: "structure.categories_collection_only", Params: params,
		})
}

// categoryUnknown names the type, never the label: which identifiers exist is not the caller's to
// learn from a refusal.
func categoryUnknown(itemType domain.ItemType) error {
	params := map[string]string{"item_type": string(itemType)}
	return shared.ErrValidation.
		WithDetail("structure.category_unknown").
		WithParams(params).
		WithFields(shared.FieldError{
			Path: "/settings/fields", Code: "structure.category_unknown", Params: params,
		})
}
