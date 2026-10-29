// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The link and reference operations (F-3, UC-PLN-01). Their outputs carry the contract's field
// names, so a handler translates the request and writes the answer as it came.
const (
	listLinkTypesUseCase     = "ListLinkTypes"
	createLinkTypeUseCase    = "CreateLinkType"
	changeLinkTypeUseCase    = "ChangeLinkType"
	deleteLinkTypeUseCase    = "DeleteLinkType"
	listEntryLinksUseCase    = "ListEntryLinks"
	linkEntriesUseCase       = "LinkEntries"
	unlinkEntriesUseCase     = "UnlinkEntries"
	listLinksAmongUseCase    = "ListLinksAmong"
	resolveReferencesUseCase = "ResolveReferences"
)

// ListLinkTypes answers GET /link-types.
func (c *RestController) ListLinkTypes(w http.ResponseWriter, r *http.Request) {
	if out, ok := c.read(w, r, listLinkTypesUseCase, usecase.Input{}); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

// CreateLinkType answers POST /link-types.
func (c *RestController) CreateLinkType(w http.ResponseWriter, r *http.Request) {
	var body openapi.LinkTypeCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{"key": body.Key, "name": body.Name}
	if body.InverseName != nil {
		in["inverse_name"] = *body.InverseName
	}
	if body.Direction != nil {
		in["direction"] = string(*body.Direction)
	}
	if body.Cardinality != nil {
		in["cardinality"] = string(*body.Cardinality)
	}
	if body.Cycles != nil {
		in["cycles"] = string(*body.Cycles)
	}
	if body.Waits != nil {
		in["waits"] = string(*body.Waits)
	}
	if body.Position != nil {
		in["position"] = *body.Position
	}
	out, ok := c.read(w, r, createLinkTypeUseCase, in)
	if !ok {
		return
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusCreated, out)
}

// ChangeLinkType answers PATCH /link-types/{key}.
func (c *RestController) ChangeLinkType(w http.ResponseWriter, r *http.Request, key string, _ openapi.ChangeLinkTypeParams) {
	var body openapi.LinkTypeChange
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{"key": key}
	if body.Name != nil {
		in["name"] = *body.Name
	}
	if body.InverseName != nil {
		in["inverse_name"] = *body.InverseName
	}
	if body.Position != nil {
		in["position"] = *body.Position
	}
	if body.Archived != nil {
		in["archived"] = *body.Archived
	}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}
	out, ok := c.read(w, r, changeLinkTypeUseCase, in)
	if !ok {
		return
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, out)
}

// DeleteLinkType answers DELETE /link-types/{key}.
func (c *RestController) DeleteLinkType(w http.ResponseWriter, r *http.Request, key string) {
	if _, ok := c.read(w, r, deleteLinkTypeUseCase, usecase.Input{"key": key}); ok {
		w.WriteHeader(http.StatusNoContent)
	}
}

// ListEntryLinks answers GET /items/{itemId}/links.
func (c *RestController) ListEntryLinks(w http.ResponseWriter, r *http.Request, itemID openapi.ItemId) {
	if out, ok := c.read(w, r, listEntryLinksUseCase, usecase.Input{"item_id": itemID.String()}); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

// LinkEntries answers POST /items/{itemId}/links.
func (c *RestController) LinkEntries(w http.ResponseWriter, r *http.Request, itemID openapi.ItemId) {
	var body openapi.EntryLinkCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	out, ok := c.read(w, r, linkEntriesUseCase, usecase.Input{
		"item_id": itemID.String(), "target_id": body.TargetId.String(), "type": body.Type,
	})
	if ok {
		writeJSON(w, r, http.StatusCreated, out)
	}
}

// UnlinkEntries answers DELETE /links/{linkId}.
func (c *RestController) UnlinkEntries(w http.ResponseWriter, r *http.Request, linkID openapi_types.UUID) {
	if _, ok := c.read(w, r, unlinkEntriesUseCase, usecase.Input{"link_id": linkID.String()}); ok {
		w.WriteHeader(http.StatusNoContent)
	}
}

// ListLinksAmong answers POST /links:query.
func (c *RestController) ListLinksAmong(w http.ResponseWriter, r *http.Request) {
	var body openapi.ListLinksAmongJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	ids := make([]any, 0, len(body.ItemIds))
	for _, id := range body.ItemIds {
		ids = append(ids, id.String())
	}
	if out, ok := c.read(w, r, listLinksAmongUseCase, usecase.Input{"item_ids": ids}); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

// ResolveReferences answers POST /references:resolve.
func (c *RestController) ResolveReferences(w http.ResponseWriter, r *http.Request) {
	var body openapi.ResolveReferencesJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	refs := make([]any, 0, len(body.References))
	for _, ref := range body.References {
		object := map[string]any{"kind": string(ref.Kind), "id": ref.Id.String()}
		if ref.Anchor != nil {
			object["anchor"] = *ref.Anchor
		}
		refs = append(refs, object)
	}
	if out, ok := c.read(w, r, resolveReferencesUseCase, usecase.Input{"references": refs}); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}
