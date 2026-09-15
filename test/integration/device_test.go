// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The devices that synchronise (N-03, offline-sync.md §6): registration by turning up, the
// refusals, forgetting, the sweep that revokes what a stale device held - and a cross-tenant
// negative for every method (gate SG-3).

func deviceRepo() postgres.DeviceRepository { return postgres.NewDeviceRepository() }

// seedSession opens a session row for the account, the way sign-in would, and answers its id.
func seedSession(ctx context.Context, t *testing.T, tenant, account shared.ID) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		VALUES ($1, $2, $3, now(), now() + interval '30 days')`,
		id.String(), tenant.String(), account.String()); err != nil {
		t.Fatalf("seeding the session: %v", err)
	}
	return id
}

func sessionRevokedAt(ctx context.Context, t *testing.T, id shared.ID) *time.Time {
	t.Helper()
	var revoked *time.Time
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT revoked_at FROM session WHERE id = $1`, id.String()).Scan(&revoked); err != nil {
		t.Fatalf("reading the session: %v", err)
	}
	return revoked
}

func touch(ctx context.Context, t *testing.T, tenant shared.ID, contact syncdomain.Contact) (syncdomain.Device, error) {
	t.Helper()
	var device syncdomain.Device
	err := write(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		device, err = deviceRepo().Touch(ctx, contact)
		return err
	})
	return device, err
}

func TestADeviceRegistersByTurningUpAndEveryContactMovesIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := seedAccount(ctx, t, tenantA)
	session := seedSession(ctx, t, tenantA, account)
	id := freshID(t)

	first, err := touch(ctx, t, tenantA, syncdomain.Contact{
		DeviceID: id, AccountID: account, CredentialID: session, Platform: "hubctl",
		DisplayName: "Anna's laptop", Now: created,
	})
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	if first.ID != id || first.AccountID != account || first.Platform != "hubctl" ||
		first.DisplayName != "Anna's laptop" || first.LastSeq != 0 || first.Blocked ||
		first.CredentialID != session || !first.CreatedAt.Equal(created) {
		t.Errorf("registered as %+v", first)
	}

	// The second contact says nothing about itself and stands further along: the name stays,
	// the position and the moment move.
	later := created.Add(time.Hour)
	second, err := touch(ctx, t, tenantA, syncdomain.Contact{
		DeviceID: id, AccountID: account, LastSeq: 42, Now: later,
	})
	if err != nil {
		t.Fatalf("touching again: %v", err)
	}
	if second.DisplayName != "Anna's laptop" || second.LastSeq != 42 || !second.LastSeenAt.Equal(later) ||
		second.CredentialID != session || !second.CreatedAt.Equal(created) {
		t.Errorf("the second contact left %+v", second)
	}

	// A position behind the one recorded does not move it back: a device pulling an older page
	// again is still where it got to.
	third, err := touch(ctx, t, tenantA, syncdomain.Contact{DeviceID: id, AccountID: account, LastSeq: 7, Now: later})
	if err != nil {
		t.Fatalf("touching a third time: %v", err)
	}
	if third.LastSeq != 42 {
		t.Errorf("the position went back to %d", third.LastSeq)
	}
}

func TestAnIdentifierAnotherAccountHoldsIsForeignAndAForgottenOneIsRevoked(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	owner, other := seedAccount(ctx, t, tenantA), seedAccount(ctx, t, tenantA)
	id := freshID(t)
	if _, err := touch(ctx, t, tenantA, syncdomain.Contact{DeviceID: id, AccountID: owner, Now: created}); err != nil {
		t.Fatalf("registering: %v", err)
	}

	_, err := touch(ctx, t, tenantA, syncdomain.Contact{DeviceID: id, AccountID: other, Now: created})
	if !errors.Is(err, shared.ErrForbidden) || shared.AsError(err).DetailCode != "sync.device_foreign" {
		t.Errorf("another account's contact answered %v, want sync.device_foreign", err)
	}

	var forgotten bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		_, forgotten, err = deviceRepo().Forget(ctx, id, owner, created.Add(time.Minute))
		return err
	}); err != nil || !forgotten {
		t.Fatalf("forgetting: %v (forgotten=%v)", err, forgotten)
	}
	_, err = touch(ctx, t, tenantA, syncdomain.Contact{DeviceID: id, AccountID: owner, Now: created})
	if !errors.Is(err, shared.ErrForbidden) || shared.AsError(err).DetailCode != "sync.device_revoked" {
		t.Errorf("the forgotten device's contact answered %v, want sync.device_revoked", err)
	}

	// Forgetting is idempotent from the caller's side, and somebody else cannot do it at all.
	for name, account := range map[string]shared.ID{"again": owner, "by another account": other} {
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			_, forgotten, err = deviceRepo().Forget(ctx, id, account, created)
			return err
		}); err != nil || forgotten {
			t.Errorf("forgetting %s: %v (forgotten=%v)", name, err, forgotten)
		}
	}
}

func TestTheSweepForgetsStaleDevicesAndRevokesTheirSessions(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := seedAccount(ctx, t, tenantA)
	staleSession := seedSession(ctx, t, tenantA, account)
	liveSession := seedSession(ctx, t, tenantA, account)
	stale, live := freshID(t), freshID(t)
	cutoff := created.Add(30 * 24 * time.Hour)

	for _, contact := range []syncdomain.Contact{
		{DeviceID: stale, AccountID: account, CredentialID: staleSession, Now: created},
		{DeviceID: live, AccountID: account, CredentialID: liveSession, Now: cutoff.Add(time.Hour)},
	} {
		if _, err := touch(ctx, t, tenantA, contact); err != nil {
			t.Fatalf("registering: %v", err)
		}
	}

	var due, removed int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if due, err = deviceRepo().CountExpired(ctx, cutoff, 100); err != nil {
			return err
		}
		removed, err = deviceRepo().DeleteExpired(ctx, cutoff, 100)
		return err
	}); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if due < 1 || removed < 1 {
		t.Errorf("due %d, removed %d, want at least the stale device", due, removed)
	}
	if sessionRevokedAt(ctx, t, staleSession) == nil {
		t.Errorf("the stale device's session was not revoked")
	}
	if sessionRevokedAt(ctx, t, liveSession) != nil {
		t.Errorf("the live device's session was revoked")
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := deviceRepo().Find(ctx, stale)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the stale device is still there: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := deviceRepo().Find(ctx, live)
		return err
	}); err != nil {
		t.Errorf("the live device went: %v", err)
	}
}

// Gate SG-3: one negative per port method.
func TestDevicesAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := seedAccount(ctx, t, tenantA)
	session := seedSession(ctx, t, tenantA, account)
	id := freshID(t)
	if _, err := touch(ctx, t, tenantA, syncdomain.Contact{
		DeviceID: id, AccountID: account, CredentialID: session, Now: created,
	}); err != nil {
		t.Fatalf("registering: %v", err)
	}
	strangerB := seedAccount(ctx, t, tenantB)

	t.Run("touch", func(t *testing.T) {
		_, err := touch(ctx, t, tenantB, syncdomain.Contact{DeviceID: id, AccountID: strangerB, Now: created})
		if shared.AsError(err).DetailCode != "sync.device_foreign" {
			t.Errorf("tenant B's contact answered %v", err)
		}
	})
	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := deviceRepo().Find(ctx, id)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B's find answered %v", err)
		}
	})
	t.Run("for account", func(t *testing.T) {
		var listed []syncdomain.Device
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			listed, err = deviceRepo().ForAccount(ctx, account)
			return err
		}); err != nil {
			t.Fatalf("listing: %v", err)
		}
		if len(listed) != 0 {
			t.Errorf("tenant B listed %d of tenant A's devices", len(listed))
		}
	})
	t.Run("forget", func(t *testing.T) {
		var forgotten bool
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			_, forgotten, err = deviceRepo().Forget(ctx, id, account, created)
			return err
		}); err != nil || forgotten {
			t.Errorf("tenant B forgot tenant A's device: %v (forgotten=%v)", err, forgotten)
		}
	})
	t.Run("sweep", func(t *testing.T) {
		var due, removed int
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			if due, err = deviceRepo().CountExpired(ctx, created.Add(365*24*time.Hour), 100); err != nil {
				return err
			}
			removed, err = deviceRepo().DeleteExpired(ctx, created.Add(365*24*time.Hour), 100)
			return err
		}); err != nil {
			t.Fatalf("sweeping: %v", err)
		}
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			_, err := deviceRepo().Find(ctx, id)
			return err
		}); err != nil {
			t.Errorf("tenant B's sweep (due %d, removed %d) took tenant A's device: %v", due, removed, err)
		}
		if sessionRevokedAt(ctx, t, session) != nil {
			t.Errorf("tenant B's sweep revoked tenant A's session")
		}
	})
}
