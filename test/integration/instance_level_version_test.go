// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Two operators saving the installation's defaults do not lose a save (UC-INS-09 check 8): of two
// saves made on one version, against the real table, the second is refused with `version_conflict`
// and the level stored is the first's. The version is read from the database, never assumed: the
// integration tests share one installation.
func TestTheSecondSaveOfTheInstanceLevelFromOneVersionIsRefused(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	writer := adminservice.InstanceWriter{
		Settings: postgres.NewInstanceSettingRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
		UnitOfWork: uow, Clock: clockadapter.System{}, IDs: clockadapter.NewUUIDv7(clockadapter.System{}),
	}
	write := adminservice.WriteInstanceSettings{Writer: writer}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.ID("0193a000-0000-7000-8000-00000000c011"),
		AccountID: shared.ID("0193a000-0000-7000-8000-00000000c012"), AccountName: "Operator",
		Scopes: []string{"admin:tenants"},
	}

	before := readInstanceLevel(ctx, t, uow)
	t.Cleanup(func() {
		if _, err := write.ExecuteAsFile(context.WithoutCancel(ctx), before, "cleanup"); err != nil {
			t.Errorf("restoring the level: %v", err)
		}
	})
	if before.Version < 1 {
		t.Fatalf("the level answers version %d; migration 0160 seeds 1", before.Version)
	}

	first, err := write.Execute(ctx, operator, levelWithMinLength(before, 15), before.Version)
	if err != nil {
		t.Fatalf("the first save was refused: %v", err)
	}
	if first.Version != before.Version+1 {
		t.Fatalf("the first save answered version %d, want %d", first.Version, before.Version+1)
	}

	_, err = write.Execute(ctx, operator, levelWithMinLength(before, 21), before.Version)
	refusal := shared.AsError(err)
	if refusal == nil || refusal.Code != "version_conflict" ||
		refusal.DetailCode != "admin.instance_settings_version_conflict" {
		t.Fatalf("the second save on the same version answered %v", err)
	}

	stored := readInstanceLevel(ctx, t, uow)
	if stored.Policy.Patch.MinLength == nil || *stored.Policy.Patch.MinLength != 15 {
		t.Fatalf("the stored minimum length is %v, want the first save's 15", stored.Policy.Patch.MinLength)
	}
	if stored.Version != first.Version {
		t.Fatalf("the refused save moved the version to %d", stored.Version)
	}

	// A save that names no version is compared with the level the server reads: it is accepted,
	// and it moves the version too.
	third, err := write.Execute(ctx, operator, levelWithMinLength(before, 16), 0)
	if err != nil {
		t.Fatalf("a save without a version was refused: %v", err)
	}
	if third.Version != first.Version+1 {
		t.Fatalf("a save without a version answered version %d, want %d", third.Version, first.Version+1)
	}
}

// Two saves at the same moment, made on one version: the second waits on the version row while the
// first's transaction is open, and is then refused - the guard is the statement, not a read before
// it (known-traps.md, "a read-then-write without a lock").
func TestTwoSavesOfTheInstanceLevelAtOnceDoNotBothPass(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	settings := postgres.NewInstanceSettingRepository()

	before := readInstanceLevel(ctx, t, uow)
	t.Cleanup(func() {
		inSystem(context.WithoutCancel(ctx), t, uow, func(ctx context.Context) error {
			_, err := settings.Write(ctx, before, 0, "", time.Now().UTC())
			return err
		})
	})

	second := make(chan bool, 1)
	failed := make(chan error, 1)
	inSystem(ctx, t, uow, func(txCtx context.Context) error {
		written, err := settings.Write(txCtx, levelWithMinLength(before, 17), before.Version, "", time.Now().UTC())
		if err != nil || !written {
			t.Fatalf("the first save (%v, %v)", written, err)
		}
		//nolint:contextcheck // a second transaction: the context handed in carries this one
		go func() {
			err := uow.Within(context.Background(), persistence.SystemScope(), func(ctx context.Context) error {
				written, err := settings.Write(ctx, levelWithMinLength(before, 22), before.Version, "", time.Now().UTC())
				second <- written
				return err
			})
			if err != nil {
				failed <- err
			}
		}()
		select {
		case <-second:
			t.Error("the second save passed while the first held the version")
		case <-time.After(500 * time.Millisecond):
		}
		return nil
	})

	select {
	case written := <-second:
		if written {
			t.Fatal("the second save from the same version was written")
		}
	case err := <-failed:
		t.Fatalf("the second save failed: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the second save never finished")
	}

	stored := readInstanceLevel(ctx, t, uow)
	if stored.Policy.Patch.MinLength == nil || *stored.Policy.Patch.MinLength != 17 {
		t.Fatalf("the stored minimum length is %v, want the first save's 17", stored.Policy.Patch.MinLength)
	}
	if stored.Version != before.Version+1 {
		t.Fatalf("the version is %d after one save, want %d", stored.Version, before.Version+1)
	}
}

func readInstanceLevel(ctx context.Context, t *testing.T, uow persistence.UnitOfWork) identityrepo.InstanceLevel {
	t.Helper()
	var level identityrepo.InstanceLevel
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		var err error
		level, err = postgres.NewInstanceSettingRepository().Read(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the level: %v", err)
	}
	return level
}

// levelWithMinLength is the level as found, with one switch changed - what a screen sends.
func levelWithMinLength(found identityrepo.InstanceLevel, minimum int) identityrepo.InstanceLevel {
	changed := found
	changed.Policy = identity.PolicyLayer{Patch: found.Policy.Patch, Locks: map[identity.PolicySwitch]bool{}}
	for name, locked := range found.Policy.Locks {
		changed.Policy.Locks[name] = locked
	}
	changed.Policy.Patch.MinLength = &minimum
	return changed
}
