// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The doubles the audit use cases are tested against. Nothing here reaches a database: what is
// under test is who may read what, what is narrowed, and what is refused - and all three are
// decisions of the application layer.

var (
	tenantID    = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	accountID   = shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	colleagueID = shared.MustParseID("0192f000-0000-7000-8000-0000000000a3")
	targetID    = shared.MustParseID("0192f000-0000-7000-8000-0000000000a4")
	now         = time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
)

func actor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
		AccountName: "Anna Beispiel", Scopes: []string{auditRead, auditExport},
	}
}

// trailStore is the trail in memory: it keeps the filter it was asked with, because what this
// package decides is mostly *the filter* rather than the answer.
type trailStore struct {
	records []repository.Record
	asked   []repository.Filter
	walked  []repository.Period
	anchor  repository.Anchor
	info    repository.PageInfo
	err     error
}

func (t *trailStore) Query(_ context.Context, filter repository.Filter) (repository.RecordPage, error) {
	t.asked = append(t.asked, filter)
	if t.err != nil {
		return repository.RecordPage{}, t.err
	}
	return repository.RecordPage{Records: t.records, Info: t.info}, nil
}

func (t *trailStore) Walk(
	_ context.Context, period repository.Period, yield func(repository.Record) error,
) error {
	t.walked = append(t.walked, period)
	for _, record := range t.records {
		if err := yield(record); err != nil {
			return err
		}
	}
	return nil
}

func (t *trailStore) LatestAnchor(context.Context) (repository.Anchor, error) {
	return t.anchor, nil
}

// authorizerDouble answers both halves of the port and records what it was asked, so that a test
// can assert the permission the use case asked for rather than only the outcome.
type authorizerDouble struct {
	// permits is what Permits answers: whether this actor may read the whole trail.
	permits bool
	err     error
	// refuse is the error Authorize answers with, nil for a permitted request.
	refuse   error
	requests []access.Request
	permitOn []access.Request
}

func (a *authorizerDouble) Authorize(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) error {
	a.requests = append(a.requests, request)
	return a.refuse
}

func (a *authorizerDouble) Permits(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) (bool, error) {
	a.permitOn = append(a.permitOn, request)
	return a.permits, a.err
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

// record builds one stored entry, with the fields a projection is asserted on.
func record(id string, seq int64, change func(*repository.Record)) repository.Record {
	stored := repository.Record{
		ID:  shared.MustParseID(id),
		Seq: seq,
		Entry: port.Entry{
			TenantID: tenantID, OccurredAt: now, Action: "container.created",
			Outcome: port.OutcomeSuccess, Severity: port.SeverityInfo,
			ActorKind: shared.ActorKind(appshared.ActorUser), ActorID: accountID,
			ActorLabel: "Anna Beispiel",
			TargetType: "container", TargetID: targetID,
			Context: port.Context{RequestID: "req-1", IPTruncated: "198.51.100.0/24"},
			Changes: port.Changes(
				port.Change{Field: "status", Classification: port.Open, From: "OPEN", To: "DONE"},
				port.Change{Field: "title", Classification: port.Sensitive, To: "Weekly shop"},
				port.Change{Field: "token", Classification: port.Secret, To: "hbt_pat_secret"},
			),
		},
		Hash: []byte{0xab, 0xcd},
	}
	change(&stored)
	return stored
}

// wholeTrailRequest is what the use cases ask for when the whole trail is meant.
func wholeTrailRequest() access.Request { return wholeTrail(actor()) }

// permissionOf is a shorthand for the assertions below.
func permissionOf(request access.Request) service.Permission { return request.Permission }

// auditSink is the trail's writing half, in memory. The audit use cases write exactly one kind of
// entry between them - a break somebody found, and an export somebody took - so what a test asks
// it is mostly "how many", and the answer is usually none.
type auditSink struct{ entries []port.Entry }

func (s *auditSink) Append(_ context.Context, entry port.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	s.entries = append(s.entries, entry)
	return nil
}
