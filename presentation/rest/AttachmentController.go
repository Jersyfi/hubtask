// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The files an entry carries. A sub-resource rather than a field of the entry, because it is
// a set: it merges as an OR-set, it lives beside the row, and neither direction spends the entry's
// version. The same split the label and member controllers draw, and for the same reason
// (offline-sync.md §4.2).

const (
	attachMediaUseCase     = "AttachMedia"
	detachMediaUseCase     = "DetachMedia"
	listAttachmentsUseCase = "ListAttachments"
)

// attachmentsResponse maps the catalogue's projection onto the contract's schema.
func attachmentsResponse(out usecase.Output) openapi.ItemAttachments {
	ids, _ := out["media_ids"].([]string)

	carried := make([]openapi_types.UUID, 0, len(ids))
	for _, id := range ids {
		carried = append(carried, uuidValue(id))
	}
	return openapi.ItemAttachments{ItemId: uuidValue(out.String("item_id")), MediaIds: carried}
}
