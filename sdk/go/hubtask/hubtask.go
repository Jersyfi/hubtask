// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package hubtask is the Go client for the Hubtask API, generated from api/openapi.yaml.
//
// Everything but this file is generated (client.gen.go, by `make generate`) and follows the
// contract exactly: one method per operation, typed parameters and bodies, and a `WithResponse`
// variant that decodes the answer into the schema the contract declares for each status. This
// file holds what a caller needs beside that and the generator does not write - and it is kept
// to a few lines on purpose, because ADR-0057 may move this package to a repository of its own,
// and every line here is one that move has to carry.
//
// The address is the installation's API root, `https://<host>/api/v1`:
//
//	client, err := hubtask.NewClientWithResponses("https://hubtask.example/api/v1",
//		hubtask.WithRequestEditorFn(hubtask.WithBearer(token)))
//	res, err := client.ListContainersWithResponse(ctx, &hubtask.ListContainersParams{})
//	if err := hubtask.Check(err, res); err != nil { ... }
//	for _, container := range res.JSON200.Items { ... }
//
// The token is whatever the installation issued - a personal access token, a service account
// token or an OIDC access token - and the client never inspects its shape.
package hubtask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
)

// WithBearer sends the token on every request.
func WithBearer(token string) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// WithIdempotencyKey sends the key on one request, for a mutation the caller may retry
// (api-guidelines.md §5). The typed parameters of every mutation carry the same field; this is
// for a caller who prefers the editor.
func WithIdempotencyKey(key string) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Idempotency-Key", key)
		return nil
	}
}

// WithIfMatch sends the entity tag the caller last read, so that a write against a state
// somebody else moved on is refused with 412 rather than applied over it.
func WithIfMatch(etag string) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("If-Match", etag)
		return nil
	}
}

// ProblemError is a refusal: the problem document (RFC 9457) the server answered, with its
// stable code. Every 4xx and 5xx of the contract is one of these.
type ProblemError struct {
	Status  int
	Problem Problem
}

func (e *ProblemError) Error() string {
	if e.Problem.DetailCode != nil && *e.Problem.DetailCode != "" {
		return fmt.Sprintf("hubtask: %d %s (%s)", e.Status, e.Problem.Code, *e.Problem.DetailCode)
	}
	return fmt.Sprintf("hubtask: %d %s", e.Status, e.Problem.Code)
}

// Answer is what every generated `*Result` type is: a status, and a raw body Check reads.
type Answer interface {
	StatusCode() int
}

// Check turns a transport error or a non-2xx answer into an error, and a 2xx into nil.
//
//	res, err := client.CreateWorkItemWithResponse(ctx, params, body)
//	if err := hubtask.Check(err, res); err != nil {
//		var problem *hubtask.ProblemError
//		if errors.As(err, &problem) && problem.Problem.Code == "version_conflict" { ... }
//	}
//
// A body that is not a problem document - a proxy's HTML, an empty 5xx - still becomes a
// ProblemError, with the status and an empty code, so that a caller has one shape to handle.
// The body is read through the `Body` field every generated result carries; reflection rather
// than an interface because the generator writes a field, and the one line here is cheaper than
// a method on six hundred types.
func Check(err error, answer Answer) error {
	if err != nil {
		return err
	}
	if answer == nil || (reflect.ValueOf(answer).Kind() == reflect.Pointer && reflect.ValueOf(answer).IsNil()) {
		return errors.New("hubtask: no answer")
	}
	status := answer.StatusCode()
	if status >= 200 && status < 300 {
		return nil
	}
	problem := Problem{Status: status}
	value := reflect.Indirect(reflect.ValueOf(answer))
	if value.Kind() == reflect.Struct {
		if body := value.FieldByName("Body"); body.IsValid() && body.Kind() == reflect.Slice {
			_ = json.Unmarshal(body.Bytes(), &problem)
		}
	}
	if problem.Status == 0 {
		problem.Status = status
	}
	return &ProblemError{Status: status, Problem: problem}
}
