// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The series beside the entries: a sub-resource of the entry, because a rule is one thing
// an entry either carries or does not - which is also why one PUT sets and changes it.

// recurrenceResponse maps the catalogue's output onto the generated schema. The mapping lives here
// because the generated types are the contract's shape rather than the domain's
// (project-structure.md §3).
func recurrenceResponse(out usecase.Output) openapi.Recurrence {
	rule := openapi.Recurrence{
		Id:          uuidValue(out.String("id")),
		ItemId:      uuidValue(out.String("item_id")),
		Rrule:       out.String("rrule"),
		TimeZone:    out.String("time_zone"),
		Mode:        openapi.RecurrenceMode(out.String("mode")),
		HorizonDays: out.Int("horizon_days"),
		CreatedAt:   timeValue(out["created_at"]),
		Version:     out.Int("version"),
	}
	if at, ok := out["ends_at"].(time.Time); ok {
		rule.EndsAt = &at
	}
	if at, ok := out["last_materialized_at"].(time.Time); ok {
		rule.LastMaterializedAt = &at
	}
	if at, ok := out["updated_at"].(time.Time); ok {
		rule.UpdatedAt = &at
	}
	if count, ok := out["max_count"].(int); ok {
		rule.MaxCount = &count
	}
	return rule
}
