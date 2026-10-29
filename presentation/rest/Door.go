// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// stepUpHeader carries a fresh proof of identity (security.md §5).
const stepUpHeader = "X-Hubtask-Step-Up"

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
	// IfMatch passes the version read as `expected_version`, or as the text IfMatchAs names.
	IfMatch   bool
	IfMatchAs string
	// StepUp passes the X-Hubtask-Step-Up proof as `step_up_token`; the use case decides it.
	StepUp       bool
	BodyRequired bool
	// ETag is the answer's member the ETag is drawn from: `version`, or a hash; empty for none.
	ETag string
	// BodyAs hands the whole body on as one input - a document the domain reads, such as a
	// form - instead of its members.
	BodyAs string
	// Location is the created resource's path below the API, its `{member}`s filled from the
	// output; empty for none.
	Location string
}

// door answers one generated operation. in holds the path's parameters; params is the generated
// parameters struct (nil for none); body is a pointer to the generated request body type (nil for
// none), decoded first so that a body the contract refuses is refused as a hand-written handler
// refuses it - an unknown member included - before its members become the input; answer is a
// pointer to the generated answer type (nil for none), which the output passes through, so the
// answer holds to the contract's shape.
func (c *RestController) door(w http.ResponseWriter, r *http.Request, d restDoor, in usecase.Input, params, body, answer any) {
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
			if d.BodyAs != "" {
				in[d.BodyAs] = sent
				sent = nil
			}
			for name, value := range sent {
				// The path names the resource; a body member of the same name never moves it. An
				// explicit null stays: in a merge patch it clears, as MCP's arguments carry it.
				if _, taken := in[name]; !taken {
					in[name] = value
				}
			}
		}
	}
	if d.IfMatch {
		match := ifMatchOf(r)
		switch {
		case d.IfMatchAs != "" && match != nil:
			in[d.IfMatchAs] = strings.Trim(strings.TrimPrefix(*match, "W/"), `"`)
		case d.IfMatchAs == "":
			if version, ok := versionFromIfMatch(match); ok {
				in["expected_version"] = version
			}
		}
	}
	if d.StepUp {
		if token := strings.TrimSpace(r.Header.Get(stepUpHeader)); token != "" {
			in["step_up_token"] = token
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
	var written any = out
	if draw, ok := answer.(answerer); ok {
		written = draw(out)
	} else if answer != nil {
		if err := remarshal(out, answer); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
		written = answer
	}
	if d.Location != "" {
		location := d.Location
		for name, value := range out {
			if text, ok := value.(string); ok {
				location = strings.ReplaceAll(location, "{"+name+"}", text)
			}
		}
		w.Header().Set("Location", APIBasePath+location)
	}
	switch value := out[d.ETag].(type) {
	case string:
		if value != "" {
			w.Header().Set("ETag", `"`+value+`"`)
		}
	case int, int64, float64:
		w.Header().Set("ETag", etag(out.Int(d.ETag)))
	}
	writeJSON(w, r, d.Status, written)
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

// remarshal moves a use case's output into a generated type through JSON - the contract's names.
func remarshal(from any, into any) error {
	raw, err := json.Marshal(from)
	if err != nil {
		return shared.ErrInternal.WithDetail("request.response_unwritable").WithCause(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return shared.ErrInternal.WithDetail("request.response_unwritable").WithCause(err)
	}
	return nil
}

// answerer draws an answer with one of the adapter's shared mappers (x-hubtask-rest-answer).
type answerer func(usecase.Output) any

func answerBy[T any](draw func(usecase.Output) T) answerer {
	return func(out usecase.Output) any { return draw(out) }
}

// answerRows draws a bare-array answer, each of the output's rows by the shared mapper.
func answerRows[T any](draw func(usecase.Output) T) answerer {
	return func(out usecase.Output) any {
		rows := rowsOf(out)
		answer := make([]T, 0, len(rows))
		for _, row := range rows {
			answer = append(answer, draw(row))
		}
		return answer
	}
}
