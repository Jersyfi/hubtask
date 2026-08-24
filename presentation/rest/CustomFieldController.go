// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The vocabulary a workspace adds to its entries (C-07). A top-level resource rather than a
// sub-resource of a collection, because a definition has two scopes: one collection's, or the
// whole workspace's, and a path that named a collection could not express the second.

const (
	defineCustomFieldUseCase = "DefineCustomField"
	listCustomFieldsUseCase  = "ListCustomFields"
	updateCustomFieldUseCase = "UpdateCustomField"
	deleteCustomFieldUseCase = "DeleteCustomField"
	setCustomFieldUseCase    = "SetCustomField"
)

// ListCustomFields answers GET /custom-fields.
func (c *RestController) ListCustomFields(
	w http.ResponseWriter, r *http.Request, params openapi.ListCustomFieldsParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{}
	if params.CollectionId != nil {
		in["collection_id"] = params.CollectionId.String()
	}

	out, err := c.UseCases.Invoke(r.Context(), listCustomFieldsUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	// An array rather than a page: a workspace's vocabulary is not something a client walks, and
	// the contract answers the list itself.
	definitions := []openapi.CustomFieldDefinition{}
	for _, row := range rowsOf(out) {
		definitions = append(definitions, customFieldResponse(row))
	}
	writeJSON(w, r, http.StatusOK, definitions)
}

// DefineCustomField answers POST /custom-fields.
func (c *RestController) DefineCustomField(
	w http.ResponseWriter, r *http.Request, _ openapi.DefineCustomFieldParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.CustomFieldDefinitionCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"key": body.Key, "kind": string(body.Kind)}
	if body.CollectionId != nil {
		in["collection_id"] = body.CollectionId.String()
	}
	if body.Options != nil {
		in["options"] = *body.Options
	}
	if body.IsRequired != nil {
		in["is_required"] = *body.IsRequired
	}
	if body.AppliesTo != nil {
		in["applies_to"] = itemTypeNames(*body.AppliesTo)
	}

	out, err := c.UseCases.Invoke(r.Context(), defineCustomFieldUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	definition := customFieldResponse(out)
	w.Header().Set("Location", APIBasePath+"/custom-fields/"+definition.Id.String())
	w.Header().Set("ETag", etag(definition.Version))
	writeJSON(w, r, http.StatusCreated, definition)
}

// itemTypeNames maps the generated enum onto the plain strings the catalogue takes.
func itemTypeNames(types []openapi.ItemType) []string {
	names := make([]string, 0, len(types))
	for _, itemType := range types {
		names = append(names, string(itemType))
	}
	return names
}

// customFieldResponse maps the catalogue's projection onto the contract's schema.
func customFieldResponse(out usecase.Output) openapi.CustomFieldDefinition {
	createdAt := timeValue(out["created_at"])
	updatedAt := timeValue(out["updated_at"])

	definition := openapi.CustomFieldDefinition{
		Id:         uuidValue(out.String("id")),
		Key:        out.String("key"),
		Kind:       openapi.CustomFieldKind(out.String("kind")),
		Options:    stringsOf(out["options"]),
		IsRequired: boolOf(out["is_required"]),
		AppliesTo:  itemTypesOf(out["applies_to"]),
		CreatedAt:  &createdAt,
		UpdatedAt:  &updatedAt,
		Version:    out.Int("version"),
	}
	// Always present, as null for a workspace-wide field: absent would say this server does not
	// know about scopes, which is a different statement from "this one is everywhere".
	if scope := out.String("collection_id"); scope != "" {
		collectionID := uuidValue(scope)
		definition.CollectionId = &collectionID
	}
	return definition
}

// stringsOf reads a projection's list of strings, as an empty array rather than null: a client
// renders the option editor from it, and null would make it special-case the kinds with no choices.
func stringsOf(value any) []string {
	values, _ := value.([]string)
	if values == nil {
		return []string{}
	}
	return values
}

func boolOf(value any) bool {
	flag, _ := value.(bool)
	return flag
}

func itemTypesOf(value any) []openapi.ItemType {
	names, _ := value.([]string)

	types := make([]openapi.ItemType, 0, len(names))
	for _, name := range names {
		types = append(types, openapi.ItemType(name))
	}
	return types
}

// UpdateCustomField answers PATCH /custom-fields/{fieldId}.
func (c *RestController) UpdateCustomField(
	w http.ResponseWriter, r *http.Request, fieldID openapi.CustomFieldId,
	params openapi.UpdateCustomFieldParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.CustomFieldDefinitionUpdate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	// A merge patch: only what the caller sent reaches the catalogue, so that "do not touch it"
	// and "set it to nothing" stay two different requests (api-guidelines.md §"Partial updates").
	in := usecase.Input{"field_id": fieldID.String()}
	if body.Options != nil {
		in["options"] = *body.Options
	}
	if body.IsRequired != nil {
		in["is_required"] = *body.IsRequired
	}
	if body.AppliesTo != nil {
		in["applies_to"] = itemTypeNames(*body.AppliesTo)
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), updateCustomFieldUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	definition := customFieldResponse(out)
	w.Header().Set("ETag", etag(definition.Version))
	writeJSON(w, r, http.StatusOK, definition)
}

// DeleteCustomField answers DELETE /custom-fields/{fieldId}.
func (c *RestController) DeleteCustomField(
	w http.ResponseWriter, r *http.Request, fieldID openapi.CustomFieldId,
	params openapi.DeleteCustomFieldParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"field_id": fieldID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	if _, err := c.UseCases.Invoke(r.Context(), deleteCustomFieldUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetCustomField answers PUT /items/{itemId}/custom-fields/{key}.
//
// One key per call, which is the merge rule showing through the URL: the values merge per key, so
// the resource a client addresses is one key rather than the document (offline-sync.md §4.2).
func (c *RestController) SetCustomField(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, key openapi.CustomFieldKey,
	params openapi.SetCustomFieldParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.CustomFieldValue
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	// The value travels as it arrived. What shape it may have is the definition's answer and the
	// application layer's to ask; an adapter that coerced it here would be deciding a rule
	// (presentation/CLAUDE.md, ADR-0005).
	in := usecase.Input{"item_id": itemID.String(), "key": string(key), "value": body.Value}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), setCustomFieldUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, workItemResponse(out))
}
