// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// PlatformSweep runs the installation's sweep of what it keeps for a purchase platform, and comes
// back. Not detached: the runner opens the system scope's transaction the sweep runs in.
type PlatformSweep struct {
	Sweep adminservice.SweepPlatformEvents
}

var _ queue.Handler = PlatformSweep{}

// Run sweeps one pass.
func (h PlatformSweep) Run(ctx context.Context, _ queue.Job) (queue.Result, error) {
	removed, err := h.Sweep.Run(ctx)
	if err != nil {
		return queue.Result{}, err
	}
	if removed > 0 {
		slog.InfoContext(ctx, "the installation's expired platform events were swept", slog.Int("removed", removed))
	}
	return queue.Result{Repeat: true, RepeatAfter: adminservice.PlatformSweepInterval}, nil
}
