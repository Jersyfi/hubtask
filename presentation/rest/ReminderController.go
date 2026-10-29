// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The reminders beside the entries: a sub-resource of the item, because a reminder is not a
// field of its row - it has its own moment, its own channels and its own recipients.

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
