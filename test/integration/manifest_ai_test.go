// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	metaservice "github.com/Jersyfi/hubtask/core/application/service/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/ai"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// QS-09's half of the manifest (J-17, issue 502): what an installation with no AI says about
// itself, asserted against a database that *has* pgvector.
//
// That last clause is the whole reason this test is here and not beside the fakes. The suite's
// database carries the extension, and so does the reference stack in `deploy/docker/compose.yaml`
// - so "the store exists" is true on every installation anybody runs, and a manifest that answered
// only that question would publish `semantic_search: true` to a workspace whose every search is
// lexical. A unit test with a store that answers `false` never meets that installation.
//
// The resolver is the real one for the same reason: "this workspace has no AI" is four different
// rows (nothing configured, NOOP, no consent, an unopenable key) and only the resolver knows all
// four.

var (
	manifestTenantNothing = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb01")
	manifestTenantChat    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb02")
	manifestTenantOff     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb03")
)

func seedManifestTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ('`+manifestTenantNothing.String()+`', 'manifest-none', 'Manifest None'),
		        ('`+manifestTenantChat.String()+`', 'manifest-chat', 'Manifest Chat'),
		        ('`+manifestTenantOff.String()+`', 'manifest-off', 'Manifest Off')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("seeding the manifest tenants: %v", err)
	}
}

// manifestFor reads the manifest as the workspace's own member would, through the real repositories
// and the real resolver.
func manifestFor(ctx context.Context, t *testing.T, tenantID shared.ID) metaservice.Capabilities {
	t.Helper()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	capabilities, err := metaservice.GetCapabilities{
		Profiles:  postgres.NewCapabilityProfileRepository(),
		Languages: postgres.NewTextLanguageRepository(),
		Semantic:  postgres.NewSemanticSearchRepository(),
		Providers: ai.Resolver{
			Providers:  postgres.NewAiProviderRepository(),
			UnitOfWork: uow,
			// No encryptor and no client: a workspace reaching either of them is one whose key
			// this test seals, and none of the three below has a key. A nil here is therefore an
			// assertion as much as a convenience - the manifest must answer without opening
			// anything.
			Clock: portclock.Fixed(time.Now().UTC()),
		},
		UnitOfWork: uow,
	}.Execute(ctx, appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantID})
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	return capabilities
}

func storePresent(ctx context.Context, t *testing.T) bool {
	t.Helper()
	var present bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT to_regclass('public.item_embedding') IS NOT NULL`).Scan(&present); err != nil {
		t.Fatalf("asking for the embedding store: %v", err)
	}
	return present
}

// The claim QS-09 is: a workspace that configured no provider is offered neither feature, on a
// database that could carry the vectors if anybody produced them.
func TestAWorkspaceWithNoProviderIsOfferedNeitherFeature(t *testing.T) {
	ctx := context.Background()
	seedManifestTenants(ctx, t)

	if !storePresent(ctx, t) {
		t.Skip("this database has no pgvector, so the interesting half of the claim is unreachable (ADR-0050)")
	}

	features := manifestFor(ctx, t, manifestTenantNothing).Features

	if features["ai_suggestions"] {
		t.Error("a workspace that configured no provider is told it can ask for a suggestion")
	}
	if features["semantic_search"] {
		t.Error("a database carrying pgvector and no provider claims semantic search")
	}
	for _, key := range []string{"ai_suggestions", "semantic_search"} {
		if _, answered := features[key]; !answered {
			t.Errorf("the manifest does not mention %q at all", key)
		}
	}
}

// A configured provider that completes and does not embed: the ordinary Ollama-with-a-chat-model
// configuration, and the one that separates the two keys. If they moved together, this would fail.
func TestAChatOnlyProviderSeparatesTheTwoKeys(t *testing.T) {
	ctx := context.Background()
	seedManifestTenants(ctx, t)

	if !storePresent(ctx, t) {
		t.Skip("this database has no pgvector, so the interesting half of the claim is unreachable (ADR-0050)")
	}

	now := time.Now().UTC()
	configured, err := domain.NewAiProvider(domain.NewAiProviderInput{
		TenantID: manifestTenantChat, Kind: domain.AiOllama,
		BaseURL: "http://ollama.invalid:11434", CompletionModel: "llama3.1",
		Jurisdiction: domain.AiSelfHosted, ProcessingAllowed: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the configuration: %v", err)
	}
	writeAiProvider(t, manifestTenantChat, configured, now)

	features := manifestFor(ctx, t, manifestTenantChat).Features

	if !features["ai_suggestions"] {
		t.Error("a workspace with a completing provider is told it cannot ask for a suggestion")
	}
	if features["semantic_search"] {
		t.Error("a provider configured with no embedding model claims semantic search")
	}
}

// Consent withheld is the same answer as nothing configured, which is the rule ai-first.md §2 asks
// for before every call - and the manifest has to agree with it, or a client renders a control for
// a route that will refuse.
func TestAWorkspaceThatWithheldConsentIsOfferedNeitherFeature(t *testing.T) {
	ctx := context.Background()
	seedManifestTenants(ctx, t)

	if !storePresent(ctx, t) {
		t.Skip("this database has no pgvector, so the interesting half of the claim is unreachable (ADR-0050)")
	}

	now := time.Now().UTC()
	configured, err := domain.NewAiProvider(domain.NewAiProviderInput{
		TenantID: manifestTenantOff, Kind: domain.AiOllama,
		BaseURL: "http://ollama.invalid:11434", CompletionModel: "llama3.1",
		EmbeddingModel: "nomic-embed-text",
		Jurisdiction:   domain.AiSelfHosted, ProcessingAllowed: false, Now: now,
	})
	if err != nil {
		t.Fatalf("building the configuration: %v", err)
	}
	writeAiProvider(t, manifestTenantOff, configured, now)

	features := manifestFor(ctx, t, manifestTenantOff).Features

	if features["ai_suggestions"] || features["semantic_search"] {
		t.Errorf("a workspace that withheld consent is offered AI: %+v", features)
	}
}

// writeAiProvider stores one workspace's configuration. Its own context rather than the caller's,
// because inTenant opens the transaction with one of its own and a parameter that never reaches
// the transaction is a parameter that lies about where the deadline goes.
func writeAiProvider(t *testing.T, tenantID shared.ID, configured domain.AiProvider, now time.Time) {
	t.Helper()
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	inTenant(t, uow, tenantID, func(ctx context.Context) error {
		_, err := postgres.NewAiProviderRepository().Upsert(ctx, configured, nil, now)
		return err
	})
}
