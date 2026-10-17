// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package platform

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// marks are the workspace's quota_reached rows: one per quota, at one period.
type marks struct{ held map[string]string }

func (m *marks) Mark(_ context.Context, quota, period string, _ time.Time) (bool, error) {
	if m.held[quota] == period {
		return false, nil
	}
	m.held[quota] = period
	return true, nil
}

func (m *marks) Clear(_ context.Context, quota string) error {
	delete(m.held, quota)
	return nil
}

type joining struct{ scopes []persistence.Scope }

func (j *joining) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	j.scopes = append(j.scopes, scope)
	return fn(ctx)
}

func (j *joining) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return j.Within(ctx, scope, fn)
}

func crossings() (Crossings, *log, *joining) {
	rec, l := recorder(row{record: maria})
	uow := &joining{}
	return Crossings{Marks: &marks{held: map[string]string{}}, Recorder: rec, UnitOfWork: uow, Clock: clock.Fixed(at)}, l, uow
}

// Refused twice at the wall, then admitted once it is below it again, then refused: two crossings,
// two events - never one per refused request (UC-INS-14 check 2).
func TestALimitReachedIsToldOncePerCrossing(t *testing.T) {
	c, l, uow := crossings()
	ctx, held := persistence.HoldAfterwards(t.Context())

	c.Refused(ctx, workspace, "items", 5000)
	if len(l.events) != 0 {
		t.Fatal("the event was written inside the refused transaction")
	}
	held.Run(ctx)
	c.Refused(t.Context(), workspace, "items", 5000)
	if len(l.events) != 1 || l.events[0].Type != event.WorkspaceLimitReached {
		t.Fatalf("two refusals at one crossing wrote %v", l.events)
	}
	if l.events[0].Payload["quota"] != "items" || l.events[0].Payload["limit"] != int64(5000) {
		t.Errorf("told %v", l.events[0].Payload)
	}
	if len(uow.scopes) == 0 || uow.scopes[0] != (persistence.Scope{TenantID: workspace}) {
		t.Errorf("written under %+v", uow.scopes)
	}

	if err := c.Admitted(t.Context(), "items"); err != nil {
		t.Fatal(err)
	}
	c.Refused(t.Context(), workspace, "items", 5000)
	if len(l.events) != 2 {
		t.Errorf("a second crossing wrote %d events in all, want 2", len(l.events))
	}

	// A changed limit is a crossing of its own.
	c.Refused(t.Context(), workspace, "items", 6000)
	if len(l.events) != 3 {
		t.Errorf("a refusal at a new limit wrote %d events in all, want 3", len(l.events))
	}
}

func TestNoCrossingIsToldWithoutTheFeed(t *testing.T) {
	c := Crossings{Marks: &marks{held: map[string]string{}}, Clock: clock.Fixed(at)}
	c.Refused(t.Context(), workspace, "items", 1)
	if err := c.Admitted(t.Context(), "items"); err != nil {
		t.Fatal(err)
	}
	if len(c.Marks.(*marks).held) != 0 {
		t.Error("a build without the feed marked a limit")
	}
}
