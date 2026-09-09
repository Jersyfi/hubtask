// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
)

// Catalogue is the slice of the use case registry this server needs.
type Catalogue interface {
	All() []usecase.Descriptor
	ByMCPTool(tool string) (usecase.Descriptor, bool)
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Server answers MCP over HTTP at /mcp (ai-first.md §1.1).
//
// It is an adapter and nothing more: it translates JSON-RPC into a catalogue call and the result
// back. There is no authorisation here and no business rule - an agent reaching a use case
// through this door is checked by the same application layer as a person reaching it through
// REST, which is what makes an agent's action as safe, and as auditable, as anybody else's
// (ADR-0005, ADR-0012).
//
// Deliberately not implemented yet: the SSE half of the streamable transport (J-13). It is a
// separate promise, and answering "method not found" is honest where an empty list would claim this
// installation has none.
type Server struct {
	Catalogue Catalogue
	// Prompts is the store the outbound adapters read, published here (J-12). Nil is an
	// installation running without it, and then the prompts capability is not declared and the two
	// methods answer "method not found" - the rule this file has always stated about itself.
	Prompts Prompts
	// Name and Version identify the server on initialize.
	Name    string
	Version string
}

// Prompts is the slice of the prompt store this server needs (core/port/ai.Prompts, filtered).
type Prompts interface {
	// Published is the prompts written for an agent rather than for this product's own provider.
	Published() []aiprovider.Prompt
	Get(id string) (aiprovider.Prompt, error)
}

// JSON-RPC 2.0 error codes (§5.1), and the one MCP adds. Only the ones this server can produce.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
	// codeResourceRefused is MCP's own code in the implementation-defined range: the resource is
	// not available to you. It carries every refusal a resource read can produce, with the exact
	// problem document in `data` - so an agent reads `forbidden` or `not_found` off the data and a
	// client that only understands the code still gets one it knows (ADR-0051 decision 4).
	codeResourceRefused = -32002
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	// Data carries the machine-readable refusal where there is one: the same document `failure`
	// renders for a tool call, so an agent corrects itself from a code rather than from a sentence
	// whichever door it came through (ADR-0011, api-guidelines.md §6).
	Data any `json:"data,omitempty"`
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// The GET half of the streamable transport opens a server-initiated stream. This server
		// initiates nothing, and saying so is better than holding a connection that never speaks.
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var call request
	if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
		write(w, r, response{JSONRPC: "2.0", Error: &rpcError{Code: codeParseError, Message: "parse error"}})
		return
	}
	if call.JSONRPC != "2.0" || call.Method == "" {
		write(w, r, response{JSONRPC: "2.0", ID: call.ID,
			Error: &rpcError{Code: codeInvalidRequest, Message: "invalid request"}})
		return
	}

	// A notification carries no identifier and expects no answer (JSON-RPC 2.0 §4.1). The one
	// that matters here is notifications/initialized, which a client sends after the handshake.
	if len(call.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	write(w, r, s.answer(r.Context(), call))
}

func (s Server) answer(ctx context.Context, call request) response {
	answer := response{JSONRPC: "2.0", ID: call.ID}

	switch call.Method {
	case "initialize":
		answer.Result = map[string]any{
			"protocolVersion": ProtocolVersion,
			// Tools and resources. Prompts are declared when they exist (J-12), because a client
			// that believes in a capability and finds nothing behind it has no way to recover.
			//
			// Both resource flags are false and both are honest: this server initiates nothing
			// until the streaming half of the transport arrives (J-13), so it cannot tell a client
			// that a list changed or that a resource it subscribed to has moved - and a client
			// that believed otherwise would wait for a message that never comes.
			"capabilities": s.capabilities(),
			"serverInfo":   map[string]any{"name": s.Name, "version": s.Version},
		}
	case "ping":
		answer.Result = map[string]any{}
	case "tools/list":
		answer.Result = map[string]any{"tools": ToolsOf(s.Catalogue.All())}
	case "tools/call":
		result, err := s.call(ctx, call.Params)
		if err != nil {
			answer.Error = err
			return answer
		}
		answer.Result = result
	case "resources/templates/list":
		answer.Result = map[string]any{"resourceTemplates": ResourceTemplates()}
	case "resources/list":
		result, err := s.list(ctx, call.Params)
		if err != nil {
			answer.Error = err
			return answer
		}
		answer.Result = result
	case "resources/read":
		result, err := s.read(ctx, call.Params)
		if err != nil {
			answer.Error = err
			return answer
		}
		answer.Result = result
	case "prompts/list":
		if s.Prompts == nil {
			answer.Error = &rpcError{Code: codeMethodNotFound, Message: "method not found"}
			return answer
		}
		answer.Result = map[string]any{"prompts": PromptsOf(s.Prompts.Published())}
	case "prompts/get":
		if s.Prompts == nil {
			answer.Error = &rpcError{Code: codeMethodNotFound, Message: "method not found"}
			return answer
		}
		result, err := s.prompt(call.Params)
		if err != nil {
			answer.Error = err
			return answer
		}
		answer.Result = result
	default:
		answer.Error = &rpcError{Code: codeMethodNotFound, Message: "method not found"}
	}
	return answer
}

type toolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// call runs one tool.
//
// The distinction it keeps is the one MCP asks for: a protocol failure - an unknown tool, params
// that are not an object - is a JSON-RPC error, while a use case that refused is a *result* with
// isError set. An agent has to be able to tell "I called this wrongly" from "the server said no",
// because only the second is worth reporting to the person it works for.
func (s Server) call(ctx context.Context, params json.RawMessage) (map[string]any, *rpcError) {
	var call toolCall
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params"}
	}

	descriptor, found := s.Catalogue.ByMCPTool(call.Name)
	if !found {
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown tool"}
	}

	actor, ok := appshared.ActorFrom(ctx)
	if !ok || !actor.IsAuthenticated() {
		// Unreachable behind the authentication middleware; a fail-closed guard rather than an
		// assumption, because a tool call without an actor would run without a tenant.
		return nil, &rpcError{Code: codeInternalError, Message: "unauthenticated"}
	}

	out, err := s.Catalogue.Invoke(ctx, descriptor.Name, actor, usecase.Input(call.Arguments))
	if err != nil {
		return failure(err), nil
	}
	return success(out), nil
}

// capabilities is what this server tells a client it can do.
//
// Nothing is claimed that is not served: a client that believes in a capability and finds nothing
// behind it has no way to recover, which is why prompts appear only where a store was wired.
//
// Every flag is false, and each one is honest. This server initiates nothing until the streaming
// half of the transport arrives (J-13), so it cannot tell a client that a list has changed or that
// a resource it subscribed to has moved.
func (s Server) capabilities() map[string]any {
	capabilities := map[string]any{
		"tools":     map[string]any{"listChanged": false},
		"resources": map[string]any{"subscribe": false, "listChanged": false},
	}
	if s.Prompts != nil {
		capabilities["prompts"] = map[string]any{"listChanged": false}
	}
	return capabilities
}

type promptGet struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
}

// prompt renders one prompt into the message sequence a client sends to a model.
//
// No actor is asked for, and that is deliberate rather than an omission: a prompt is a text this
// build carries, the same for every caller, and it reads nothing. What a *resource* argument
// produces is a link, and the permission behind it is asked when the client resolves it through
// `resources/read` - which is the one place it can be asked correctly, because that is where the
// read happens (ADR-0051).
func (s Server) prompt(params json.RawMessage) (map[string]any, *rpcError) {
	var call promptGet
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params"}
	}

	id, version, pinned := strings.Cut(call.Name, "@")
	prompt, err := s.Prompts.Get(id)
	if err != nil || !prompt.Published() {
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown prompt"}
	}
	// A pinned version that this build no longer carries is refused rather than answered with a
	// different text. A client pins a version precisely so that the words do not move underneath
	// it, and handing it the newest instead would be the one failure pinning exists to prevent.
	if pinned && version != prompt.Version {
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown prompt version"}
	}

	messages, err := PromptMessages(prompt, call.Arguments)
	if err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: err.Error()}
	}
	return map[string]any{"description": prompt.Description, "messages": messages}, nil
}

type resourceList struct {
	Cursor string `json:"cursor"`
}

type resourceRead struct {
	URI string `json:"uri"`
}

// list answers the resources the actor may see.
//
// Two things are enumerated and one deliberately is not (ADR-0051 decision 3). The hubs and the
// caller's own saved views have tenant-wide reads in the catalogue; entries do not - `ListWorkItems`
// wants a collection and `SearchItems` wants words - and enumerating every entry in a workspace is a
// tenant asking for everything, which is what every list in this system is paged to prevent. An
// agent reaches one by URI, from `resources/templates/list`, or finds it with `search_items`.
//
// The views ride on the first page because their read answers a bare list rather than a page: it is
// the caller's own views, bounded by how many a person makes. The hubs are paged by the catalogue's
// own cursor, passed through as the MCP cursor - opaque on both sides, and signed by the server that
// issued it. Nothing here chooses a page size: the use case clamps it to the same ceiling every
// list in this product has (api-guidelines.md §4).
func (s Server) list(ctx context.Context, params json.RawMessage) (map[string]any, *rpcError) {
	var call resourceList
	if len(params) > 0 {
		if err := json.Unmarshal(params, &call); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params"}
		}
	}

	actor, rpcErr := agentFrom(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}

	resources := make([]Resource, 0, 16)
	next := ""

	for _, kind := range ResourceKinds() {
		switch {
		case kind.ListUseCase == "":
			// The kind that cannot be enumerated. An agent reaches one by URI.
			continue
		case !kind.Paged && call.Cursor != "":
			// A bare list rides on the first page only. A cursor means the walk is already inside
			// the paged kind, and repeating the unpaged ones would make an agent read them once
			// per page.
			continue
		}

		input := usecase.Input{}
		if kind.Paged {
			input["cursor"] = call.Cursor
		}
		out, err := s.Catalogue.Invoke(ctx, kind.ListUseCase, actor, input)
		if err != nil {
			return nil, refusal(err)
		}
		resources = appendResources(resources, kind, out)
		if kind.Paged {
			next = nextCursorOf(out)
		}
	}

	answer := map[string]any{"resources": resources}
	if next != "" {
		answer["nextCursor"] = next
	}
	return answer, nil
}

// read answers one resource.
//
// The URI is the argument, and everything after parsing it is the catalogue's: the permission, the
// tenant, the audit entry. What this function decides is only which of two things a failure was -
// a URI this server cannot address, which is the client having called wrongly, or a read the
// application layer refused, which is the server saying no.
func (s Server) read(ctx context.Context, params json.RawMessage) (map[string]any, *rpcError) {
	var call resourceRead
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params"}
	}

	kind, id, err := ParseResourceURI(call.URI)
	if err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: err.Error()}
	}

	actor, rpcErr := agentFrom(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}

	out, err := s.Catalogue.Invoke(ctx, kind.UseCase, actor, usecase.Input{
		kind.Field: id.String(),
	})
	if err != nil {
		return nil, refusal(err)
	}

	return map[string]any{"contents": []map[string]any{{
		"uri":      call.URI,
		"mimeType": MimeType,
		"text":     encode(out),
	}}}, nil
}

// agentFrom is the fail-closed guard both resource methods share. Unreachable behind the
// authentication middleware; a guard rather than an assumption, because a read without an actor
// would run without a tenant.
func agentFrom(ctx context.Context) (appshared.ActorContext, *rpcError) {
	actor, ok := appshared.ActorFrom(ctx)
	if !ok || !actor.IsAuthenticated() {
		return appshared.ActorContext{}, &rpcError{Code: codeInternalError, Message: "unauthenticated"}
	}
	return actor, nil
}

// refusal renders a use case's refusal as a JSON-RPC error carrying the problem document.
//
// One code for every refusal, and the exact one in `data`. MCP's `resources/read` result has no
// place to put an error - unlike a tool result, which carries `isError` - so a refusal has to be a
// JSON-RPC error or it is nothing a client can see. What the tools half keeps, this keeps too: the
// distinction between having called wrongly (`codeInvalidParams`, decided before the catalogue is
// reached) and having been told no (`codeResourceRefused`, decided by the application layer).
//
// Nothing here decides whether something exists. A URI naming another workspace's identifier
// reaches this function as the `not_found` the read itself answered, because row level security has
// already made "not yours" and "not there" the same answer (multi-tenancy.md §2).
func refusal(err error) *rpcError {
	problem := failure(err)["structuredContent"]
	return &rpcError{Code: codeResourceRefused, Message: "resource unavailable", Data: problem}
}

// appendResources renders the rows of one catalogue list as resource entries.
func appendResources(into []Resource, kind ResourceKind, out usecase.Output) []Resource {
	rows, _ := out["data"].([]usecase.Output)
	for _, row := range rows {
		if resource, ok := resourceOf(kind, row); ok {
			into = append(into, resource)
		}
	}
	return into
}

// nextCursorOf reads the continuation out of a page, or answers empty on the last one.
func nextCursorOf(out usecase.Output) string {
	page, ok := out["page"].(map[string]any)
	if !ok {
		return ""
	}
	cursor, _ := page["next_cursor"].(string)
	return cursor
}

// success answers with both shapes MCP defines: the text block every client can render, and the
// structured content a client that knows the schema can use directly.
func success(out usecase.Output) map[string]any {
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": encode(out)}},
		"structuredContent": out,
		"isError":           false,
	}
}

// failure renders a refusal the way the REST layer renders it: a stable code, a detail code and
// parameters, never prose (api-guidelines.md §6, ADR-0011). An agent corrects itself from a code;
// it cannot correct itself from a sentence.
func failure(err error) map[string]any {
	domainErr := shared.AsError(err)

	problem := map[string]any{"code": domainErr.Code}
	if domainErr.DetailCode != "" {
		problem["detail_code"] = domainErr.DetailCode
	}
	if len(domainErr.Params) > 0 {
		problem["params"] = domainErr.Params
	}
	if len(domainErr.Fields) > 0 {
		fields := make([]map[string]any, 0, len(domainErr.Fields))
		for _, field := range domainErr.Fields {
			finding := map[string]any{"path": field.Path, "code": field.Code}
			if len(field.Params) > 0 {
				finding["params"] = field.Params
			}
			fields = append(fields, finding)
		}
		problem["field_errors"] = fields
	}

	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": encode(problem)}},
		"structuredContent": problem,
		"isError":           true,
	}
}

func encode(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		// Every value here came from a use case result or from the error model, both of which
		// serialise; an empty object is a better answer than a panic on the request path.
		return "{}"
	}
	return string(encoded)
}

func write(w http.ResponseWriter, r *http.Request, answer response) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	if err := json.NewEncoder(w).Encode(answer); err != nil {
		slog.WarnContext(r.Context(), "writing the MCP response failed",
			slog.String("error", err.Error()))
	}
}

// Path is where the server is mounted. Named here rather than in the composition root, because
// the endpoint is part of what this package promises an agent (api-guidelines.md §2).
const Path = "/mcp"
