// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/service/platform"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func platformRecorder(ids clock.IDGenerator) platform.Recorder {
	return platform.Recorder{
		Events: postgres.NewPlatformEventRepository(), Workspaces: postgres.NewAdminTenantRepository(),
		IDs: ids, Tenancy: env.TenancyMulti,
	}
}

// platformEventsOf reads the whole feed under the installation's scope and keeps the events of
// the named workspaces: the integration tests share one database, and the feed is the
// installation's.
func platformEventsOf(ctx context.Context, t *testing.T, uow persistence.UnitOfWork, after int64,
	workspaces ...shared.ID,
) []adminrepo.PlatformEvent {
	t.Helper()
	wanted := map[shared.ID]bool{}
	for _, w := range workspaces {
		wanted[w] = true
	}
	var kept []adminrepo.PlatformEvent
	events := postgres.NewPlatformEventRepository()
	for {
		var page []adminrepo.PlatformEvent
		if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			read, err := events.After(ctx, after, 200)
			page = read
			return err
		}); err != nil {
			t.Fatalf("reading the feed: %v", err)
		}
		for _, e := range page {
			if wanted[e.Envelope.TenantID] {
				kept = append(kept, e)
			}
			after = e.Number
		}
		if len(page) < 200 {
			return kept
		}
	}
}

func lastPlatformNumber(ctx context.Context, t *testing.T) int64 {
	t.Helper()
	var number int64
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT coalesce(max(number), 0) FROM platform_event`).Scan(&number); err != nil {
		t.Fatal(err)
	}
	return number
}

// An event is written with its change and read back from the installation with the state the
// change left; a change that rolls back leaves none.
func TestAnInstallationEventCommitsWithItsChange(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	recorder := platformRecorder(clockadapter.NewUUIDv7(clockadapter.System{}))
	kept, gone := seedWorkspace(ctx, t, "kept"), seedWorkspace(ctx, t, "gone")
	start := lastPlatformNumber(ctx, t)

	if err := uow.Within(ctx, persistence.Scope{TenantID: kept}, func(ctx context.Context) error {
		return recorder.Record(ctx, event.WorkspaceSuspended, event.Actor{Kind: shared.ActorSystem}, time.Now(), nil)
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}
	refused := errors.New("the change was refused")
	err := uow.Within(ctx, persistence.Scope{TenantID: gone}, func(ctx context.Context) error {
		if err := recorder.Record(ctx, event.WorkspaceSuspended, event.Actor{Kind: shared.ActorSystem}, time.Now(), nil); err != nil {
			return err
		}
		return refused
	})
	if !errors.Is(err, refused) {
		t.Fatalf("the refused change answered %v", err)
	}

	events := platformEventsOf(ctx, t, uow, start, kept, gone)
	if len(events) != 1 || events[0].Envelope.TenantID != kept || events[0].Envelope.Type != event.WorkspaceSuspended {
		t.Fatalf("the feed holds %+v", events)
	}
	state, _ := events[0].Envelope.Payload["workspace"].(map[string]any)
	if state["id"] != kept.String() || state["status"] != "ACTIVE" || state["version"] == nil {
		t.Errorf("the event carries %v", state)
	}
}

// Two transactions writing events at once: the one that numbers first holds the lock until it
// ends, so the other's number is drawn after it commits. A reader during the first sees neither,
// and reading on from its cursor returns every event once, in commit order (UC-INS-14 check 6).
func TestTheFeedReturnsConcurrentEventsInCommitOrderAndNoneTwice(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	recorder := platformRecorder(clockadapter.NewUUIDv7(clockadapter.System{}))
	first, second := seedWorkspace(ctx, t, "first"), seedWorkspace(ctx, t, "second")
	start := lastPlatformNumber(ctx, t)

	numbered := make(chan struct{})
	release := make(chan struct{})
	var released sync.Once
	// However the test ends, the first transaction ends too: it holds the installation's numbering
	// lock, and every other event writer in the shared database would wait on it for ever.
	releaseFirst := func() { released.Do(func() { close(release) }) }
	t.Cleanup(releaseFirst)
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		firstDone <- uow.Within(ctx, persistence.Scope{TenantID: first}, func(ctx context.Context) error {
			if err := recorder.Record(ctx, event.WorkspaceSuspended, event.Actor{Kind: shared.ActorSystem}, time.Now(), nil); err != nil {
				return err
			}
			close(numbered)
			<-release
			return nil
		})
	}()
	select {
	case <-numbered:
	case err := <-firstDone:
		t.Fatalf("the first transaction ended before it held the lock: %v", err)
	}
	go func() {
		secondDone <- uow.Within(ctx, persistence.Scope{TenantID: second}, func(ctx context.Context) error {
			return recorder.Record(ctx, event.WorkspaceResumed, event.Actor{Kind: shared.ActorSystem}, time.Now(), nil)
		})
	}()

	select {
	case err := <-secondDone:
		t.Fatalf("the second transaction numbered its event while the first held the lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if open := platformEventsOf(ctx, t, uow, start, first, second); len(open) != 0 {
		t.Errorf("the feed shows %d events of transactions still open", len(open))
	}

	releaseFirst()
	if err := <-firstDone; err != nil {
		t.Fatalf("the first transaction: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("the second transaction: %v", err)
	}

	events := platformEventsOf(ctx, t, uow, start, first, second)
	if len(events) != 2 || events[0].Envelope.TenantID != first || events[1].Envelope.TenantID != second ||
		events[0].Number >= events[1].Number {
		t.Fatalf("the feed holds %+v, want the first's event before the second's", events)
	}
	if again := platformEventsOf(ctx, t, uow, events[1].Number, first, second); len(again) != 0 {
		t.Errorf("reading on from the last cursor returned %d events again", len(again))
	}
}

// The purge removes the platform's reference from its own workspace's events and from no other's.
func TestTheReferenceIsForgottenOnlyInItsOwnWorkspace(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	recorder := platformRecorder(clockadapter.NewUUIDv7(clockadapter.System{}))
	tenants := postgres.NewAdminTenantRepository()
	purged, other := seedWorkspace(ctx, t, "purged"), seedWorkspace(ctx, t, "other")
	start := lastPlatformNumber(ctx, t)

	for _, w := range []shared.ID{purged, other} {
		if err := uow.Within(ctx, persistence.Scope{TenantID: w}, func(ctx context.Context) error {
			record, err := tenants.Find(ctx)
			if err != nil {
				return err
			}
			written, err := tenants.SetReference(ctx, "cus_"+planName(w.String()[30:]), record.Version, time.Now())
			if err == nil && !written {
				t.Error("the reference was not set, so its removal would prove nothing")
			}
			if err != nil {
				return err
			}
			return recorder.Record(ctx, event.WorkspaceSuspended, event.Actor{Kind: shared.ActorSystem}, time.Now(), nil)
		}); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}
	if err := uow.Within(ctx, persistence.Scope{TenantID: purged}, func(ctx context.Context) error {
		record, err := tenants.Find(ctx)
		if err != nil {
			return err
		}
		return recorder.RecordDeleted(ctx, record, event.Actor{Kind: shared.ActorSystem}, time.Now())
	}); err != nil {
		t.Fatalf("recording the purge: %v", err)
	}

	for _, e := range platformEventsOf(ctx, t, uow, start, purged, other) {
		reference := e.Envelope.Payload["workspace"].(map[string]any)["external_reference"]
		switch {
		case e.Envelope.TenantID == purged && reference != nil:
			t.Errorf("the purged workspace's %s still carries its reference", e.Envelope.Type)
		case e.Envelope.TenantID == other && reference == nil:
			t.Error("another workspace's event lost its reference")
		}
	}
}

// The sweep takes what is past the window and leaves what is inside it.
func TestTheSweepTakesOnlyExpiredInstallationEvents(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	recorder := platformRecorder(clockadapter.NewUUIDv7(clockadapter.System{}))
	workspace := seedWorkspace(ctx, t, "swept")
	start := lastPlatformNumber(ctx, t)
	now := time.Now()

	for _, at := range []time.Time{now.Add(-31 * 24 * time.Hour), now.Add(-29 * 24 * time.Hour)} {
		if err := uow.Within(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
			return recorder.Record(ctx, event.WorkspaceSuspended, event.Actor{Kind: shared.ActorSystem}, at, nil)
		}); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := postgres.NewPlatformEventRepository().Sweep(ctx, now.Add(-30*24*time.Hour), 1000)
		return err
	}); err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	kept := platformEventsOf(ctx, t, uow, start, workspace)
	if len(kept) != 1 || kept[0].Envelope.OccurredAt.Before(now.Add(-30*24*time.Hour)) {
		t.Errorf("after the sweep the feed holds %+v", kept)
	}
}
