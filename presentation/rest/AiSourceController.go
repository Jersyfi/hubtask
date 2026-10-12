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

// Which AI answers a workspace (ADR-0072 §2, §3), over REST.

const (
	readAiSourceUseCase   = "ReadAiSource"
	chooseAiSourceUseCase = "ChooseAiSource"
	setOwnAiBudgetUseCase = "SetOwnAiBudget"

	setAiPersonOptOutUseCase = "SetAiPersonOptOut"
)

// ReadAiSource answers GET /ai-source.
func (c *RestController) ReadAiSource(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), readAiSourceUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, aiSourceResponse(out))
}

// ChooseAiSource answers PUT /ai-source.
func (c *RestController) ChooseAiSource(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.AiSourceChoice
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{"source": string(body.Source)}
	if body.OfferedModelId != nil {
		in["offered_model_id"] = body.OfferedModelId.String()
	}
	if body.Sources != nil {
		in["sources"] = string(*body.Sources)
	}
	out, err := c.UseCases.Invoke(r.Context(), chooseAiSourceUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, aiSourceResponse(out))
}

// SetOwnAiBudget answers PUT /ai-budget: the workspace's own budget for its own model, and the two
// AI standings after it. A null leaves the field out of the input, which is "no budget".
func (c *RestController) SetOwnAiBudget(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.OwnAiBudget
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{}
	if body.OwnTokensPerDay != nil {
		in["own_tokens_per_day"] = *body.OwnTokensPerDay
	}
	out, err := c.UseCases.Invoke(r.Context(), setOwnAiBudgetUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, quotaStandingsResponse(out))
}

// SetAiPersonOptOut answers PUT /ai-person-opt-out.
func (c *RestController) SetAiPersonOptOut(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.AiPersonOptOutChoice
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{"person_opt_out": string(body.PersonOptOut)}
	if body.Basis != nil {
		in["basis"] = string(*body.Basis)
	}
	if body.Contact != nil {
		in["contact"] = *body.Contact
	}
	out, err := c.UseCases.Invoke(r.Context(), setAiPersonOptOutUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, aiSourceResponse(out))
}

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
	if count, held := out["keeping_out_count"].(int); held {
		answer.KeepingOutCount = count
	}
	return answer
}
