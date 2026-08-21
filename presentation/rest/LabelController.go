// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// A collection's vocabulary (B-09). The handlers hold no rules: the permission check, the
// uniqueness of the name and the four records a write owes all happen in the application layer,
// once, whichever channel the call came through (ADR-0005, arc42 §4).

const (
	createLabelUseCase = "CreateLabel"
	listLabelsUseCase  = "ListLabels"
	updateLabelUseCase = "UpdateLabel"
	deleteLabelUseCase = "DeleteLabel"
)

// CreateLabel answers POST /containers/{containerId}/labels.
func (c *RestController) CreateLabel(
	w http.ResponseWriter, r *http.Request, containerID openapi.ContainerId,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	var body openapi.LabelCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), createLabelUseCase, actor, usecase.Input{
		"collection_id": containerID.String(),
		"name":          body.Name,
		"color_token":   body.ColorToken,
		"description":   optionalStringField(body.Description),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	label := labelResponse(out)
	// Location and ETag are what let a client follow up without guessing: where the label is, and
	// which version it may write against (api-guidelines.md §5).
	w.Header().Set("Location",
		APIBasePath+"/containers/"+containerID.String()+"/labels/"+label.Id.String())
	w.Header().Set("ETag", etag(label.Version))
	writeJSON(w, r, http.StatusCreated, label)
}

// ListLabels answers GET /containers/{containerId}/labels.
//
// A plain array rather than a page, as the contract declares: a vocabulary people have to agree on
// is as long as a person can hold in their head (api-guidelines.md §2).
func (c *RestController) ListLabels(
	w http.ResponseWriter, r *http.Request, containerID openapi.ContainerId,
) {
	out, ok := c.read(w, r, listLabelsUseCase, usecase.Input{
		"collection_id": containerID.String(),
	})
	if !ok {
		return
	}

	vocabulary := []openapi.Label{}
	for _, row := range rowsOf(out) {
		vocabulary = append(vocabulary, labelResponse(row))
	}
	writeJSON(w, r, http.StatusOK, vocabulary)
}

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
	return label
}

// UpdateLabel answers PATCH /containers/{containerId}/labels/{labelId}.
//
// The collection in the path is not passed on, for the reason a bucket's is not: the label already
// knows which vocabulary it belongs to, and a path segment that could disagree with the row would
// be a second answer to a question that has one.
func (c *RestController) UpdateLabel(
	w http.ResponseWriter, r *http.Request,
	_ openapi.ContainerId, labelID openapi.LabelId, _ openapi.UpdateLabelParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	var body openapi.LabelUpdate
	present, err := decodeJSONWithPresence(r, &body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"label_id": labelID.String()}
	// Only the fields the client sent. A merge patch says "leave it alone" by omission, and a
	// handler that passed every field would clear the description of every client that only meant
	// to rename something.
	if present["name"] {
		// Null is not an instruction here: the contract types `name` as a plain string, a label
		// without one cannot exist, and the catalogue refuses an empty one by name. The colour is
		// the same case for the same reason - a label is rendered as a chip and nothing else.
		in["name"] = stringOrEmpty(body.Name)
	}
	if present["color_token"] {
		in["color_token"] = stringOrEmpty(body.ColorToken)
	}
	if present["description"] {
		// Null and the empty string are one instruction here: clear it. That is what makes them a
		// different request from omitting the field, which leaves the description alone.
		in["description"] = stringOrEmpty(body.Description)
	}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), updateLabelUseCase, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	label := labelResponse(out)
	w.Header().Set("ETag", etag(label.Version))
	writeJSON(w, r, http.StatusOK, label)
}

// DeleteLabel answers DELETE /containers/{containerId}/labels/{labelId}.
//
// 204 rather than the 200 a deleted column answers with. Nothing became of anything that a client
// could not work out for itself: the entries that carried the label stop showing it, and there is
// no destination to report.
func (c *RestController) DeleteLabel(
	w http.ResponseWriter, r *http.Request,
	_ openapi.ContainerId, labelID openapi.LabelId, _ openapi.DeleteLabelParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	in := usecase.Input{"label_id": labelID.String()}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}

	if _, err := c.UseCases.Invoke(r.Context(), deleteLabelUseCase, actor, in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
