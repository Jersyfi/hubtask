// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"strconv"
	"time"

	lifecyclerepo "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// PollTriggerEventsName is the use case behind GET /integrations/triggers/{eventType}.
const PollTriggerEventsName = "PollTriggerEvents"

// TriggerPolledAction is what a poll performs. Info severity and not required, for the reason a
// webhook listing is not: an entry per poll per trigger per minute would bury the entries a review
// is looking for under a feed nobody reads (audit.md §4).
const TriggerPolledAction audit.Action = "triggers.polled"

// defaultPollLimit is the page size automation.md §3.2 writes into the endpoint's own address, and
// maxPollLimit is api-guidelines.md §4's ceiling for any page.
const (
	defaultPollLimit = 100
	maxPollLimit     = 200
)

// Cursors turns a position in the outbox into an opaque string and back.
//
// An interface here and a keyed HMAC in infrastructure/security, for the change stream's reason:
// the installation secret has no business above the adapters (security.md §8), and the application
// never looks inside the string - which is what "opaque" means from this side, and what lets the
// encoding change without the contract changing with it.
type Cursors interface {
	Encode(position outbox.Position) string
	Decode(cursor string) (outbox.Position, error)
}

// EventRendering turns an envelope into the document a subscriber receives.
//
// The CloudEvents mapping is an adapter's (ADR-0007), and this layer may not reach into one. It is
// held behind an interface so that the pull half renders through the very same function the push
// half delivers - one schema and two transports, rather than two renderings that agree until one of
// them is changed.
type EventRendering interface {
	Render(envelope event.Envelope) map[string]any
}

// Page is one poll's answer: the events, and where the walk now stands.
type Page struct {
	// Events are the rendered documents, oldest first.
	Events []map[string]any
	// Cursor is the position after this page. It advances even when the page is empty, so that a
	// poller crawling a quiet stretch does not re-read it.
	Cursor outbox.Position
	// More says the page came back full, so the caller comes straight back rather than waiting for
	// its next scheduled poll.
	More bool
}

// PollTriggerEvents answers the tenant's events of one type, oldest first, from a cursor.
//
// The pull half of the stream G-03 pushes (automation.md §3.2), for a platform with no address a
// webhook could reach. The same events, the same document, the same identifier to deduplicate on.
//
// Three boundaries hold it together, and each is drawn once, here:
//
// The window is the outbox's retention period, which is the tenant's own (G-02, ADR-0020). A cursor
// older than it is refused rather than answered from the beginning of what is left: a poller that
// missed more than the window has to be told that it missed, or it goes on reporting a consistency
// it does not have.
//
// The horizon is how the cursor stays gapless, and Poll's own documentation says why.
//
// The authorisation is two scopes and both are checked. `automation:manage` is the endpoint's, the
// same one a webhook subscription needs - reading this workspace's whole event stream and pointing
// it at an address outside the workspace are the same power, and the pull half must not be the
// cheaper way to it. The event type's own read scope is the second: a token scoped to read items
// polls item events and is refused what it is not scoped for, which falls out of the scopes the
// catalogue already declares rather than out of a polling permission invented beside them.
type PollTriggerEvents struct {
	Events     outbox.Pollable
	Policies   lifecyclerepo.Policies
	Cursors    Cursors
	Rendering  EventRendering
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Lag is how far behind the present a page may reach. See environment.QueueConfig.
	Lag time.Duration
}

// PollTriggerEventsCommand is one poll.
type PollTriggerEventsCommand struct {
	// EventType is the full type, `de.hubtask.work.item.created.v1`.
	EventType string
	// Cursor is where the last poll stopped. Empty starts at the oldest event still inside the
	// window.
	Cursor string
	// Limit is the page size the caller asked for. Zero takes the default.
	Limit int
}

// Execute answers one page.
func (h PollTriggerEvents) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd PollTriggerEventsCommand,
) (Page, error) {
	eventType := event.Type(cmd.EventType)
	if !eventType.Valid() {
		// By name rather than with an empty page. A trigger configured against a typo would
		// otherwise poll forever and report nothing wrong, which is the failure a person debugging
		// their automation has no way to see.
		return Page{}, shared.ErrValidation.
			WithDetail("triggers.event_type_unknown").
			WithParams(map[string]string{"event_type": cmd.EventType}).
			WithFields(shared.FieldError{Path: "/eventType", Code: "triggers.event_type_unknown"})
	}

	// The endpoint's scope first, then the event's. Both, because ADR-0005 makes the scope the
	// second bound beside the role and this endpoint's role bound is the workspace-wide one every
	// other integration route carries.
	if err := actor.RequireScope(automationScope); err != nil {
		return Page{}, err
	}
	if err := actor.RequireScope(eventType.ReadScope()); err != nil {
		return Page{}, err
	}

	now := h.Clock.Now()

	var (
		policy    lifecycle.Policy
		envelopes []event.Envelope
		from      outbox.Position
	)
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		switch policy, err = h.Policies.Find(ctx, lifecycle.KindOutboxEvent); {
		case errors.Is(err, shared.ErrNotFound):
			// A tenant the retention run has not reached yet. The documented default is the period
			// in force for it - that is what the run is about to write - and answering the caller
			// a 404 for the event type they asked about would be a lie about which thing is
			// missing (ADR-0020: periods are data, and the default is one of the data).
			policy = defaultOutboxPolicy()
		case err != nil:
			return err
		}

		if from, err = h.resume(cmd.Cursor, policy, now); err != nil {
			return err
		}
		envelopes, err = h.Events.Poll(ctx, eventType, from, now.Add(-h.Lag), boundedLimit(cmd.Limit))
		return err
	})
	if err != nil {
		return Page{}, err
	}

	rendered := make([]map[string]any, 0, len(envelopes))
	for _, envelope := range envelopes {
		rendered = append(rendered, h.Rendering.Render(envelope))
	}

	// The position of the last row read, and the one it started from when nothing was read. A
	// cursor that stalled on an empty page would re-walk the same quiet stretch on every poll.
	cursor := from
	if last := len(envelopes) - 1; last >= 0 {
		cursor = outbox.Position{OccurredAt: envelopes[last].OccurredAt, ID: envelopes[last].ID}
	}

	return Page{
		Events: rendered,
		Cursor: cursor,
		More:   len(envelopes) == boundedLimit(cmd.Limit),
	}, nil
}

// resume decides where the walk starts, and refuses a cursor the window no longer covers.
func (h PollTriggerEvents) resume(
	cursor string, policy lifecycle.Policy, now time.Time,
) (outbox.Position, error) {
	cutoff := policy.Cutoff(now)

	if cursor == "" {
		// The oldest event still inside the window, rather than the newest. A caller with no
		// cursor is one that has never polled or one that lost its cursor, and both need what is
		// there - beginning at the present would hand them a gap they cannot see, and the window
		// is what bounds how much "what is there" can be.
		//
		// The identifier is the zero value, which the adapter reads as "before every row".
		return outbox.Position{OccurredAt: cutoff}, nil
	}

	from, err := h.Cursors.Decode(cursor)
	if err != nil {
		return outbox.Position{}, err
	}
	if from.OccurredAt.Before(cutoff) {
		// The only honest answer. The outbox keeps the retention period and no longer, so the
		// events between this cursor and the cutoff are gone; answering from the cutoff would
		// silently omit them, and a poller that was quietly restarted would go on reporting a
		// completeness it does not have.
		return outbox.Position{}, shared.ErrGone.
			WithDetail("triggers.cursor_expired").
			WithParams(map[string]string{"retain_days": strconv.Itoa(policy.RetainDays)})
	}
	return from, nil
}

// defaultOutboxPolicy is the period a tenant has before the retention run writes it one. Read off
// the documented defaults rather than restated, so that the two cannot drift.
func defaultOutboxPolicy() lifecycle.Policy {
	for _, policy := range lifecycle.DefaultPolicies() {
		if policy.DataKind == lifecycle.KindOutboxEvent {
			return policy
		}
	}
	// Unreachable while KindOutboxEvent is in DefaultPolicies, and a zero period would be a window
	// of nothing rather than a window nobody set. TestTheOutboxDefaultIsReadOffTheCatalogue holds
	// the two together.
	return lifecycle.Policy{DataKind: lifecycle.KindOutboxEvent}
}

// boundedLimit narrows a page size the caller asked for. Zero takes automation.md §3.2's hundred,
// and nothing exceeds api-guidelines.md §4's ceiling - a limit the specification happens to allow
// today is not a limit this layer wants to learn from a request tomorrow.
func boundedLimit(limit int) int {
	switch {
	case limit < 1:
		return defaultPollLimit
	case limit > maxPollLimit:
		return maxPollLimit
	default:
		return limit
	}
}

// Descriptor is the catalogue entry.
func (h PollTriggerEvents) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: PollTriggerEventsName,
		Summary: "The workspace's events of one type, oldest first, from a cursor - the pull " +
			"half of what a webhook subscription is pushed. The body of each entry is the same " +
			"CloudEvent a delivery would have carried, and `id` is the same value " +
			"`X-Hubtask-Event-Id` names, so a consumer deduplicates on it either way. A cursor " +
			"older than the outbox's retention period is refused rather than answered from the " +
			"beginning: a poller that missed more than the window has to be told that it missed.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "event_type", Kind: usecase.KindString, Required: true,
				Description: "The type to poll, in full: de.hubtask.work.item.created.v1.",
			},
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "Where the last poll stopped. Absent starts at the oldest event " +
					"still inside the retention window.",
			},
			{
				Name: "limit", Kind: usecase.KindInt,
				Description: "How many events at most. A hundred by default, two hundred at most.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TriggerPolledAction, TargetType: triggerTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A poll is a read of the event log, not something that happened to an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// triggerTarget is what a poll is recorded against: the event type, which is the only thing about
// a poll that is worth naming and the only thing about it that is not user content (rule 10).
const triggerTarget = "trigger"

func (h PollTriggerEvents) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	page, err := h.Execute(ctx, actor, PollTriggerEventsCommand{
		EventType: in.String("event_type"),
		Cursor:    in.String("cursor"),
		Limit:     in.Int("limit"),
	})
	if err != nil {
		return nil, err
	}

	rows := make([]any, 0, len(page.Events))
	for _, rendered := range page.Events {
		rows = append(rows, rendered)
	}
	return usecase.Output{
		"data":        rows,
		"next_cursor": h.Cursors.Encode(page.Cursor),
		"has_more":    page.More,
	}, nil
}
