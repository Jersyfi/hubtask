// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// sweptLog removes what it is asked to in batches, from a backlog of `expired`.
type sweptLog struct {
	platformLog
	expired int
	cutoffs []time.Time
}

func (l *sweptLog) Sweep(_ context.Context, cutoff time.Time, batch int) (int, error) {
	l.cutoffs = append(l.cutoffs, cutoff)
	removed := min(l.expired, batch)
	l.expired -= removed
	return removed, nil
}

func TestTheSweepTakesWhatIsPastThirtyDaysAndADayBatchByBatch(t *testing.T) {
	log := &sweptLog{expired: 2*platformSweepBatch + 7}
	sweep := SweepPlatformEvents{Events: log, Clock: clock.Fixed(now), Tenancy: env.TenancyMulti}

	removed, err := sweep.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2*platformSweepBatch+7 || log.expired != 0 || len(log.cutoffs) != 3 {
		t.Errorf("removed %d in %d rounds, %d left", removed, len(log.cutoffs), log.expired)
	}
	if !log.cutoffs[0].Equal(now.Add(-31 * 24 * time.Hour)) {
		t.Errorf("cut off at %s", log.cutoffs[0])
	}
}

func TestTheSweepIsSeededAsAnInstallationJobUnderItsDedupeKey(t *testing.T) {
	jobs := &jobsFake{}
	work := &unitOfWork{}
	sweep := SweepPlatformEvents{Jobs: jobs, UnitOfWork: work, Clock: clock.Fixed(now), Tenancy: env.TenancyMulti}
	if err := sweep.Seed(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindPlatformSweep ||
		!jobs.requests[0].TenantID.IsZero() || jobs.requests[0].DedupeKey == "" {
		t.Errorf("seeded %+v", jobs.requests)
	}
	if len(work.scopes) != 1 || work.scopes[0] != persistence.SystemScope() {
		t.Errorf("seeded under %+v", work.scopes)
	}

	single := SweepPlatformEvents{Jobs: &jobsFake{}, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now)}
	if err := single.Seed(t.Context()); err != nil || len(single.Jobs.(*jobsFake).requests) != 0 {
		t.Errorf("single mode seeded a sweep: %v", err)
	}
}

type sweptDeliveries struct {
	deliveryStore
	expired int
}

func (d *sweptDeliveries) Sweep(_ context.Context, _ time.Time, batch int) (int, error) {
	removed := min(d.expired, batch)
	d.expired -= removed
	return removed, nil
}

func TestTheSweepTakesTheDeliveriesToo(t *testing.T) {
	deliveries := &sweptDeliveries{expired: platformSweepBatch + 3}
	sweep := SweepPlatformEvents{Events: &sweptLog{}, Deliveries: deliveries, Clock: clock.Fixed(now), Tenancy: env.TenancyMulti}
	removed, err := sweep.Run(t.Context())
	if err != nil || removed != platformSweepBatch+3 || deliveries.expired != 0 {
		t.Errorf("removed %d, %d left, %v", removed, deliveries.expired, err)
	}
}
