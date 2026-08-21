// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package idempotency

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/idempotency"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant = shared.ID("018f2a1b-0000-7000-8000-0000000000ab")
	key    = repository.Key{Key: "5f9d1f8e-0000-4000-8000-000000000001", Endpoint: "POST /api/v1/containers"}
	hash   = []byte("the-request-hash")
)

type unitOfWork struct{ scopes []persistence.Scope }

func (u *unitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, scope, fn)
}

type store struct {
	existing repository.Record
	reserved bool
	findErr  error

	completedStatus int
	completedBody   []byte
	completeCalls   int
}

func (s *store) Reserve(context.Context, repository.Key, []byte) (repository.Record, bool, error) {
	return s.existing, s.reserved, s.findErr
}

func (s *store) Complete(_ context.Context, _ repository.Key, status int, body []byte) error {
	s.completeCalls++
	s.completedStatus, s.completedBody = status, body
	return nil
}

func actor() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant}
}

func TestAFirstAttemptRuns(t *testing.T) {
	uow := &unitOfWork{}
	guard := Guard{Store: &store{reserved: true}, UnitOfWork: uow}

	attempt, err := guard.Begin(t.Context(), actor(), key, hash)
	if err != nil {
		t.Fatalf("begin failed: %v", err)
	}
	if attempt.IsReplay() {
		t.Error("a fresh key was answered as a replay")
	}
	if len(uow.scopes) != 1 || uow.scopes[0].TenantID != tenant {
		t.Errorf("scopes = %+v", uow.scopes)
	}
}

func TestARepeatOfTheSameRequestIsReplayed(t *testing.T) {
	guard := Guard{
		Store: &store{existing: repository.Record{
			RequestHash: hash, Status: 201, Body: []byte(`{"id":"x"}`),
		}},
		UnitOfWork: &unitOfWork{},
	}

	attempt, err := guard.Begin(t.Context(), actor(), key, hash)
	if err != nil {
		t.Fatalf("begin failed: %v", err)
	}
	if !attempt.IsReplay() {
		t.Fatal("the repeat was not recognised")
	}
	if attempt.Replay.Status != 201 || string(attempt.Replay.Body) != `{"id":"x"}` {
		t.Errorf("replay = %+v", attempt.Replay)
	}
}

// The same key with a different body is a client bug. Answering it with the first request's
// result would confirm something the client never asked for.
func TestTheSameKeyForADifferentRequestIsAConflict(t *testing.T) {
	guard := Guard{
		Store:      &store{existing: repository.Record{RequestHash: []byte("a-different-hash"), Status: 201}},
		UnitOfWork: &unitOfWork{},
	}

	_, err := guard.Begin(t.Context(), actor(), key, hash)
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
	var domainErr *shared.Error
	if errors.As(err, &domainErr) && domainErr.DetailCode != "idempotency.key_reused" {
		t.Errorf("detail = %q", domainErr.DetailCode)
	}
}

// Two requests arriving together: one reserves, the other finds a record with no answer yet. It
// is a race, not a repeat, and answering it from an empty record would invent a response.
func TestARequestArrivingDuringTheFirstIsAConflict(t *testing.T) {
	guard := Guard{
		Store:      &store{existing: repository.Record{RequestHash: hash}},
		UnitOfWork: &unitOfWork{},
	}

	_, err := guard.Begin(t.Context(), actor(), key, hash)
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
	var domainErr *shared.Error
	if errors.As(err, &domainErr) && domainErr.DetailCode != "idempotency.in_progress" {
		t.Errorf("detail = %q", domainErr.DetailCode)
	}
}

func TestAnAnswerIsStored(t *testing.T) {
	store := &store{}
	guard := Guard{Store: store, UnitOfWork: &unitOfWork{}}

	if err := guard.Complete(t.Context(), actor(), key, 201, []byte(`{"id":"x"}`)); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if store.completedStatus != 201 || string(store.completedBody) != `{"id":"x"}` {
		t.Errorf("stored %d %q", store.completedStatus, store.completedBody)
	}
}

// A 5xx is not a decision the server stands behind. Storing it would turn one bad minute into a
// permanent answer for that key.
func TestAServerFailureIsNotStored(t *testing.T) {
	store := &store{}
	guard := Guard{Store: store, UnitOfWork: &unitOfWork{}}

	for _, status := range []int{500, 502, 503} {
		if err := guard.Complete(t.Context(), actor(), key, status, nil); err != nil {
			t.Fatalf("complete failed: %v", err)
		}
	}
	if store.completeCalls != 0 {
		t.Errorf("%d server failures were stored", store.completeCalls)
	}
}

// A 4xx is a decision: the same request will be refused the same way, so a repeat may as well be
// answered from the record.
func TestAClientErrorIsStored(t *testing.T) {
	store := &store{}
	guard := Guard{Store: store, UnitOfWork: &unitOfWork{}}

	if err := guard.Complete(t.Context(), actor(), key, 422, []byte(`{"code":"validation_failed"}`)); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if store.completeCalls != 1 {
		t.Error("a client error was not stored")
	}
}

func TestAStoreFailureSurfaces(t *testing.T) {
	guard := Guard{
		Store:      &store{findErr: shared.ErrUnavailable},
		UnitOfWork: &unitOfWork{},
	}

	if _, err := guard.Begin(t.Context(), actor(), key, hash); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("error = %v", err)
	}
}
