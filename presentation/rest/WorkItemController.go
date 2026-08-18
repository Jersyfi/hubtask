// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// CreateWorkItem answers POST /items.
//
// Like CreateContainer, it holds no rules: it reads the request, hands it to the catalogue, and
// maps the result. Which type may sit under which, and how deep, is decided in the application
// layer from the capability profiles - so this handler knows nothing about tasks, work packages
// or activities, and a level configured later needs no change here (ADR-0006, arc42 §4).
func (c *RestController) CreateWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.CreateWorkItemParams) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	var body openapi.WorkItemCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"type":          string(body.Type),
		"title":         body.Title,
		"collection_id": optionalUUIDField(body.CollectionId),
		"parent_id":     optionalUUIDField(body.ParentId),
		"notes":         optionalStringField(body.Notes),
	}
	withUnservedItemFields(body, in)

	out, err := c.UseCases.Invoke(r.Context(), createWorkItemUseCase, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	item := workItemResponse(out)
	// Location and ETag are what let a client follow up without guessing: where the item is, and
	// which version it may write against (api-guidelines.md §5).
	w.Header().Set("Location", APIBasePath+"/items/"+item.Id.String())
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusCreated, item)
}

// createWorkItemUseCase is the catalogue name. The route it is reached through comes from the
// specification; the two are reconciled by the parity test rather than by this constant.
const createWorkItemUseCase = "CreateWorkItem"

// withUnservedItemFields passes on the fields of the contract that no use case writes yet, so
// that the catalogue refuses them by name.
//
// The specification is the whole of 1.0 and this installation serves a part of it - the same
// situation the pending set handles for entire operations, one level down. Dropping these fields
// silently is the failure that would be worst here: a client that sets a due date and receives a
// 201 believes there is a due date, and finds out when the reminder does not arrive.
//
// A field is passed on only when the client actually sent it. Sending it always would refuse
// every request, since the catalogue does not declare these names.
//
// Each entry disappears from this list when its use case lands: the bucket and the labels with
// B-09, the ordering with B-08, assignment and the cover in 0.3.0, the due date in 0.4.0.
func withUnservedItemFields(body openapi.WorkItemCreate, in usecase.Input) {
	if body.BucketId != nil {
		in["bucket_id"] = body.BucketId.String()
	}
	if body.BeforeItemId != nil {
		in["before_item_id"] = body.BeforeItemId.String()
	}
	if body.AssigneeId != nil {
		in["assignee_id"] = body.AssigneeId.String()
	}
	if body.AutoAssign != nil {
		in["auto_assign"] = *body.AutoAssign
	}
	if body.LabelIds != nil {
		in["label_ids"] = uuidList(*body.LabelIds)
	}
	if body.MemberIds != nil {
		in["member_ids"] = uuidList(*body.MemberIds)
	}
	if body.DueAt != nil {
		in["due_at"] = body.DueAt.String()
	}
	if body.DueDateOnly != nil {
		in["due_date_only"] = *body.DueDateOnly
	}
	if body.DueTimeZone != nil {
		in["due_time_zone"] = *body.DueTimeZone
	}
	if body.Cover != nil {
		in["cover"] = map[string]any{}
	}
	if body.CustomFields != nil {
		in["custom_fields"] = map[string]any(*body.CustomFields)
	}
}

func uuidList(values []openapi_types.UUID) []any {
	list := make([]any, 0, len(values))
	for _, value := range values {
		list = append(list, value.String())
	}
	return list
}

// workItemResponse maps the catalogue's output onto the generated schema. The mapping lives here
// because the generated types are the contract's shape rather than the domain's
// (project-structure.md §3).
func workItemResponse(out usecase.Output) openapi.WorkItem {
	path := out.String("path")
	depth := out.Int("depth")
	orderKey := out.String("order_key")
	createdBy := uuidValue(out.String("created_by"))
	createdAt := timeValue(out["created_at"])
	updatedAt := timeValue(out["updated_at"])

	item := openapi.WorkItem{
		Id:           uuidValue(out.String("id")),
		Type:         openapi.ItemType(out.String("type")),
		CollectionId: uuidValue(out.String("collection_id")),
		Title:        out.String("title"),
		Completion:   completionResponse(out["completion"]),
		Path:         &path,
		Depth:        &depth,
		OrderKey:     &orderKey,
		CreatedBy:    &createdBy,
		CreatedAt:    &createdAt,
		UpdatedAt:    &updatedAt,
		Version:      out.Int("version"),
	}
	if parent := out.String("parent_id"); parent != "" {
		parentID := uuidValue(parent)
		item.ParentId = &parentID
	}
	if notes := out.String("notes"); notes != "" {
		item.Notes = &notes
	}
	return item
}

// completionResponse maps the done/open state. It is sent even on a create, where it is always
// open: the schema makes it required, and a client that has to special-case one response is a
// client that will forget to.
func completionResponse(value any) openapi.Completion {
	state, _ := value.(map[string]any)
	isCompleted, _ := state["is_completed"].(bool)

	completion := openapi.Completion{IsCompleted: &isCompleted}
	if at, ok := state["completed_at"].(time.Time); ok {
		completion.CompletedAt = &at
	}
	if by, ok := state["completed_by"].(string); ok && by != "" {
		completedBy := uuidValue(by)
		completion.CompletedBy = &completedBy
	}
	return completion
}
