// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

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
	if body.AutoAssign != nil {
		in["auto_assign"] = autoAssignInput(*body.AutoAssign)
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

// autoAssignInput maps the request's auto_assign key onto the catalogue's document shape - the
// same decoded-JSON form every channel hands in, so the parse in the domain is one code path.
func autoAssignInput(policy openapi.AutoAssignPolicy) map[string]any {
	candidates := make([]any, 0, len(policy.Candidates))
	for _, candidate := range policy.Candidates {
		candidates = append(candidates, map[string]any{
			"kind": string(candidate.Kind), "id": candidate.Id.String(),
		})
	}
	document := map[string]any{
		"strategy":   string(policy.Strategy),
		"candidates": candidates,
	}
	if policy.Enabled != nil {
		document["enabled"] = *policy.Enabled
	}
	return document
}

// ArchiveContainer answers POST /containers/{containerId}:archive.
//
// No body. The container is named by the path, and there is nothing to decide about archiving it -
// which is what makes the action suffix right for it: a status field would have to be sent, read
// and validated to say the one thing this route says by existing (api-guidelines.md §2).
func (c *RestController) ArchiveContainer(
	w http.ResponseWriter, r *http.Request,
	containerID openapi.ContainerId, _ openapi.ArchiveContainerParams,
) {
	c.containerLifecycle(w, r, archiveContainerUseCase, containerID)
}

// UnarchiveContainer answers POST /containers/{containerId}:unarchive.
func (c *RestController) UnarchiveContainer(
	w http.ResponseWriter, r *http.Request,
	containerID openapi.ContainerId, _ openapi.UnarchiveContainerParams,
) {
	c.containerLifecycle(w, r, unarchiveContainerUseCase, containerID)
}

const (
	archiveContainerUseCase   = "ArchiveContainer"
	unarchiveContainerUseCase = "UnarchiveContainer"
	trashContainerUseCase     = "TrashContainer"
	restoreContainerUseCase   = "RestoreContainer"
)

// TrashContainer answers DELETE /containers/{containerId}.
//
// A DELETE rather than an action suffix, and 204 rather than the container: it is gone from every
// list the client draws, and there is nothing about it left worth sending back. That it is a soft
// delete is the server's business - what the client has to know is where to find it again, which is
// the trash rather than this response (api-guidelines.md §2).
func (c *RestController) TrashContainer(
	w http.ResponseWriter, r *http.Request,
	containerID openapi.ContainerId, params openapi.TrashContainerParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"container_id": containerID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	if _, err := c.UseCases.Invoke(r.Context(), trashContainerUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RestoreContainer answers POST /containers/{containerId}:restore.
func (c *RestController) RestoreContainer(
	w http.ResponseWriter, r *http.Request,
	containerID openapi.ContainerId, _ openapi.RestoreContainerParams,
) {
	c.containerLifecycle(w, r, restoreContainerUseCase, containerID)
}

// containerLifecycle is the shape every action that answers with the container shares: a path
// parameter, an optional If-Match, and the container back with its new tag. The deletion is not one
// of them - it answers 204, and has its own handler above.
//
// The If-Match is read off the request rather than off generated parameters, because the
// specification does not declare one for an action - `ifMatchOf` is what the item's own actions
// use, and reading the header directly keeps the two consistent.
func (c *RestController) containerLifecycle(
	w http.ResponseWriter, r *http.Request, useCase string, containerID openapi.ContainerId,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"container_id": containerID.String()}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), useCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, containerResponse(out))
}

// MoveContainer answers POST /containers/{containerId}:move.
//
// The body is required and `target_parent_id` with it: unlike an item, a collection has no level
// above its hub to be moved to, so there is no null that would mean anything here.
func (c *RestController) MoveContainer(
	w http.ResponseWriter, r *http.Request,
	containerID openapi.ContainerId, _ openapi.MoveContainerParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.MoveContainerJSONBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"container_id": containerID.String()}
	// The contract makes `target_parent_id` required and the generated type is a value, so a body
	// that omitted it decodes to the nil UUID. Passing that on would send the catalogue an identifier
	// nothing can have, and it would come back as "that hub does not exist" - which is not what went
	// wrong. Left out here, it is the missing field the catalogue names.
	if body.TargetParentId != (openapi_types.UUID{}) {
		in["target_parent_id"] = body.TargetParentId.String()
	}
	// Null and absent are one instruction here - "append to the end of the level" - so presence is
	// not read: there is no third thing for the client to have meant.
	if body.BeforeContainerId != nil {
		in["before_container_id"] = body.BeforeContainerId.String()
	}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), moveContainerUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, containerResponse(out))
}

const moveContainerUseCase = "MoveContainer"
