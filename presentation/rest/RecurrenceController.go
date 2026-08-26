// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The series beside the entries (D-04): a sub-resource of the entry, because a rule is one thing
// an entry either carries or does not - which is also why one PUT sets and changes it.

const (
	getRecurrenceUseCase    = "GetRecurrence"
	setRecurrenceUseCase    = "SetRecurrence"
	removeRecurrenceUseCase = "RemoveRecurrence"
	skipOccurrenceUseCase   = "SkipOccurrence"
)

// GetRecurrence answers GET /items/{itemId}/recurrence.
func (c *RestController) GetRecurrence(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
) {
	out, ok := c.read(w, r, getRecurrenceUseCase, usecase.Input{"item_id": itemID.String()})
	if !ok {
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, recurrenceResponse(out))
}

// SetRecurrence answers PUT /items/{itemId}/recurrence.
func (c *RestController) SetRecurrence(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	params openapi.SetRecurrenceParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.SetRecurrenceJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"item_id":   itemID.String(),
		"rrule":     body.Rrule,
		"time_zone": body.TimeZone,
		"mode":      string(body.Mode),
	}
	if body.HorizonDays != nil {
		in["horizon_days"] = *body.HorizonDays
	}
	if body.EndsAt != nil {
		in["ends_at"] = body.EndsAt.Format(time.RFC3339)
	}
	if body.MaxCount != nil {
		in["max_count"] = *body.MaxCount
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), setRecurrenceUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, recurrenceResponse(out))
}

// RemoveRecurrence answers DELETE /items/{itemId}/recurrence. 204: the caller asked for the series
// to be gone, and the occurrences it produced are read through the entries they are.
func (c *RestController) RemoveRecurrence(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	params openapi.RemoveRecurrenceParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	if _, err := c.UseCases.Invoke(r.Context(), removeRecurrenceUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SkipOccurrence answers POST /items/{itemId}/recurrence:skip.
func (c *RestController) SkipOccurrence(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	_ openapi.SkipOccurrenceParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), skipOccurrenceUseCase, actorOf(r),
		usecase.Input{"item_id": itemID.String()})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, recurrenceResponse(out))
}

// recurrenceResponse maps the catalogue's output onto the generated schema. The mapping lives here
// because the generated types are the contract's shape rather than the domain's
// (project-structure.md §3).
func recurrenceResponse(out usecase.Output) openapi.Recurrence {
	rule := openapi.Recurrence{
		Id:          uuidValue(out.String("id")),
		ItemId:      uuidValue(out.String("item_id")),
		Rrule:       out.String("rrule"),
		TimeZone:    out.String("time_zone"),
		Mode:        openapi.RecurrenceMode(out.String("mode")),
		HorizonDays: out.Int("horizon_days"),
		CreatedAt:   timeValue(out["created_at"]),
		Version:     out.Int("version"),
	}
	if at, ok := out["ends_at"].(time.Time); ok {
		rule.EndsAt = &at
	}
	if at, ok := out["last_materialized_at"].(time.Time); ok {
		rule.LastMaterializedAt = &at
	}
	if at, ok := out["updated_at"].(time.Time); ok {
		rule.UpdatedAt = &at
	}
	if count, ok := out["max_count"].(int); ok {
		rule.MaxCount = &count
	}
	return rule
}
