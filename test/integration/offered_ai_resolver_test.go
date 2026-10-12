// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	metaservice "github.com/Jersyfi/hubtask/core/application/service/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/ai"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Which source answers, through the real resolver against PostgreSQL as the application role
// (UC-AI-05 checks 2, 4; UC-AI-06 checks 2, 4, 5): one case per `ai.sources` value, and the rule
// narrowed under a workspace that keeps its own configuration whole.

var (
	resolverTenant = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000a1c01")
	resolverModel  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000a1c11")
)

type resolverWorld struct {
	uow      persistence.UnitOfWork
	instance postgres.InstanceSettingRepository
	resolver ai.Resolver
	before   identityrepo.InstanceLevel
}

func newResolverWorld(t *testing.T) *resolverWorld {
	t.Helper()
	ctx := context.Background()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'resolver-ai', 'Resolver AI')
		 ON CONFLICT (id) DO NOTHING`, resolverTenant.String()); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	rules := integrationservice.AiRules{
		Instance: postgres.NewInstanceSettingRepository(), Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Offered: postgres.NewOfferedAiRepository(), Choices: postgres.NewAiChoiceRepository(),
		Own: postgres.NewAiProviderRepository(),
	}
	world := &resolverWorld{
		uow: uow, instance: postgres.NewInstanceSettingRepository(),
		resolver: ai.Resolver{
			Providers: postgres.NewAiProviderRepository(), UnitOfWork: uow, Sources: rules,
			Clock: portclock.Fixed(time.Now().UTC()),
		},
	}
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		var err error
		world.before, err = world.instance.Read(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the level: %v", err)
	}

	// The workspace's own model, consented, with no key - the encryptor is never needed.
	now := time.Now().UTC()
	own, err := domain.NewAiProvider(domain.NewAiProviderInput{
		TenantID: resolverTenant, Kind: domain.AiOpenAiCompatible, BaseURL: "https://own.example/v1",
		CompletionModel: "shared-name", EmbeddingModel: "shared-embed",
		Jurisdiction: domain.AiAdequacy, ProcessingAllowed: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the own model: %v", err)
	}
	writeAiProvider(t, resolverTenant, own, now)

	// An offered model whose embedding model has the same name as the own one.
	offered, err := domain.NewOfferedAiProvider(domain.NewOfferedAiProviderInput{
		ID: resolverModel, DisplayName: "Resolver model", Processor: "A processor",
		Kind: domain.AiOpenAiCompatible, BaseURL: "https://offered.example/v1",
		CompletionModel: "shared-name", EmbeddingModel: "shared-embed", Jurisdiction: domain.AiEEA, Now: now,
	})
	if err != nil {
		t.Fatalf("building the offered model: %v", err)
	}
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		_, err := postgres.NewOfferedAiRepository().Insert(ctx, offered, nil, now)
		return err
	})

	t.Cleanup(func() {
		world.level(context.WithoutCancel(ctx), t, world.before)
		done := context.WithoutCancel(ctx)
		_, _ = admin.Exec(done, `DELETE FROM ai_source WHERE tenant_id = $1`, resolverTenant.String())
		_, _ = admin.Exec(done, `DELETE FROM ai_provider WHERE tenant_id = $1 OR id = $2`,
			resolverTenant.String(), resolverModel.String())
	})
	return world
}

func (w *resolverWorld) level(ctx context.Context, t *testing.T, level identityrepo.InstanceLevel) {
	t.Helper()
	inSystem(ctx, t, w.uow, func(ctx context.Context) error {
		return w.instance.Write(ctx, level, "", time.Now().UTC())
	})
}

func (w *resolverWorld) rule(t *testing.T, sources domain.AiSources, minimum domain.AiMinJurisdiction) {
	t.Helper()
	level := w.before
	level.Ai = domain.AiLayer{Sources: &sources, MinJurisdiction: &minimum, Locks: map[domain.AiSwitch]bool{}}
	w.level(context.Background(), t, level)
}

func (w *resolverWorld) choose(t *testing.T, source domain.AiSourceKind, offered shared.ID) {
	t.Helper()
	choice, err := domain.NewAiChoice(resolverTenant, source, offered, time.Now().UTC())
	if err != nil {
		t.Fatalf("building the choice: %v", err)
	}
	inTenant(t, w.uow, resolverTenant, func(ctx context.Context) error {
		_, err := postgres.NewAiChoiceRepository().Save(ctx, choice)
		return err
	})
}

// answering says which configuration the resolver would call, or the refusal every call answers.
func (w *resolverWorld) answering(t *testing.T) string {
	t.Helper()
	provider, err := w.resolver.For(context.Background(), appshared.ActorContext{Kind: appshared.ActorUser, TenantID: resolverTenant})
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if refused, isRefused := provider.(ai.Refused); isRefused {
		return "refused " + refused.Detail
	}
	if strings.HasPrefix(provider.Capabilities().EmbeddingModel, "offered:") {
		return "offered"
	}
	if provider.Capabilities().Kind == ai.NoopKind {
		return "noop"
	}
	return "own"
}

// One case per `ai.sources` value, each with every choice a workspace can hold.
func TestEachSourcesValueDecidesWhichModelAnswers(t *testing.T) {
	world := newResolverWorld(t)

	for _, testCase := range []struct {
		sources domain.AiSources
		own     string
		offered string
	}{
		{domain.AiSourcesOwn, "own", "refused ai.source_not_allowed"},
		{domain.AiSourcesOffered, "refused ai.source_not_allowed", "offered"},
		{domain.AiSourcesEither, "own", "offered"},
		{domain.AiSourcesNone, "refused ai.source_not_allowed", "refused ai.source_not_allowed"},
	} {
		t.Run(string(testCase.sources), func(t *testing.T) {
			world.rule(t, testCase.sources, domain.AiMinAny)

			world.choose(t, domain.AiSourceOwn, "")
			if got := world.answering(t); got != testCase.own {
				t.Errorf("the own choice answers %q, want %q", got, testCase.own)
			}
			world.choose(t, domain.AiSourceOffered, resolverModel)
			if got := world.answering(t); got != testCase.offered {
				t.Errorf("the offered choice answers %q, want %q", got, testCase.offered)
			}
			world.choose(t, domain.AiSourceNone, "")
			if got := world.answering(t); got != "refused ai.unavailable" {
				t.Errorf("no AI answers %q", got)
			}
		})
	}
}

// UC-AI-06 check 5: narrowing stops the own model and deletes nothing; widening again brings it
// back as it was, key and consent included. And check 4's minimum stops it the same way.
func TestANarrowedRuleKeepsTheOwnConfiguration(t *testing.T) {
	world := newResolverWorld(t)
	world.choose(t, domain.AiSourceOwn, "")

	world.rule(t, domain.AiSourcesOffered, domain.AiMinAny)
	if got := world.answering(t); got != "refused ai.source_not_allowed" {
		t.Fatalf("the narrowed rule answers %q", got)
	}
	inTenant(t, world.uow, resolverTenant, func(ctx context.Context) error {
		kept, err := postgres.NewAiProviderRepository().Find(ctx)
		if err != nil || !kept.ProcessingAllowed || kept.CompletionModel != "shared-name" {
			t.Errorf("the own configuration did not survive the narrowing: %+v (%v)", kept, err)
		}
		return nil
	})

	world.rule(t, domain.AiSourcesEither, domain.AiMinAny)
	if got := world.answering(t); got != "own" {
		t.Errorf("widening again answers %q, want the own model", got)
	}

	world.rule(t, domain.AiSourcesEither, domain.AiMinEEA)
	if got := world.answering(t); got != "refused ai.source_not_allowed" {
		t.Errorf("an own model below the minimum answers %q", got)
	}
}

// UC-AI-05 check 4: the manifest offers the AI features with the offered model exactly as with an
// own one - the resolver is the manifest's.
func TestTheManifestOffersAiWithTheOfferedModel(t *testing.T) {
	ctx := context.Background()
	world := newResolverWorld(t)
	world.rule(t, domain.AiSourcesOffered, domain.AiMinAny)
	world.choose(t, domain.AiSourceOffered, resolverModel)

	capabilities, err := metaservice.GetCapabilities{
		Profiles: postgres.NewCapabilityProfileRepository(), Languages: postgres.NewTextLanguageRepository(),
		Semantic: postgres.NewSemanticSearchRepository(), Providers: world.resolver, UnitOfWork: world.uow,
	}.Execute(ctx, appshared.ActorContext{Kind: appshared.ActorUser, TenantID: resolverTenant})
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	if !capabilities.Features["ai_suggestions"] {
		t.Error("a workspace using the offered model is not offered suggestions")
	}

	world.choose(t, domain.AiSourceNone, "")
	capabilities, err = metaservice.GetCapabilities{
		Profiles: postgres.NewCapabilityProfileRepository(), Languages: postgres.NewTextLanguageRepository(),
		Semantic: postgres.NewSemanticSearchRepository(), Providers: world.resolver, UnitOfWork: world.uow,
	}.Execute(ctx, appshared.ActorContext{Kind: appshared.ActorUser, TenantID: resolverTenant})
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	if capabilities.Features["ai_suggestions"] || capabilities.Features["semantic_search"] {
		t.Errorf("a workspace that chose no AI is offered it: %v", capabilities.Features)
	}
}
