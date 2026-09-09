// SPDX-License-Identifier: BUSL-1.1
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

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/mcp"
)

// J-11's acceptance, against a real database: an MCP resource is a catalogue read, so it is bounded
// by the same permission, narrowed by the same row level security, and audited by the same
// application layer as the identical read through any other door.
//
// Everything below the adapters is the production wiring - only the clock is fixed.

func readCatalogueFor(t *testing.T) *usecase.Registry {
	t.Helper()
	ctx := context.Background()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  unitOfWork,
		Audit:       postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed)),
		Clock:       fixed,
	}
	containers, items := containerRepo(), itemRepo()

	registry, err := usecase.NewRegistry(nil,
		work.GetContainer{
			Containers: containers, Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.ListContainers{
			Containers: containers, Authorizer: authorizer, Reader: authorizer,
			UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.GetWorkItem{
			Items: items, ItemLabels: postgres.NewItemLabelRepository(), Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.ListSavedViews{
			Views: postgres.NewSavedViewRepository(), Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.GetSavedView{
			Views: postgres.NewSavedViewRepository(), Containers: containers,
			Permits: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// agentFor is the actor an MCP call arrives as - which is to say a *person's*, with the scopes an
// agent token carries for reading.
//
// The kind is deliberately not set here (J-14). What makes a call an agent's is the door it came
// through, and the adapter stamps it: a test that stamped it itself would pass on the day the
// adapter stopped, which is exactly the state this suite was in before J-14.
func agentFor(tenant, account shared.ID) appshared.ActorContext {
	actor := administrator(tenant, account)
	actor.Scopes = []string{"containers:read", "items:read", "views:read"}
	return actor
}

// overMCP sends one JSON-RPC call as the given actor and answers the decoded response.
func overMCP(
	ctx context.Context, t *testing.T, registry *usecase.Registry,
	actor appshared.ActorContext, body string,
) map[string]any {
	t.Helper()

	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor),
		http.MethodPost, mcp.Path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("the MCP channel answered %d: %s", recorder.Code, recorder.Body)
	}
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the MCP answer is not JSON: %v", err)
	}
	return answer
}

func readResource(
	ctx context.Context, t *testing.T, registry *usecase.Registry,
	actor appshared.ActorContext, uri string,
) map[string]any {
	t.Helper()
	return overMCP(ctx, t, registry, actor,
		`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"`+uri+`"}}`)
}

// The same read, twice: once addressed by URI and once called as a tool. Both go through the
// catalogue, so both answer the same projection - which is what "a resource is a read that is
// already in the catalogue" means when it is asked of a running system rather than of a comment.
func TestAResourceAndItsToolAnswerTheSameRead(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := readCatalogueFor(t)
	agent := agentFor(tenantA, authorA)

	hub, _ := hubWithCollection(ctx, t, tenantA, authorA)

	viaResource := readResource(ctx, t, registry, agent, "hubtask://containers/"+hub.String())
	contents := contentOf(t, viaResource)

	viaTool := overMCP(ctx, t, registry, agent,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_container",`+
			`"arguments":{"container_id":"`+hub.String()+`"}}}`)
	result, _ := viaTool["result"].(map[string]any)
	if isError, _ := result["isError"].(bool); isError {
		t.Fatalf("the tool call failed: %v", result)
	}

	wanted, err := json.Marshal(result["structuredContent"])
	if err != nil {
		t.Fatalf("re-encoding the tool result: %v", err)
	}
	if !equalJSON(t, contents, string(wanted)) {
		t.Errorf("the two doors answered differently:\n  resource: %s\n  tool:     %s",
			contents, wanted)
	}
}

// The tenant boundary, on the new door. A URI is a global-looking address, which is exactly why it
// is the shape somebody would try: naming another workspace's identifier must be a not-found, never
// a forbidden that confirms the thing exists.
func TestAResourceUriNamingAnotherWorkspaceIsANotFound(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := readCatalogueFor(t)

	hub, _ := hubWithCollection(ctx, t, tenantA, authorA)

	// The same URI, read by the workspace next door.
	answer := readResource(ctx, t, registry, agentFor(tenantB, authorA),
		"hubtask://containers/"+hub.String())

	failure, isFailure := answer["error"].(map[string]any)
	if !isFailure {
		t.Fatalf("the workspace next door read %s: %v", hub, answer)
	}
	problem, _ := failure["data"].(map[string]any)
	if problem["code"] != "not_found" {
		t.Errorf("the refusal is %v, want a not-found that confirms nothing", problem)
	}
	if _, answered := answer["result"]; answered {
		t.Errorf("a refused read answered content as well: %v", answer)
	}
}

// A read the actor may not perform is refused by the application layer and recorded there, as
// `AI_AGENT` - the same entry the same refusal writes through any other channel (audit.md §2).
// Nothing in the MCP adapter writes an entry of its own, and nothing skips one.
func TestARefusedResourceReadIsAuditedAsTheAgentItWas(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := readCatalogueFor(t)

	hub, _ := hubWithCollection(ctx, t, tenantA, authorA)

	// An agent whose token carries no read scope at all: the refusal is the application layer's,
	// and it is the one an auditor has to be able to find.
	agent := agentFor(tenantA, authorA)
	agent.Scopes = nil

	before := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE target_id = $1`, hub.String())
	answer := readResource(ctx, t, registry, agent, "hubtask://containers/"+hub.String())
	if _, isFailure := answer["error"]; !isFailure {
		t.Fatalf("a scopeless agent read the hub: %v", answer)
	}

	after := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE target_id = $1`, hub.String())
	if after != before+1 {
		t.Fatalf("the refusal wrote %d entries, want exactly one", after-before)
	}

	var actorType, outcome string
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT actor_type, outcome FROM audit_log
		WHERE target_id = $1 ORDER BY seq DESC LIMIT 1`, hub.String()).
		Scan(&actorType, &outcome); err != nil {
		t.Fatalf("reading the audit entry: %v", err)
	}
	// AI_AGENT, and the actor that arrived was a person's: the door decided it, which is what
	// makes ai-first.md §1.1's promise about resources true and not only about tools (J-14).
	if actorType != string(appshared.ActorAIAgent) {
		t.Errorf("the refusal was recorded as %s, want the agent it was", actorType)
	}
	if outcome == "SUCCESS" {
		t.Errorf("a refusal was recorded as %s", outcome)
	}
}

// An ordinary read writes nothing, because GetContainer declares its audit as not required: a trail
// with an entry per read is a trail nobody reads (audit.md §4). What matters is that the resource
// door does not invent one.
func TestAnOrdinaryResourceReadInventsNoAuditEntry(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := readCatalogueFor(t)

	hub, _ := hubWithCollection(ctx, t, tenantA, authorA)
	before := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE target_id = $1`, hub.String())

	if answer := readResource(ctx, t, registry, agentFor(tenantA, authorA),
		"hubtask://containers/"+hub.String()); answer["error"] != nil {
		t.Fatalf("the read was refused: %v", answer)
	}

	if after := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE target_id = $1`, hub.String()); after != before {
		t.Errorf("an ordinary read wrote %d audit entries", after-before)
	}
}

// The list is narrowed to what the actor may see, paged, and never crosses the boundary either.
func TestTheResourceListIsWhatTheActorMaySeeAndIsPaged(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := readCatalogueFor(t)

	hub, _ := hubWithCollection(ctx, t, tenantA, authorA)
	mine := "hubtask://containers/" + hub.String()

	// Walked rather than read off the first page: the hub level is tenant-wide and this suite
	// shares a database, so by the time this test runs the workspace holds every hub every other
	// file in the package created. Which page its own lands on is not the point - that it is in
	// the walk at all is.
	if !walkFinds(ctx, t, registry, agentFor(tenantA, authorA), mine) {
		t.Errorf("the agent's own hub is nowhere in its resource list")
	}

	// And the workspace next door never finds it, however many pages it walks.
	if walkFinds(ctx, t, registry, agentFor(tenantB, authorA), mine) {
		t.Errorf("the workspace next door listed %s", mine)
	}
}

// maxResourcePages bounds the walk. It is a test's patience rather than a promise about the
// product: what it proves when it is not reached is that the list ends, which is what "paged and
// bounded" means for a method a client walks in a loop.
const maxResourcePages = 20

// walkFinds pages through one actor's resource list looking for a URI, and fails the test if the
// walk does not end.
func walkFinds(
	ctx context.Context, t *testing.T, registry *usecase.Registry,
	actor appshared.ActorContext, uri string,
) bool {
	t.Helper()

	next, found := "", false
	for range maxResourcePages {
		body := `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`
		if next != "" {
			body = `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{"cursor":"` + next + `"}}`
		}
		answer := overMCP(ctx, t, registry, actor, body)
		if resourceURIs(t, answer)[uri] {
			found = true
		}

		result, _ := answer["result"].(map[string]any)
		cursor, more := result["nextCursor"].(string)
		if !more || cursor == "" {
			return found
		}
		next = cursor
	}
	t.Errorf("the walk did not end in %d pages, which is a list that is not bounded",
		maxResourcePages)
	return found
}

// contentOf reads the one content block a resource read answers.
func contentOf(t *testing.T, answer map[string]any) string {
	t.Helper()
	if failure, refused := answer["error"]; refused {
		t.Fatalf("the read was refused: %v", failure)
	}
	result, _ := answer["result"].(map[string]any)
	contents, _ := result["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("the read answered %v", result)
	}
	content, _ := contents[0].(map[string]any)
	text, _ := content["text"].(string)
	return text
}

func resourceURIs(t *testing.T, answer map[string]any) map[string]bool {
	t.Helper()
	if failure, refused := answer["error"]; refused {
		t.Fatalf("the list was refused: %v", failure)
	}
	result, _ := answer["result"].(map[string]any)
	entries, _ := result["resources"].([]any)

	uris := make(map[string]bool, len(entries))
	for _, entry := range entries {
		resource, _ := entry.(map[string]any)
		uri, _ := resource["uri"].(string)
		uris[uri] = true
	}
	return uris
}

// equalJSON compares two encodings by their decoded value, because map iteration order is not part
// of what either channel promises.
func equalJSON(t *testing.T, left, right string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal([]byte(left), &a); err != nil {
		t.Fatalf("the resource content is not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(right), &b); err != nil {
		t.Fatalf("the tool content is not JSON: %v", err)
	}
	first, _ := json.Marshal(a)
	second, _ := json.Marshal(b)
	return string(first) == string(second)
}
