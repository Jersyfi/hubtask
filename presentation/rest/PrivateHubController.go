// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

func privateHubSummaryResponse(row usecase.Output) openapi.PrivateHubSummary {
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
	if emergency, held := row["emergency_access"].(usecase.Output); held {
		hub.EmergencyAccess.Offered, _ = emergency["offered"].(bool)
		hub.EmergencyAccess.OpenUntil = optionalTimeField(emergency["open_until"])
	}
	if purge := row.String("purge_on"); purge != "" {
		if day, err := time.Parse(openapi_types.DateFormat, purge); err == nil {
			hub.PurgeOn = &openapi_types.Date{Time: day}
		}
	}
	return hub
}

// privateHubAccessResponse is one emergency access as every route answers it.
func privateHubAccessResponse(out usecase.Output) openapi.PrivateHubAccess {
	answer := openapi.PrivateHubAccess{
		Id:       uuidValue(out.String("id")),
		HubId:    uuidValue(out.String("hub_id")),
		Reason:   out.String("reason"),
		OpenedAt: timeValue(out["opened_at"]),
		EndsAt:   timeValue(out["ends_at"]),
	}
	if opener := out.String("opened_by"); opener != "" {
		id := uuidValue(opener)
		answer.OpenedBy = &id
	}
	return answer
}
