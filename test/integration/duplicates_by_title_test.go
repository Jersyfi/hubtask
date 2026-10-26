// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/mcp"
)

// Duplicates of a title before it is an entry (UC-AI-04 check 7), against PostgreSQL as the
// application role: the real neighbourhood statement, the real authoriser asked along each
// candidate's own path, and the real entries. Only the embedding of the typed words is stood in
// for - a provider is somebody else's machine - with a vector on the fixture's synthetic axis.

// typedMeaning embeds any words as one direction, which is what makes the test's question exact.
type typedMeaning struct{ vector []float32 }

func (m typedMeaning) Of(context.Context, appshared.ActorContext, string) ([]float32, string, error) {
	return m.vector, "test-embed-1", nil
}

type lookalikeScene struct {
	privateWorld
	axis int
	// near is an entry in the shared hub that means the same as the private one.
	near shared.ID
	// hubOnly holds MEMBER on the shared hub and no role in the workspace.
	hubOnly shared.ID
}

func seedLookalikes(ctx context.Context, t *testing.T) lookalikeScene {
	t.Helper()
	requirePgvector(ctx, t)
	scene := lookalikeScene{privateWorld: seedPrivateWorld(ctx, t), axis: freshAxis()}
	scene.near, scene.hubOnly = freshID(t), freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := itemRepo().Insert(ctx, taskIn(tenantA, authorA, scene.sharedColl, scene.near,
			"Call the tiler about Thursday", "a0")); err != nil {
			return err
		}
		embeddings := postgres.NewEmbeddingRepository()
		for _, entry := range []struct {
			id     shared.ID
			title  string
			vector []float32
		}{
			{scene.item, scene.title, meaningOf(scene.axis)},
			{scene.near, "Call the tiler about Thursday", meaningNear(scene.axis, 0.97)},
		} {
			if err := embeddings.Store(ctx, repository.StoredEmbedding{
				ItemID: entry.id, Model: "test-embed-1", Vector: entry.vector,
				SourceDigest: suggestiondomain.Digest(entry.title, ""), UpdatedAt: time.Now().UTC(),
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the lookalikes: %v", err)
	}

	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Hanna Hub')`,
			[]any{scene.hubOnly.String(), tenantA.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		  VALUES ($1, $2, $3, 'HUB', $4, 'MEMBER')`,
			[]any{freshID(t).String(), tenantA.String(), scene.hubOnly.String(), scene.sharedHub.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the hub's member: %v", err)
		}
	}
	return scene
}

func lookalikeRegistry(ctx context.Context, t *testing.T, axis int) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, suggestion.FindDuplicatesByTitle{
		Meaning:    typedMeaning{vector: meaningOf(axis)},
		Neighbours: postgres.NewEmbeddingRepository(),
		Reader:     realAuthoriser(ctx, t),
		Items:      itemRepo(),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
	}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return registry
}

func lookalikesFor(ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext) map[string]string {
	t.Helper()
	out, err := registry.Invoke(ctx, suggestion.FindDuplicatesByTitleName, actor, usecase.Input{"title": "Book the tiler"})
	if err != nil {
		t.Fatalf("asking as %s: %v", actor.AccountID, err)
	}
	found := map[string]string{}
	items, _ := out["items"].([]usecase.Output)
	for _, item := range items {
		found[item.String("item_id")] = item.String("title")
	}
	return found
}

// Who finds what: each lookalike is asked about along its own path, and no question is asked at
// the workspace.
func TestALookalikeIsFoundByWhoeverMayReadIt(t *testing.T) {
	ctx := context.Background()
	scene := seedLookalikes(ctx, t)
	registry := lookalikeRegistry(ctx, t, scene.axis)

	for _, testCase := range []struct {
		name    string
		actor   appshared.ActorContext
		want    []shared.ID
		notWant []shared.ID
	}{
		{"the private hub's member finds both", reader(tenantA, scene.member),
			[]shared.ID{scene.item, scene.near}, nil},
		// A workspace owner outside the private hub: the shared hub's entry, never the private one.
		{"the outsider of the private hub", reader(tenantA, scene.administrator),
			[]shared.ID{scene.near}, []shared.ID{scene.item}},
		// A role on one hub and none in the workspace: the hub's own entry.
		{"a member of one hub only", reader(tenantA, scene.hubOnly),
			[]shared.ID{scene.near}, []shared.ID{scene.item}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			found := lookalikesFor(ctx, t, registry, testCase.actor)
			for _, id := range testCase.want {
				if _, held := found[id.String()]; !held {
					t.Errorf("%s was not found: %v", id, found)
				}
			}
			for _, id := range testCase.notWant {
				if _, held := found[id.String()]; held {
					t.Errorf("%s was answered to somebody who may not read it", id)
				}
			}
			if title, held := found[scene.near.String()]; held && title != "Call the tiler about Thursday" {
				t.Errorf("the lookalike is named %q", title)
			}
		})
	}
}

// SG-3: the same words asked from the workspace next door find nothing of A's.
func TestALookalikeIsNeverAnsweredNextDoor(t *testing.T) {
	ctx := context.Background()
	scene := seedLookalikes(ctx, t)
	registry := lookalikeRegistry(ctx, t, scene.axis)

	found := lookalikesFor(ctx, t, registry, reader(tenantB, authorB))
	if len(found) != 0 {
		t.Errorf("B found %v", found)
	}
}

// The same read through the MCP server: an agent asks with a token that reads entries, and is
// answered what its person may read.
func TestLookalikesAreAnsweredOverMCP(t *testing.T) {
	ctx := context.Background()
	scene := seedLookalikes(ctx, t)
	registry := lookalikeRegistry(ctx, t, scene.axis)

	encoded, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "find_duplicates_by_title", "arguments": map[string]any{"title": "Book the tiler"}},
	})
	if err != nil {
		t.Fatalf("encoding the call: %v", err)
	}
	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, reader(tenantA, scene.hubOnly)),
		http.MethodPost, mcp.Path, strings.NewReader(string(encoded)))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	var answer struct {
		Result struct {
			IsError    bool           `json:"isError"`
			Structured map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer.Result.IsError {
		t.Fatalf("the call failed: %v %s", err, recorder.Body)
	}
	items, _ := answer.Result.Structured["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["item_id"] != scene.near.String() {
		t.Errorf("the agent was answered %v, want the shared hub's entry alone", answer.Result.Structured)
	}
}
