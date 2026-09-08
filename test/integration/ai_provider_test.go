// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The AI provider's surface against the real boundary (J-02). Gate SG-3: one workspace's provider,
// its sealed key and its consent are invisible and unusable next door - and the six repository
// methods are covered here rather than five, because a method a cross-tenant test never calls is a
// method the boundary has never been asked about.

var (
	aiTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fa01")
	aiTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fa02")
)

func seedAiProviderTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ('`+aiTenantA.String()+`', 'ai-a', 'AI A'),
		        ('`+aiTenantB.String()+`', 'ai-b', 'AI B')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("seeding the AI provider tenants: %v", err)
	}
}

func aiConfigurationFor(t *testing.T, tenantID shared.ID, now time.Time) domain.AiProvider {
	t.Helper()
	configured, err := domain.NewAiProvider(domain.NewAiProviderInput{
		TenantID: tenantID, Kind: domain.AiOpenAiCompatible,
		BaseURL: "https://api.a.example/v1", CompletionModel: "a-model",
		Jurisdiction: domain.AiEEA, ProcessingAllowed: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the configuration: %v", err)
	}
	return configured
}

func TestOneWorkspacesAiProviderIsInvisibleNextDoor(t *testing.T) {
	ctx := context.Background()
	seedAiProviderTenants(ctx, t)

	providers := postgres.NewAiProviderRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()
	configured := aiConfigurationFor(t, aiTenantA, now)
	sealed := cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("A's sealed API key")}

	inTenant(t, uow, aiTenantA, func(ctx context.Context) error {
		stored, err := providers.Upsert(ctx, configured, &sealed, now)
		if err != nil {
			t.Fatalf("writing A's provider: %v", err)
		}
		if !stored.HasAPIKey || stored.Version != 1 || !stored.ProcessingAllowed {
			t.Errorf("A's provider came back as %+v", stored)
		}
		return nil
	})

	// Gate SG-3: every read answers nothing next door, and neither write reaches A's row.
	inTenant(t, uow, aiTenantB, func(ctx context.Context) error {
		if _, err := providers.Find(ctx); err == nil {
			t.Error("A's provider is visible next door")
		} else if !isNotFound(err) {
			t.Errorf("B's read answered %v, want not found", err)
		}
		if _, envelope, err := providers.FindWithKey(ctx); err == nil || envelope != nil {
			t.Error("A's sealed key is readable next door")
		}
		moved, err := providers.RewrapKey(ctx,
			cryptoport.Sealed{KeyID: "k2", Ciphertext: []byte("B's")}, "k1")
		if err != nil {
			t.Fatalf("B's rewrap: %v", err)
		}
		if moved {
			t.Error("B re-sealed A's key")
		}
		removed, err := providers.Delete(ctx)
		if err != nil {
			t.Fatalf("B's delete: %v", err)
		}
		if removed {
			t.Error("B deleted A's provider")
		}
		return nil
	})

	inTenant(t, uow, aiTenantA, func(ctx context.Context) error {
		found, envelope, err := providers.FindWithKey(ctx)
		if err != nil {
			t.Fatalf("reading A's provider back: %v", err)
		}
		if found.CompletionModel != "a-model" || found.Jurisdiction != domain.AiEEA {
			t.Errorf("A's provider is %+v", found)
		}
		if envelope == nil || string(envelope.Ciphertext) != "A's sealed API key" ||
			envelope.KeyID != "k1" {
			t.Error("A's envelope did not come back as it was stored")
		}
		return nil
	})
}

// The two upserts differ in exactly one thing, and it is the thing the port promises: one replaces
// the envelope and the other leaves it alone. Proved against the real column rather than a fake,
// because "keep what is there" is a property of the statement.
func TestKeepingTheKeyLeavesTheEnvelopeAndReplacingItDoesNot(t *testing.T) {
	ctx := context.Background()
	seedAiProviderTenants(ctx, t)

	providers := postgres.NewAiProviderRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()
	configured := aiConfigurationFor(t, aiTenantB, now)
	sealed := cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("B's sealed API key")}

	inTenant(t, uow, aiTenantB, func(ctx context.Context) error {
		if _, err := providers.Upsert(ctx, configured, &sealed, now); err != nil {
			t.Fatalf("writing B's provider: %v", err)
		}

		// A model changed and no key sent: the envelope stays, the version rises.
		changed := configured
		changed.CompletionModel = "another-model"
		stored, err := providers.UpsertKeepingKey(ctx, changed, now.Add(time.Minute))
		if err != nil {
			t.Fatalf("reconfiguring: %v", err)
		}
		if !stored.HasAPIKey || stored.Version != 2 || stored.CompletionModel != "another-model" {
			t.Errorf("the reconfiguration answered %+v", stored)
		}
		_, envelope, err := providers.FindWithKey(ctx)
		if err != nil {
			t.Fatalf("reading back: %v", err)
		}
		if envelope == nil || string(envelope.Ciphertext) != "B's sealed API key" {
			t.Error("keeping the key did not keep it")
		}

		// A key cleared: the envelope goes, and the pair goes together - the migration's
		// constraint would refuse a ciphertext without its label.
		cleared, err := providers.Upsert(ctx, changed, nil, now.Add(2*time.Minute))
		if err != nil {
			t.Fatalf("clearing the key: %v", err)
		}
		if cleared.HasAPIKey {
			t.Error("a cleared key is still reported as stored")
		}
		_, envelope, err = providers.FindWithKey(ctx)
		if err != nil {
			t.Fatalf("reading back after clearing: %v", err)
		}
		if envelope != nil {
			t.Error("the envelope survived being cleared")
		}
		return nil
	})

	// And the row is one row, not three.
	var rows int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM ai_provider WHERE tenant_id = $1`, aiTenantB.String(),
	).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("B holds %d provider rows (%v), want exactly one", rows, err)
	}

	// Clean up after itself: this package shares its database across files, so a workspace left
	// with a provider would surprise whoever asserts a census next.
	inTenant(t, uow, aiTenantB, func(ctx context.Context) error {
		if _, err := providers.Delete(ctx); err != nil {
			t.Fatalf("clearing up: %v", err)
		}
		return nil
	})
}
