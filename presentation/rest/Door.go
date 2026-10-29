// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// restDoor is how the generated mapping (Doors.gen.go, tools/doors) answers one operation marked
// `x-hubtask-rest: generated` (F-7). It translates and dispatches only: the request's members
// reach the use case under the contract's names, as MCP's arguments do, and the use case's output
// - which carries the contract's names already - is the answer. Nothing is decided here (rule 2).
type restDoor struct {
	UseCase string
	// Status is the operation's successful answer; 204 writes no body.
	Status int
	// Query is the query parameters the use case reads, by the contract's names.
	Query []string
	// IfMatch passes the version read as `expected_version`.
	IfMatch      bool
	BodyRequired bool
	// ETag sets the answer's `version` as the ETag.
	ETag bool
}

// door answers one generated operation. in holds the path's parameters; params is the generated
// parameters struct (nil for none); body is a pointer to the generated request body type (nil for
// none), decoded first so that a body the contract refuses is refused as a hand-written handler
// refuses it - an unknown member included - before its members become the input.
func (c *RestController) door(w http.ResponseWriter, r *http.Request, d restDoor, in usecase.Input, params, body any) {
	requestID := correlation.RequestIDFrom(r.Context())
	if params != nil && len(d.Query) > 0 {
		values, err := members(params)
		if err != nil {
			WriteProblem(w, err, requestID)
			return
		}
		for _, name := range d.Query {
			if value, ok := values[name]; ok && value != nil {
				in[name] = value
			}
		}
	}
	if body != nil {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			WriteProblem(w, shared.ErrMalformedRequest.WithDetail("request.body_unreadable").WithCause(err), requestID)
			return
		}
		if len(bytes.TrimSpace(raw)) > 0 || d.BodyRequired {
			if err := decodeFrom(bytes.NewReader(raw), body); err != nil {
				WriteProblem(w, err, requestID)
				return
			}
			var sent map[string]any
			if err := json.Unmarshal(raw, &sent); err != nil {
				WriteProblem(w, shared.ErrMalformedRequest.WithDetail("request.body_malformed").WithCause(err), requestID)
				return
			}
			for name, value := range sent {
				// The path names the resource; a body member of the same name never moves it.
				if _, taken := in[name]; !taken && value != nil {
					in[name] = value
				}
			}
		}
	}
	if d.IfMatch {
		if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
			in["expected_version"] = version
		}
	}

	out, ok := c.read(w, r, d.UseCase, in)
	if !ok {
		return
	}
	if d.Status == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, versioned := out["version"]; d.ETag && versioned {
		w.Header().Set("ETag", etag(out.Int("version")))
	}
	writeJSON(w, r, d.Status, out)
}

// members reads the generated parameters struct by its JSON names - the contract's.
func members(params any) (map[string]any, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, shared.ErrInternal.WithCause(err)
	}
	values := map[string]any{}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, shared.ErrInternal.WithCause(err)
	}
	return values, nil
}
