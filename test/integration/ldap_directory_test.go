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
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A workspace's LDAP directory (migration 0285, UC-ID-31) against the real boundary: stored, its
// run recorded, whom it holds read by the prefix - and none of it seen or written from the
// workspace next door (gate SG-3).

func ldapRepo() postgres.LdapDirectoryRepository { return postgres.NewLdapDirectoryRepository() }

func TestAnLdapDirectoryIsItsWorkspacesOnly(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedAccount(ctx, t, tenantA)
	directory, err := identity.NewLdapDirectory(tenantA, identity.LdapDirectoryInput{
		URL: "ldaps://dc.example.com", BindDN: "cn=reader,dc=example,dc=com",
		UserBase: "ou=people,dc=example,dc=com", GroupBase: "ou=groups,dc=example,dc=com", RunAs: runAs,
	}, changedAt)
	if err != nil {
		t.Fatal(err)
	}
	sealed := crypto.Sealed{KeyID: "k1", Ciphertext: []byte("sealed")}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return ldapRepo().Save(ctx, directory, sealed, runAs)
	}); err != nil {
		t.Fatalf("storing: %v", err)
	}
	ran := directory.Ran(changedAt, identity.LdapPartial, "directory.owner_kept", identity.LdapCounts{Seen: 3, Kept: 1})
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return ldapRepo().RecordRun(ctx, ran)
	}); err != nil {
		t.Fatalf("recording a run: %v", err)
	}
	person := seedAccount(ctx, t, tenantA)
	external := identity.LdapExternalPrefix + freshName(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return directoryRepo().SetAccountExternalID(ctx, person, external, changedAt)
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, kept, err := ldapRepo().Find(ctx)
		if err != nil || found.GroupBase != directory.GroupBase || found.RunAs != runAs || string(kept.Ciphertext) != "sealed" ||
			found.LastOutcome != identity.LdapPartial || found.LastCounts == nil || found.LastCounts.Kept != 1 {
			t.Errorf("read back %+v, %v", found, err)
		}
		held, err := ldapRepo().Held(ctx)
		if err != nil || !holds(held, person, external) {
			t.Errorf("held %v, %v", held, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// The workspace next door has no directory, holds nobody of A's, and removes nothing.
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, _, err := ldapRepo().Find(ctx); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("read across workspaces: %v", err)
		}
		held, err := ldapRepo().Held(ctx)
		if err != nil || holds(held, person, external) {
			t.Errorf("held across workspaces: %v, %v", held, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return ldapRepo().RecordRun(ctx, ran)
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a run recorded across workspaces: %v", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error { return ldapRepo().Delete(ctx) }); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("removed across workspaces: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return ldapRepo().Delete(ctx) }); err != nil {
		t.Errorf("removing: %v", err)
	}
}

func holds(held []identity.LdapHeld, id shared.ID, external string) bool {
	for _, person := range held {
		if person.AccountID == id && person.ExternalID == external {
			return true
		}
	}
	return false
}
