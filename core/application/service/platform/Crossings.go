// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package platform

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	quotarepo "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Crossings tells a purchase platform that a workspace reached one of its limits or spent its AI
// budget, once per crossing rather than once per refused request (UC-INS-14 check 2). The quota
// guard calls it; the mark that makes it once is the workspace's `quota_reached` row.
type Crossings struct {
	Marks      quotarepo.Reached
	Recorder   Recorder
	UnitOfWork persistence.UnitOfWork
	// Scopes names the workspace a metering's transaction runs for, which the budget's event is
	// written for once that transaction has ended.
	Scopes persistence.ScopeSource
	Clock  clock.Clock
}

// On reports whether anybody listens: a caller may skip what it would read only to tell.
func (c Crossings) On() bool { return c.Marks != nil && c.Recorder.On() }

func (c Crossings) on() bool { return c.On() }

// Admitted is told a create of the quota was let through: the workspace is below the limit again,
// so the next refusal is the next crossing. In the create's own transaction.
func (c Crossings) Admitted(ctx context.Context, quota string) error {
	if !c.on() {
		return nil
	}
	return c.Marks.Clear(ctx, quota)
}

// Refused is told a create of the quota was refused at `limit`. The refused transaction rolls back
// with the refusal, so the mark and the event are written once it has ended, in a transaction of
// their own (persistence.After); only the call that sets the mark writes the event.
func (c Crossings) Refused(ctx context.Context, workspace shared.ID, quota string, limit int64) {
	if !c.on() || workspace.IsZero() {
		return
	}
	persistence.After(ctx, func(ctx context.Context) {
		now := c.Clock.Now()
		err := c.UnitOfWork.Within(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
			marked, err := c.Marks.Mark(ctx, quota, strconv.FormatInt(limit, 10), now)
			if err != nil || !marked {
				return err
			}
			return c.Recorder.Record(ctx, event.WorkspaceLimitReached, event.Actor{Kind: shared.ActorSystem}, now,
				map[string]any{"quota": quota, "limit": limit})
		})
		if err != nil {
			// The refusal has been answered; what it owed the platform is lost for this crossing, and
			// the next refusal tries again, since no mark was set.
			slog.WarnContext(ctx, "a limit reached could not be told to the platform",
				slog.String("quota", quota), slog.String("error", err.Error()))
		}
	})
}

// Spent is told the workspace's day of `budget` has been spent by the metering in progress. The
// mark and the event are written once the metering's transaction has ended, in one of their own: an
// answered AI call stays metered whatever becomes of telling it, and the day's mark makes it once,
// also when two meterings cross it at once.
func (c Crossings) Spent(ctx context.Context, budget string, day, at time.Time) {
	if !c.on() || c.Scopes == nil {
		return
	}
	scope, ok := c.Scopes.ScopeFromContext(ctx)
	if !ok || scope.TenantID.IsZero() {
		return
	}
	period := day.Format(time.DateOnly)
	persistence.After(ctx, func(ctx context.Context) {
		err := c.UnitOfWork.Within(ctx, persistence.Scope{TenantID: scope.TenantID}, func(ctx context.Context) error {
			marked, err := c.Marks.Mark(ctx, budget, period, at)
			if err != nil || !marked {
				return err
			}
			return c.Recorder.Record(ctx, event.WorkspaceAiBudgetSpent, event.Actor{Kind: shared.ActorSystem}, at,
				map[string]any{"budget": budget, "day": period})
		})
		if err != nil {
			slog.WarnContext(ctx, "an AI budget spent could not be told to the platform",
				slog.String("budget", budget), slog.String("error", err.Error()))
		}
	})
}
