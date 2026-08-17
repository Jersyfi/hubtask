// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ProblemContentType is the media type of RFC 9457. Not application/json - a client
// distinguishes an error from a payload by the media type.
const ProblemContentType = "application/problem+json"

// docsBaseURL is where the type and docs links point. It is not configurable: the URL is part of
// the published contract, and a self-hosted installation documents the same error codes.
const docsBaseURL = "https://docs.hubtask.dev/errors/"

// Problem is the error format from api-guidelines.md §6. The field names follow the schema in
// api/openapi.yaml; changing one is a breaking change (versioning-release.md §2).
//
// What is deliberately absent: any free text. There is no `detail` field with a sentence in it,
// because the server produces no display text (ADR-0011) - the client builds the sentence from
// DetailCode and Params.
type Problem struct {
	Type        string            `json:"type"`
	Title       string            `json:"title"`
	Status      int               `json:"status"`
	Code        string            `json:"code"`
	DetailCode  string            `json:"detail_code,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	FieldErrors []ProblemField    `json:"field_errors,omitempty"`
	RequestID   string            `json:"request_id,omitempty"`
	Docs        string            `json:"docs"`
}

// ProblemField is one finding on one field. Path is a JSON Pointer (RFC 6901).
type ProblemField struct {
	Path   string            `json:"path"`
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

// statusByCategory is the mapping from api-guidelines.md §6. It is a table rather than a switch
// so that it can be read against the document line by line.
var statusByCategory = map[shared.Category]int{
	shared.CategoryValidation:      http.StatusUnprocessableEntity, // 422
	shared.CategoryNotFound:        http.StatusNotFound,            // 404
	shared.CategoryConflict:        http.StatusConflict,            // 409
	shared.CategoryForbidden:       http.StatusForbidden,           // 403
	shared.CategoryUnauthenticated: http.StatusUnauthorized,        // 401
	shared.CategoryGone:            http.StatusGone,                // 410
	shared.CategoryRateLimited:     http.StatusTooManyRequests,     // 429
	shared.CategoryUnavailable:     http.StatusServiceUnavailable,  // 503
	shared.CategoryInternal:        http.StatusInternalServerError, // 500
}

// malformed_request is the one validation code that is a 400 rather than a 422: the request could
// not be read at all, so no field can be named (api-guidelines.md §6).
const codeMalformedRequest = "malformed_request"

// codeMethodNotAllowed is the one contract code with no domain error behind it. The request
// matched a route but not one of its methods, so no use case is ever entered - which is also why
// 405 is absent from the mapping in api-guidelines.md §6: that table maps domain categories to
// statuses, and this is the router answering for itself. Adding a code is additive and therefore
// not a breaking change (api-guidelines.md §8).
const codeMethodNotAllowed = "method_not_allowed"

// ProblemFrom maps any error to problem details. An error that is not a domain error becomes
// INTERNAL, and nothing of its text reaches the response - it may contain a connection string,
// a query fragment, or a path (security.md §9).
//
// requestID comes from the request ID middleware and is what a user quotes in a support request:
// it is the only handle connecting a response to a log entry.
func ProblemFrom(err error, requestID string) Problem {
	domainErr := shared.AsError(err)
	if domainErr == nil {
		domainErr = shared.ErrInternal
	}

	status, ok := statusByCategory[domainErr.Category]
	if !ok {
		// Unreachable through AsError, which already normalises an unknown category. Kept as a
		// fail-closed default rather than a panic in the error path of all things.
		status = http.StatusInternalServerError
	}
	if domainErr.Code == codeMalformedRequest {
		status = http.StatusBadRequest
	}

	problem := Problem{
		Type:       docsBaseURL + domainErr.Code,
		Title:      domainErr.Code,
		Status:     status,
		Code:       domainErr.Code,
		DetailCode: domainErr.DetailCode,
		Params:     domainErr.Params,
		RequestID:  requestID,
		Docs:       docsBaseURL + domainErr.Code,
	}

	// Field errors only exist below 500. Above it there is nothing to say about a field, and
	// what an internal error knows must not be reflected back.
	if status < http.StatusInternalServerError && len(domainErr.Fields) > 0 {
		problem.FieldErrors = make([]ProblemField, 0, len(domainErr.Fields))
		for _, f := range domainErr.Fields {
			problem.FieldErrors = append(problem.FieldErrors, ProblemField{
				Path: f.Path, Code: f.Code, Params: f.Params,
			})
		}
	}
	if status >= http.StatusInternalServerError {
		// An internal error says what happened by its code and by the request ID. Parameters
		// come from the place that raised it, and that place may be a driver.
		problem.Params = nil
		problem.DetailCode = ""
	}

	return problem
}

// WriteProblem sends the error as RFC 9457. It never fails visibly: at that point the status is
// already written, and a broken response body is a log entry, not a second error.
func WriteProblem(w http.ResponseWriter, err error, requestID string) {
	writeProblem(w, ProblemFrom(err, requestID))
}

// WriteMethodNotAllowed answers a request that reached a route but not one of its methods. The
// caller sets the Allow header first - without it a 405 tells a client nothing it can act on.
func WriteMethodNotAllowed(w http.ResponseWriter, requestID string) {
	writeProblem(w, Problem{
		Type:      docsBaseURL + codeMethodNotAllowed,
		Title:     codeMethodNotAllowed,
		Status:    http.StatusMethodNotAllowed,
		Code:      codeMethodNotAllowed,
		RequestID: requestID,
		Docs:      docsBaseURL + codeMethodNotAllowed,
	})
}

func writeProblem(w http.ResponseWriter, problem Problem) {
	w.Header().Set("Content-Type", ProblemContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}
