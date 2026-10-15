// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// ADR-0072 §6 and the record's review finding F1: while a workspace moves from its own model to an
// offered one of the same name, a query embedded by the offered model is never compared with the
// own model's vectors - the offered model's vectors live in a space of their own - and search by
// meaning answers nothing for them until the pass re-embeds, while search by words keeps working.
func TestAnOfferedModelsQueryIsNeverComparedWithTheOwnModelsVectors(t *testing.T) {
	ctx := context.Background()
	requirePgvector(ctx, t)
	seedContainerTenants(ctx, t)

	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	own := freshID(t)
	axis := freshAxis()
	offeredModel := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000a1d11")

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		key, err := service.OrderKeyAfter("")
		if err != nil {
			return err
		}
		task := taskIn(tenantA, authorA, collection, own, "Kartoffelsuppe kochen", key)
		task.ContentLanguage = "de"
		if err := itemRepo().Insert(ctx, task); err != nil {
			return err
		}
		return postgres.NewEmbeddingRepository().Store(ctx, repository.StoredEmbedding{
			ItemID: own, Model: "shared-embed", Vector: meaningOf(axis),
			SourceDigest: suggestion.Digest("Kartoffelsuppe kochen", ""), UpdatedAt: time.Now().UTC(),
		})
	}); err != nil {
		t.Fatalf("seeding the own model's vector: %v", err)
	}

	page := searched(ctx, t, tenantA, repository.TextSearch{
		Anchor:  repository.Anchor{Kind: repository.AnchorCollection, CollectionID: collection, IncludeDescendants: true},
		Request: view.Search{Words: "zzz-nothing-carries-this", Language: "de", Size: 50},
		Meaning: meaningOf(axis), MeaningModel: domain.OfferedEmbeddingSpace(offeredModel, "shared-embed"),
	})
	for _, hit := range page.Hits {
		if hit.Item.ID == own {
			t.Errorf("the own model's vector was ranked against the offered model's query: %v", hitTitles(page))
		}
	}

	// And the screen says so: vectors of another space than the one in use remain.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		remains, err := postgres.NewEmbeddingRepository().OtherSpaceRemains(ctx, domain.OfferedEmbeddingSpace(offeredModel, "shared-embed"))
		if err != nil {
			return err
		}
		if !remains {
			t.Error("the rebuild is not noticed while the own model's vectors remain")
		}
		return nil
	}); err != nil {
		t.Fatalf("asking: %v", err)
	}
}

type allowEveryone struct{}

func (allowEveryone) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return nil
}

func (allowEveryone) Permits(context.Context, appshared.ActorContext, access.Request) (bool, error) {
	return true, nil
}

// Switching the source seeds the embedding pass in the same transaction as the choice and its
// trail entry - against PostgreSQL, where a write outside the transaction would be refused.
func TestSwitchingTheSourceSeedsTheEmbeddingPassWithTheChoice(t *testing.T) {
	ctx := context.Background()
	world := newResolverWorld(t)
	world.rule(t, domain.AiSourcesEither, domain.AiMinAny)
	uow := world.uow
	rules := integrationservice.AiRules{
		Instance: postgres.NewInstanceSettingRepository(), Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Offered: postgres.NewOfferedAiRepository(), Choices: postgres.NewAiChoiceRepository(),
		Own: postgres.NewAiProviderRepository(),
	}
	writer := integrationservice.AiSourceWriter{
		Rules: rules, Authorizer: allowEveryone{}, Audit: postgres.NewAuditSink(generator{t}),
		UnitOfWork: uow, Clock: clockadapter.System{}, Spaces: postgres.NewEmbeddingRepository(),
		Reindex: integrationservice.EmbeddingSeeder{
			Jobs: postgres.NewQueue(generator{t}, clockadapter.System{}), Clock: clockadapter.System{},
		},
	}
	registry, err := usecase.NewRegistry(nil,
		integrationservice.ChooseAiSource{Writer: writer}.Descriptor(),
		integrationservice.ReadAiSource{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	actor := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: resolverTenant, AccountName: "Owner",
		Scopes: []string{"ai:manage"}}

	admin := adminPool(ctx, t)
	_, _ = admin.Exec(ctx, `DELETE FROM job WHERE tenant_id = $1 AND kind = 'ai.embed'`, resolverTenant.String())
	t.Cleanup(func() {
		_, _ = admin.Exec(context.WithoutCancel(ctx), `DELETE FROM job WHERE tenant_id = $1`, resolverTenant.String())
	})

	if _, err := registry.Invoke(ctx, integrationservice.ChooseAiSourceName, actor, usecase.Input{
		"source": "OFFERED", "offered_model_id": resolverModel.String(),
	}); err != nil {
		t.Fatalf("choosing the offered model: %v", err)
	}

	var jobs, trail int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'ai.embed'`, resolverTenant.String()).Scan(&jobs); err != nil {
		t.Fatalf("counting jobs: %v", err)
	}
	if jobs != 1 {
		t.Errorf("switching the source queued %d embedding passes, want one", jobs)
	}
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'ai.source_chosen'`, resolverTenant.String()).Scan(&trail); err != nil {
		t.Fatalf("counting the trail: %v", err)
	}
	if trail < 1 {
		t.Error("the choice - the consent to the offered model - is not in the trail")
	}

	read, err := registry.Invoke(ctx, integrationservice.ReadAiSourceName, actor, usecase.Input{})
	if err != nil || read["source"] != "OFFERED" {
		t.Errorf("read back %v (%v)", read, err)
	}
}
