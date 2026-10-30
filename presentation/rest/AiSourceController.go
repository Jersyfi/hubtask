// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Which AI answers a workspace (ADR-0072 §2, §3), over REST.

const ()

// aiSourceResponse maps the use case's answer. An offered model carries what the use case gives
// it - name, processor, jurisdiction - and this mapper has no field for anything more.
func aiSourceResponse(out usecase.Output) openapi.AiSource {
	answer := openapi.AiSource{
		Source:                openapi.AiSourceKind(out.String("source")),
		Sources:               openapi.AiSources(out.String("sources")),
		InstallationSources:   openapi.AiSources(out.String("installation_sources")),
		SourcesOrigin:         openapi.AiSourceSourcesOrigin(out.String("sources_origin")),
		MinJurisdiction:       openapi.AiMinJurisdiction(out.String("min_jurisdiction")),
		MinJurisdictionOrigin: openapi.AiSourceMinJurisdictionOrigin(out.String("min_jurisdiction_origin")),
		Offered:               []openapi.OfferedAiModelSummary{},
	}
	if lock := out.String("sources_lock"); lock != "" {
		value := openapi.AiSourceSourcesLock(lock)
		answer.SourcesLock = &value
	}
	if lock := out.String("min_jurisdiction_lock"); lock != "" {
		value := openapi.AiSourceMinJurisdictionLock(lock)
		answer.MinJurisdictionLock = &value
	}
	if narrowing := out.String("narrowing"); narrowing != "" {
		value := openapi.AiSources(narrowing)
		answer.Narrowing = &value
	}
	if id := out.String("offered_model_id"); id != "" {
		value := uuidValue(id)
		answer.OfferedModelId = (*openapi_types.UUID)(&value)
	}
	rows, _ := out["offered"].([]usecase.Output)
	for _, row := range rows {
		answer.Offered = append(answer.Offered, openapi.OfferedAiModelSummary{
			Id: uuidValue(row.String("id")), DisplayName: row.String("display_name"),
			Processor: row.String("processor"), Jurisdiction: openapi.AiJurisdiction(row.String("jurisdiction")),
		})
	}
	if rebuilding, held := out["search_rebuilding"].(bool); held {
		answer.SearchRebuilding = &rebuilding
	}
	if updated, held := out["updated_at"].(time.Time); held && !updated.IsZero() {
		answer.UpdatedAt = &updated
	}
	if version, held := out["version"].(int); held {
		answer.Version = version
	}
	answer.PersonOptOut = openapi.AiPersonOptOut(out.String("person_opt_out"))
	answer.PersonOptOutOrigin = openapi.AiSourcePersonOptOutOrigin(out.String("person_opt_out_origin"))
	if lock := out.String("person_opt_out_lock"); lock != "" {
		value := openapi.AiSourcePersonOptOutLock(lock)
		answer.PersonOptOutLock = &value
	}
	if basis := out.String("person_opt_out_basis"); basis != "" {
		value := openapi.AiOptOutBasis(basis)
		answer.PersonOptOutBasis = &value
	}
	if contact := out.String("person_opt_out_contact"); contact != "" {
		answer.PersonOptOutContact = &contact
	}
	if since, held := out["person_opt_out_since"].(time.Time); held && !since.IsZero() {
		answer.PersonOptOutSince = &since
	}
	answer.PrepareOnArrival = out["prepare_on_arrival"] == true
	if count, held := out["keeping_out_count"].(int); held {
		answer.KeepingOutCount = count
	}
	return answer
}
