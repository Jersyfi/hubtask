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
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A question about a collection - how it stands - is decided along the collection's own path, so
// a member of its hub alone may ask it and answer what it proposed, as the screen offers; a viewer
// may not, and an outsider of a private hub learns nothing of the collection.
func TestAskingAboutACollectionIsDecidedAlongItsPath(t *testing.T) {
	ctx := context.Background()
	scene := seedPathsScene(ctx, t)
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
		suggestion.AiSummarizeContainer{Cases: cases, AI: aiAvailable{}, Queue: &keptQueue{}}.Descriptor(),
		suggestion.DismissSuggestion{Cases: cases}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	ask := func(account shared.ID, collection shared.ID) error {
		_, err := registry.Invoke(ctx, suggestion.AiSummarizeContainerName, writer(tenantA, account),
			usecase.Input{"container_id": collection.String()})
		return err
	}

	if err := ask(scene.hubMember, scene.sharedColl); err != nil {
		t.Errorf("a member of the collection's hub alone asking: %v", err)
	}
	if err := ask(scene.viewer, scene.sharedColl); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a viewer asking answered %v, want forbidden", err)
	}
	if err := ask(scene.administrator, scene.collection); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the owner outside the private hub answered %v, want not found", err)
	}

	now := time.Now().UTC()
	recorded, err := domain.New(domain.NewInput{
		ID: freshID(t), TenantID: tenantA, TargetType: domain.TargetContainer, TargetID: scene.sharedColl,
		Kind: domain.KindFields, Payload: map[string]any{"notes": "Half of it is done."},
		Provenance:  domain.Provenance{Model: "m", PromptID: "summarize-collection", PromptVersion: "v1", ProducedAt: now},
		InputDigest: domain.Digest("x", ""), Now: now,
	})
	if err != nil {
		t.Fatalf("building the proposal: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return cases.Suggestions.Record(ctx, recorded)
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}
	// Answered with the dismissal - the one answer a collection's summary has - by a member of its
	// hub alone. Stale or not is the read's question, after the permission's.
	_, err = registry.Invoke(ctx, suggestion.DismissSuggestionName, writer(tenantA, scene.hubMember),
		usecase.Input{"suggestion_id": recorded.ID.String()})
	if errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a member of the hub was refused the dismissal: %v", err)
	}
}
