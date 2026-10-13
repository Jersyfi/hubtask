// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	metaservice "github.com/Jersyfi/hubtask/core/application/service/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portai "github.com/Jersyfi/hubtask/core/port/ai"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/ai"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/instancefile"
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

// UC-AI-05 check 2: a workspace that never chose a source has no ai_source row, and under every
// rule that allows the offered model the resolver answers no model at all - nothing is sent before
// the workspace switches it on.
func TestAWorkspaceThatNeverChoseCallsNoModel(t *testing.T) {
	ctx := context.Background()
	world := newResolverWorld(t)
	admin := adminPool(ctx, t)
	// Neither a choice nor an own model: only what the installation offers.
	for _, statement := range []string{
		`DELETE FROM ai_source WHERE tenant_id = $1`,
		`DELETE FROM ai_provider WHERE tenant_id = $1`,
	} {
		if _, err := admin.Exec(ctx, statement, resolverTenant.String()); err != nil {
			t.Fatalf("clearing the workspace: %v", err)
		}
	}

	for _, sources := range []domain.AiSources{domain.AiSourcesOffered, domain.AiSourcesEither} {
		t.Run(string(sources), func(t *testing.T) {
			//nolint:contextcheck // the helper opens a transaction of its own, as every test of this package does
			world.rule(t, sources, domain.AiMinAny)
			provider, source, err := world.resolver.Sourced(ctx,
				appshared.ActorContext{Kind: appshared.ActorUser, TenantID: resolverTenant})
			if err != nil {
				t.Fatalf("resolving: %v", err)
			}
			if _, refused := provider.(ai.Refused); !refused || source != portai.SourceNone {
				t.Errorf("a workspace that never chose is answered by %T (source %q)", provider, source)
			}
		})
	}

	var rows int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM ai_source WHERE tenant_id = $1`,
		resolverTenant.String()).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("resolving wrote a choice: %d rows (%v)", rows, err)
	}
}

// UC-AI-06 check 7: a model the instance file offers - by `seed`, then by `enforce` - switches
// nobody's AI on, even where the file tries to: a consent key in its `ai` area and on the model
// are read as nothing. The workspace stays without a model until it chooses one itself.
func TestTheInstanceFileOffersAModelAndSwitchesNobodysAiOn(t *testing.T) {
	ctx := context.Background()
	world := newResolverWorld(t)
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`DELETE FROM ai_source WHERE tenant_id = $1`,
		`DELETE FROM ai_provider WHERE tenant_id = $1`,
	} {
		if _, err := admin.Exec(ctx, statement, resolverTenant.String()); err != nil {
			t.Fatalf("clearing the workspace: %v", err)
		}
	}
	// Seed writes only where nothing is offered.
	if _, err := admin.Exec(ctx, `DELETE FROM ai_provider WHERE tenant_id IS NULL`); err != nil {
		t.Fatalf("clearing the offered models: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.WithoutCancel(ctx), `DELETE FROM ai_provider WHERE tenant_id IS NULL`)
	})
	var choicesBefore int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM ai_source`).Scan(&choicesBefore); err != nil {
		t.Fatalf("counting the choices: %v", err)
	}

	instance := adminservice.InstanceWriter{
		Settings: postgres.NewInstanceSettingRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
		UnitOfWork: world.uow, Clock: clockadapter.System{}, IDs: clockadapter.NewUUIDv7(clockadapter.System{}),
	}
	writer := adminservice.InstanceAiWriter{Instance: instance, Offered: postgres.NewOfferedAiRepository()}
	apply := func(t *testing.T, name string, enforce bool) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "instance.json")
		document := `{
		  "ai": {"sources": {"value": "OFFERED", "locked": true},
		         "consent": {"value": "ON"}, "processing_allowed": {"value": "true"}},
		  "ai_models": [{"display_name": "` + name + `", "processor": "A processor",
		    "kind": "OPENAI_COMPATIBLE", "base_url": "https://offered.example/v1",
		    "completion_model": "large", "embedding_model": "embed", "jurisdiction": "EEA",
		    "processing_allowed": true, "consent": true}]
		}`
		if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
			t.Fatalf("writing the file: %v", err)
		}
		read, _, err := instancefile.Read(path)
		if err != nil {
			t.Fatalf("reading the file: %v", err)
		}
		models, _, err := instancefile.AiModels(read, func(string) (string, bool) { return "", false }, path)
		if err != nil {
			t.Fatalf("reading the models: %v", err)
		}
		commands := make([]adminservice.ConfigureInstanceAiCommand, 0, len(models))
		for _, model := range models {
			commands = append(commands, adminservice.ConfigureInstanceAiCommand{
				DisplayName: model.DisplayName, Processor: model.Processor,
				Kind: domain.AiProviderKind(model.Kind), BaseURL: model.BaseURL,
				CompletionModel: model.CompletionModel, EmbeddingModel: model.EmbeddingModel,
				Jurisdiction: domain.AiJurisdiction(model.Jurisdiction),
			})
		}
		if err := writer.ApplyFile(ctx, commands, enforce, path); err != nil {
			t.Fatalf("applying the models: %v", err)
		}
		level, err := adminservice.InstanceLevelOf(read)
		if err != nil {
			t.Fatalf("reading the level: %v", err)
		}
		if _, err := (adminservice.WriteInstanceSettings{Writer: instance}).ExecuteAsFile(ctx, level, path); err != nil {
			t.Fatalf("writing the level: %v", err)
		}
	}

	var offeredID shared.ID
	for _, step := range []struct {
		mode    string
		name    string
		enforce bool
	}{{"seed", "File model (seed)", false}, {"enforce", "File model (enforce)", true}} {
		t.Run(step.mode, func(t *testing.T) {
			apply(t, step.name, step.enforce)

			inSystem(ctx, t, world.uow, func(ctx context.Context) error {
				offered, err := postgres.NewOfferedAiRepository().List(ctx)
				if err != nil {
					return err
				}
				if len(offered) != 1 || offered[0].DisplayName != step.name {
					t.Fatalf("the file offered %+v", offered)
				}
				if offered[0].ProcessingAllowed {
					t.Error("the offered model was stored as consented")
				}
				offeredID = offered[0].ID
				return nil
			})
			//nolint:contextcheck // the helper opens a transaction of its own, as every test of this package does
			if got := world.answering(t); !strings.HasPrefix(got, "refused") {
				t.Errorf("after the file a workspace that never chose answers %q", got)
			}
			var choices int
			if err := admin.QueryRow(ctx, `SELECT count(*) FROM ai_source`).Scan(&choices); err != nil || choices != choicesBefore {
				t.Errorf("the file wrote a choice: %d rows, %d before (%v)", choices, choicesBefore, err)
			}
		})
	}

	// The model the file offered is usable: the workspace's own choice is what switches it on.
	world.choose(t, domain.AiSourceOffered, offeredID)
	if got := world.answering(t); got != "offered" {
		t.Errorf("the workspace's own choice answers %q", got)
	}
}
