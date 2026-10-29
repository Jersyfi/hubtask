// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

type scheduledPull struct {
	next time.Time
	ran  shared.ID
}

func (s *scheduledPull) RunScheduled(_ context.Context, _, id shared.ID) (time.Time, error) {
	s.ran = id
	return s.next, nil
}

func TestAConnectorsPullRepeatsAtItsNextMomentAndFinishesWhenUnscheduled(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	tenant := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f001")
	conn := "01936f2a-7c1e-7000-8000-00000000f002"
	s := &scheduledPull{next: now.Add(30 * time.Minute)}
	h := CmdbConnectorPull{Connectors: s, Now: func() time.Time { return now }}
	job := queue.Job{TenantID: tenant, Kind: queue.KindCmdbConnectorPull, Payload: map[string]any{"connector_id": conn}}
	r, err := h.Run(context.Background(), job)
	if err != nil || !r.Repeat || r.RepeatAfter != 30*time.Minute || s.ran.String() != conn {
		t.Fatalf("repeats at the next moment: %+v %v", r, err)
	}
	s.next = time.Time{}
	if r, _ = h.Run(context.Background(), job); r.Repeat {
		t.Fatal("an unscheduled connector's job finishes")
	}
	if _, err := h.Run(context.Background(), queue.Job{TenantID: tenant}); err == nil {
		t.Fatal("a job without a connector is a programming error")
	}
}
