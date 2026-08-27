// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// pollLag is the harness's lag. Any positive value does; what the tests exercise is that events
// younger than it are withheld and that the cursor is withheld with them.
const pollLag = time.Minute

// eventLog is the outbox in memory, kept in the order the table is read in.
type eventLog struct {
	rows []event.Envelope
	err  error
	// asked records every call, so a test can prove which horizon and which position the service
	// derived rather than only what came back.
	asked []pollCall
}

type pollCall struct {
	eventType event.Type
	after     outbox.Position
	horizon   time.Time
	limit     int
}

func (l *eventLog) Poll(
	_ context.Context, eventType event.Type, after outbox.Position, horizon time.Time, limit int,
) ([]event.Envelope, error) {
	l.asked = append(l.asked, pollCall{eventType, after, horizon, limit})
	if l.err != nil {
		return nil, l.err
	}

	var page []event.Envelope
	for _, row := range l.rows {
		if row.Type != eventType || row.Replay {
			continue
		}
		if row.OccurredAt.After(horizon) {
			continue
		}
		if !beyond(after, row) {
			continue
		}
		page = append(page, row)
		if len(page) == limit {
			break
		}
	}
	return page, nil
}

// beyond is the keyset the query expresses as `(occurred_at, id) > (…, …)`: strictly after the
// position, with the identifier breaking a tie inside one microsecond.
func beyond(after outbox.Position, row event.Envelope) bool {
	if after.OccurredAt.Equal(row.OccurredAt) {
		return after.ID < row.ID
	}
	return after.OccurredAt.Before(row.OccurredAt)
}

// policies answers the tenant's retention period, or reports that it has none yet.
type policies struct {
	policy  lifecycle.Policy
	missing bool
}

func (p policies) Ensure(context.Context, []lifecycle.Policy) error { return nil }

func (p policies) Find(_ context.Context, kind lifecycle.DataKind) (lifecycle.Policy, error) {
	if p.missing {
		return lifecycle.Policy{}, shared.ErrNotFound.WithDetail("retention.policy_not_found")
	}
	if p.policy.DataKind != kind {
		return lifecycle.Policy{}, shared.ErrNotFound.WithDetail("retention.policy_not_found")
	}
	return p.policy, nil
}

// plainCursors is the codec without the signature: the application never looks inside the string,
// so a test of the application does not need a keyed one. The signing is proved where it lives.
type plainCursors struct{ refuse bool }

func (c plainCursors) Encode(position outbox.Position) string {
	if position.OccurredAt.IsZero() && position.ID == "" {
		return ""
	}
	return fmt.Sprintf("%d|%s", position.OccurredAt.UnixMicro(), position.ID)
}

func (c plainCursors) Decode(cursor string) (outbox.Position, error) {
	if c.refuse {
		return outbox.Position{}, shared.ErrValidation.WithDetail("triggers.cursor_invalid")
	}
	var micros int64
	var id string
	if _, err := fmt.Sscanf(cursor, "%d|%s", &micros, &id); err != nil {
		return outbox.Position{}, shared.ErrValidation.WithDetail("triggers.cursor_invalid")
	}
	return outbox.Position{OccurredAt: time.UnixMicro(micros).UTC(), ID: shared.ID(id)}, nil
}

// rendering stands in for the CloudEvents mapping. What this layer owes is that every envelope
// went through it exactly once and in order; the shape itself is the adapter's contract test.
type rendering struct{}

func (rendering) Render(envelope event.Envelope) map[string]any {
	return map[string]any{"id": envelope.ID.String(), "type": envelope.Type.String()}
}

func pollingActor(scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: author, AccountName: "Anna",
		Kind: shared.ActorUser, Scopes: scopes,
	}
}

// bothScopes is what an ordinary poller holds: the endpoint's, and the event's.
func bothScopes() []string { return []string{automationScope, event.ItemCreated.ReadScope()} }

func emitted(at time.Time, id string, eventType event.Type) event.Envelope {
	return event.Envelope{
		ID: shared.ID(id), Type: eventType, TenantID: tenant, OccurredAt: at,
		Payload: map[string]any{},
	}
}

type pollHarness struct {
	handler PollTriggerEvents
	log     *eventLog
}

func newPollHarness(rows ...event.Envelope) *pollHarness {
	log := &eventLog{rows: rows}
	return &pollHarness{
		log: log,
		handler: PollTriggerEvents{
			Events:   log,
			Policies: policies{policy: lifecycle.Policy{DataKind: lifecycle.KindOutboxEvent, RetainDays: 7}},
			Cursors:  plainCursors{}, Rendering: rendering{},
			UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), Lag: pollLag,
		},
	}
}

func TestAPollAnswersTheTypeItWasAskedFor(t *testing.T) {
	wanted := emitted(now.Add(-2*time.Hour), "01936f2a-0000-7000-8000-000000000001", event.ItemCreated)
	other := emitted(now.Add(-2*time.Hour), "01936f2a-0000-7000-8000-000000000002", event.ItemCompleted)
	h := newPollHarness(wanted, other)

	page, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{EventType: string(event.ItemCreated)})
	if err != nil {
		t.Fatalf("polling: %v", err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("%d events, want 1", len(page.Events))
	}
	if got := page.Events[0]["id"]; got != wanted.ID.String() {
		t.Errorf("answered %v, want %s", got, wanted.ID)
	}
	if page.Cursor.ID != wanted.ID {
		t.Errorf("the cursor stands at %s, want %s", page.Cursor.ID, wanted.ID)
	}
}

// The window's near end. Rows younger than the lag are withheld from the page, and the cursor is
// withheld with them - a cursor that had moved past an event still in flight would step over it.
func TestAPollWithholdsWhatIsYoungerThanTheLag(t *testing.T) {
	settled := emitted(now.Add(-2*pollLag), "01936f2a-0000-7000-8000-000000000001", event.ItemCreated)
	fresh := emitted(now.Add(-pollLag/2), "01936f2a-0000-7000-8000-000000000002", event.ItemCreated)
	h := newPollHarness(settled, fresh)

	page, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{EventType: string(event.ItemCreated)})
	if err != nil {
		t.Fatalf("polling: %v", err)
	}
	if len(page.Events) != 1 || page.Events[0]["id"] != settled.ID.String() {
		t.Fatalf("answered %v, want only the settled event", page.Events)
	}
	if page.Cursor.ID != settled.ID {
		t.Errorf("the cursor stands at %s, want %s - it must not pass the horizon",
			page.Cursor.ID, settled.ID)
	}
	if horizon := h.log.asked[0].horizon; !horizon.Equal(now.Add(-pollLag)) {
		t.Errorf("horizon %v, want %v", horizon, now.Add(-pollLag))
	}
}

// Two polls with the returned cursor neither repeat an event nor step over one.
func TestTwoPollsWithTheCursorNeitherRepeatNorSkip(t *testing.T) {
	var rows []event.Envelope
	for i := range 5 {
		rows = append(rows, emitted(now.Add(-time.Duration(10-i)*time.Minute),
			fmt.Sprintf("01936f2a-0000-7000-8000-00000000000%d", i+1), event.ItemCreated))
	}
	h := newPollHarness(rows...)

	first, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{EventType: string(event.ItemCreated), Limit: 3})
	if err != nil {
		t.Fatalf("the first poll: %v", err)
	}
	if !first.More {
		t.Error("a full page did not report more")
	}

	second, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{
			EventType: string(event.ItemCreated),
			Cursor:    plainCursors{}.Encode(first.Cursor),
		})
	if err != nil {
		t.Fatalf("the second poll: %v", err)
	}

	var seen []string
	for _, rendered := range append(first.Events, second.Events...) {
		seen = append(seen, rendered["id"].(string))
	}
	if len(seen) != len(rows) {
		t.Fatalf("saw %d events over two polls, want %d: %v", len(seen), len(rows), seen)
	}
	for i, row := range rows {
		if seen[i] != row.ID.String() {
			t.Errorf("position %d is %s, want %s", i, seen[i], row.ID)
		}
	}
}

// An empty page still advances nothing, and a poller crawling a quiet stretch does not re-walk it.
func TestAnEmptyPageKeepsTheCursorItStartedFrom(t *testing.T) {
	h := newPollHarness()

	from := outbox.Position{
		OccurredAt: now.Add(-time.Hour), ID: shared.ID("01936f2a-0000-7000-8000-000000000009"),
	}
	page, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{
			EventType: string(event.ItemCreated), Cursor: plainCursors{}.Encode(from),
		})
	if err != nil {
		t.Fatalf("polling: %v", err)
	}
	if len(page.Events) != 0 || page.More {
		t.Fatalf("an empty log answered %d events (more=%v)", len(page.Events), page.More)
	}
	if page.Cursor != from {
		t.Errorf("the cursor moved to %v, want %v", page.Cursor, from)
	}
}

// The far end of the window. A poller that missed more than the retention period has to be told
// that it missed, or it reports a completeness it does not have.
func TestACursorOlderThanTheRetentionIsRefused(t *testing.T) {
	h := newPollHarness()

	stale := outbox.Position{
		OccurredAt: now.AddDate(0, 0, -8), ID: shared.ID("01936f2a-0000-7000-8000-000000000001"),
	}
	_, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{
			EventType: string(event.ItemCreated), Cursor: plainCursors{}.Encode(stale),
		})
	if !errors.Is(err, shared.ErrGone) {
		t.Fatalf("error %v, want ErrGone", err)
	}

	var coded *shared.Error
	if !errors.As(err, &coded) {
		t.Fatalf("the refusal carries no code: %v", err)
	}
	if coded.DetailCode != "triggers.cursor_expired" {
		t.Errorf("detail %q, want triggers.cursor_expired", coded.DetailCode)
	}
	if coded.Params["retain_days"] != "7" {
		t.Errorf("params %v, want the window in days", coded.Params)
	}
	if len(h.log.asked) != 0 {
		t.Error("the log was read despite the refusal")
	}
}

// Without a cursor the walk starts at the oldest event the window still covers, never at the
// present: a caller that lost its cursor needs what is there, not a gap it cannot see.
func TestAPollWithNoCursorStartsAtTheWindow(t *testing.T) {
	h := newPollHarness()

	if _, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{EventType: string(event.ItemCreated)}); err != nil {
		t.Fatalf("polling: %v", err)
	}

	from := h.log.asked[0].after
	if want := now.AddDate(0, 0, -7); !from.OccurredAt.Equal(want) {
		t.Errorf("started at %v, want the retention cutoff %v", from.OccurredAt, want)
	}
	if !from.ID.IsZero() {
		t.Errorf("started at the identifier %s, want the zero value", from.ID)
	}
}

// A tenant the retention run has not reached yet has the documented period, not a 404 about an
// event type that is perfectly real.
func TestATenantWithNoPolicyGetsTheDocumentedWindow(t *testing.T) {
	h := newPollHarness()
	h.handler.Policies = policies{missing: true}

	if _, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{EventType: string(event.ItemCreated)}); err != nil {
		t.Fatalf("polling: %v", err)
	}
	if want := now.AddDate(0, 0, -7); !h.log.asked[0].after.OccurredAt.Equal(want) {
		t.Errorf("started at %v, want the default window %v", h.log.asked[0].after.OccurredAt, want)
	}
}

func TestTheOutboxDefaultIsReadOffTheCatalogue(t *testing.T) {
	if got := defaultOutboxPolicy(); got.DataKind != lifecycle.KindOutboxEvent || got.RetainDays < 1 {
		t.Fatalf("the default is %+v, which is not a window", got)
	}
}

// The acceptance criterion: scope enforcement is per event type, and a token holding the wrong one
// is refused.
func TestPollingIsRefusedWithoutTheEventsOwnScope(t *testing.T) {
	cases := []struct {
		name      string
		scopes    []string
		eventType event.Type
		wantScope string
	}{
		{
			name: "the endpoint's scope alone", scopes: []string{automationScope},
			eventType: event.ItemCreated, wantScope: "items:read",
		},
		{
			name:      "another context's read scope",
			scopes:    []string{automationScope, "containers:read"},
			eventType: event.ItemCreated, wantScope: "items:read",
		},
		{
			name:      "the item scope against a container event",
			scopes:    []string{automationScope, "items:read"},
			eventType: event.ContainerCreated, wantScope: "containers:read",
		},
		{
			name:      "the event's scope without the endpoint's",
			scopes:    []string{"items:read"},
			eventType: event.ItemCreated, wantScope: automationScope,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newPollHarness()

			_, err := h.handler.Execute(context.Background(), pollingActor(tc.scopes...),
				PollTriggerEventsCommand{EventType: string(tc.eventType)})
			if !errors.Is(err, shared.ErrForbidden) {
				t.Fatalf("error %v, want ErrForbidden", err)
			}

			var coded *shared.Error
			if !errors.As(err, &coded) {
				t.Fatalf("the refusal carries no code: %v", err)
			}
			if coded.Params["scope"] != tc.wantScope {
				t.Errorf("the refusal names %q, want %q", coded.Params["scope"], tc.wantScope)
			}
			if len(h.log.asked) != 0 {
				t.Error("the log was read despite the refusal")
			}
		})
	}
}

// A type nobody declares is refused by name. A trigger configured against a typo would otherwise
// poll forever and report nothing wrong.
func TestAnUnknownEventTypeIsRefusedByName(t *testing.T) {
	h := newPollHarness()

	_, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{EventType: "de.hubtask.work.item.invented.v1"})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want ErrValidation", err)
	}

	var coded *shared.Error
	if !errors.As(err, &coded) {
		t.Fatalf("the refusal carries no code: %v", err)
	}
	if coded.DetailCode != "triggers.event_type_unknown" {
		t.Errorf("detail %q, want triggers.event_type_unknown", coded.DetailCode)
	}
	if len(h.log.asked) != 0 {
		t.Error("the log was read for a type that does not exist")
	}
}

// The type is checked before the scope, because a scope named for a type nobody declares would be
// the empty string - and "you are missing the scope ”" tells a caller nothing they can act on.
func TestAnUnknownTypeIsRefusedBeforeTheScope(t *testing.T) {
	h := newPollHarness()

	_, err := h.handler.Execute(context.Background(), pollingActor(),
		PollTriggerEventsCommand{EventType: "de.hubtask.work.item.invented.v1"})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want the validation refusal rather than the scope one", err)
	}
}

func TestThePageSizeIsBounded(t *testing.T) {
	cases := map[string]struct{ asked, want int }{
		"absent":         {0, defaultPollLimit},
		"negative":       {-1, defaultPollLimit},
		"one":            {1, 1},
		"over the limit": {maxPollLimit + 1, maxPollLimit},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newPollHarness()

			if _, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
				PollTriggerEventsCommand{
					EventType: string(event.ItemCreated), Limit: tc.asked,
				}); err != nil {
				t.Fatalf("polling: %v", err)
			}
			if got := h.log.asked[0].limit; got != tc.want {
				t.Errorf("limit %d, want %d", got, tc.want)
			}
		})
	}
}

// A cursor that does not decode is the caller's mistake, and the log is not read for it.
func TestAnInvalidCursorIsRefused(t *testing.T) {
	h := newPollHarness()
	h.handler.Cursors = plainCursors{refuse: true}

	_, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{EventType: string(event.ItemCreated), Cursor: "forged"})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want ErrValidation", err)
	}
	if len(h.log.asked) != 0 {
		t.Error("the log was read with a cursor that does not decode")
	}
}

// A restore's events go to nobody outward-facing, on the pull half as on the push half.
func TestAReplayedEventIsNotPolled(t *testing.T) {
	replayed := emitted(now.Add(-2*time.Hour), "01936f2a-0000-7000-8000-000000000001", event.ItemCreated)
	replayed.Replay = true
	h := newPollHarness(replayed)

	page, err := h.handler.Execute(context.Background(), pollingActor(bothScopes()...),
		PollTriggerEventsCommand{EventType: string(event.ItemCreated)})
	if err != nil {
		t.Fatalf("polling: %v", err)
	}
	if len(page.Events) != 0 {
		t.Errorf("a replayed event was answered: %v", page.Events)
	}
}

// The output the registry hands to REST, MCP and automation alike.
func TestThePollOutputCarriesTheCursorAndThePage(t *testing.T) {
	row := emitted(now.Add(-2*time.Hour), "01936f2a-0000-7000-8000-000000000001", event.ItemCreated)
	h := newPollHarness(row)

	out, err := h.handler.invoke(context.Background(), pollingActor(bothScopes()...),
		map[string]any{"event_type": string(event.ItemCreated)})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}

	rows, ok := out["data"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("data is %v, want one rendered event", out["data"])
	}
	page, ok := out["page"].(map[string]any)
	if !ok {
		t.Fatalf("page is %v, want the paging block api-guidelines.md §4 declares", out["page"])
	}
	if page["next_cursor"] == "" {
		t.Error("no cursor was answered")
	}
	if page["has_more"] != false {
		t.Errorf("has_more is %v, want false", page["has_more"])
	}
}
