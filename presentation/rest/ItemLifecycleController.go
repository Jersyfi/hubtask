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

// The lifecycle of an entry over REST: the archive, and in the steps that follow it the trash.
//
// The catalogue names. The routes they are reached through come from the specification; the two are
// reconciled by the parity test rather than by these constants.
const (
	archiveWorkItemUseCase   = "ArchiveWorkItem"
	unarchiveWorkItemUseCase = "UnarchiveWorkItem"
	trashWorkItemUseCase     = "TrashWorkItem"
	restoreWorkItemUseCase   = "RestoreWorkItem"
)

// ArchiveWorkItem answers POST /items/{itemId}:archive.
//
// No body. The entry is named by the path and there is nothing to decide about archiving it, which
// is what makes the action suffix right for it: a status field would have to be sent, read and
// validated to say the one thing this route says by existing (api-guidelines.md §2).
func (c *RestController) ArchiveWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.ArchiveWorkItemParams,
) {
	c.itemLifecycle(w, r, archiveWorkItemUseCase, itemID)
}

// UnarchiveWorkItem answers POST /items/{itemId}:unarchive.
func (c *RestController) UnarchiveWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.UnarchiveWorkItemParams,
) {
	c.itemLifecycle(w, r, unarchiveWorkItemUseCase, itemID)
}

// TrashWorkItem answers DELETE /items/{itemId}.
//
// A DELETE rather than an action suffix, and 204 rather than the entry: the entry is gone from every
// list the client draws, and there is nothing about it left worth sending back. That it is a soft
// delete is the server's business - what the client has to know is where to find it again, which is
// the trash rather than this response (api-guidelines.md §2).
//
// The subtree goes with it. Nothing here says so, because nothing here decides it: the invariant is
// the application layer's, and a controller that mentioned the cascade would be a second place
// stating a rule.
func (c *RestController) TrashWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, params openapi.TrashWorkItemParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	in := usecase.Input{"item_id": itemID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	if _, err := c.UseCases.Invoke(r.Context(), trashWorkItemUseCase, actor, in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RestoreWorkItem answers POST /items/{itemId}:restore.
func (c *RestController) RestoreWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.RestoreWorkItemParams,
) {
	c.itemLifecycle(w, r, restoreWorkItemUseCase, itemID)
}

// itemLifecycle is the shape every lifecycle action shares: a path parameter, an optional If-Match,
// and the entry back with its new tag.
//
// The If-Match is read off the request rather than off generated parameters, because the
// specification declares none for an action - the container's archive actions read it the same way,
// and doing it identically here is what keeps the two consistent.
//
// 200 rather than 204: the version moved, and the body is where the client learns the state it now
// has without a second request (api-guidelines.md §5).
func (c *RestController) itemLifecycle(
	w http.ResponseWriter, r *http.Request, useCase string, itemID openapi.ItemId,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	in := usecase.Input{"item_id": itemID.String()}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), useCase, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, workItemResponse(out))
}
