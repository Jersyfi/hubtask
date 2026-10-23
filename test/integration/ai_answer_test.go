// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

// A question's material against PostgreSQL as the application role (UC-AI-08 check 3): it is read
// through the ordinary reads as the asker, so an entry in a private hub the asker holds nothing on
// is not in it, and nothing of another workspace is - whatever words the question uses.
func TestAQuestionIsAnsweredOnlyFromEntriesTheAskerMayRead(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)

	// Another workspace holds an entry with the same word, read by its own owner.
	_, otherCollection := hubWithCollection(ctx, t, tenantB, authorB)
	elsewhere := freshID(t)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(tenantB, authorB, otherCollection, elsewhere, w.title, "a0"))
	}); err != nil {
		t.Fatalf("seeding the other workspace's entry: %v", err)
	}
	// The other workspace's owner comes from the suite's shared grants: a grant of its own would
	// collide with theirs whichever test ran first.
	seedMemberships(ctx, t)

	sources := suggestion.CatalogueSources{Catalogue: askReaders{readerRegistry(ctx, t)}}
	ask := func(actor appshared.ActorContext) suggestion.Material {
		t.Helper()
		material, err := sources.Material(ctx, actor, suggestion.Question{
			TargetType: suggestiondomain.TargetAccount, TargetID: actor.AccountID,
			PromptID: "answer-question", Words: "What is left about " + w.title + "?",
		})
		if err != nil {
			t.Fatalf("the material could not be read: %v", err)
		}
		return material
	}
	shown := func(material suggestion.Material, id shared.ID) bool {
		return material.Answer != nil && slices.Contains(material.Answer.Entries, id.String()) ||
			strings.Contains(material.Content, id.String())
	}

	member := ask(reader(w.tenant, w.member))
	if !shown(member, w.item) {
		t.Fatalf("the hub's member was not answered from its entry, so the test proves nothing:\n%s", member.Content)
	}
	if shown(member, elsewhere) {
		t.Error("another workspace's entry is in the member's material")
	}

	owner := ask(reader(w.tenant, w.administrator))
	if shown(owner, w.item) || strings.Contains(owner.Content, w.title) {
		t.Errorf("the workspace's owner, who holds nothing on the private hub, was answered from it:\n%s", owner.Content)
	}

	stranger := ask(reader(tenantB, authorB))
	if !shown(stranger, elsewhere) {
		t.Fatalf("the other workspace's owner was not answered from their own entry:\n%s", stranger.Content)
	}
	if shown(stranger, w.item) {
		t.Error("another workspace's account was answered from this workspace's entry")
	}
}

// askReaders is the readers' catalogue with the directory's reads answered empty: the people and
// labels a question names are the rule draft's reads, proved by their own suites.
type askReaders struct{ registry *usecase.Registry }

func (r askReaders) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	switch name {
	case "ListMemberships", "ListLabels":
		return usecase.Output{"data": []usecase.Output{}}, nil
	case "GetAccount":
		return nil, shared.ErrNotFound
	}
	return r.registry.Invoke(ctx, name, actor, in)
}
