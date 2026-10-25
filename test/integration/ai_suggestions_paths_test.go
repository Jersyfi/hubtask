// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// An AI request about an entry is decided along the entry's own path (UC-AI-04 checks 6 and 8,
// domain-model.md §3.2), against PostgreSQL as the application role: the real authoriser, the real
// memberships and the entry's own read. Before, every request asked the workspace, and a role held
// on a hub alone was refused although the screen offered the question.

type pathsScene struct {
	privateWorld
	entry, assigned                         shared.ID
	hubMember, viewer, contributor, auditor shared.ID
}

func seedPathsScene(ctx context.Context, t *testing.T) pathsScene {
	t.Helper()
	scene := pathsScene{
		privateWorld: seedPrivateWorld(ctx, t),
		entry:        freshID(t), assigned: freshID(t),
		hubMember: freshID(t), viewer: freshID(t), contributor: freshID(t), auditor: freshID(t),
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := itemRepo().Insert(ctx, taskIn(tenantA, authorA, scene.sharedColl, scene.entry, "Tile the bathroom", "a0")); err != nil {
			return err
		}
		_, err := itemRepo().Insert(ctx, taskIn(tenantA, authorA, scene.sharedColl, scene.assigned, "Grout the bathroom", "a1"))
		return err
	}); err != nil {
		t.Fatalf("seeding the entries: %v", err)
	}
	admin := adminPool(ctx, t)
	defer func() {
		// The assignment, once the contributor exists for the reference to name.
		if _, err := admin.Exec(ctx, `UPDATE work_item SET assignee_id = $1 WHERE id = $2`,
			scene.contributor.String(), scene.assigned.String()); err != nil {
			t.Fatalf("assigning: %v", err)
		}
	}()
	for _, person := range []struct {
		id    shared.ID
		name  string
		scope string
		role  string
	}{
		{scene.hubMember, "Hanna Hub", "HUB", "MEMBER"},
		{scene.viewer, "Vera View", "TENANT", "VIEWER"},
		{scene.contributor, "Cora Contrib", "TENANT", "CONTRIBUTOR"},
		{scene.auditor, "Audrey Audit", "TENANT", "AUDITOR"},
	} {
		if _, err := admin.Exec(ctx, `INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, $3)`,
			person.id.String(), tenantA.String(), person.name); err != nil {
			t.Fatalf("seeding %s: %v", person.name, err)
		}
		var scopeID any
		if person.scope == "HUB" {
			scopeID = scene.sharedHub.String()
		}
		if _, err := admin.Exec(ctx, `INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			freshID(t).String(), tenantA.String(), person.id.String(), person.scope, scopeID, person.role); err != nil {
			t.Fatalf("seeding %s's role: %v", person.name, err)
		}
	}
	return scene
}

// pathsRegistry is the asks, the listing and the decisions the way main.go builds them, over the
// entry readers and a queue that keeps what it is handed.
func pathsRegistry(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()
	readers := readerRegistry(ctx, t)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	cases := suggestion.Cases{
		Suggestions: postgres.NewSuggestionRepository(pageCursors()),
		Targets:     suggestion.EntryTargets{Catalogue: readers},
		Authorizer:  realAuthoriser(ctx, t), Catalogue: readers,
		Audit: postgres.NewAuditSink(ids), UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock: clockadapter.System{},
	}
	registry, err := usecase.NewRegistry(nil,
		suggestion.AiSuggestFields{Cases: cases, AI: aiAvailable{}, Queue: &keptQueue{}}.Descriptor(),
		suggestion.ListSuggestions{Cases: cases}.Descriptor(),
		suggestion.DismissSuggestion{Cases: cases}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	return registry
}

func writer(tenant, account shared.ID) appshared.ActorContext {
	return actorIn(tenant, account, "items:read", "items:write", "containers:read")
}

func TestAskingAboutAnEntryIsDecidedAlongItsPath(t *testing.T) {
	ctx := context.Background()
	scene := seedPathsScene(ctx, t)
	registry := pathsRegistry(ctx, t)
	ask := func(actor appshared.ActorContext, item shared.ID) error {
		_, err := registry.Invoke(ctx, suggestion.AiSuggestFieldsName, actor, usecase.Input{"item_id": item.String()})
		return err
	}

	for _, testCase := range []struct {
		name  string
		actor appshared.ActorContext
		item  shared.ID
		want  error
	}{
		{"a member of the entry's hub alone", writer(tenantA, scene.hubMember), scene.entry, nil},
		{"a viewer", writer(tenantA, scene.viewer), scene.entry, shared.ErrForbidden},
		{"a contributor, on their own entry", writer(tenantA, scene.contributor), scene.assigned, nil},
		{"a contributor, on somebody else's", writer(tenantA, scene.contributor), scene.entry, shared.ErrForbidden},
		{"an auditor, who reads no entry", writer(tenantA, scene.auditor), scene.entry, shared.ErrForbidden},
		{"the owner, outside the private hub", writer(tenantA, scene.administrator), scene.item, shared.ErrNotFound},
		{"another workspace", writer(tenantB, authorB), scene.entry, shared.ErrNotFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := ask(testCase.actor, testCase.item)
			if testCase.want == nil && err != nil {
				t.Fatalf("asking was refused: %v", err)
			}
			if testCase.want != nil && !errors.Is(err, testCase.want) {
				t.Fatalf("asking answered %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestAMemberOfOneHubListsAndDecidesItsProposals(t *testing.T) {
	ctx := context.Background()
	scene := seedPathsScene(ctx, t)
	registry := pathsRegistry(ctx, t)

	now := time.Now().UTC()
	recorded, err := domain.New(domain.NewInput{
		ID: freshID(t), TenantID: tenantA, TargetType: domain.TargetWorkItem, TargetID: scene.entry,
		Kind: domain.KindFields, Payload: map[string]any{"title": "Tile the bathroom by Friday"},
		Provenance:  domain.Provenance{Model: "m", PromptID: "suggest-item-fields", PromptVersion: "v3", ProducedAt: now},
		InputDigest: domain.Digest("Tile the bathroom", ""), Now: now,
	})
	if err != nil {
		t.Fatalf("building the proposal: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewSuggestionRepository(pageCursors()).Record(ctx, recorded)
	}); err != nil {
		t.Fatalf("recording the proposal: %v", err)
	}

	out, err := registry.Invoke(ctx, suggestion.ListSuggestionsName, writer(tenantA, scene.hubMember), usecase.Input{
		"target_type": "WORK_ITEM", "target_id": scene.entry.String(),
	})
	if err != nil {
		t.Fatalf("listing as a member of the hub: %v", err)
	}
	if items, _ := out["items"].([]usecase.Output); len(items) != 1 {
		t.Errorf("listed %v", out)
	}

	if _, err := registry.Invoke(ctx, suggestion.DismissSuggestionName, writer(tenantA, scene.viewer),
		usecase.Input{"suggestion_id": recorded.ID.String()}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a viewer dismissing answered %v, want forbidden", err)
	}
	if _, err := registry.Invoke(ctx, suggestion.DismissSuggestionName, writer(tenantA, scene.hubMember),
		usecase.Input{"suggestion_id": recorded.ID.String()}); err != nil {
		t.Errorf("a member of the hub dismissing: %v", err)
	}
}
