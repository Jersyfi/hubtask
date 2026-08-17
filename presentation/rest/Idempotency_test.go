// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/idempotency"
	usecase "github.com/Jersyfi/hubtask/core/application/service/idempotency"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const idempotencyKey = "5f9d1f8e-0000-4000-8000-000000000001"

type guard struct {
	replay *repository.Record
	err    error

	beganWith  repository.Key
	beganHash  []byte
	completed  bool
	gotStatus  int
	gotBody    []byte
	beginCalls int
}

func (g *guard) Begin(
	_ context.Context, _ appshared.ActorContext, key repository.Key, hash []byte,
) (usecase.Attempt, error) {
	g.beginCalls++
	g.beganWith, g.beganHash = key, hash
	if g.err != nil {
		return usecase.Attempt{}, g.err
	}
	return usecase.Attempt{Replay: g.replay}, nil
}

func (g *guard) Complete(
	_ context.Context, _ appshared.ActorContext, _ repository.Key, status int, body []byte,
) error {
	g.completed = true
	g.gotStatus, g.gotBody = status, body
	return nil
}

func idempotentRequest(t *testing.T, method, body string, withKey bool) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), method, APIBasePath+"/containers", strings.NewReader(body))
	if withKey {
		r.Header.Set(IdempotencyKeyHeader, idempotencyKey)
	}
	actor := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantID}
	return r.WithContext(appshared.ContextWithActor(r.Context(), actor))
}

func serveIdempotent(t *testing.T, g *guard, r *http.Request, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	routes := NewMux()
	routes.HandleFunc(http.MethodPost+" "+APIBasePath+"/containers", handler)
	routes.HandleFunc(http.MethodGet+" "+APIBasePath+"/containers", handler)

	response := httptest.NewRecorder()
	Idempotent{Guard: g, Routes: routes, Next: routes}.ServeHTTP(response, r)
	return response
}

func created(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"id":"018f0000-0000-7000-8000-000000000001"}`))
}

func TestAFirstKeyedPostRunsAndIsStored(t *testing.T) {
	g := &guard{}

	response := serveIdempotent(t, g, idempotentRequest(t, http.MethodPost, `{"name":"a"}`, true), created)

	if response.Code != http.StatusCreated {
		t.Fatalf("status %d", response.Code)
	}
	if !g.completed || g.gotStatus != http.StatusCreated {
		t.Errorf("stored %v %d", g.completed, g.gotStatus)
	}
	if !json.Valid(g.gotBody) {
		t.Errorf("stored body = %q", g.gotBody)
	}
	// The endpoint half of the key is the route template, not the path.
	if g.beganWith.Endpoint != http.MethodPost+" "+APIBasePath+"/containers" {
		t.Errorf("endpoint = %q", g.beganWith.Endpoint)
	}
}

func TestARepeatIsAnsweredFromTheRecordWithoutRunning(t *testing.T) {
	g := &guard{replay: &repository.Record{Status: http.StatusCreated, Body: []byte(`{"id":"x"}`)}}
	ran := false

	response := serveIdempotent(t, g, idempotentRequest(t, http.MethodPost, `{"name":"a"}`, true),
		func(http.ResponseWriter, *http.Request) { ran = true })

	if ran {
		t.Error("the operation ran a second time")
	}
	if response.Code != http.StatusCreated {
		t.Fatalf("status %d", response.Code)
	}
	if response.Body.String() != `{"id":"x"}` {
		t.Errorf("body = %q", response.Body.String())
	}
	if response.Header().Get(ReplayedHeader) != "true" {
		t.Error("a replayed answer is not marked as one")
	}
}

// The body reaches the handler even though the middleware had to read it to hash it.
func TestTheHandlerStillSeesTheBody(t *testing.T) {
	var seen bytes.Buffer

	serveIdempotent(t, &guard{}, idempotentRequest(t, http.MethodPost, `{"name":"a"}`, true),
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = seen.ReadFrom(r.Body)
			w.WriteHeader(http.StatusCreated)
		})

	if seen.String() != `{"name":"a"}` {
		t.Errorf("the handler read %q", seen.String())
	}
}

func TestOnlyKeyedPostsAreGuarded(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		withKey bool
	}{
		{"a POST without a key", http.MethodPost, false},
		{"a GET with a key", http.MethodGet, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := &guard{}
			serveIdempotent(t, g, idempotentRequest(t, c.method, "", c.withKey), created)

			if g.beginCalls != 0 {
				t.Error("the guard was consulted")
			}
		})
	}
}

// The contract types the header as a UUID. A key a client can guess is a way to read another
// client's answer inside the same tenant.
func TestAKeyThatIsNotAUUIDIsRefused(t *testing.T) {
	for _, key := range []string{"not-a-uuid", "12345", strings.Repeat("a", 36), ""} {
		g := &guard{}
		r := idempotentRequest(t, http.MethodPost, `{}`, false)
		if key != "" {
			r.Header.Set(IdempotencyKeyHeader, key)
		}

		response := serveIdempotent(t, g, r, created)

		if key == "" {
			// No key at all is not an error - the header is optional.
			continue
		}
		if response.Code != http.StatusUnprocessableEntity {
			t.Errorf("key %q: status %d, want 422", key, response.Code)
		}
		if g.beginCalls != 0 {
			t.Errorf("key %q: the guard was consulted with a malformed key", key)
		}
	}
}

func TestAnUppercaseUUIDIsAccepted(t *testing.T) {
	g := &guard{}
	r := idempotentRequest(t, http.MethodPost, `{}`, false)
	r.Header.Set(IdempotencyKeyHeader, strings.ToUpper(idempotencyKey))

	serveIdempotent(t, g, r, created)

	if g.beginCalls != 1 {
		t.Error("an upper-case UUID was refused")
	}
}

func TestAConflictFromTheGuardReachesTheClient(t *testing.T) {
	g := &guard{err: shared.ErrConflict.WithDetail("idempotency.key_reused")}
	ran := false

	response := serveIdempotent(t, g, idempotentRequest(t, http.MethodPost, `{}`, true),
		func(http.ResponseWriter, *http.Request) { ran = true })

	if ran {
		t.Error("the operation ran despite the conflict")
	}
	if response.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", response.Code)
	}
	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.DetailCode != "idempotency.key_reused" {
		t.Errorf("detail code %q", problem.DetailCode)
	}
}

// A request without an actor has no tenant, and a record has nowhere to live. Unreachable behind
// the authentication middleware, and refused rather than assumed.
func TestAKeyedPostWithoutAnActorIsRefused(t *testing.T) {
	g := &guard{}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/containers", strings.NewReader("{}"))
	r.Header.Set(IdempotencyKeyHeader, idempotencyKey)

	response := serveIdempotent(t, g, r, created)

	if response.Code != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", response.Code)
	}
	if g.beginCalls != 0 {
		t.Error("the guard was consulted without a tenant")
	}
}

// The same body under the same key at a different operation must not be recognised as a repeat.
func TestTheHashCoversTheOperationAsWellAsTheBody(t *testing.T) {
	first := requestHash(http.MethodPost, "POST "+APIBasePath+"/containers", []byte(`{"name":"a"}`))
	elsewhere := requestHash(http.MethodPost, "POST "+APIBasePath+"/items", []byte(`{"name":"a"}`))
	otherBody := requestHash(http.MethodPost, "POST "+APIBasePath+"/containers", []byte(`{"name":"b"}`))

	if bytes.Equal(first, elsewhere) {
		t.Error("two operations hash the same")
	}
	if bytes.Equal(first, otherBody) {
		t.Error("two bodies hash the same")
	}
	if !bytes.Equal(first, requestHash(http.MethodPost, "POST "+APIBasePath+"/containers", []byte(`{"name":"a"}`))) {
		t.Error("the same request hashes differently twice")
	}
}

// A response too large to store is answered normally and simply not kept: a truncated body
// replayed as a whole one would be worse than no replay at all.
func TestAnOversizedAnswerIsNotStored(t *testing.T) {
	g := &guard{}
	big := strings.Repeat("x", maxStoredResponseBytes+1)

	response := serveIdempotent(t, g, idempotentRequest(t, http.MethodPost, `{}`, true),
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(big))
		})

	if response.Body.Len() != len(big) {
		t.Errorf("the client received %d bytes of %d", response.Body.Len(), len(big))
	}
	if len(g.gotBody) != 0 {
		t.Errorf("%d bytes were stored", len(g.gotBody))
	}
	if g.gotStatus != http.StatusCreated {
		t.Errorf("stored status %d", g.gotStatus)
	}
}

// A key sent to a path that matches no route must not reserve anything: the record would name an
// empty operation, and the client's next attempt at a real route would collide with it.
func TestAKeyOnAnUnknownRouteReservesNothing(t *testing.T) {
	g := &guard{}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/nothing-here",
		strings.NewReader("{}"))
	r.Header.Set(IdempotencyKeyHeader, idempotencyKey)
	actor := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantID}

	response := serveIdempotent(t, g, r.WithContext(appshared.ContextWithActor(r.Context(), actor)), created)

	if g.beginCalls != 0 {
		t.Error("a key was reserved against an unknown route")
	}
	if response.Code != http.StatusNotFound {
		t.Errorf("status %d, want the router's 404", response.Code)
	}
}
