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
