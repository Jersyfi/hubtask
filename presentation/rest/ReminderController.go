// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The reminders beside the entries (D-02): a sub-resource of the item, because a reminder is not a
// field of its row - it has its own moment, its own channels and its own recipients.

const (
	listRemindersUseCase  = "ListReminders"
	createReminderUseCase = "CreateReminder"
	updateReminderUseCase = "UpdateReminder"
	deleteReminderUseCase = "DeleteReminder"
)

// ListReminders answers GET /items/{itemId}/reminders.
func (c *RestController) ListReminders(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
) {
	out, ok := c.read(w, r, listRemindersUseCase, usecase.Input{"item_id": itemID.String()})
	if !ok {
		return
	}

	// An array rather than an envelope: what one entry may carry is bounded, so there is no page
	// state to report.
	reminders := []openapi.Reminder{}
	for _, row := range rowsOf(out) {
		reminders = append(reminders, reminderResponse(row))
	}
	writeJSON(w, r, http.StatusOK, reminders)
}

// CreateReminder answers POST /items/{itemId}/reminders.
func (c *RestController) CreateReminder(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	_ openapi.CreateReminderParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.CreateReminderJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String(), "offset_spec": body.OffsetSpec}
	if body.Channels != nil {
		in["channels"] = channelNames(*body.Channels)
	}
	if body.Recipients != nil {
		in["recipients"] = accountIDs(*body.Recipients)
	}

	out, err := c.UseCases.Invoke(r.Context(), createReminderUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusCreated, reminderResponse(out))
}

// UpdateReminder answers PATCH /items/{itemId}/reminders/{reminderId}.
func (c *RestController) UpdateReminder(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	reminderID openapi.ReminderId, params openapi.UpdateReminderParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.UpdateReminderJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String(), "reminder_id": reminderID.String()}
	if body.OffsetSpec != nil {
		in["offset_spec"] = *body.OffsetSpec
	}
	// Presence rather than the value: an absent list means "not touched" and an empty one means
	// "the assignee and the members", and a handler that could not tell them apart would answer
	// one caller's request with the other's meaning.
	if body.Channels != nil {
		in["channels"] = channelNames(*body.Channels)
	}
	if body.Recipients != nil {
		in["recipients"] = accountIDs(*body.Recipients)
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), updateReminderUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, reminderResponse(out))
}

// DeleteReminder answers DELETE /items/{itemId}/reminders/{reminderId}. 204: the caller asked for
// the reminder to be gone, and it is - there is no tombstone to read.
func (c *RestController) DeleteReminder(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	reminderID openapi.ReminderId, params openapi.DeleteReminderParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String(), "reminder_id": reminderID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	if _, err := c.UseCases.Invoke(r.Context(), deleteReminderUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// channelNames maps the contract's channel values onto the plain strings the catalogue takes. The
// generated type is a string underneath, and an unknown value travels rather than being dropped
// here: which channels exist is the domain's answer, and its refusal names the value it refused.
func channelNames(channels []openapi.ReminderChannel) []string {
	names := make([]string, 0, len(channels))
	for _, channel := range channels {
		names = append(names, string(channel))
	}
	return names
}

// accountIDs maps the contract's recipients onto the catalogue's identifiers.
func accountIDs(recipients []openapi_types.UUID) []string {
	ids := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		ids = append(ids, recipient.String())
	}
	return ids
}

// reminderResponse maps the catalogue's output onto the generated schema. The mapping lives here
// because the generated types are the contract's shape rather than the domain's
// (project-structure.md §3).
func reminderResponse(out usecase.Output) openapi.Reminder {
	reminder := openapi.Reminder{
		Id:         uuidValue(out.String("id")),
		ItemId:     uuidValue(out.String("item_id")),
		OffsetSpec: out.String("offset_spec"),
		Channels:   []openapi.ReminderChannel{},
		Recipients: []openapi_types.UUID{},
		State:      openapi.ReminderState(out.String("state")),
		CreatedAt:  timeValue(out["created_at"]),
		Version:    out.Int("version"),
	}
	for _, channel := range stringsOf(out["channels"]) {
		reminder.Channels = append(reminder.Channels, openapi.ReminderChannel(channel))
	}
	for _, recipient := range stringsOf(out["recipients"]) {
		reminder.Recipients = append(reminder.Recipients, uuidValue(recipient))
	}
	if at, ok := out["fire_at"].(time.Time); ok {
		reminder.FireAt = &at
	}
	if at, ok := out["updated_at"].(time.Time); ok {
		reminder.UpdatedAt = &at
	}
	return reminder
}
