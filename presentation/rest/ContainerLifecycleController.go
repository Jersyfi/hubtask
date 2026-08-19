// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The catalogue names. The routes they are reached through come from the specification; the two are
// reconciled by the parity test rather than by these constants.
const (
	renameContainerUseCase         = "RenameContainer"
	updateContainerPoliciesUseCase = "UpdateContainerPolicies"
)

// RenameContainer answers PATCH /containers/{containerId}.
//
// A JSON Merge Patch (RFC 7386), which is why presence is read rather than the value alone: an
// absent `icon` means "leave it alone" and `"icon": null` means "clear it", and both arrive as a
// nil pointer in the generated struct. A handler that could not tell them apart would clear the
// icon of every client that only meant to rename something.
//
// Null reaches the catalogue as the empty string. The domain holds these fields as strings whose
// empty value is "not set", so cleared and empty are one state there - and inventing a second
// spelling for it in this layer would be a distinction only this layer believed in.
func (c *RestController) RenameContainer(
	w http.ResponseWriter, r *http.Request,
	containerID openapi.ContainerId, params openapi.RenameContainerParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.ContainerUpdate
	present, err := decodeJSONWithPresence(r, &body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"container_id": containerID.String()}
	if present["name"] {
		// Null is not an instruction here: the contract types `name` as a plain string, a container
		// without one cannot exist, and the catalogue refuses an empty one by name.
		in["name"] = stringOrEmpty(body.Name)
	}
	for field, sent := range map[string]*string{
		"description": body.Description,
		"icon":        body.Icon,
		"color_token": body.ColorToken,
	} {
		if present[field] {
			in[field] = stringOrEmpty(sent)
		}
	}

	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), renameContainerUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	// The ETag is the version after the change, which is what a client needs in order to follow the
	// update with another one (api-guidelines.md §5).
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, containerResponse(out))
}

// UpdateContainerPolicies answers PUT /containers/{containerId}/policies.
//
// A PUT rather than a merge patch, so presence is not read: a key the caller omitted is the default
// rather than what happens to be stored. Passing the field through unconditionally is what makes
// that true - the catalogue reads an absent `completion_policy` as MANUAL, and a handler that
// omitted the key when the client did would leave the use case unable to tell "not sent" from
// "sent as the default", which is a distinction this operation deliberately does not have.
func (c *RestController) UpdateContainerPolicies(
	w http.ResponseWriter, r *http.Request,
	containerID openapi.ContainerId, params openapi.UpdateContainerPoliciesParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.ContainerPolicies
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"container_id": containerID.String()}
	if body.CompletionPolicy != nil {
		in["completion_policy"] = string(*body.CompletionPolicy)
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), updateContainerPoliciesUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, containerResponse(out))
}
