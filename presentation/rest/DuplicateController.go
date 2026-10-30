// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Copying an entry. Nothing here holds a rule: what a copy carries, what the destination can
// resolve and what it therefore reports back are decided in the application layer, once, whichever
// channel the call came through (ADR-0005, arc42 §4).

// duplicateResultResponse maps the catalogue's answer onto the contract's DuplicateResult.
func duplicateResultResponse(out usecase.Output) openapi.DuplicateResult {
	nested, _ := out["item"].(usecase.Output)

	return openapi.DuplicateResult{
		Item:              workItemResponse(nested),
		Copied:            out.Int("copied"),
		DroppedReferences: droppedReferencesResponse(out),
	}
}
