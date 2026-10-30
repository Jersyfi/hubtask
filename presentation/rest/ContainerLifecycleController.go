// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Renaming, the privacy and the lifecycle of a container are answered by the generated mapping
// (Doors.gen.go); the policies and a move are written here. The routes come from the
// specification and are reconciled with the catalogue by the parity test.
const updateContainerPoliciesUseCase = "UpdateContainerPolicies"

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

const moveContainerUseCase = "MoveContainer"

const reorderContainerUseCase = "ReorderContainer"
