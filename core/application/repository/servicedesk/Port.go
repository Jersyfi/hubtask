// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package servicedesk is the persistence port of the service desk's building blocks: business
// calendars, SLA policies and each entry's SLA record (migration 0240).
package servicedesk

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Calendars stores the workspace's business calendars.
type Calendars interface {
	// Insert writes a new calendar; a key the workspace holds is `calendars.key_taken`.
	Insert(ctx context.Context, cal servicedesk.Calendar) error
	// Update writes the calendar where its version is still expectedVersion.
	Update(ctx context.Context, cal servicedesk.Calendar, expectedVersion int) error
	// Find answers the calendar of a key; shared.ErrNotFound when there is none.
	Find(ctx context.Context, key string) (servicedesk.Calendar, error)
	List(ctx context.Context) ([]servicedesk.Calendar, error)
	// Delete removes the calendar; one a policy names is `calendars.in_use`.
	Delete(ctx context.Context, key string) error
}

// Policies stores the workspace's SLA policies.
type Policies interface {
	// Insert writes a new policy; a key the workspace holds is `sla_policies.key_taken`, a
	// calendar it does not hold `sla_policies.calendar_unknown`.
	Insert(ctx context.Context, policy servicedesk.SlaPolicy) error
	Update(ctx context.Context, policy servicedesk.SlaPolicy, expectedVersion int) error
	Find(ctx context.Context, key string) (servicedesk.SlaPolicy, error)
	// List answers every policy by position and key.
	List(ctx context.Context) ([]servicedesk.SlaPolicy, error)
	Delete(ctx context.Context, key string) error
}

// Records stores what became of each goal on an entry.
type Records interface {
	// Save writes the goal's record whole, one per entry and goal.
	Save(ctx context.Context, record servicedesk.EntrySla, now time.Time) error
	// OfItem answers the entry's records, by goal.
	OfItem(ctx context.Context, itemID shared.ID) ([]servicedesk.EntrySla, error)
	// Report counts the goals armed in [from, to) per policy, goal and standing.
	Report(ctx context.Context, from, to time.Time) ([]servicedesk.ReportRow, error)
}

// ReplyTemplates stores the workspace's canned replies.
type ReplyTemplates interface {
	// Insert writes a new template; a key the workspace holds is `reply_templates.key_taken`.
	Insert(ctx context.Context, template servicedesk.ReplyTemplate) error
	Update(ctx context.Context, template servicedesk.ReplyTemplate, expectedVersion int) error
	// Find answers the template of a key; shared.ErrNotFound when there is none.
	Find(ctx context.Context, key string) (servicedesk.ReplyTemplate, error)
	// List answers every template by key.
	List(ctx context.Context) ([]servicedesk.ReplyTemplate, error)
	Delete(ctx context.Context, key string) error
}
