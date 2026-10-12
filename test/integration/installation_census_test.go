// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	sealingrepo "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	integration "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The installation's census (UC-INS-16 check 1): every workspace's own values, and the
// installation's own sign-in provider and offered model once each - not once per workspace that
// can read them.

var (
	censusOwnProviderA   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe61")
	censusOwnProviderB   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe62")
	censusSharedProvider = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe63")
	censusOfferedModel   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe64")
)

func TestTheInstallationsCensusCountsEachSecretOnceAndByKind(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	providers := postgres.NewIdentityProviderRepository()
	census := postgres.NewSealingRepository()
	now := time.Now().UTC()
	const keyID = "census-both-levels"
	sealed := cryptoport.Sealed{KeyID: keyID, Ciphertext: []byte("x")}

	admin := adminPool(ctx, t)
	t.Cleanup(func() {
		done := context.WithoutCancel(ctx)
		_, _ = admin.Exec(done, `DELETE FROM identity_provider WHERE id IN ($1, $2, $3)`,
			censusOwnProviderA.String(), censusOwnProviderB.String(), censusSharedProvider.String())
		_, _ = admin.Exec(done, `DELETE FROM ai_provider WHERE id = $1`, censusOfferedModel.String())
	})

	for tenant, row := range map[shared.ID]shared.ID{idpTenantA: censusOwnProviderA, idpTenantB: censusOwnProviderB} {
		own, err := identity.NewIdentityProvider(identity.NewIdentityProviderInput{
			ID: row, TenantID: tenant, Issuer: "https://id.census-" + row.String()[30:] + ".example",
			ClientID: "hubtask", Enabled: true, Now: now,
		})
		if err != nil {
			t.Fatalf("building a workspace's provider: %v", err)
		}
		inTenant(t, uow, tenant, func(ctx context.Context) error {
			_, err := providers.Insert(ctx, own, sealed)
			return err
		})
	}
	installationProvider, err := identity.NewIdentityProvider(identity.NewIdentityProviderInput{
		ID: censusSharedProvider, Issuer: "https://id.census-shared.example", ClientID: "hubtask",
		Provisioning: string(identity.ProvisionInvitedOnly), Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the installation's provider: %v", err)
	}
	offered, err := integration.NewOfferedAiProvider(integration.NewOfferedAiProviderInput{
		ID: censusOfferedModel, DisplayName: "Census model", Processor: "A processor",
		Kind: integration.AiOpenAiCompatible, BaseURL: "https://census.example/v1",
		CompletionModel: "chat", Jurisdiction: integration.AiEEA, Now: now,
	})
	if err != nil {
		t.Fatalf("building the offered model: %v", err)
	}
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		if _, err := providers.Insert(ctx, installationProvider, sealed); err != nil {
			return err
		}
		_, err := postgres.NewOfferedAiRepository().Insert(ctx, offered, &sealed, now)
		return err
	})

	counted, err := sealing.Count(ctx, uow, census,
		[]adminrepo.TenantRecord{{ID: idpTenantA}, {ID: idpTenantB}})
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	want := []sealingrepo.StoreCount{
		{Store: sealingrepo.StoreAiProvider, Count: 1},
		{Store: sealingrepo.StoreIdentityProvider, Count: 3},
	}
	got := counted.Stores(keyID)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the key's census by kind %+v, want %+v", got, want)
	}

	// Gate SG-3: Census.CountInstallation - asked from a workspace's scope, it still answers only
	// the rows of no workspace, never that workspace's own.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		installation, err := census.CountInstallation(ctx)
		if err != nil {
			t.Fatalf("the installation's census from a workspace: %v", err)
		}
		if n := installation.ByKey()[keyID]; n != 2 {
			t.Errorf("the installation's census answered %d values under the key, want its own two", n)
		}
		return nil
	})
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		own, err := census.Count(ctx)
		if err != nil {
			return err
		}
		if n := own.ByKey()[keyID]; n != 0 {
			t.Errorf("a workspace's census in the installation's scope counted %d", n)
		}
		return nil
	}); err != nil {
		t.Fatalf("the workspace census in the installation's scope: %v", err)
	}
}
