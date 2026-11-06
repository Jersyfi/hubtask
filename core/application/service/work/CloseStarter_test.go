// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type firstStart struct {
	pending, skipped bool
	closed           int
}

func (f *firstStart) Pending(context.Context) (bool, error) { return f.pending, nil }
func (f *firstStart) Skipped(context.Context) (bool, error) { return f.skipped, nil }
func (f *firstStart) Hold(context.Context) error            { return nil }
func (f *firstStart) Close(context.Context, time.Time) (bool, error) {
	f.closed++
	was := f.pending
	f.pending = false
	return was, nil
}

type permits struct{ may bool }

func (p permits) Permits(context.Context, appshared.ActorContext, access.Request) (bool, error) {
	return p.may, nil
}

type baselines struct{ taken int }

func (b *baselines) Baseline(context.Context, appshared.ActorContext, shared.ID) error {
	b.taken++
	return nil
}

func starterFixture(may bool) (Starter, *firstStart, *baselines, *authorizer, *sink) {
	starts, base, gate, trail := &firstStart{pending: true}, &baselines{}, &authorizer{}, &sink{}
	if !may {
		gate.err = shared.ErrForbidden
	}
	return Starter{
		Starts: starts, Permits: permits{may: may}, Authorizer: gate, Audit: trail,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), Baseline: base,
	}, starts, base, gate, trail
}

// UC-WRK-30 check 5: the first person is asked once - the question is offered while it is
// pending to whoever may shape the workspace, closing it records it once with its baseline and
// an audit entry, and closing it again changes nothing.
func TestTheFirstStartIsAskedOnce(t *testing.T) {
	s, starts, base, _, trail := starterFixture(true)
	ctx := context.Background()

	offered, err := s.StarterOffered(ctx, actor())
	if err != nil || !offered {
		t.Fatalf("a pending start is offered: %v %v", offered, err)
	}
	registry, err := usecase.NewRegistry(nil, CloseStarter{Starter: s}.Descriptor())
	if err != nil {
		t.Fatalf("the catalogue refused the entry: %v", err)
	}
	if _, err := registry.Invoke(ctx, CloseStarterName, actor(), usecase.Input{}); err != nil {
		t.Fatalf("closing: %v", err)
	}
	if starts.closed != 1 || base.taken != 1 || len(trail.entries) != 1 || trail.entries[0].Action != StarterClosedAction {
		t.Fatalf("closed %d, baselines %d, trail %d", starts.closed, base.taken, len(trail.entries))
	}
	if offered, _ := s.StarterOffered(ctx, actor()); offered {
		t.Fatal("a closed start is offered again")
	}
	if _, err := registry.Invoke(ctx, CloseStarterName, actor(), usecase.Input{}); err != nil {
		t.Fatalf("closing again: %v", err)
	}
	if base.taken != 1 || len(trail.entries) != 1 {
		t.Fatalf("closing again wrote again: baselines %d, trail %d", base.taken, len(trail.entries))
	}

	starts.skipped = true
	if skipped, err := s.StarterSkipped(ctx, actor()); err != nil || !skipped {
		t.Fatalf("a skipped start: %v %v", skipped, err)
	}
}

// Rule 2: whoever may not shape the workspace is neither offered the start nor closes it, and a
// build without the store offers nothing rather than failing.
func TestTheFirstStartIsTheWorkspaceShapersAlone(t *testing.T) {
	s, starts, _, _, trail := starterFixture(false)
	ctx := context.Background()

	if offered, err := s.StarterOffered(ctx, actor()); err != nil || offered {
		t.Fatalf("offered to somebody who may not shape: %v %v", offered, err)
	}
	if skipped, err := s.StarterSkipped(ctx, actor()); err != nil || skipped {
		t.Fatalf("skipped told to somebody who may not shape: %v %v", skipped, err)
	}
	if err := (CloseStarter{Starter: s}).Execute(ctx, actor()); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("closed by somebody who may not shape: %v", err)
	}
	if starts.closed != 0 || len(trail.entries) != 0 {
		t.Fatal("a refused close wrote something")
	}

	if offered, err := (Starter{}).StarterOffered(ctx, actor()); err != nil || offered {
		t.Fatalf("a build without the store: %v %v", offered, err)
	}
	if skipped, err := (Starter{}).StarterSkipped(ctx, actor()); err != nil || skipped {
		t.Fatalf("a build without the store: %v %v", skipped, err)
	}
}
