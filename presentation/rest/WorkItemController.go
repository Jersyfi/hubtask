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
		"bucket_id":     optionalUUIDField(body.BucketId),
		// Empty is not a value here but the absence of one: the use case then takes the creator's
		// locale, which is what an unstated content language falls back to (C-08).
		"content_language": optionalStringField(body.ContentLanguage),
	}
	// The two assignment fields the create path serves since C-02: a named person, or the
	// collection's policy asked for explicitly. Both optional, and the catalogue refuses the
	// combination.
	if body.AssigneeId != nil {
		in["assignee_id"] = body.AssigneeId.String()
	}
	if body.AutoAssign != nil {
		in["auto_assign"] = *body.AutoAssign
	}
	// The schedule the create path serves since D-01: the start as the create's own field, and
	// the due trio dispatching into the writer the due route owns.
	if body.StartAt != nil {
		in["start_at"] = body.StartAt.Format(time.RFC3339Nano)
	}
	if body.DueAt != nil {
		in["due_at"] = body.DueAt.Format(time.RFC3339Nano)
	}
	if body.DueDateOnly != nil {
		in["due_date_only"] = *body.DueDateOnly
	}
	if body.DueTimeZone != nil {
		in["due_time_zone"] = *body.DueTimeZone
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

// UpdateWorkItem answers PATCH /items/{itemId}.
//
// A JSON Merge Patch (RFC 7386), which is why presence is read rather than the value alone: an
// absent `notes` means "leave them alone" and `"notes": null` means "clear them", and both arrive
// as a nil pointer in the generated struct. A handler that could not tell them apart would clear
// the notes of every client that only meant to rename something.
//
// Null reaches the catalogue as the empty string. The domain holds `notes` as a string whose empty
// value is "none", so cleared and empty are one state there - and inventing a second spelling for
// it in this layer would be a distinction only this layer believed in.
func (c *RestController) UpdateWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, params openapi.UpdateWorkItemParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.WorkItemUpdate
	present, err := decodeJSONWithPresence(r, &body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String()}
	if present["title"] {
		// Null is not an instruction here: the contract types `title` as a plain string, an item
		// without one cannot exist, and the catalogue refuses an empty one by name.
		in["title"] = stringOrEmpty(body.Title)
	}
	if present["notes"] {
		in["notes"] = stringOrEmpty(body.Notes)
	}
	if present["bucket_id"] {
		// Null and the empty string are one instruction: take the entry off the board. That is what
		// makes them a different request from omitting the field, which leaves the column alone.
		in["bucket_id"] = uuidOrEmpty(body.BucketId)
	}
	if present["content_language"] {
		// The same shape: null clears the statement, and omitting the member leaves the entry
		// indexed under the language it already declared.
		in["content_language"] = stringOrEmpty(body.ContentLanguage)
	}
	// The schedule (D-01), by presence like everything above. Null reaches the catalogue as the
	// empty string; for `due_at` that clears the trio whole, per the contract.
	if present["start_at"] {
		in["start_at"] = ""
		if body.StartAt != nil {
			in["start_at"] = body.StartAt.Format(time.RFC3339Nano)
		}
	}
	if present["due_at"] {
		in["due_at"] = ""
		if body.DueAt != nil {
			in["due_at"] = body.DueAt.Format(time.RFC3339Nano)
		}
	}
	if present["due_date_only"] {
		in["due_date_only"] = body.DueDateOnly != nil && *body.DueDateOnly
	}
	if present["due_time_zone"] {
		in["due_time_zone"] = ""
		if body.DueTimeZone != nil {
			in["due_time_zone"] = *body.DueTimeZone
		}
	}
	withUnservedItemUpdateFields(body, present, in)

	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), updateWorkItemUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	// The ETag is the version after the change, which is what a client needs in order to follow the
	// update with another one (api-guidelines.md §5).
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, workItemResponse(out))
}

// updateWorkItemUseCase is the catalogue name; the route comes from the specification.
const updateWorkItemUseCase = "UpdateWorkItem"

// stringOrEmpty reads a merge patch member that may be null. Null and empty are one instruction for
// these fields - "make it say nothing" - so both come back as the empty string.
func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// uuidOrEmpty is the identifier counterpart: null and an absent value are the empty string, which
// the catalogue reads as "clear it". The difference between clearing and leaving alone is carried
// by the presence map, not by the value.
func uuidOrEmpty(value *openapi_types.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

// withUnservedItemUpdateFields passes on the members of WorkItemUpdate that no use case writes yet,
// so that the catalogue refuses them by name.
//
// The same reasoning as withUnservedItemFields on the create path, and the failure it prevents is
// worse here: a client that clears a due date with a merge patch and receives a 200 believes the
// reminder is gone. Presence rather than the value, because null is what clearing looks like and
// dropping it would be exactly the silence this exists to prevent.
//
// `assignee_id` stays here now that C-01 has landed, and for a different reason from the rest: an
// assignment is not a field of a merge patch, it is `POST /items/{id}:assign`. Two ways to write one
// column would be two places deciding whether the person being given the entry may see it, which is
// the check that makes the assignment mean anything. Refused by name here, so that a client sending
// it is told where to send it instead rather than believing it was stored.
//
// The others disappear when their use case lands: the cover in 0.3.0, the custom fields with
// theirs. The bucket left this list with B-09, and the due date with D-01 - the patch now
// dispatches the trio into the writer the due route owns.
func withUnservedItemUpdateFields(body openapi.WorkItemUpdate, present map[string]bool, in usecase.Input) {
	if present["assignee_id"] {
		in["assignee_id"] = ""
		if body.AssigneeId != nil {
			in["assignee_id"] = body.AssigneeId.String()
		}
	}
	if present["cover"] {
		in["cover"] = map[string]any{}
	}
	if present["custom_fields"] {
		in["custom_fields"] = map[string]any{}
		if body.CustomFields != nil {
			in["custom_fields"] = map[string]any(*body.CustomFields)
		}
	}
}

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
// Each entry disappears from this list when the create path serves it. `assignee_id` and
// `auto_assign` left it with C-02, which took the decision C-01 deferred: an entry may be created
// already assigned, by name or by the collection's policy. `member_ids` stays for the reason
// `label_ids` stayed after B-09: the endpoint that owns the set is its own
// (`/items/{id}/members/{accountId}`), and no task has yet decided that a create may seed it.
// The cover follows in 0.3.0. The bucket left this list with B-09, and the due date with D-01.
func withUnservedItemFields(body openapi.WorkItemCreate, in usecase.Input) {
	if body.BeforeItemId != nil {
		in["before_item_id"] = body.BeforeItemId.String()
	}
	if body.LabelIds != nil {
		in["label_ids"] = uuidList(*body.LabelIds)
	}
	if body.MemberIds != nil {
		in["member_ids"] = uuidList(*body.MemberIds)
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
	if language := out.String("content_language"); language != "" {
		item.ContentLanguage = &language
	}
	if parent := out.String("parent_id"); parent != "" {
		parentID := uuidValue(parent)
		item.ParentId = &parentID
	}
	if notes := out.String("notes"); notes != "" {
		item.Notes = &notes
	}
	if bucket := out.String("bucket_id"); bucket != "" {
		bucketID := uuidValue(bucket)
		item.BucketId = &bucketID
	}
	if assignee := out.String("assignee_id"); assignee != "" {
		assigneeID := uuidValue(assignee)
		item.AssigneeId = &assigneeID
	}
	// Present exactly when automatic assignment ran (C-02): the outcome of an :auto-assign call,
	// or of a create a policy applied to. Absent means it did not run, which is a different
	// answer from "it ran and assigned nobody".
	if outcome, ran := out["auto_assign"].(map[string]any); ran {
		assigned, _ := outcome["assigned"].(bool)
		strategy, _ := outcome["strategy"].(string)
		result := &openapi.AutoAssignOutcome{
			Assigned: assigned, Strategy: openapi.AutoAssignStrategy(strategy),
		}
		if code, told := outcome["code"].(string); told {
			result.Code = &code
		}
		item.AutoAssign = result
	}
	// Present only when the caller asked for it with `expand=labels`, and then an empty array when
	// the entry carries none: absent means "not asked for", which is a different answer from "none".
	if ids, asked := out["label_ids"].([]string); asked {
		labels := make([]openapi_types.UUID, 0, len(ids))
		for _, id := range ids {
			labels = append(labels, uuidValue(id))
		}
		item.LabelIds = &labels
	}
	item.Cover = coverResponse(out["cover"])
	if values, carried := out["custom_fields"].(map[string]any); carried {
		item.CustomFields = &values
	}
	// The schedule (D-01). The flag rides along only when a due date is there: the schema defaults
	// it to false, and a flag on an entry with no date is a state nothing can store.
	item.StartAt, item.DueAt = optionalTimeField(out["start_at"]), optionalTimeField(out["due_at"])
	if item.DueAt != nil {
		flag, _ := out["due_date_only"].(bool)
		item.DueDateOnly = &flag
		if zone := out.String("due_time_zone"); zone != "" {
			item.DueTimeZone = &zone
		}
	}
	item.ArchivedAt, item.DeletedAt = optionalTimeField(out["archived_at"]), optionalTimeField(out["deleted_at"])
	// What a retention rule has announced about this entry, present only while one has
	// (data-retention.md §6). Absent means nothing is coming, which is a different answer from an
	// object full of nulls.
	item.Retention = retentionStateResponse(out["retention"])
	return item
}

// coverResponse maps the cover, or nothing when the entry carries none. Absent rather than an
// object with three nulls: the schema makes the whole field optional, and a client reading `cover`
// as present would draw a card with a picture nobody chose (C-06).
func coverResponse(value any) *openapi.Cover {
	fields, carried := value.(map[string]any)
	if !carried {
		return nil
	}

	kind := openapi.CoverKind(stringOf(fields["kind"]))
	cover := openapi.Cover{Kind: &kind}
	if token := stringOf(fields["color_token"]); token != "" {
		cover.ColorToken = &token
	}
	if mediaID := stringOf(fields["media_id"]); mediaID != "" {
		id := uuidValue(mediaID)
		cover.MediaId = &id
	}
	return &cover
}

// stringOf reads an optional text out of a projection, where an absent value is an untyped nil
// rather than an empty string.
func stringOf(value any) string {
	text, _ := value.(string)
	return text
}

// optionalTimeField maps a projection's optional instant. The generated fields carry omitempty, so an
// unset one is absent rather than null - which the schema allows either way, and which is the shape a
// create already produced before the read side existed.
func optionalTimeField(value any) *time.Time {
	at, ok := value.(time.Time)
	if !ok {
		return nil
	}
	return &at
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

// retentionStateResponse maps what a retention rule has announced about an entry.
//
// `can_retain` is what a client puts a button behind, and it is the server's answer rather than the
// client's inference: taking an entry out is a permission question, and a client that decided it
// from the presence of a date would show a button that then refuses.
func retentionStateResponse(value any) *openapi.RetentionState {
	announced, present := value.(map[string]any)
	if !present {
		return nil
	}

	state := &openapi.RetentionState{}
	if action, named := announced["action"].(string); named {
		state.Action = &action
	}
	if at := timeValue(announced["effective_at"]); !at.IsZero() {
		moment := at
		state.EffectiveAt = &moment
	}
	if policy, named := announced["policy_id"].(string); named && policy != "" {
		id := uuidValue(policy)
		state.PolicyId = &id
	}
	if canRetain, told := announced["can_retain"].(bool); told {
		state.CanRetain = &canRetain
	}
	return state
}
