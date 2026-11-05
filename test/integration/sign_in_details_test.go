// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The people list's addresses and authenticator apps are read of the reading workspace only: an
// identifier of another workspace's person answers nothing of them (T-04). An enrolment never
// confirmed is no factor, and a service account is no person.
func TestSignInDetailsStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)

	person := func(tenant shared.ID, email string) shared.ID {
		t.Helper()
		id := freshID(t)
		if _, err := admin.Exec(ctx,
			"INSERT INTO account (id, tenant_id, display_name, email) VALUES ($1, $2, $3, NULLIF($4, ''))",
			id.String(), tenant.String(), freshName(t), email); err != nil {
			t.Fatalf("seeding the account: %v", err)
		}
		return id
	}
	enrol := func(tenant, account shared.ID, confirmed bool) {
		t.Helper()
		if _, err := admin.Exec(ctx, `
			INSERT INTO account_mfa (account_id, tenant_id, secret_enc, secret_key_id, confirmed_at, created_at, updated_at)
			VALUES ($1, $2, '\x00', 'test', CASE WHEN $3 THEN now() END, now(), now())`,
			account.String(), tenant.String(), confirmed); err != nil {
			t.Fatalf("seeding the enrolment: %v", err)
		}
	}
	armed := person(tenantA, freshName(t)+"@a.example")
	enrol(tenantA, armed, true)
	begun := person(tenantA, freshName(t)+"@a.example")
	enrol(tenantA, begun, false)
	managed := person(tenantA, "")
	machine := person(tenantA, "")
	if _, err := admin.Exec(ctx, "UPDATE account SET kind = 'SERVICE_ACCOUNT' WHERE id = $1", machine.String()); err != nil {
		t.Fatalf("making a service account: %v", err)
	}
	other := person(tenantB, freshName(t)+"@b.example")
	enrol(tenantB, other, true)

	details := postgres.NewAccountRepository()
	readAs := func(tenant shared.ID, ids ...shared.ID) map[shared.ID]repository.SignInDetail {
		t.Helper()
		var got []repository.SignInDetail
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			got, err = details.SignInDetails(ctx, ids)
			return err
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		by := map[shared.ID]repository.SignInDetail{}
		for _, d := range got {
			by[d.AccountID] = d
		}
		return by
	}

	got := readAs(tenantA, armed, begun, managed, machine, other)
	if len(got) != 3 {
		t.Fatalf("tenant A read %+v, want its three people only", got)
	}
	if !got[armed].AuthenticatorApp || got[armed].Email == "" {
		t.Errorf("the armed person read %+v", got[armed])
	}
	if got[begun].AuthenticatorApp {
		t.Errorf("an enrolment never confirmed read as a factor: %+v", got[begun])
	}
	if d := got[managed]; d.Email != "" || d.AuthenticatorApp {
		t.Errorf("the managed person read %+v", d)
	}
	if got := readAs(tenantB, armed, begun, managed); len(got) != 0 {
		t.Errorf("tenant B read tenant A's people: %+v", got)
	}
	if got := readAs(tenantB, other); !got[other].AuthenticatorApp {
		t.Errorf("tenant B read %+v of its own", got)
	}
}
