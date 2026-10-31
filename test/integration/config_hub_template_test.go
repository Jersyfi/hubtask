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
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A hub template (migration 0271, configuration.md §6): a template version keeps the part a hub takes
// beside the workspace's, and the part a hub took is recorded for that hub - not for the
// workspace, not for another hub, and the workspace next door reads none of it.
func TestAHubsTemplateStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewConfigRepository()
	hub, _ := seedHubWithCollection(ctx, t, tenantA, authorA)
	other, _ := seedHubWithCollection(ctx, t, tenantA, authorA)
	now := time.Now().UTC().Truncate(time.Microsecond)
	key := "hub_desk_" + time.Now().Format("150405000000")
	part := domain.Document{"scheme": domain.Section{"incident": domain.Block{"workflow": "incident"}}}
	template := domain.Template{
		ID: freshID(t), TenantID: tenantA, Key: key, Version: 1, Name: "Hub desk", Origin: domain.OriginWorkspace,
		Document: domain.Document{}, Hub: part, CreatedBy: authorA, CreatedAt: now,
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.InsertTemplate(ctx, template); err != nil {
			return err
		}
		return store.PutTemplateUse(ctx, domain.TemplateUse{HubID: hub.ID, TemplateKey: key, Version: 1, Document: part, At: now})
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := store.FindTemplate(ctx, key, 1)
		if err != nil {
			return err
		}
		if !found.ForHubs() || found.Hub["scheme"]["incident"]["workflow"] != "incident" {
			t.Errorf("the hub part read back %v", found.Hub)
		}
		uses, err := store.TemplateUses(ctx, hub.ID)
		if err != nil {
			return err
		}
		if len(uses) != 1 || uses[0].HubID != hub.ID || uses[0].Version != 1 || uses[0].Document["scheme"] == nil {
			t.Errorf("the hub's use read back %+v", uses)
		}
		for _, elsewhere := range []shared.ID{"", other.ID} {
			uses, err := store.TemplateUses(ctx, elsewhere)
			if err != nil {
				return err
			}
			for _, use := range uses {
				if use.TemplateKey == key {
					t.Errorf("the hub's use is read for %q: %+v", elsewhere, use)
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if uses, err := store.TemplateUses(ctx, hub.ID); err != nil || len(uses) != 0 {
			t.Errorf("B reads A's hub's template: %v %v", uses, err)
		}
		if _, err := store.FindTemplate(ctx, key, 1); err == nil {
			t.Error("B finds A's hub template")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A collection template (migration 0310, configuration.md §6): a template version keeps the part a
// collection takes, the collection's use is recorded under the collection - not its hub - and a
// proposal keeps the collection it would record it for; the workspace next door reads none of it.
func TestACollectionsTemplateStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewConfigRepository()
	hub, collection := seedHubWithCollection(ctx, t, tenantA, authorA)
	now := time.Now().UTC().Truncate(time.Microsecond)
	key := "queue_kit_" + time.Now().Format("150405000000")
	part := domain.Document{"collection_views": domain.Section{"board": domain.Block{"name": "Board", "layout": "LIST_EXPANDED"}}}
	template := domain.Template{
		ID: freshID(t), TenantID: tenantA, Key: key, Version: 1, Name: "Queue kit", Origin: domain.OriginWorkspace,
		Document: domain.Document{}, Collection: part, CreatedBy: authorA, CreatedAt: now,
	}
	proposal := domain.Proposal{
		ID: freshID(t), TenantID: tenantA, State: domain.ProposalWaiting, Document: part, Mode: domain.ModeMerge,
		BaseHash: "base", PlanHash: "plan", Template: template.Provenance(), ProposedBy: authorA, CreatedAt: now,
		HubID: hub.ID, CollectionID: collection.ID,
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.InsertTemplate(ctx, template); err != nil {
			return err
		}
		if err := store.InsertProposal(ctx, proposal); err != nil {
			return err
		}
		return store.PutTemplateUse(ctx, domain.TemplateUse{HubID: collection.ID, TemplateKey: key, Version: 1, Document: part, At: now})
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := store.FindTemplate(ctx, key, 1)
		if err != nil {
			return err
		}
		if !found.ForCollections() || found.ForHubs() || found.Collection["collection_views"]["board"]["name"] != "Board" {
			t.Errorf("the collection part read back %v, hub part %v", found.Collection, found.Hub)
		}
		kept, err := store.FindProposal(ctx, proposal.ID)
		if err != nil {
			return err
		}
		if kept.CollectionID != collection.ID || kept.HubID != hub.ID {
			t.Errorf("the proposal read back for %q in %q", kept.CollectionID, kept.HubID)
		}
		if uses, err := store.TemplateUses(ctx, collection.ID); err != nil || len(uses) != 1 || uses[0].HubID != collection.ID {
			t.Errorf("the collection's use read back %+v %v", uses, err)
		}
		if uses, err := store.TemplateUses(ctx, hub.ID); err != nil || len(uses) != 0 {
			t.Errorf("the collection's use is read for its hub: %+v %v", uses, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if uses, err := store.TemplateUses(ctx, collection.ID); err != nil || len(uses) != 0 {
			t.Errorf("B reads A's collection's template: %v %v", uses, err)
		}
		if _, err := store.FindProposal(ctx, proposal.ID); err == nil {
			t.Error("B finds A's proposal for a collection")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
