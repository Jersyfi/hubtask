// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The labels an entry carries. A sub-resource with PUT and DELETE rather than a field on the
// entry, because a set is not a field: two devices adding two different labels at once is the case
// the OR-set exists to serve, and a merge patch carrying the whole array would let the later of the
// two erase the other's (offline-sync.md §4.2).

// itemLabelsResponse maps the catalogue's answer onto the contract's ItemLabels. The array is empty
// rather than null when the entry carries nothing: a client that iterates it should not have to
// nil-check the field.
func itemLabelsResponse(out usecase.Output) openapi.ItemLabels {
	set := openapi.ItemLabels{
		ItemId:   uuidValue(out.String("item_id")),
		LabelIds: []openapi_types.UUID{},
	}
	ids, _ := out["label_ids"].([]string)
	for _, id := range ids {
		set.LabelIds = append(set.LabelIds, uuidValue(id))
	}
	return set
}
