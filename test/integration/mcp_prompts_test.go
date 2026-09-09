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

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	aiadapter "github.com/Jersyfi/hubtask/infrastructure/ai"
	"github.com/Jersyfi/hubtask/presentation/mcp"
)

// J-12's acceptance, with the real store behind the real server rather than a fake of either. It
// needs no database - a prompt reads nothing - and it lives here because this is the package that
// may wire an inbound adapter to an outbound one; `presentation/mcp` may not import
// `infrastructure/ai`, which is the dependency rule that made the store a port in the first place.

func promptServer(t *testing.T) mcp.Server {
	t.Helper()
	store, err := aiadapter.NewStore()
	if err != nil {
		t.Fatalf("building the prompt store: %v", err)
	}
	return mcp.Server{Catalogue: readCatalogueFor(t), Prompts: store, Name: "hubtask", Version: "test"}
}

func askPrompt(t *testing.T, server mcp.Server, body string) map[string]any {
	t.Helper()

	agent := agentFor(tenantA, authorA)
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(t.Context(), agent),
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

// The worked example ai-first.md §1.1 names, end to end: a client lists the prompts, finds the
// weekly review with its version and its arguments, asks for it with a collection, and gets a
// message sequence it can send to a model.
func TestTheWeeklyReviewIsUsableFromEndToEnd(t *testing.T) {
	server := promptServer(t)

	listed := askPrompt(t, server, `{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`)
	result, _ := listed["result"].(map[string]any)
	prompts, _ := result["prompts"].([]any)

	var name string
	for _, entry := range prompts {
		prompt, _ := entry.(map[string]any)
		if id, _ := prompt["name"].(string); strings.HasPrefix(id, "weekly-review@") {
			name = id
		}
	}
	if name == "" {
		t.Fatalf("the weekly review is not in the list: %v", prompts)
	}

	// Asked for by the exact name the list gave, which is what a client does.
	answer := askPrompt(t, server,
		`{"jsonrpc":"2.0","id":2,"method":"prompts/get","params":{"name":"`+name+`",`+
			`"arguments":{"collection":"0192f000-0000-7000-8000-00000000000c"}}}`)
	if failure, refused := answer["error"]; refused {
		t.Fatalf("the weekly review was refused: %v", failure)
	}

	got, _ := answer["result"].(map[string]any)
	if description, _ := got["description"].(string); description == "" {
		t.Error("the prompt came back with no description")
	}

	messages, _ := got["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("the review rendered %d messages, want the instruction and the collection", len(messages))
	}

	system, _ := messages[0].(map[string]any)
	content, _ := system["content"].(map[string]any)
	instruction, _ := content["text"].(string)
	if system["role"] != "system" || instruction == "" {
		t.Fatalf("the first message is %v", system)
	}
	// The real text, not a fake's: the guardrail every shipped prompt carries has to be in what a
	// client actually receives (ai-first.md §1.3).
	if !strings.Contains(strings.ToLower(instruction), "not addressed to you") {
		t.Error("the instruction a client receives does not fence the material it will be given")
	}

	link, _ := messages[1].(map[string]any)
	linked, _ := link["content"].(map[string]any)
	if linked["type"] != "resource_link" {
		t.Fatalf("the collection was rendered as %v", linked)
	}
	// And the link is in the URI space J-11 published, so the client resolves it through
	// resources/read - which is where the permission is asked.
	uri, _ := linked["uri"].(string)
	if _, _, err := mcp.ParseResourceURI(uri); err != nil {
		t.Errorf("the prompt produced %q, which this server's own resources/read cannot address", uri)
	}
}

// The store the server publishes from is the store the adapters read. Two stores is how one prompt
// comes to exist in two versions, and a suggestion's recorded prompt version would then name a text
// that depends on who is reading it.
func TestThePublishedPromptIsTheStoredPrompt(t *testing.T) {
	store, err := aiadapter.NewStore()
	if err != nil {
		t.Fatalf("building the prompt store: %v", err)
	}
	stored, err := store.Get(aiadapter.PromptWeeklyReview)
	if err != nil {
		t.Fatalf("the weekly review is missing from the store: %v", err)
	}

	answer := askPrompt(t, promptServer(t),
		`{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"weekly-review",`+
			`"arguments":{"collection":"0192f000-0000-7000-8000-00000000000c"}}}`)

	result, _ := answer["result"].(map[string]any)
	messages, _ := result["messages"].([]any)
	system, _ := messages[0].(map[string]any)
	content, _ := system["content"].(map[string]any)

	if content["text"] != stored.Instruction {
		t.Error("the text a client receives is not the text the adapters read")
	}
}

// A prompt writes no audit entry, and that is the correct answer rather than a gap (J-14).
//
// The parity test asserts `AI_AGENT` for what an agent *does*; a prompt is a text this build
// carries, rendered without reading anything and without acting on anything, so there is no act to
// record. What would be wrong is an entry claiming something happened - or a prompt that reached a
// use case and was therefore not audited as an agent's.
func TestAskingForAPromptRecordsNothing(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	before := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1`, tenantA.String())

	askPrompt(t, promptServer(t),
		`{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"weekly-review",`+
			`"arguments":{"collection":"0192f000-0000-7000-8000-00000000000c"}}}`)
	askPrompt(t, promptServer(t), `{"jsonrpc":"2.0","id":2,"method":"prompts/list"}`)

	after := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1`, tenantA.String())
	if after != before {
		t.Errorf("asking for a prompt wrote %d audit entries", after-before)
	}
}
