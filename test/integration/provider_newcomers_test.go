// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// What newcomers get (UC-ID-11 check 5, migration 0132), against the real boundary: the setting
// comes back as it was stored, a workspace cannot name another workspace's group, and a group
// deleted leaves the provider in place with its newcomers getting nothing.

var (
	ncTenantA   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132a01")
	ncTenantB   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132a02")
	ncGroupA    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132a11")
	ncGroupGone = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132a12")
	ncGroupB    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132a13")
	ncRowRole   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132a21")
	ncRowGroup  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132a22")
	ncRowB      = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132a23")
)

func seedNewcomerTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	statements := []string{
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ('` + ncTenantA.String() + `', 'newcomers-a', 'Newcomers A'),
		        ('` + ncTenantB.String() + `', 'newcomers-b', 'Newcomers B')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO account_group (id, tenant_id, name)
		 VALUES ('` + ncGroupA.String() + `', '` + ncTenantA.String() + `', 'Staff'),
		        ('` + ncGroupGone.String() + `', '` + ncTenantA.String() + `', 'Soon gone'),
		        ('` + ncGroupB.String() + `', '` + ncTenantB.String() + `', 'Next door')
		 ON CONFLICT (id) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding the newcomer tenants: %v", err)
		}
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx,
			`DELETE FROM identity_provider WHERE tenant_id IN ($1, $2)`,
			ncTenantA.String(), ncTenantB.String()); err != nil {
			t.Errorf("cleaning up the providers: %v", err)
		}
	})
}

func newcomerProvider(t *testing.T, id, tenant shared.ID, issuer, role string, group shared.ID) domain.IdentityProvider {
	t.Helper()
	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: id, TenantID: tenant, Issuer: issuer, ClientID: "hubtask",
		Provisioning: string(domain.ProvisionAny), NewcomerRole: role, NewcomerGroupID: group,
		Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	return configured
}

func TestWhatNewcomersGetIsStoredAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedNewcomerTenants(ctx, t)
	providers := postgres.NewIdentityProviderRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	sealed := cryptoport.Sealed{KeyID: "k132", Ciphertext: []byte("newcomers' sealed client secret")}

	inTenant(t, uow, ncTenantA, func(ctx context.Context) error {
		withRole, err := providers.Insert(ctx,
			newcomerProvider(t, ncRowRole, ncTenantA, "https://role.newcomers.example", "MEMBER", ""), sealed)
		if err != nil {
			t.Fatalf("writing the role: %v", err)
		}
		if withRole.NewcomerRole != domain.RoleMember || !withRole.NewcomerGroupID.IsZero() {
			t.Errorf("the role came back as %q/%q", withRole.NewcomerRole, withRole.NewcomerGroupID)
		}
		withGroup, err := providers.Insert(ctx,
			newcomerProvider(t, ncRowGroup, ncTenantA, "https://group.newcomers.example", "", ncGroupGone), sealed)
		if err != nil {
			t.Fatalf("writing the group: %v", err)
		}
		if withGroup.NewcomerGroupID != ncGroupGone || withGroup.NewcomerRole != "" {
			t.Errorf("the group came back as %q/%q", withGroup.NewcomerRole, withGroup.NewcomerGroupID)
		}
		// The arrival reads the row with its secret; what a newcomer gets is decided from it.
		found, _, err := providers.FindWithSecret(ctx, ncRowGroup)
		if err != nil || found.NewcomerGroupID != ncGroupGone {
			t.Errorf("the arrival's read answered %q (%v)", found.NewcomerGroupID, err)
		}
		// Set whole: a replace without newcomers gives them nothing.
		cleared := withRole
		cleared.NewcomerRole = ""
		stored, ok, err := providers.Reconfigure(ctx, cleared, nil, time.Now().UTC())
		if err != nil || !ok || stored.NewcomerRole != "" {
			t.Errorf("clearing answered %q (%v, %v)", stored.NewcomerRole, ok, err)
		}
		return nil
	})

	// B names A's group: the tenant-scoped key refuses it, whatever the application checked.
	err := uow.Within(ctx, persistence.Scope{TenantID: ncTenantB}, func(ctx context.Context) error {
		_, err := providers.Insert(ctx,
			newcomerProvider(t, ncRowB, ncTenantB, "https://b.newcomers.example", "", ncGroupA), sealed)
		return err
	})
	if err == nil {
		t.Error("a workspace's provider named another workspace's group")
	}

	// The group goes; the provider stays, and its newcomers get nothing.
	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM account_group WHERE id = $1`, ncGroupGone.String()); err != nil {
		t.Fatalf("deleting the group: %v", err)
	}
	inTenant(t, uow, ncTenantA, func(ctx context.Context) error {
		found, err := providers.Find(ctx, ncRowGroup)
		if err != nil {
			t.Fatalf("the provider went with its group: %v", err)
		}
		if found.GivesNewcomers() {
			t.Errorf("a deleted group is still given: %q", found.NewcomerGroupID)
		}
		return nil
	})
}
