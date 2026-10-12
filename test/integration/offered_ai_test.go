// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The repositories of offered models and of a workspace's choice, against the real boundary as the
// application role: every method asked from the installation's scope and from a workspace's, and
// a workspace's choice invisible next door.

var (
	offeredRepoTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc01")
	offeredRepoTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc02")
	offeredRepoModel   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc11")
)

func seedOfferedRepoTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ($1, 'offered-a', 'Offered A'), ($2, 'offered-b', 'Offered B')
		 ON CONFLICT (id) DO NOTHING`, offeredRepoTenantA.String(), offeredRepoTenantB.String()); err != nil {
		t.Fatalf("seeding tenants: %v", err)
	}
	t.Cleanup(func() {
		done := context.WithoutCancel(ctx)
		_, _ = admin.Exec(done, `DELETE FROM ai_source WHERE tenant_id IN ($1, $2)`,
			offeredRepoTenantA.String(), offeredRepoTenantB.String())
		_, _ = admin.Exec(done, `DELETE FROM ai_provider WHERE id = $1`, offeredRepoModel.String())
	})
}

func offeredModel(t *testing.T, name string, now time.Time) domain.AiProvider {
	t.Helper()
	offered, err := domain.NewOfferedAiProvider(domain.NewOfferedAiProviderInput{
		ID: offeredRepoModel, DisplayName: name, Processor: "A processor",
		Kind: domain.AiOpenAiCompatible, BaseURL: "https://offered.example/v1",
		CompletionModel: "offered-chat", EmbeddingModel: "offered-embed",
		Jurisdiction: domain.AiEEA, Now: now,
	})
	if err != nil {
		t.Fatalf("building the offered model: %v", err)
	}
	return offered
}

func inSystem(t *testing.T, uow persistence.UnitOfWork, fn func(context.Context) error) {
	t.Helper()
	if err := uow.Within(context.Background(), persistence.SystemScope(), fn); err != nil {
		t.Fatalf("installation transaction: %v", err)
	}
}

// errRolledBack ends a transaction whose writes were refused on purpose: after a refused statement
// PostgreSQL aborts the transaction, and a commit would only report that again.
var errRolledBack = errors.New("rolled back on purpose")

func inTenantRolledBack(t *testing.T, uow persistence.UnitOfWork, tenant shared.ID, fn func(context.Context)) {
	t.Helper()
	err := uow.Within(context.Background(), persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
		fn(ctx)
		return errRolledBack
	})
	if !errors.Is(err, errRolledBack) {
		t.Fatalf("transaction: %v", err)
	}
}

func TestTheInstallationOffersAModelEveryWorkspaceReadsAndNoneWrites(t *testing.T) {
	ctx := context.Background()
	seedOfferedRepoTenants(ctx, t)
	offered := postgres.NewOfferedAiRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()
	sealed := cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("offered sealed key")}

	inSystem(t, uow, func(ctx context.Context) error {
		stored, err := offered.Insert(ctx, offeredModel(t, "Repository model", now), &sealed, now)
		if err != nil {
			t.Fatalf("offering: %v", err)
		}
		if stored.ID != offeredRepoModel || !stored.HasAPIKey || stored.Version != 1 {
			t.Errorf("stored as %+v", stored)
		}
		changed := offeredModel(t, "Repository model, renamed", now)
		if _, err := offered.UpdateKeepingKey(ctx, changed, now.Add(time.Minute)); err != nil {
			t.Fatalf("renaming: %v", err)
		}
		_, envelope, err := offered.FindWithKey(ctx, offeredRepoModel)
		if err != nil || envelope == nil || string(envelope.Ciphertext) != "offered sealed key" {
			t.Errorf("keeping the key did not keep it: %v", err)
		}
		return nil
	})

	// A workspace reads it - its adapter needs the row and the key - and changes nothing.
	inTenantRolledBack(t, uow, offeredRepoTenantA, func(ctx context.Context) {
		listed, err := offered.List(ctx)
		if err != nil {
			t.Fatalf("listing from a workspace: %v", err)
		}
		seen := false
		for _, model := range listed {
			seen = seen || model.ID == offeredRepoModel
		}
		if !seen {
			t.Error("a workspace cannot see the offered model")
		}
		if _, envelope, err := offered.FindWithKey(ctx, offeredRepoModel); err != nil || envelope == nil {
			t.Errorf("the workspace's adapter cannot read the offered key: %v", err)
		}
		if _, err := offered.Update(ctx, offeredModel(t, "Taken over", now), nil, now); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a workspace's update answered %v, want not found", err)
		}
		if removed, err := offered.Delete(ctx, offeredRepoModel); err != nil || removed {
			t.Errorf("a workspace withdrew the installation's model (%v, %v)", removed, err)
		}
		if _, err := offered.Insert(ctx, offeredModel(t, "Forged", now), nil, now); err == nil {
			t.Error("a workspace offered a model")
		}
	})

	inSystem(t, uow, func(ctx context.Context) error {
		found, err := offered.Find(ctx, offeredRepoModel)
		if err != nil || found.DisplayName != "Repository model, renamed" || found.Version != 2 {
			t.Errorf("the model came back as %+v (%v)", found, err)
		}
		return nil
	})
}

func TestAWorkspacesChoiceIsItsOwnAndOutlivesAWithdrawnModelAsNothing(t *testing.T) {
	ctx := context.Background()
	seedOfferedRepoTenants(ctx, t)
	offered := postgres.NewOfferedAiRepository()
	choices := postgres.NewAiChoiceRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	inSystem(t, uow, func(ctx context.Context) error {
		_, err := offered.Insert(ctx, offeredModel(t, "Choice model", now), nil, now)
		return err
	})

	inTenant(t, uow, offeredRepoTenantA, func(ctx context.Context) error {
		before, err := choices.Find(ctx, offeredRepoTenantA)
		if err != nil || before.Source != domain.AiSourceOwn || before.Version != 0 {
			t.Errorf("a workspace that never chose answered %+v (%v), want OWN", before, err)
		}
		choice, _ := domain.NewAiChoice(offeredRepoTenantA, domain.AiSourceOffered, offeredRepoModel, now)
		saved, err := choices.Save(ctx, choice)
		if err != nil || saved.Version != 1 || saved.OfferedProviderID != offeredRepoModel {
			t.Fatalf("saving the choice: %+v (%v)", saved, err)
		}
		return nil
	})

	inTenant(t, uow, offeredRepoTenantB, func(ctx context.Context) error {
		next, err := choices.Find(ctx, offeredRepoTenantB)
		if err != nil || next.Source != domain.AiSourceOwn {
			t.Errorf("A's choice is visible next door: %+v (%v)", next, err)
		}
		return nil
	})

	inSystem(t, uow, func(ctx context.Context) error {
		removed, err := offered.Delete(ctx, offeredRepoModel)
		if err != nil || !removed {
			t.Fatalf("withdrawing: %v %v", removed, err)
		}
		return nil
	})

	inTenantRolledBack(t, uow, offeredRepoTenantA, func(ctx context.Context) {
		after, err := choices.Find(ctx, offeredRepoTenantA)
		if err != nil || after.Source != domain.AiSourceOffered || !after.OfferedProviderID.IsZero() {
			t.Errorf("after the withdrawal the choice is %+v (%v), want OFFERED pointing at nothing", after, err)
		}
		// Choosing the withdrawn model again is refused by the reference, not stored.
		stale, _ := domain.NewAiChoice(offeredRepoTenantA, domain.AiSourceOffered, offeredRepoModel, now)
		if _, err := choices.Save(ctx, stale); shared.AsError(err) == nil ||
			shared.AsError(err).DetailCode != "ai.offered_model_unknown" {
			t.Errorf("choosing a withdrawn model answered %v", err)
		}
	})
}
