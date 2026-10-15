// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"context"
	"errors"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// TrailRemovedAction is the entry a workspace's trail gets when a month of it went for good: the
// going is itself recorded (UC-AUD-07 check 4, audit.md §3), in the trail it is about, where the
// workspace's own administrator reads it.
const TrailRemovedAction port.Action = "audit.trail_removed"

// TrailRemoval is one workspace's share of a month that went: which month, how many entries.
type TrailRemoval struct {
	TenantID  shared.ID
	Partition string
	Month     string
	Entries   int64
}

// Payload is the job's: identifiers, a month and a count - nothing an entry said.
func (r TrailRemoval) Payload() map[string]any {
	return map[string]any{"partition": r.Partition, "month": r.Month, "entries": r.Entries}
}

// TrailRemovalOf reads a job's payload back.
func TrailRemovalOf(payload map[string]any, tenantID shared.ID) (TrailRemoval, error) {
	partition, _ := payload["partition"].(string)
	month, _ := payload["month"].(string)
	if tenantID.IsZero() || partition == "" || month == "" {
		return TrailRemoval{}, shared.Internalf("audit: a trail removal job without its workspace or month")
	}
	var entries int64
	switch value := payload["entries"].(type) {
	case float64:
		entries = int64(value)
	case int64:
		entries = value
	case int:
		entries = int64(value)
	}
	return TrailRemoval{TenantID: tenantID, Partition: partition, Month: month, Entries: entries}, nil
}

// Workspaces answers whether the workspace still exists; a removal of one deleted since needs no
// entry, and its trail went with it.
type Workspaces interface {
	Find(ctx context.Context) (identity.Workspace, error)
}

// RecordTrailRemoval writes the entry, in the workspace's own transaction and chain.
type RecordTrailRemoval struct {
	Workspaces Workspaces
	Audit      port.Sink
	Clock      clock.Clock
}

// Execute records one removal, and answers false where the workspace is gone.
func (r RecordTrailRemoval) Execute(ctx context.Context, removal TrailRemoval) (bool, error) {
	if r.Workspaces != nil {
		if _, err := r.Workspaces.Find(ctx); err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
	}
	err := r.Audit.Append(ctx, port.Entry{
		TenantID: removal.TenantID, OccurredAt: r.Clock.Now(),
		Action: TrailRemovedAction, Outcome: port.OutcomeSuccess, Severity: port.SeverityNotice,
		ActorKind: appshared.ActorSystem, ActorLabel: "the installation",
		TargetType: trailTarget, TargetID: removal.TenantID,
		Changes: port.Changes(
			port.Change{Field: "month", Classification: port.Open, To: removal.Month},
			port.Change{Field: "entries", Classification: port.Open, To: removal.Entries},
		),
	})
	if err != nil {
		return false, err
	}
	return true, nil
}
