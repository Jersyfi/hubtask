// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

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
	if uid := out.String("calendar_uid"); uid != "" {
		item.CalendarUid = &uid
	}
	if number, ok := out["number"].(int64); ok {
		item.Number = &number
	}
	if entryType := out.String("entry_type"); entryType != "" {
		item.EntryType = &entryType
	}
	if status := out.String("status"); status != "" {
		item.Status = &status
	}
	if parent := out.String("parent_id"); parent != "" {
		parentID := uuidValue(parent)
		item.ParentId = &parentID
	}
	if notes := out.String("notes"); notes != "" {
		item.Notes = &notes
	}
	if amount := out.String("amount"); amount != "" {
		item.Amount = &amount
	}
	if _, counted := out["completion_count"]; counted {
		count := out.Int("completion_count")
		item.CompletionCount = &count
	}
	if bucket := out.String("bucket_id"); bucket != "" {
		bucketID := uuidValue(bucket)
		item.BucketId = &bucketID
	}
	if assignee := out.String("assignee_id"); assignee != "" {
		assigneeID := uuidValue(assignee)
		item.AssigneeId = &assigneeID
	}
	if origin := out.String("origin_jumble_id"); origin != "" {
		originID := uuidValue(origin)
		item.OriginJumbleId = &originID
	}
	if rule := out.String("recurrence_rule_id"); rule != "" {
		ruleID := uuidValue(rule)
		item.RecurrenceRuleId = &ruleID
	}
	if source := out.String("recurrence_source_id"); source != "" {
		sourceID := uuidValue(source)
		item.RecurrenceSourceId = &sourceID
	}
	// What the entry's collection hides of it, where the read answered it - every read of an
	// entry does on a wired server (domain-model.md §2).
	if mark, marked := out["hidden"].(map[string]any); marked {
		level, _ := mark["level"].(bool)
		capabilities, _ := mark["capabilities"].([]string)
		if capabilities == nil {
			capabilities = []string{}
		}
		below, _ := mark["below"].(int)
		item.Hidden = &openapi.EntryHidden{Level: level, Capabilities: capabilities, Below: below}
	}
	// Present exactly when automatic assignment ran: the outcome of an :auto-assign call,
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
	// Present only with `expand=counts` on a query, as the labels are.
	if counts, asked := out["counts"].(usecase.Output); asked {
		item.Counts = &struct {
			Attachments int `json:"attachments"`
			Comments    int `json:"comments"`
			Parts       int `json:"parts"`
			PartsDone   int `json:"parts_done"`
		}{
			Attachments: counts.Int("attachments"),
			Comments:    counts.Int("comments"),
			Parts:       counts.Int("parts"),
			PartsDone:   counts.Int("parts_done"),
		}
	}
	item.Cover = coverResponse(out["cover"])
	if values, carried := out["custom_fields"].(map[string]any); carried {
		item.CustomFields = &values
	}
	// The schedule. The flag rides along only when a due date is there: the schema defaults
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
// as present would draw a card with a picture nobody chose.
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
	if blocked, held := announced["blocked_by"].(string); held && blocked != "" {
		reason := openapi.RetentionStateBlockedBy(blocked)
		state.BlockedBy = &reason
	}
	return state
}
