// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The directory's identifiers on accounts and groups (migration 0256, UC-ID-28), against the real
// boundary: written, found, unique per workspace, and invisible from the workspace next door
// (gate SG-3).

func directoryRepo() postgres.DirectoryRepository { return postgres.NewDirectoryRepository() }

func TestADirectoryIdentifierIsFoundInItsWorkspaceOnly(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	person := seedAccount(ctx, t, tenantA)
	external := freshName(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return directoryRepo().SetAccountExternalID(ctx, person, external, changedAt)
	}); err != nil {
		t.Fatalf("writing the identifier: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := directoryRepo().FindAccountByExternalID(ctx, external)
		if err != nil || found != person {
			t.Errorf("found %s, %v; want the person", found, err)
		}
		held, err := directoryRepo().AccountExternalIDs(ctx, []shared.ID{person})
		if err != nil || held[person] != external {
			t.Errorf("held %v, %v", held, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Another person of the workspace cannot take it.
	other := seedAccount(ctx, t, tenantA)
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return directoryRepo().SetAccountExternalID(ctx, other, external, changedAt)
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second holder: %v, want a conflict", err)
	}

	// The workspace next door neither finds the identifier nor writes to the person.
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := directoryRepo().FindAccountByExternalID(ctx, external); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("found across workspaces: %v", err)
		}
		held, err := directoryRepo().AccountExternalIDs(ctx, []shared.ID{person})
		if err != nil || len(held) != 0 {
			t.Errorf("read across workspaces: %v, %v", held, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err = write(ctx, t, tenantB, func(ctx context.Context) error {
		return directoryRepo().SetAccountExternalID(ctx, person, "taken-over", changedAt)
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("written across workspaces: %v, want not found", err)
	}
}

func TestADirectorySwitchesAPersonOffAndOn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	person := seedAccount(ctx, t, tenantA)

	switchTo := func(tenant shared.ID, status identity.AccountStatus) bool {
		var moved bool
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			moved, err = directoryRepo().SetAccountStatus(ctx, person, status, changedAt)
			return err
		}); err != nil {
			t.Fatalf("switching: %v", err)
		}
		return moved
	}
	if switchTo(tenantB, identity.AccountDisabled) {
		t.Error("the workspace next door switched the person off")
	}
	if !switchTo(tenantA, identity.AccountDisabled) {
		t.Error("the person was not switched off")
	}
	if !switchTo(tenantA, identity.AccountActive) {
		t.Error("the person was not switched on")
	}

	// A restricted person is never moved by a directory.
	if _, err := adminPool(ctx, t).Exec(ctx, "UPDATE account SET status = 'RESTRICTED' WHERE id = $1", person.String()); err != nil {
		t.Fatal(err)
	}
	if switchTo(tenantA, identity.AccountDisabled) {
		t.Error("a restricted person was switched off")
	}
}

func TestADirectoryGroupIsFoundByNameAndIdentifierInItsWorkspaceOnly(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	group := groupNamed(ctx, t, tenantA, freshName(t))
	external := freshName(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return directoryRepo().SetGroupExternalID(ctx, group.ID, external)
	}); err != nil {
		t.Fatalf("writing the identifier: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		if found, err := directoryRepo().FindGroupByExternalID(ctx, external); err != nil || found != group.ID {
			t.Errorf("by identifier: %s, %v", found, err)
		}
		if found, err := directoryRepo().FindGroupByName(ctx, group.Name); err != nil || found != group.ID {
			t.Errorf("by name: %s, %v", found, err)
		}
		ids, total, err := directoryRepo().PageGroups(ctx, 0, 1000)
		if err != nil || total < 1 || len(ids) == 0 {
			t.Errorf("paged %d of %d, %v", len(ids), total, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := directoryRepo().FindGroupByExternalID(ctx, external); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("by identifier across workspaces: %v", err)
		}
		if _, err := directoryRepo().FindGroupByName(ctx, group.Name); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("by name across workspaces: %v", err)
		}
		ids, _, err := directoryRepo().PageGroups(ctx, 0, 1000)
		for _, id := range ids {
			if id == group.ID {
				t.Error("paged across workspaces")
			}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return directoryRepo().SetGroupExternalID(ctx, group.ID, "taken-over")
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("written across workspaces: %v, want not found", err)
	}
}
