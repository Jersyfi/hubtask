// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// A collection's vocabulary, answered by the generated mapping (Doors.gen.go); labelResponse
// draws a label wherever one is answered.

// labelResponse maps the catalogue's answer onto the contract's Label. The description is written
// whether or not it is set, as an explicit null: a client renders the label from this.
func labelResponse(out usecase.Output) openapi.Label {
	label := openapi.Label{
		Id:           uuidValue(out.String("id")),
		CollectionId: uuidValue(out.String("collection_id")),
		Name:         out.String("name"),
		ColorToken:   out.String("color_token"),
		Version:      out.Int("version"),
	}
	if description := out.String("description"); description != "" {
		label.Description = &description
	}
	if deletedAt, ok := out["deleted_at"].(time.Time); ok {
		label.DeletedAt = &deletedAt
	}
	return label
}

