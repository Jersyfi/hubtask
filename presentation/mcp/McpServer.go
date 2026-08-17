// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
// Deliberately not implemented yet: the SSE half of the streamable transport, resources, and
// prompts. Every one of them is a separate promise, and answering "method not found" is honest
// where an empty list would claim this installation has none.
type Server struct {
	Catalogue Catalogue
	// Name and Version identify the server on initialize.
	Name    string
	Version string
}

// JSON-RPC 2.0 error codes (§5.1). Only the ones this server can produce.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
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
			// Tools only. Resources and prompts are declared when they exist, because a client
			// that believes in a capability and finds nothing behind it has no way to recover.
			"capabilities": map[string]any{"tools": map[string]any{"listChanged": false}},
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
