// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The agent interface from outside the process (J-16). What is asked here is the protocol: the
// handshake happens first, the session it hands back is carried, and a JSON-RPC refusal - which
// arrives with a 200 - is an error rather than a success.

// rpcStub answers a handshake and then one method, recording what it was asked.
func rpcStub(t *testing.T, result string) (*installation, *[]string) {
	t.Helper()
	methods := &[]string{}
	sessions := &[]string{}

	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		// `serve` has already drained the body into stub.body, so it is read from there rather
		// than from a reader nothing is left in.
		var call struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(stub.body), &call)
		*methods = append(*methods, call.Method)
		*sessions = append(*sessions, r.Header.Get(mcpSessionHeader))

		w.Header().Set("Content-Type", "application/json")
		if call.Method == "initialize" {
			w.Header().Set(mcpSessionHeader, "the-session")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":` + result + `}`))
	})
	stub.sessions = sessions
	return stub, methods
}

// The handshake comes first and the session it answers is carried on the call after it - which is
// the whole of the transport's state, and the reason this CLI keeps none on disk.
func TestAnMcpCommandHandshakesAndCarriesTheSession(t *testing.T) {
	stub, methods := rpcStub(t, `{"tools":[{"name":"create_container","description":"Creates a hub. Writes it.","annotations":{"readOnlyHint":false,"destructiveHint":false}}]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "mcp", "tools")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(*methods) != 2 || (*methods)[0] != "initialize" || (*methods)[1] != "tools/list" {
		t.Fatalf("the calls were %v", *methods)
	}
	if got := (*stub.sessions)[1]; got != "the-session" {
		t.Errorf("the second call carried the session %q", got)
	}
	if !strings.Contains(out, "create_container") {
		t.Errorf("the table does not list the tool: %q", out)
	}
}

// A JSON-RPC refusal arrives with a 200 and an error object, so a client that read the status alone
// would call every refusal a success.
func TestAJsonRpcRefusalIsAnErrorRatherThanASuccess(t *testing.T) {
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(stub.body, "initialize") {
			w.Header().Set(mcpSessionHeader, "the-session")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
			return
		}
		// A 200 with an error object, which is what the protocol says.
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"error":{"code":-32002,` +
			`"message":"resource unavailable","data":{"code":"not_found"}}}`))
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"mcp", "read", "hubtask://items/"+itemID)
	if code == exitOK {
		t.Fatal("a refusal was reported as success")
	}
	if !strings.Contains(errOut, "resource unavailable") || !strings.Contains(errOut, "not_found") {
		t.Errorf("the refusal does not carry what the server said: %q", errOut)
	}
}

// Reading a resource prints what an agent would receive, whole: the content is a use case's own
// answer, and reshaping it would defeat the point of looking.
func TestReadingAResourcePrintsWhatAnAgentWouldSee(t *testing.T) {
	stub, methods := rpcStub(t, `{"contents":[{"uri":"hubtask://items/`+itemID+
		`","mimeType":"application/json","text":"{\"id\":\"`+itemID+`\",\"title\":\"Buy milk\"}"}]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"mcp", "read", "hubtask://items/"+itemID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if (*methods)[1] != "resources/read" {
		t.Errorf("the call was %q", (*methods)[1])
	}
	if !strings.Contains(out, "Buy milk") {
		t.Errorf("the content was not printed: %q", out)
	}
}

// The prompts list carries the version in the name and says which arguments are required, which is
// what a client needs before it can call one.
func TestThePromptListShowsVersionsAndArguments(t *testing.T) {
	stub, _ := rpcStub(t, `{"prompts":[{"name":"weekly-review@v1","title":"Weekly review",`+
		`"description":"one collection's week","arguments":[{"name":"collection","required":true},`+
		`{"name":"focus","required":false}]}]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "mcp", "prompts")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "weekly-review@v1") {
		t.Errorf("the version is not in the name: %q", out)
	}
	// Required plainly, optional in brackets: a client has to know which it may leave out.
	if !strings.Contains(out, "collection") || !strings.Contains(out, "[focus]") {
		t.Errorf("the arguments do not say which are required: %q", out)
	}
}

// An argument is name=value, and anything else is a usage error rather than a request the server
// has to refuse.
func TestAPromptArgumentIsNameEqualsValue(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a malformed argument reached the installation")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"mcp", "prompt", "weekly-review", "--argument", "collection")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "name=value") {
		t.Errorf("the message %q does not say the shape", errOut)
	}
}

// The templates are the URI shapes, which is how a client learns to construct one for content it
// cannot enumerate.
func TestTheResourceTemplatesArePublished(t *testing.T) {
	stub, methods := rpcStub(t, `{"resourceTemplates":[{"uriTemplate":"hubtask://items/{id}",`+
		`"name":"items","title":"Work item","description":"One task."}]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"mcp", "resources", "--templates")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if (*methods)[1] != "resources/templates/list" {
		t.Errorf("the call was %q", (*methods)[1])
	}
	if !strings.Contains(out, "hubtask://items/{id}") {
		t.Errorf("the shape is not printed: %q", out)
	}
}
