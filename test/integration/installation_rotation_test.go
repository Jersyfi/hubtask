// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	integration "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/worker"
)

var (
	drillOwnProvider          = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe71")
	drillInstallationProvider = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe72")
	drillOfferedModel         = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe73")
)

// SC-12's acceptance: the rotation drill with an installation provider and an installation AI model
// present (UC-INS-16 checks 1, 2, 4; UC-INS-11 check 6; UC-AI-06 check 1). A workspace's own
// provider secret and AI key, the installation's provider secret and an offered model's key, all
// under the old key; the workspace's round in its transaction and the installation's round through
// the worker in the scope with no tenant, as the queue would run them; the census at zero for the
// old key; and every value opening under a ring that holds the new key alone.
func TestARotationCompletesWithTheInstallationsOwnSecretsPresent(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	providers := postgres.NewIdentityProviderRepository()
	ownAi := postgres.NewAiProviderRepository()
	offeredAi := postgres.NewOfferedAiRepository()
	census := postgres.NewSealingRepository()
	now := time.Now().UTC()

	admin := adminPool(ctx, t)
	t.Cleanup(func() {
		done := context.WithoutCancel(ctx)
		_, _ = admin.Exec(done, `DELETE FROM identity_provider WHERE id IN ($1, $2)`,
			drillOwnProvider.String(), drillInstallationProvider.String())
		_, _ = admin.Exec(done, `DELETE FROM ai_provider WHERE id = $1 OR tenant_id = $2`,
			drillOfferedModel.String(), idpTenantA.String())
	})

	// Keys of this drill's own (the shared database's other fixtures name others).
	suffix := freshID(t).String()
	suffix = suffix[len(suffix)-8:]
	oldKey := crypto.KeyMaterial{ID: "inst_a_" + suffix, Material: secret.New(drillKeyOne)}
	newKey := crypto.KeyMaterial{ID: "inst_b_" + suffix, Material: secret.New(drillKeyTwo)}
	before := ring(t, oldKey)
	seal := func(value string, purpose cryptoport.Purpose) cryptoport.Sealed {
		sealed, err := before.Seal(ctx, secret.New(value), purpose)
		if err != nil {
			t.Fatalf("sealing under the old key: %v", err)
		}
		return sealed
	}

	ownProviderPurpose := identityservice.ClientSecretPurpose(idpTenantA)
	installationProviderPurpose := identityservice.ClientSecretPurpose(shared.ID(""))
	ownAiPurpose := integrationservice.AiKeyPurpose(idpTenantA)
	offeredPurpose := integrationservice.OfferedAiKeyPurpose(drillOfferedModel)

	own, err := identity.NewIdentityProvider(identity.NewIdentityProviderInput{
		ID: drillOwnProvider, TenantID: idpTenantA, Issuer: "https://id.drill-own.example",
		ClientID: "hubtask", Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the workspace's provider: %v", err)
	}
	ownModel, err := integration.NewAiProvider(integration.NewAiProviderInput{
		TenantID: idpTenantA, Kind: integration.AiOpenAiCompatible, BaseURL: "https://own.drill.example/v1",
		CompletionModel: "chat", Jurisdiction: integration.AiEEA, Now: now,
	})
	if err != nil {
		t.Fatalf("building the workspace's model: %v", err)
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		if _, err := providers.Insert(ctx, own, seal("own-client-secret", ownProviderPurpose)); err != nil {
			return err
		}
		sealed := seal("own-api-key", ownAiPurpose)
		_, err := ownAi.Upsert(ctx, ownModel, &sealed, now)
		return err
	})

	installationProvider, err := identity.NewIdentityProvider(identity.NewIdentityProviderInput{
		ID: drillInstallationProvider, Issuer: "https://id.drill-installation.example",
		ClientID: "hubtask-platform", Provisioning: string(identity.ProvisionInvitedOnly),
		Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the installation's provider: %v", err)
	}
	offered, err := integration.NewOfferedAiProvider(integration.NewOfferedAiProviderInput{
		ID: drillOfferedModel, DisplayName: "Drill model", Processor: "A processor",
		Kind: integration.AiOpenAiCompatible, BaseURL: "https://offered.drill.example/v1",
		CompletionModel: "chat", Jurisdiction: integration.AiEEA, Now: now,
	})
	if err != nil {
		t.Fatalf("building the offered model: %v", err)
	}
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		if _, err := providers.Insert(ctx, installationProvider,
			seal("platform-client-secret", installationProviderPurpose)); err != nil {
			return err
		}
		sealed := seal("offered-api-key", offeredPurpose)
		_, err := offeredAi.Insert(ctx, offered, &sealed, now)
		return err
	})

	count := func() map[string]int64 {
		t.Helper()
		counted, err := sealing.Count(ctx, uow, census, []adminrepo.TenantRecord{{ID: idpTenantA}, {ID: idpTenantB}})
		if err != nil {
			t.Fatalf("the census: %v", err)
		}
		return counted.ByKey()
	}
	if counts := count(); counts[oldKey.ID] != 4 {
		t.Fatalf("before the rotation the census counts %d under the old key, want 4", counts[oldKey.ID])
	}

	// The rotation: the new key in front, the old one kept. The workspace's round in its own
	// transaction, the installation's through the worker in the scope the runner opens for a job
	// without a tenant.
	after := ring(t, newKey, oldKey)
	round := sealing.RunReseal{
		Resealers: []sealing.Resealer{
			identityservice.IdentityProviderResealer{Providers: providers, Encryptor: after},
			integrationservice.AiProviderResealer{Providers: ownAi, Encryptor: after},
		},
		Encryptor: after, Audit: &drillTrail{}, Clock: clockadapter.System{},
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		_, err := round.Execute(ctx, appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: idpTenantA})
		return err
	})
	handler := worker.SecretReseal{Round: round, Installation: sealing.RunInstallationReseal{
		Resealers: []sealing.Resealer{
			identityservice.IdentityProviderResealer{Providers: providers, Encryptor: after},
			integrationservice.OfferedAiProviderResealer{Providers: offeredAi, Encryptor: after},
		},
		Encryptor: after, Journal: postgres.NewInstanceJournal(pageCursors()), Clock: clockadapter.System{},
		IDs: clockadapter.NewUUIDv7(clockadapter.System{}),
	}}
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		_, err := handler.Run(ctx, queue.Job{Kind: queue.KindSecretReseal})
		return err
	})

	// Step 4 of the procedure: nothing names the old key, at either level.
	if counts := count(); counts[oldKey.ID] != 0 || counts[newKey.ID] != 4 {
		t.Fatalf("after both rounds the census answers old %d, new %d", counts[oldKey.ID], counts[newKey.ID])
	}

	// The installation's round left its evidence in the installation's journal: counts and the
	// key's name, not a byte of any value.
	var details string
	if err := admin.QueryRow(ctx,
		`SELECT details::text FROM instance_event
		 WHERE action = 'instance.secrets_resealed' AND details->>'active_key_id' = $1`,
		newKey.ID).Scan(&details); err != nil {
		t.Fatalf("the journal entry: %v", err)
	}
	for _, plaintext := range []string{"platform-client-secret", "offered-api-key"} {
		if strings.Contains(details, plaintext) {
			t.Errorf("the journal entry holds a secret: %s", details)
		}
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.WithoutCancel(ctx),
			`DELETE FROM instance_event WHERE details->>'active_key_id' = $1`, newKey.ID)
	})

	// The retirement: a ring of the new key alone opens all four, each under its own purpose.
	retired := ring(t, newKey)
	opens := func(sealed cryptoport.Sealed, purpose cryptoport.Purpose, want string) {
		t.Helper()
		if sealed.KeyID != newKey.ID {
			t.Errorf("a value still names %q", sealed.KeyID)
		}
		if opened, err := retired.Open(ctx, sealed, purpose); err != nil || opened.Reveal() != want {
			t.Errorf("%q does not open after the retirement: %v", want, err)
		}
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		_, sealed, err := providers.FindWithSecret(ctx, drillOwnProvider)
		if err != nil {
			return err
		}
		opens(sealed, ownProviderPurpose, "own-client-secret")
		_, key, err := ownAi.FindWithKey(ctx)
		if err != nil {
			return err
		}
		opens(*key, ownAiPurpose, "own-api-key")
		return nil
	})
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		_, sealed, err := providers.FindWithSecret(ctx, drillInstallationProvider)
		if err != nil {
			return err
		}
		opens(sealed, installationProviderPurpose, "platform-client-secret")
		_, key, err := offeredAi.FindWithKey(ctx, drillOfferedModel)
		if err != nil {
			return err
		}
		opens(*key, offeredPurpose, "offered-api-key")
		return nil
	}); err != nil {
		t.Fatalf("reading the installation's secrets back: %v", err)
	}
}

// Gate SG-3: OfferedAiSealings.ListSealed and RewrapKey - a workspace's transaction reads the
// offered keys (the read policy admits them, the call needs them) but moves none, and never sees a
// workspace's own key among them.
func TestAnOfferedKeyIsReSealedOnlyByTheInstallation(t *testing.T) {
	ctx := context.Background()
	seedOfferedRepoTenants(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	offeredAi := postgres.NewOfferedAiRepository()
	now := time.Now().UTC()
	sealed := cryptoport.Sealed{KeyID: "offered-reseal-k1", Ciphertext: []byte("x")}

	inSystem(ctx, t, uow, func(ctx context.Context) error {
		_, err := offeredAi.Insert(ctx, offeredModel(t, "Reseal model", now), &sealed, now)
		return err
	})
	ownModel, err := integration.NewAiProvider(integration.NewAiProviderInput{
		TenantID: offeredRepoTenantA, Kind: integration.AiOpenAiCompatible,
		BaseURL: "https://own.reseal.example/v1", CompletionModel: "chat",
		Jurisdiction: integration.AiEEA, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	ownSealed := cryptoport.Sealed{KeyID: "own-reseal-k1", Ciphertext: []byte("y")}
	inTenant(t, uow, offeredRepoTenantA, func(ctx context.Context) error {
		_, err := postgres.NewAiProviderRepository().Upsert(ctx, ownModel, &ownSealed, now)
		return err
	})
	t.Cleanup(func() {
		_, _ = adminPool(ctx, t).Exec(context.WithoutCancel(ctx),
			`DELETE FROM ai_provider WHERE tenant_id = $1`, offeredRepoTenantA.String())
	})

	inTenant(t, uow, offeredRepoTenantB, func(ctx context.Context) error {
		listed, err := offeredAi.ListSealed(ctx)
		if err != nil {
			return err
		}
		for _, row := range listed {
			if row.Sealed.KeyID == ownSealed.KeyID {
				t.Error("a workspace's own key is among the offered ones")
			}
		}
		if moved, err := offeredAi.RewrapKey(ctx, offeredRepoModel, movedSecret("k2"), sealed.KeyID); err != nil || moved {
			t.Errorf("a workspace rewrapped an offered key (%v, %v)", moved, err)
		}
		return nil
	})

	inSystem(ctx, t, uow, func(ctx context.Context) error {
		if moved, err := offeredAi.RewrapKey(ctx, offeredRepoModel, movedSecret("k2"), sealed.KeyID); err != nil || !moved {
			t.Fatalf("the installation's rewrap (%v, %v)", moved, err)
		}
		if moved, err := offeredAi.RewrapKey(ctx, offeredRepoModel, movedSecret("k3"), sealed.KeyID); err != nil || moved {
			t.Errorf("a rewrap guarded on a stale key moved the row (%v, %v)", moved, err)
		}
		return nil
	})
}
