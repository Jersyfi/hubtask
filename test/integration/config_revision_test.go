// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The configuration's history against the real boundary (migration 0221, F-6, gate SG-3): a
// workspace's versions, its agents' proposals, its templates and the template it took are its
// own - the workspace next door neither reads nor decides any of it - and a version is written
// once.
func TestAWorkspacesConfigurationHistoryIsItsOwn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	for _, statement := range []string{
		`DELETE FROM config_revision WHERE tenant_id IN ('` + tenantA.String() + `', '` + tenantB.String() + `')`,
		`DELETE FROM config_proposal WHERE tenant_id IN ('` + tenantA.String() + `', '` + tenantB.String() + `')`,
		`DELETE FROM config_template WHERE tenant_id IN ('` + tenantA.String() + `', '` + tenantB.String() + `')`,
		`DELETE FROM config_template_use WHERE tenant_id IN ('` + tenantA.String() + `', '` + tenantB.String() + `')`,
	} {
		if _, err := adminPool(ctx, t).Exec(ctx, statement); err != nil {
			t.Fatalf("clearing an earlier run: %v", err)
		}
	}

	store := postgres.NewConfigRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Microsecond)
	document := domain.Document{"types": domain.Section{"incident": domain.Block{"level": "TASK", "name": "Incident"}}}
	revision := domain.Revision{
		TenantID: tenantA, Version: 1, Document: document, Hash: domain.Hash(document), PlanHash: "plan",
		ActorKind: shared.ActorUser, ActorID: authorA, Counts: domain.Counts{Added: 1}, At: now,
	}
	proposal := domain.Proposal{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc01"), TenantID: tenantA, State: domain.ProposalWaiting,
		Document: document, Mode: domain.ModeMerge, BaseHash: "base", PlanHash: "plan", ProposedBy: authorA, CreatedAt: now,
	}
	template := domain.Template{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc02"), TenantID: tenantA, Key: "service_desk",
		Version: 1, Name: "Service desk", Origin: domain.OriginWorkspace, Document: document, CreatedBy: authorA, CreatedAt: now,
	}

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if err := store.Insert(ctx, revision); err != nil {
			t.Fatalf("writing A's first version: %v", err)
		}
		if err := store.InsertProposal(ctx, proposal); err != nil {
			t.Fatalf("keeping A's proposal: %v", err)
		}
		if err := store.InsertTemplate(ctx, template); err != nil {
			t.Fatalf("publishing A's template: %v", err)
		}
		if err := store.PutTemplateUse(ctx, domain.TemplateUse{TemplateKey: "service_desk", Version: 1, Document: document, At: now}); err != nil {
			t.Fatalf("recording the template A took: %v", err)
		}
		return nil
	})
	// A refused write ends its transaction, so each is tried in one of its own.
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return store.Insert(ctx, revision)
	}); err == nil {
		t.Fatal("a version was written twice")
	}
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return store.InsertTemplate(ctx, template)
	}); err == nil {
		t.Fatal("a template version was written twice")
	}

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if _, held, err := store.Latest(ctx); err != nil || held {
			t.Fatalf("B reads A's latest version: %v %v", held, err)
		}
		if _, err := store.Find(ctx, 1); err == nil {
			t.Fatal("B finds A's version")
		}
		if versions, err := store.List(ctx, "", 0, 50); err != nil || len(versions) != 0 {
			t.Fatalf("B lists A's versions: %v %v", versions, err)
		}
		if _, err := store.FindProposal(ctx, proposal.ID); err == nil {
			t.Fatal("B finds A's proposal")
		}
		if proposals, err := store.ListProposals(ctx, "", 50); err != nil || len(proposals) != 0 {
			t.Fatalf("B lists A's proposals: %v %v", proposals, err)
		}
		if decided, err := store.DecideProposal(ctx, proposal.ID, domain.ProposalApplied, authorA, now); err != nil || decided {
			t.Fatalf("B decides A's proposal: %v %v", decided, err)
		}
		if templates, err := store.ListTemplates(ctx); err != nil || len(templates) != 0 {
			t.Fatalf("B lists A's templates: %v %v", templates, err)
		}
		if _, err := store.FindTemplate(ctx, "service_desk", 1); err == nil {
			t.Fatal("B finds A's template")
		}
		if uses, err := store.TemplateUses(ctx); err != nil || len(uses) != 0 {
			t.Fatalf("B reads the template A took: %v %v", uses, err)
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		latest, held, err := store.Latest(ctx)
		if err != nil || !held || latest.Version != 1 || latest.Hash != revision.Hash {
			t.Fatalf("A's latest version: %+v %v %v", latest, held, err)
		}
		found, err := store.FindProposal(ctx, proposal.ID)
		if err != nil || found.State != domain.ProposalWaiting {
			t.Fatalf("A's proposal was moved from B: %+v %v", found, err)
		}
		if decided, err := store.DecideProposal(ctx, proposal.ID, domain.ProposalDeclined, authorA, now); err != nil || !decided {
			t.Fatalf("A declines its proposal: %v %v", decided, err)
		}
		if again, err := store.DecideProposal(ctx, proposal.ID, domain.ProposalApplied, authorA, now); err != nil || again {
			t.Fatalf("a decided proposal was decided again: %v %v", again, err)
		}
		uses, err := store.TemplateUses(ctx)
		if err != nil || len(uses) != 1 || uses[0].Version != 1 {
			t.Fatalf("the template A took: %+v %v", uses, err)
		}
		return nil
	})

	// A hub's version (migration 0223) takes the workspace's next number, names its hub, and is
	// listed in the hub's history only - never in the workspace's, never next door.
	// A bare identifier, as the actors are: no hub row is needed.
	hubA := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc03")
	hubVersion := revision
	hubVersion.Version, hubVersion.HubID = 2, hubA
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if err := store.Insert(ctx, hubVersion); err != nil {
			t.Fatalf("writing the hub's version: %v", err)
		}
		found, err := store.Find(ctx, 2)
		if err != nil || found.HubID != hubA {
			t.Fatalf("the hub's version does not name its hub: %+v %v", found, err)
		}
		workspace, err := store.List(ctx, "", 0, 50)
		if err != nil || len(workspace) != 1 || workspace[0].Version != 1 {
			t.Fatalf("the workspace's history: %+v %v", workspace, err)
		}
		hub, err := store.List(ctx, hubA, 0, 50)
		if err != nil || len(hub) != 1 || hub[0].Version != 2 {
			t.Fatalf("the hub's history: %+v %v", hub, err)
		}
		return nil
	})
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if versions, err := store.List(ctx, hubA, 0, 50); err != nil || len(versions) != 0 {
			t.Fatalf("B lists A's hub's versions: %v %v", versions, err)
		}
		return nil
	})
}
