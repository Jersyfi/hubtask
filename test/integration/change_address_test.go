// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-INS-19 checks 7 and 8 against the database (migration 0209): a change of address replaces the
// main row in one transaction, so the old host resolves to nothing at once and another workspace
// takes the old name; a member without an address holds the notice until acknowledged; and a
// change made in one workspace's transaction reaches no other's rows.
var (
	movingTenant   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000209a01")
	takingTenant   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000209a02")
	movingMailed   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000209a11")
	movingUnmailed = shared.MustParseID("01936f2a-7c1e-7000-8000-000000209a12")
)

func seedMovingTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`DELETE FROM tenant_host WHERE tenant_id IN ('` + movingTenant.String() + `', '` + takingTenant.String() + `')`,
		`DELETE FROM tenant WHERE id IN ('` + movingTenant.String() + `', '` + takingTenant.String() + `')`,
		`INSERT INTO tenant (id, slug, display_name) VALUES
		 ('` + movingTenant.String() + `', 'move-meier', 'Meier family'),
		 ('` + takingTenant.String() + `', 'move-taker', 'Taker')`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status) VALUES
		 ('` + movingMailed.String() + `', '` + movingTenant.String() + `', 'USER', 'anna@example.org', 'Anna', 'ACTIVE')`,
		`INSERT INTO account (id, tenant_id, kind, display_name, status, sign_in_name) VALUES
		 ('` + movingUnmailed.String() + `', '` + movingTenant.String() + `', 'USER', 'Ben', 'ACTIVE', 'ben-meier')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	servedAt(ctx, t, movingTenant, "move-meier.hub.example")
	servedAt(ctx, t, takingTenant, "move-taker.hub.example")
}

func mainRow(t *testing.T, host string) domain.TenantHost {
	t.Helper()
	row, err := domain.NewCanonicalHost(domain.NewCanonicalHostInput{
		TenantID: movingTenant, Host: host, Verification: "hbt-dv-integration-move", Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestAChangeOfAddressFreesTheOldHostAtOnce(t *testing.T) {
	ctx := context.Background()
	seedMovingTenants(ctx, t)
	_, directory, uow := realWriter(ctx, t)
	addresses := postgres.NewWorkspaceAddressRepository()

	inTenant(t, uow, movingTenant, func(ctx context.Context) error {
		mailed, unmailed, err := addresses.PeopleToTell(ctx)
		if err != nil {
			return err
		}
		if len(mailed) != 1 || mailed[0] != movingMailed || len(unmailed) != 1 || unmailed[0] != movingUnmailed {
			t.Errorf("told: mailed %v, unmailed %v", mailed, unmailed)
		}
		previous, err := addresses.ChangeAddress(ctx, "move-familie", mainRow(t, "move-familie.hub.example"), time.Now())
		if err != nil {
			return err
		}
		if previous != "move-meier.hub.example" {
			t.Errorf("replaced %q", previous)
		}
		return addresses.Record(ctx, movingUnmailed, previous, "move-familie.hub.example", time.Now())
	})

	resolve := func(host string) shared.ID {
		var named shared.ID
		err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			id, err := directory.ResolveHost(ctx, host)
			named = id
			return err
		})
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			t.Fatal(err)
		}
		return named
	}
	if named := resolve("move-meier.hub.example"); !named.IsZero() {
		t.Errorf("the old host still names %s", named)
	}
	if named := resolve("move-familie.hub.example"); named != movingTenant {
		t.Errorf("the new host names %q", named)
	}

	// Another workspace takes the old name and host at once.
	inTenant(t, uow, takingTenant, func(ctx context.Context) error {
		row := mainRow(t, "move-meier.hub.example")
		row.TenantID = takingTenant
		_, err := addresses.ChangeAddress(ctx, "move-meier", row, time.Now())
		return err
	})
	if named := resolve("move-meier.hub.example"); named != takingTenant {
		t.Errorf("the old host names %q after another workspace took it", named)
	}

	// A taken name and a taken host are refused at the field.
	inTenantRolledBack(t, uow, movingTenant, func(ctx context.Context) {
		_, err := addresses.ChangeAddress(ctx, "move-meier", mainRow(t, "move-other.hub.example"), time.Now())
		if shared.AsError(err).DetailCode != "admin.slug_taken" {
			t.Errorf("a taken name: %v", err)
		}
	})
	inTenantRolledBack(t, uow, movingTenant, func(ctx context.Context) {
		_, err := addresses.ChangeAddress(ctx, "move-again", mainRow(t, "move-meier.hub.example"), time.Now())
		if shared.AsError(err).DetailCode != "admin.host_taken" {
			t.Errorf("a taken host: %v", err)
		}
	})

	// The notice: held for the member without an address until acknowledged.
	inTenant(t, uow, movingTenant, func(ctx context.Context) error {
		notices, err := addresses.Of(ctx, movingUnmailed)
		if err != nil {
			return err
		}
		if len(notices) != 1 || notices[0].From != "move-meier.hub.example" || notices[0].To != "move-familie.hub.example" {
			t.Errorf("notices %+v", notices)
		}
		return addresses.Acknowledge(ctx, movingUnmailed)
	})
	inTenant(t, uow, movingTenant, func(ctx context.Context) error {
		notices, err := addresses.Of(ctx, movingUnmailed)
		if len(notices) != 0 {
			t.Errorf("acknowledged, still %+v", notices)
		}
		return err
	})
}

// Cross-tenant negative (SG-3): a change and a notice written in one workspace's transaction reach
// none of another's rows.
func TestAChangeOfAddressStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedMovingTenants(ctx, t)
	_, _, uow := realWriter(ctx, t)
	addresses := postgres.NewWorkspaceAddressRepository()

	inTenantRolledBack(t, uow, takingTenant, func(ctx context.Context) {
		row := mainRow(t, "move-elsewhere.hub.example")
		row.TenantID = takingTenant
		if _, err := addresses.ChangeAddress(ctx, "move-elsewhere", row, time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, unmailed, err := addresses.PeopleToTell(ctx); err != nil || len(unmailed) != 0 {
			t.Errorf("the other workspace's people: %v, %v", unmailed, err)
		}
		if notices, err := addresses.Of(ctx, movingUnmailed); err != nil || len(notices) != 0 {
			t.Errorf("another workspace's notice: %v, %v", notices, err)
		}
	})
	var host string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT host FROM tenant_host WHERE tenant_id = $1 AND is_canonical`, movingTenant.String()).Scan(&host); err != nil {
		t.Fatal(err)
	}
	if host != "move-meier.hub.example" {
		t.Errorf("the moving workspace's main row became %q", host)
	}
}

// UC-INS-19 check 3 against the database (migration 0210): every change of address stamps its
// workspace's row, which the owner's cooldown is measured from, read plain or under the row's lock -
// and a change in one workspace stamps no other's.
func TestAChangeOfAddressStampsOnlyItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedMovingTenants(ctx, t)
	_, _, uow := realWriter(ctx, t)
	addresses := postgres.NewWorkspaceAddressRepository()
	at := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)

	inTenant(t, uow, movingTenant, func(ctx context.Context) error {
		last, err := addresses.LastAddressChange(ctx)
		if err != nil || !last.IsZero() {
			t.Errorf("before any change: %v, %v", last, err)
		}
		_, err = addresses.ChangeAddress(ctx, "move-stamped", mainRow(t, "move-stamped.hub.example"), at)
		return err
	})
	inTenant(t, uow, movingTenant, func(ctx context.Context) error {
		for name, read := range map[string]func(context.Context) (time.Time, error){
			"plain": addresses.LastAddressChange, "locked": addresses.LockAddressChange,
		} {
			last, err := read(ctx)
			if err != nil || !last.Equal(at) {
				t.Errorf("%s read after the change: %v, %v", name, last, err)
			}
		}
		return nil
	})

	// Cross-tenant negative (SG-3): the other workspace reads its own row, never changed.
	inTenant(t, uow, takingTenant, func(ctx context.Context) error {
		last, err := addresses.LockAddressChange(ctx)
		if err != nil || !last.IsZero() {
			t.Errorf("the other workspace's last change: %v, %v", last, err)
		}
		return nil
	})
}
