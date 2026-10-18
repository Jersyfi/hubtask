// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"strconv"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListPlatformEventsName = "ListPlatformEvents"

	// PlatformEventsRetainDays is how long the installation's events are kept, and so how far back
	// a cursor may reach (docs/privacy/data-catalog.md): long enough for a platform down over a
	// holiday, short enough not to keep a reference longer than the deliveries do.
	PlatformEventsRetainDays = 30

	platformEventsReadAction audit.Action = "instance.platform_events_read"

	defaultPlatformEventsPage = 50
	maxPlatformEventsPage     = 200
)

// PlatformEventsRetention is the window as a duration.
const PlatformEventsRetention = PlatformEventsRetainDays * 24 * time.Hour

// PlatformCursors turns a position in the installation's events into an opaque cursor and back. An
// interface here and a keyed HMAC in the adapters, for the trigger cursor's reason: the installation
// secret has no business above them.
type PlatformCursors interface {
	Encode(position adminrepo.PlatformPosition) string
	Decode(cursor string) (adminrepo.PlatformPosition, error)
}

// EventRendering turns an envelope into the CloudEvent a subscriber receives - the adapter's
// mapping, so the feed answers the very document a delivery sends.
type EventRendering interface {
	Render(envelope event.Envelope) map[string]any
}

// ListPlatformEvents reads the installation's events forward from a cursor (UC-INS-14 check 6):
// the same events the installation's subscriptions are sent, so a platform that was down reads what
// it missed. Numbered in commit order, so a page never skips an event that commits later.
type ListPlatformEvents struct {
	Writer    InstanceWriter
	Events    adminrepo.PlatformEvents
	Cursors   PlatformCursors
	Rendering EventRendering
	Tenancy   env.TenancyMode
}

// PlatformEventsPage is one read: the rendered events, oldest first, the position after them, and
// whether to read again at once.
type PlatformEventsPage struct {
	Events []map[string]any
	Cursor adminrepo.PlatformPosition
	More   bool
}

// Execute reads one page.
func (h ListPlatformEvents) Execute(
	ctx context.Context, actor appshared.ActorContext, cursor string, size int,
) (PlatformEventsPage, error) {
	if err := h.Writer.authorize(ctx, actor, platformScopes...); err != nil {
		return PlatformEventsPage{}, err
	}
	if err := requireMulti(h.Tenancy); err != nil {
		return PlatformEventsPage{}, err
	}

	now := h.Writer.Clock.Now()
	from := adminrepo.PlatformPosition{}
	if cursor != "" {
		decoded, err := h.Cursors.Decode(cursor)
		if err != nil {
			return PlatformEventsPage{}, err
		}
		if decoded.At.Before(now.Add(-PlatformEventsRetention)) {
			// What followed this cursor may be swept already; answering from what is left would
			// hand the platform a gap it cannot see.
			return PlatformEventsPage{}, shared.ErrGone.
				WithDetail("admin.events_cursor_expired").
				WithParams(map[string]string{"retain_days": strconv.Itoa(PlatformEventsRetainDays)})
		}
		from = decoded
	}

	limit := platformEventsPageSize(size)
	var read []adminrepo.PlatformEvent
	err := h.Writer.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			events, err := h.Events.After(ctx, from.Number, limit+1)
			read = events
			return err
		})
	if err != nil {
		return PlatformEventsPage{}, err
	}

	page := PlatformEventsPage{More: len(read) > limit}
	if page.More {
		read = read[:limit]
	}
	page.Events = make([]map[string]any, 0, len(read))
	for _, e := range read {
		page.Events = append(page.Events, h.Rendering.Render(e.Envelope))
	}
	// Caught up, the cursor carries the read's own moment: everything until now has been seen, and
	// a platform that finds nothing new for a month must not be told it missed something. Mid-way,
	// it carries the last event's moment, since the rest has not been read.
	page.Cursor = adminrepo.PlatformPosition{Number: from.Number, At: now}
	if len(read) > 0 {
		page.Cursor.Number = read[len(read)-1].Number
		if page.More {
			page.Cursor.At = read[len(read)-1].Envelope.OccurredAt
		}
	}
	return page, nil
}

func platformEventsPageSize(requested int) int {
	switch {
	case requested < 1:
		return defaultPlatformEventsPage
	case requested > maxPlatformEventsPage:
		return maxPlatformEventsPage
	default:
		return requested
	}
}

// Descriptor is the catalogue entry.
func (h ListPlatformEvents) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListPlatformEventsName,
		Summary: "The installation's events, oldest first, from a cursor: what a purchase platform " +
			"learns about the workspaces - created, moved to another plan, suspended, active again, " +
			"deletion requested, cancelled and done, an owner changed, a limit reached, the AI " +
			"budget spent, an export finished. The same CloudEvents the installation's " +
			"subscriptions are sent, numbered in commit order; identifiers and states only. A " +
			"cursor older than the 30 days kept is refused rather than answered from what is left.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		AlsoScope:   provisioningScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "cursor", Kind: usecase.KindString,
				Description: "Where the last read stopped. Absent starts at the oldest event kept."},
			{Name: "limit", Kind: usecase.KindInt,
				Description: "How many events at most: fifty by default, two hundred at most."},
		},
		Audit: usecase.AuditDeclaration{
			Action: platformEventsReadAction, TargetType: instanceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListPlatformEvents) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	page, err := h.Execute(ctx, actor, in.String("cursor"), in.Int("limit"))
	if err != nil {
		return nil, err
	}
	rows := make([]any, 0, len(page.Events))
	for _, rendered := range page.Events {
		rows = append(rows, rendered)
	}
	// The cursor is always present: it is where the next read starts, even after an empty page.
	return usecase.Output{
		"data": rows,
		"page": map[string]any{
			"next_cursor": h.Cursors.Encode(page.Cursor),
			"has_more":    page.More,
		},
	}, nil
}
