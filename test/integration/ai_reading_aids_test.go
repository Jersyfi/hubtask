// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The reading aids are offered to everybody who may read the entry (UC-AI-04 check 8) - the
// milestone's "reader test for translation" - against PostgreSQL as the application role. A
// viewer of the workspace, a viewer of one hub and a guest on one shared entry each pass the
// permission of a translation and of a summary of the discussion; the summary applied to the notes
// still asks what changing the entry asks; and the two scopes a token may carry for the summary
// both work, so no token minted before stops working.
func TestEveryReaderMayAskTheReadingAids(t *testing.T) {
	ctx := context.Background()
	scene := seedPathsScene(ctx, t)
	hubViewer, guest := freshID(t), freshID(t)
	admin := adminPool(ctx, t)
	for _, grant := range []struct {
		id      shared.ID
		name    string
		scope   string
		scopeID shared.ID
		role    string
	}{
		{hubViewer, "Hugo Hubview", "HUB", scene.sharedHub, "VIEWER"},
		{guest, "Gwen Guest", "ITEM", scene.entry, "GUEST"},
	} {
		if _, err := admin.Exec(ctx, `INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, $3)`,
			grant.id.String(), tenantA.String(), grant.name); err != nil {
			t.Fatalf("seeding %s: %v", grant.name, err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
			VALUES ($1, $2, $3, $4, $5, $6)`, freshID(t).String(), tenantA.String(), grant.id.String(),
			grant.scope, grant.scopeID.String(), grant.role); err != nil {
			t.Fatalf("seeding %s's role: %v", grant.name, err)
		}
	}

	readers := readerRegistry(ctx, t)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	cases := suggestion.Cases{
		Suggestions: postgres.NewSuggestionRepository(pageCursors()),
		Targets:     suggestion.EntryTargets{Catalogue: readers},
		Authorizer:  realAuthoriser(ctx, t), Catalogue: readers,
		Audit: postgres.NewAuditSink(ids), UnitOfWork: unitOfWork, Clock: clockadapter.System{},
	}
	queued := &keptQueue{}
	registry, err := usecase.NewRegistry(nil,
		suggestion.AiSummarizeThread{Cases: cases, AI: aiAvailable{}, Queue: queued}.Descriptor(),
		// No provider: a translation that passes its permission answers that AI is out of reach,
		// which is the proof it was not refused the read.
		work.AiTranslate{
			Reader: work.GetWorkItem{
				Items: itemRepo(), ItemLabels: postgres.NewItemLabelRepository(), Containers: containerRepo(),
				Authorizer: realAuthoriser(ctx, t), UnitOfWork: unitOfWork,
			},
			Audit: postgres.NewAuditSink(ids), UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	asReader := func(account shared.ID, scopes ...string) appshared.ActorContext {
		return actorIn(tenantA, account, scopes...)
	}

	for _, reader := range []struct {
		name    string
		account shared.ID
	}{
		{"a viewer of the workspace", scene.viewer},
		{"a viewer of one hub", hubViewer},
		{"a guest on the shared entry", guest},
	} {
		t.Run(reader.name, func(t *testing.T) {
			actor := asReader(reader.account, "items:read")
			if _, err := registry.Invoke(ctx, suggestion.AiSummarizeThreadName, actor,
				usecase.Input{"item_id": scene.entry.String()}); err != nil {
				t.Errorf("asking for the discussion's summary: %v", err)
			}
			_, err := registry.Invoke(ctx, work.AiTranslateName, actor,
				usecase.Input{"item_id": scene.entry.String(), "target_locale": "de"})
			if got := shared.AsError(err); got == nil || got.DetailCode != "ai.unavailable" {
				t.Errorf("translating answered %v, want past the permission to the missing provider", err)
			}
		})
	}

	t.Run("applied to the notes, the summary asks what changing the entry asks", func(t *testing.T) {
		_, err := registry.Invoke(ctx, suggestion.AiSummarizeThreadName, asReader(scene.viewer, "items:read", "items:write"),
			usecase.Input{"item_id": scene.entry.String(), "apply": true})
		if !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("a viewer applying answered %v, want forbidden", err)
		}
	})

	// A token that reads entries asks it now, and one minted for asking - which also reads the
	// entry, since the entry's own read asks for it - asks it as before.
	t.Run("a reading token and an asking token both ask the summary", func(t *testing.T) {
		for _, scopes := range [][]string{{"items:read"}, {"items:read", "items:write"}} {
			if _, err := registry.Invoke(ctx, suggestion.AiSummarizeThreadName, asReader(scene.hubMember, scopes...),
				usecase.Input{"item_id": scene.entry.String()}); err != nil {
				t.Errorf("a token with %v: %v", scopes, err)
			}
		}
	})
}
