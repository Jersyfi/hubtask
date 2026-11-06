// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// UC-ID-25 checks 5 and 6 against the database (migration 0213): the device's list takes an entry
// only while its host is a live workspace's main address - not the host a change of address left
// behind, not one beside a main own domain, not a deleted workspace's - and only one this
// installation signed.
var deviceListGone = shared.MustParseID("01936f2a-7c1e-7000-8000-000000213a03")

func TestTheDeviceListAnswersOnlyAMainAddress(t *testing.T) {
	ctx := context.Background()
	seedDomainTenants(ctx, t)
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`DELETE FROM tenant_host WHERE tenant_id = '` + deviceListGone.String() + `'`,
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + deviceListGone.String() + `', 'device-gone', 'Gone')
		 ON CONFLICT (id) DO NOTHING`,
		`UPDATE tenant SET deleted_at = now() WHERE id = '` + deviceListGone.String() + `'`,
		// The workspace's own domain is its main address; its subdomain stays, no longer main.
		`UPDATE tenant_host SET is_canonical = false WHERE host = 'domain-acme.hub.example'`,
		`INSERT INTO tenant_host (tenant_id, host, kind, state, verification, verified_at, is_canonical, created_at)
		 VALUES ('` + domainTenant.String() + `', 'tasks.own-domain.example', 'OWN_DOMAIN', 'ACTIVE', 'hbt-dv-integration', now(), true, now())`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	servedAt(ctx, t, deviceListGone, "device-gone.hub.example")

	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	hosts := postgres.NewOwnDomainRepository()
	codec := security.NewDeviceListEntryCodec(secret.New("0123456789abcdef0123456789abcdef"))
	check := identity.CheckDeviceList{Signer: codec, Hosts: hosts, UnitOfWork: uow, Multi: true}
	sign := func(host string) string { return codec.Sign(domain.DeviceListEntry{Host: host, Name: "A workspace"}) }
	foreign := security.NewDeviceListEntryCodec(secret.New("fedcba9876543210fedcba9876543210")).
		Sign(domain.DeviceListEntry{Host: "domain-north.hub.example", Name: "Northwind Studio"})

	checked, err := check.Execute(ctx, []string{
		sign("tasks.own-domain.example"), sign("domain-acme.hub.example"), sign("domain-north.hub.example"),
		sign("device-gone.hub.example"), sign("nobody.hub.example"), foreign,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"tasks.own-domain.example": true, "domain-acme.hub.example": false, "domain-north.hub.example": true,
		"device-gone.hub.example": false, "nobody.hub.example": false,
	}
	if len(checked) != len(want) {
		t.Fatalf("answered %d entries, want %d (the foreign one left out): %+v", len(checked), len(want), checked)
	}
	for _, each := range checked {
		if each.Answers != want[each.Host] {
			t.Errorf("%s answers %v, want %v", each.Host, each.Answers, want[each.Host])
		}
	}

	// Cross-tenant: inside another workspace's transaction the question answers the same yes or no
	// and nothing more - no row of the other workspace is readable through it.
	inTenantRolledBack(t, uow, domainOther, func(ctx context.Context) {
		main, err := hosts.IsMainHost(ctx, "tasks.own-domain.example")
		if err != nil || !main {
			t.Errorf("from another workspace: %v, %v", main, err)
		}
		if main, err := hosts.IsMainHost(ctx, "TASKS.own-domain.example."); err != nil || !main {
			t.Errorf("a host as a request writes it: %v, %v", main, err)
		}
	})
}
