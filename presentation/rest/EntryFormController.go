// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	setEntryTypeFormUseCase  = "SetEntryTypeForm"
	evaluateEntryFormUseCase = "EvaluateEntryForm"
)

// SetEntryTypeForm answers PUT /entry-types/{key}/form (UC-FLW-01 check 2).
func (c *RestController) SetEntryTypeForm(w http.ResponseWriter, r *http.Request, key string, _ openapi.SetEntryTypeFormParams) {
	var body openapi.EntryForm
	document, err := decodeDocument(r, &body)
	if err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{"key": key, "form": document}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}
	out, ok := c.read(w, r, setEntryTypeFormUseCase, in)
	if !ok {
		return
	}
	changed := entryTypeResponse(out)
	w.Header().Set("ETag", etag(changed.Version))
	writeJSON(w, r, http.StatusOK, changed)
}

// EvaluateEntryForm answers POST /entry-types/{key}/form:evaluate.
func (c *RestController) EvaluateEntryForm(w http.ResponseWriter, r *http.Request, key string) {
	var body openapi.EntryFormDraft
	if _, err := decodeDocument(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{
		"entry_type": key, "item_id": optionalUUIDField(body.ItemId),
		"collection_id": optionalUUIDField(body.CollectionId),
	}
	if body.Title != nil {
		in["title"] = *body.Title
	}
	if body.Notes != nil {
		in["notes"] = *body.Notes
	}
	if body.CustomFields != nil {
		in["custom_fields"] = *body.CustomFields
	}
	out, ok := c.read(w, r, evaluateEntryFormUseCase, in)
	if !ok {
		return
	}
	var answer openapi.EntryFormState
	if err := remarshal(out, &answer); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	writeJSON(w, r, http.StatusOK, answer)
}

// decodeDocument reads a body into its generated type, so that an unknown member is refused, and
// hands the same bytes on untyped: a form is a document the domain reads, not a list of scalars.
func decodeDocument[T any](r *http.Request, into *T) (map[string]any, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, shared.ErrMalformedRequest.WithDetail("request.body_unreadable").WithCause(err)
	}
	if err := decodeFrom(bytes.NewReader(raw), into); err != nil {
		return nil, err
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, shared.ErrMalformedRequest.WithDetail("request.body_malformed").WithCause(err)
	}
	return document, nil
}

// remarshal moves a use case's document into its generated type through the shape both share.
func remarshal(from any, into any) error {
	raw, err := json.Marshal(from)
	if err != nil {
		return shared.ErrInternal.WithDetail("request.response_unwritable").WithCause(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return shared.ErrInternal.WithDetail("request.response_unwritable").WithCause(err)
	}
	return nil
}
