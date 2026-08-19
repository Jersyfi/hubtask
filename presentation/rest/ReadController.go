// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The read side of the containers and the items (B-04). Four handlers, no rules: each reads the
// request, calls one catalogue entry, and maps the result - the permission check, the projection and
// the paging all happen in the application layer, once, for every channel (ADR-0005, arc42 §4).

const (
	getContainerUseCase   = "GetContainer"
	listContainersUseCase = "ListContainers"
	getWorkItemUseCase    = "GetWorkItem"
	listWorkItemsUseCase  = "ListWorkItems"
)

// GetContainer answers GET /containers/{containerId}.
func (c *RestController) GetContainer(w http.ResponseWriter, r *http.Request, containerID openapi.ContainerId) {
	out, ok := c.read(w, r, getContainerUseCase, usecase.Input{
		"container_id": containerID.String(),
	})
	if !ok {
		return
	}

	// The ETag is what makes the round trip possible at all: it is the version the client may write
	// against, and the value it sends back as If-Match (api-guidelines.md §5).
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, containerResponse(out))
}

// ListContainers answers GET /containers.
//
// No ETag. A page is not an entity with a version - there is nothing for a client to write back
// against, and an entity tag over a list would have to change whenever any row in it did.
func (c *RestController) ListContainers(
	w http.ResponseWriter, r *http.Request, params openapi.ListContainersParams,
) {
	in := usecase.Input{
		"parent_id":        optionalUUIDField(params.ParentId),
		"include_archived": optionalBoolField(params.IncludeArchived),
		"cursor":           optionalStringField(params.Cursor),
		"size":             optionalIntField(params.Size),
	}
	if params.Type != nil {
		in["type"] = string(*params.Type)
	}

	out, ok := c.read(w, r, listContainersUseCase, in)
	if !ok {
		return
	}

	page := openapi.ContainerPage{Data: []openapi.Container{}, Page: pageResponse(out)}
	for _, row := range rowsOf(out) {
		page.Data = append(page.Data, containerResponse(row))
	}
	writeJSON(w, r, http.StatusOK, page)
}

// GetWorkItem answers GET /items/{itemId}.
func (c *RestController) GetWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, params openapi.GetWorkItemParams,
) {
	// `expand` is in the contract and has no implementation yet: the relations it can ask for -
	// children, labels, the assignee, the cover - belong to use cases that have not landed. Refused
	// rather than ignored, on the same reasoning that makes an unknown request field a 422: a client
	// that asked for children and received an item without them cannot tell that from an item that
	// has none, and would render an empty tree (api-guidelines.md §6).
	if expandsAnything(params.Expand) {
		WriteProblem(w, shared.ErrValidation.
			WithDetail("items.expand_not_supported").
			WithFields(shared.FieldError{Path: "/expand", Code: "items.expand_not_supported"}),
			correlation.RequestIDFrom(r.Context()))
		return
	}

	out, ok := c.read(w, r, getWorkItemUseCase, usecase.Input{"item_id": itemID.String()})
	if !ok {
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, workItemResponse(out))
}

// expandsAnything reports whether the caller actually asked for a relation.
//
// `?expand=` binds as one empty entry rather than as no parameter at all, and a client that always
// sends the parameter and leaves it empty is asking for the plain item. Refusing that would be
// refusing a request that can be answered exactly as asked.
func expandsAnything(expand *[]string) bool {
	if expand == nil {
		return false
	}
	for _, relation := range *expand {
		if strings.TrimSpace(relation) != "" {
			return true
		}
	}
	return false
}

// ListWorkItems answers GET /items.
func (c *RestController) ListWorkItems(
	w http.ResponseWriter, r *http.Request, params openapi.ListWorkItemsParams,
) {
	out, ok := c.read(w, r, listWorkItemsUseCase, usecase.Input{
		"collection_id":    params.CollectionId.String(),
		"parent_id":        optionalUUIDField(params.ParentId),
		"include_archived": optionalBoolField(params.IncludeArchived),
		"cursor":           optionalStringField(params.Cursor),
		"size":             optionalIntField(params.Size),
	})
	if !ok {
		return
	}

	page := openapi.WorkItemPage{Data: []openapi.WorkItem{}, Page: pageResponse(out)}
	for _, row := range rowsOf(out) {
		page.Data = append(page.Data, workItemResponse(row))
	}
	writeJSON(w, r, http.StatusOK, page)
}

// read runs one catalogue entry and reports whether the caller may go on to write a body. The four
// handlers above share it because the three lines it holds - the wiring check, the actor, the problem
// document - are identical in all of them, and a fourth copy is a fourth place to forget one.
func (c *RestController) read(
	w http.ResponseWriter, r *http.Request, name string, in usecase.Input,
) (usecase.Output, bool) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return nil, false
	}
	actor, _ := appshared.ActorFrom(r.Context())

	out, err := c.UseCases.Invoke(r.Context(), name, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return nil, false
	}
	return out, true
}

// rowsOf reads the page's rows out of the catalogue's answer.
//
// Empty rather than nil when the shape is not what it should be. A missing `data` is a defect in a use
// case, and the handler's job at that point is to answer the shape the contract promises rather than to
// panic on a type assertion halfway through writing a body.
func rowsOf(out usecase.Output) []usecase.Output {
	rows, _ := out["data"].([]usecase.Output)
	return rows
}

// pageResponse maps the walk's state. next_cursor is a pointer with no omitempty in the generated
// type, so the last page carries an explicit null rather than no field at all - which is what lets a
// client read the field unconditionally.
func pageResponse(out usecase.Output) openapi.PageInfo {
	state, _ := out["page"].(map[string]any)
	hasMore, _ := state["has_more"].(bool)

	info := openapi.PageInfo{HasMore: hasMore}
	if cursor, ok := state["next_cursor"].(string); ok && cursor != "" {
		info.NextCursor = &cursor
	}
	return info
}

// optionalBoolField and optionalIntField are optionalStringField for the other two query parameter
// kinds: an absent parameter reaches the catalogue as an absent entry rather than as false or zero, so
// that every channel sees the same input and the use case's own default applies.
func optionalBoolField(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalIntField(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
