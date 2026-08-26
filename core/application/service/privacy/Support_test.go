// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The doubles the privacy use cases are tested against. Nothing here reaches a database: what is
// under test is which permission a step asks for, what the state machine allows, and what the
// trail records - all three decisions of the application layer.

var (
	tenantID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	accountID = shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	subjectID = shared.MustParseID("0192f000-0000-7000-8000-0000000000a3")
	targetID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000a4")
	now       = time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
)

func actor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
		AccountName: "Anna Beispiel", Scopes: []string{privacyRead, privacyManage},
	}
}

// operator is an actor whose credential carries the instance scope - the bound on the one
// operation that crosses the tenant boundary.
func operator() appshared.ActorContext {
	person := actor()
	person.Scopes = append(person.Scopes, instanceScope)
	return person
}

// requestStore is the cases in memory.
type requestStore struct {
	stored    map[shared.ID]domain.Request
	order     []shared.ID
	asked     []repository.Filter
	deadlines repository.Deadlines
	missing   bool
	err       error
}

func newRequestStore() *requestStore {
	return &requestStore{stored: map[shared.ID]domain.Request{}}
}

func (s *requestStore) Insert(_ context.Context, request domain.Request) error {
	if s.err != nil {
		return s.err
	}
	s.stored[request.ID] = request
	s.order = append(s.order, request.ID)
	return nil
}

func (s *requestStore) Find(_ context.Context, id shared.ID) (domain.Request, error) {
	request, found := s.stored[id]
	if !found {
		return domain.Request{}, shared.ErrNotFound.WithDetail(domain.CodeRequestNotFound)
	}
	return request, nil
}

func (s *requestStore) Save(_ context.Context, request domain.Request) (bool, error) {
	if s.missing {
		return false, nil
	}
	s.stored[request.ID] = request
	return true, nil
}

func (s *requestStore) List(_ context.Context, filter repository.Filter) (repository.Page, error) {
	s.asked = append(s.asked, filter)

	var out []domain.Request
	for _, id := range s.order {
		request := s.stored[id]
		if !filter.IncludeClosed && request.Status.Closed() {
			continue
		}
		if !filter.DueBefore.IsZero() && !request.DueAt.Before(filter.DueBefore) {
			continue
		}
		out = append(out, request)
	}
	return repository.Page{Requests: out}, nil
}

func (s *requestStore) Deadlines(context.Context, time.Time) (repository.Deadlines, error) {
	return s.deadlines, nil
}

// authorizerDouble records what it was asked and answers what the test told it to.
type authorizerDouble struct {
	refuse   error
	requests []access.Request
}

func (a *authorizerDouble) Authorize(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) error {
	a.requests = append(a.requests, request)
	return a.refuse
}

type queueDouble struct {
	requests []queue.Request
	err      error
}

func (q *queueDouble) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	q.requests = append(q.requests, request)
	if q.err != nil {
		return "", q.err
	}
	return shared.MustParseID("0192f000-0000-7000-8000-0000000000c1"), nil
}

type auditSink struct{ entries []audit.Entry }

func (s *auditSink) Append(_ context.Context, entry audit.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	s.entries = append(s.entries, entry)
	return nil
}

type unitOfWork struct{ reads, writes int }

func (u *unitOfWork) Within(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	u.writes++
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	u.reads++
	return fn(ctx)
}

type idSource struct{ issued int }

func (i *idSource) NewID() shared.ID {
	i.issued++
	return shared.MustParseID("0192f000-0000-7000-8000-00000000010" + string(rune('0'+i.issued)))
}
