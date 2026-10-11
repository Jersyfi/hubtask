// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const listPrivateHubsUseCase = "ListPrivateHubs"

// ListPrivateHubs answers GET /private-hubs.
func (c *RestController) ListPrivateHubs(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), listPrivateHubsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, privateHubsResponse(out))
}

func privateHubsResponse(out usecase.Output) []openapi.PrivateHubSummary {
	rows, _ := out["data"].([]usecase.Output)
	hubs := make([]openapi.PrivateHubSummary, 0, len(rows))
	for _, row := range rows {
		hub := openapi.PrivateHubSummary{
			Id:          uuidValue(row.String("id")),
			Owners:      []openapi_types.UUID{},
			Collections: row.Int("collections"),
			Entries:     row.Int("entries"),
			CreatedAt:   timeValue(row["created_at"]),
			TrashedAt:   optionalTimeField(row["trashed_at"]),
		}
		if bytes, held := row["attachment_bytes"].(int64); held {
			hub.AttachmentBytes = bytes
		}
		if owners, held := row["owners"].([]any); held {
			for _, owner := range owners {
				if id, ok := owner.(string); ok {
					hub.Owners = append(hub.Owners, uuidValue(id))
				}
			}
		}
		if purge := row.String("purge_on"); purge != "" {
			if day, err := time.Parse(openapi_types.DateFormat, purge); err == nil {
				hub.PurgeOn = &openapi_types.Date{Time: day}
			}
		}
		hubs = append(hubs, hub)
	}
	return hubs
}
