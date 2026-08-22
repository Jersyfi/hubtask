// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The one person an entry is on (C-01). An action route rather than a field of the merge patch,
// exactly as completing an entry is: the assignee is a scalar on the entry's own row, and both
// operations move that row, spend a version and announce something a rule reacts to.
//
// The member list is a sub-resource instead, because it is a set. The split between these two
// controllers is the split between the two merge rules (offline-sync.md §4.2).

const (
	assignWorkItemUseCase   = "AssignWorkItem"
	unassignWorkItemUseCase = "UnassignWorkItem"
)

// AssignWorkItem answers POST /items/{itemId}:assign.
func (c *RestController) AssignWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	params openapi.AssignWorkItemParams,
) {
	var body openapi.Assignment
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}

	in := usecase.Input{"item_id": itemID.String(), "account_id": body.AccountId.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}
	c.assignment(w, r, assignWorkItemUseCase, in)
}

// UnassignWorkItem answers POST /items/{itemId}:unassign. No body: there is nothing to say beyond
// which entry, since an entry has one assignee and taking them off names nobody.
func (c *RestController) UnassignWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	params openapi.UnassignWorkItemParams,
) {
	in := usecase.Input{"item_id": itemID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}
	c.assignment(w, r, unassignWorkItemUseCase, in)
}

// assignment runs one direction and writes the entry back.
//
// The ETag is the version after the change, which is what a client needs in order to follow the
// assignment with an edit (api-guidelines.md §5). 200 with the entry rather than 204: the caller
// knows who it named, and not what version that produced.
func (c *RestController) assignment(
	w http.ResponseWriter, r *http.Request, name string, in usecase.Input,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), name, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, workItemResponse(out))
}
