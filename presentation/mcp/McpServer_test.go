// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// catalogue is the registry as this server sees it, built from a real descriptor so that the
// tool, its schema and its name are derived exactly as they are in production.
type catalogue struct {
	descriptors []usecase.Descriptor
	invokedName string
	invokedIn   usecase.Input
	out         usecase.Output
	err         error
}

func (c *catalogue) All() []usecase.Descriptor { return c.descriptors }

func (c *catalogue) ByMCPTool(tool string) (usecase.Descriptor, bool) {
	for _, descriptor := range c.descriptors {
		if descriptor.MCPTool() == tool {
			return descriptor, true
		}
	}
	return usecase.Descriptor{}, false
}

func (c *catalogue) Invoke(_ context.Context, name string, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c.invokedName, c.invokedIn = name, in
	return c.out, c.err
}

func descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        "CreateContainer",
		Summary:     "Creates a hub or a collection.",
		SideEffects: "Writes the container and announces it.",
		Input: []usecase.Field{
			{Name: "type", Kind: usecase.KindString, Required: true, Enum: []string{"HUB", "COLLECTION"},
				Description: "HUB for a top level workspace."},
			{Name: "name", Kind: usecase.KindString, Required: true},
			{Name: "parent_id", Kind: usecase.KindID},
			{Name: "pinned", Kind: usecase.KindBool},
		},
	}
}

func serverWith(store *catalogue) Server {
	return Server{Catalogue: store, Name: "hubtask", Version: "0.1.0"}
}

func rpc(t *testing.T, server Server, body string, authenticated bool) map[string]any {
	t.Helper()

	ctx := t.Context()
	if authenticated {
		ctx = appshared.ContextWithActor(ctx, appshared.ActorContext{
			Kind: appshared.ActorAIAgent, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
			AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		})
	}

	request := httptest.NewRequestWithContext(ctx, http.MethodPost, Path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if recorder.Body.Len() == 0 {
		return nil
	}

	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	return answer
}

func TestInitializeAnnouncesWhatThisServerCanDo(t *testing.T) {
	answer := rpc(t, serverWith(&catalogue{}), `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, true)

	result, _ := answer["result"].(map[string]any)
	if result["protocolVersion"] != ProtocolVersion {
		t.Errorf("protocol version %v", result["protocolVersion"])
	}
	capabilities, _ := result["capabilities"].(map[string]any)
	if _, tools := capabilities["tools"]; !tools {
		t.Errorf("the server does not announce tools: %v", capabilities)
	}
	// Nothing else is claimed: a client that believes in a capability and finds nothing behind it
	// has no way to recover.
	if _, resources := capabilities["resources"]; resources {
		t.Error("the server announces resources it does not serve")
	}
}

// The list is generated from the catalogue, which is what keeps the agent interface from falling
// behind the API (ADR-0012).
func TestTheToolListComesFromTheCatalogue(t *testing.T) {
	answer := rpc(t, serverWith(&catalogue{descriptors: []usecase.Descriptor{descriptor()}}),
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, true)

	result, _ := answer["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("%d tools, want 1", len(tools))
	}

	tool, _ := tools[0].(map[string]any)
	if tool["name"] != "create_container" {
		t.Errorf("tool name %v, want the use case name in snake_case", tool["name"])
	}
	if description, _ := tool["description"].(string); !strings.Contains(description, "side") &&
		!strings.Contains(description, "announces") {
		t.Errorf("the description does not carry the side effects: %q", description)
	}

	schema, _ := tool["inputSchema"].(map[string]any)
	if schema["additionalProperties"] != false {
		t.Error("the schema accepts fields the use case does not declare")
	}
	required, _ := schema["required"].([]any)
	if len(required) != 2 {
		t.Errorf("required = %v, want type and name", required)
	}

	properties, _ := schema["properties"].(map[string]any)
	parent, _ := properties["parent_id"].(map[string]any)
	if parent["type"] != "string" || parent["format"] != "uuid" {
		t.Errorf("an identifier is not described as a uuid: %v", parent)
	}
	pinned, _ := properties["pinned"].(map[string]any)
	if pinned["type"] != "boolean" {
		t.Errorf("a boolean is described as %v", pinned["type"])
	}
	containerType, _ := properties["type"].(map[string]any)
	if values, _ := containerType["enum"].([]any); len(values) != 2 {
		t.Errorf("the enum did not survive: %v", containerType)
	}
}

// A tool call is a catalogue call, under the use case's own name, with the actor of the request.
func TestAToolCallReachesTheUseCase(t *testing.T) {
	store := &catalogue{
		descriptors: []usecase.Descriptor{descriptor()},
		out:         usecase.Output{"id": "0192f000-0000-7000-8000-00000000000b", "name": "Private"},
	}

	answer := rpc(t, serverWith(store),
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_container","arguments":{"type":"HUB","name":"Private"}}}`,
		true)

	if store.invokedName != "CreateContainer" {
		t.Errorf("the catalogue was asked for %q", store.invokedName)
	}
	if store.invokedIn["name"] != "Private" {
		t.Errorf("the arguments did not arrive: %v", store.invokedIn)
	}

	result, _ := answer["result"].(map[string]any)
	if result["isError"] != false {
		t.Errorf("a successful call was reported as an error: %v", result)
	}
	structured, _ := result["structuredContent"].(map[string]any)
	if structured["id"] != "0192f000-0000-7000-8000-00000000000b" {
		t.Errorf("the result is missing: %v", result)
	}
	// The text block is what every client can render, even one that ignores the structured form.
	content, _ := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content = %v", result["content"])
	}
	block, _ := content[0].(map[string]any)
	if block["type"] != "text" || !strings.Contains(block["text"].(string), "Private") {
		t.Errorf("the text block does not carry the result: %v", block)
	}
}

// The distinction MCP asks for: a refusal by the use case is a result with isError, not a
// protocol error - an agent has to tell "I called this wrongly" from "the server said no".
func TestARefusedCallIsAResultRatherThanAProtocolError(t *testing.T) {
	store := &catalogue{
		descriptors: []usecase.Descriptor{descriptor()},
		err: shared.ErrForbidden.
			WithDetail("access.not_permitted").
			WithParams(map[string]string{"permission": "STRUCTURE"}),
	}

	answer := rpc(t, serverWith(store),
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"create_container","arguments":{"type":"HUB","name":"Private"}}}`,
		true)

	if _, isProtocolError := answer["error"]; isProtocolError {
		t.Fatalf("a refusal was reported as a protocol error: %v", answer["error"])
	}
	result, _ := answer["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("the refusal was reported as a success: %v", result)
	}

	problem, _ := result["structuredContent"].(map[string]any)
	if problem["code"] != "forbidden" || problem["detail_code"] != "access.not_permitted" {
		t.Errorf("the refusal carries no machine-readable code: %v", problem)
	}
	params, _ := problem["params"].(map[string]any)
	if params["permission"] != "STRUCTURE" {
		t.Errorf("the parameters an agent would correct itself from are missing: %v", problem)
	}
}

// A validation failure keeps its field findings, which is what lets an agent fix its own call.
func TestAValidationFailureKeepsItsFieldFindings(t *testing.T) {
	store := &catalogue{
		descriptors: []usecase.Descriptor{descriptor()},
		err: shared.ErrValidation.
			WithDetail("usecase.input_invalid").
			WithFields(shared.FieldError{Path: "/name", Code: "usecase.field_required"}),
	}

	answer := rpc(t, serverWith(store),
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"create_container","arguments":{"type":"HUB"}}}`,
		true)

	result, _ := answer["result"].(map[string]any)
	problem, _ := result["structuredContent"].(map[string]any)
	findings, _ := problem["field_errors"].([]any)
	if len(findings) != 1 {
		t.Fatalf("field errors = %v", problem)
	}
	finding, _ := findings[0].(map[string]any)
	if finding["path"] != "/name" || finding["code"] != "usecase.field_required" {
		t.Errorf("the finding does not point at the field: %v", finding)
	}
}

func TestProtocolFailuresAreJSONRPCErrors(t *testing.T) {
	cases := map[string]struct {
		body string
		code float64
	}{
		"a body that is not JSON":     {`{"jsonrpc":`, -32700},
		"a request without a version": {`{"id":1,"method":"tools/list"}`, -32600},
		"a method nobody serves":      {`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`, -32601},
		"a tool nobody registered":    {`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"delete_everything"}}`, -32602},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			answer := rpc(t, serverWith(&catalogue{descriptors: []usecase.Descriptor{descriptor()}}), c.body, true)

			failure, _ := answer["error"].(map[string]any)
			if failure["code"] != c.code {
				t.Errorf("error = %v, want code %v", answer, c.code)
			}
		})
	}
}

// A notification expects no answer at all (JSON-RPC 2.0 §4.1) - the one a client sends after the
// handshake.
func TestANotificationIsAcknowledgedWithoutABody(t *testing.T) {
	if answer := rpc(t, serverWith(&catalogue{}),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`, true); answer != nil {
		t.Errorf("a notification was answered: %v", answer)
	}
}

// Fail closed: without an actor there is no tenant, and a tool call must not run.
func TestAToolCallWithoutAnActorIsRefused(t *testing.T) {
	store := &catalogue{descriptors: []usecase.Descriptor{descriptor()}, out: usecase.Output{}}

	answer := rpc(t, serverWith(store),
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"create_container","arguments":{}}}`,
		false)

	if _, failed := answer["error"]; !failed {
		t.Errorf("a call without an actor was served: %v", answer)
	}
	if store.invokedName != "" {
		t.Error("the use case ran without an actor")
	}
}

// The transport is POST. A GET opens the server-initiated stream of the streamable transport,
// which this server does not have.
func TestOnlyPostIsServed(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, Path, nil)
	recorder := httptest.NewRecorder()
	serverWith(&catalogue{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("Allow is %q", allow)
	}
}
