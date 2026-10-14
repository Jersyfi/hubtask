// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/port/audit"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// deniedEntries counts the DENIED entries the trail holds about one actor and one target.
func deniedEntries(ctx context.Context, t *testing.T, tenant, actor, target shared.ID) int {
	t.Helper()
	var count int
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		WHERE tenant_id = $1 AND actor_id = $2 AND target_id = $3 AND outcome = 'DENIED'`,
		tenant.String(), actor.String(), target.String()).Scan(&count); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	return count
}

// audit.md §7, UC-ID-16 check 2: a refusal is in the trail even when the reader that was refused
// was asked from inside another reader's read-only transaction. ListSuggestions reads its target
// through the catalogue inside one; the target's own reader refuses the workspace's owner, who
// holds nothing in the private hub, and its DENIED entry used to fail there
// (`cannot execute INSERT in a read-only transaction`) and be lost.
func TestARefusalInsideAReadOnlyReadIsInTheTrail(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	registry := readerRegistry(ctx, t)
	owner := reader(w.tenant, w.administrator)

	before := deniedEntries(ctx, t, w.tenant, w.administrator, w.item)
	_, err := registry.Invoke(ctx, suggestion.ListSuggestionsName, owner, usecase.Input{
		"target_type": string(suggestiondomain.TargetWorkItem), "target_id": w.item.String(),
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the owner was answered %v, want not found", err)
	}

	if got := deniedEntries(ctx, t, w.tenant, w.administrator, w.item) - before; got != 1 {
		t.Errorf("the trail gained %d DENIED entries about the entry, want one", got)
	}
}

// The port's two halves against PostgreSQL: beside a read-only transaction the write commits on its
// own, whatever the read does afterwards; inside a read-write one it joins and shares its fate -
// opening a second transaction there could wait for ever on a lock its own caller holds.
func TestWithinWritableStandsBesideAReadAndJoinsAWrite(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	sink := postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed))
	scope := persistence.Scope{TenantID: tenantA}
	readFailed := errors.New("the read failed after the write")

	entry := func(target shared.ID) audit.Entry {
		return audit.Entry{
			TenantID: tenantA, OccurredAt: created, Action: "item.read", Outcome: audit.OutcomeDenied,
			Severity: audit.SeverityNotice, ActorKind: appshared.ActorUser, ActorID: freshID(t),
			TargetType: "work_item", TargetID: target,
		}
	}
	stored := func(target shared.ID) int {
		var count int
		if err := adminPool(ctx, t).QueryRow(ctx,
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND target_id = $2`,
			tenantA.String(), target.String()).Scan(&count); err != nil {
			t.Fatalf("reading the trail: %v", err)
		}
		return count
	}

	beside := freshID(t)
	err := unitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		if err := unitOfWork.WithinWritable(ctx, scope, func(ctx context.Context) error {
			return sink.Append(ctx, entry(beside))
		}); err != nil {
			t.Fatalf("writing beside the read: %v", err)
		}
		return readFailed
	})
	if !errors.Is(err, readFailed) {
		t.Fatalf("the read answered %v", err)
	}
	if got := stored(beside); got != 1 {
		t.Errorf("beside a read-only transaction: %d entries stored, want one", got)
	}

	joined := freshID(t)
	err = unitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := unitOfWork.WithinWritable(ctx, scope, func(ctx context.Context) error {
			return sink.Append(ctx, entry(joined))
		}); err != nil {
			t.Fatalf("writing inside the write: %v", err)
		}
		return readFailed
	})
	if !errors.Is(err, readFailed) {
		t.Fatalf("the write answered %v", err)
	}
	if got := stored(joined); got != 0 {
		t.Errorf("inside a read-write transaction that rolled back: %d entries stored, want none", got)
	}
}

// suggestionCases is the suggestion use cases' parts the way main.go builds them, reading their
// target through the readers' catalogue.
func suggestionCases(ctx context.Context, t *testing.T) suggestion.Cases {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	sink := postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed))
	return suggestion.Cases{
		Suggestions: postgres.NewSuggestionRepository(pageCursors()),
		Targets:     suggestion.EntryTargets{Catalogue: readerRegistry(ctx, t)},
		Authorizer: access.Service{
			Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork,
			Audit: sink, Clock: fixed,
		},
		Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
	}
}

// aiOffered answers that the workspace may ask, so that a question reaches the read of its target.
type aiOffered struct{}

func (aiOffered) CanSuggest(context.Context, appshared.ActorContext) (bool, error) { return true, nil }

// jobsNever fails a test that reaches the queue: a refused question queues nothing.
type jobsNever struct{ t *testing.T }

func (j jobsNever) Enqueue(context.Context, queue.Request) (shared.ID, error) {
	j.t.Error("a refused question was queued")
	return "", errors.New("queued")
}

// audit.md §7: asking about an entry the asker cannot read is refused as not found, and the
// target's reader's DENIED entry stays in the trail - it used to join the asking's read-write
// transaction and be rolled back with the refusal.
func TestARefusedQuestionIsInTheTrail(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	owner := reader(w.tenant, w.administrator)
	owner.Scopes = append(owner.Scopes, "items:write", "suggestions:write")

	before := deniedEntries(ctx, t, w.tenant, w.administrator, w.item)
	err := suggestion.SuggestDecomposition{
		Cases: suggestionCases(ctx, t), AI: aiOffered{}, Queue: jobsNever{t},
	}.Execute(ctx, owner, w.item)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the owner was answered %v, want not found", err)
	}

	if got := deniedEntries(ctx, t, w.tenant, w.administrator, w.item) - before; got != 1 {
		t.Errorf("the trail gained %d DENIED entries about the entry, want one", got)
	}
}

// The same for answering a proposal about an entry the person cannot read: dismissing it reads the
// entry for its freshness, and the reader's refusal used to be rolled back with the answer.
func TestARefusedAnswerIsInTheTrail(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	cases := suggestionCases(ctx, t)
	owner := reader(w.tenant, w.administrator)
	owner.Scopes = append(owner.Scopes, "items:write", "suggestions:write")

	proposal, err := suggestiondomain.New(suggestiondomain.NewInput{
		ID: freshID(t), TenantID: w.tenant,
		TargetType: suggestiondomain.TargetWorkItem, TargetID: w.item,
		Kind:    suggestiondomain.KindFields,
		Payload: map[string]any{"title": "Buy oat milk"},
		Provenance: suggestiondomain.Provenance{
			Model: "a-model", PromptID: "suggest-fields", PromptVersion: "v1", ProducedAt: created,
		},
		InputDigest: suggestiondomain.Digest(w.title, ""),
		Now:         created,
	})
	if err != nil {
		t.Fatalf("building the proposal: %v", err)
	}
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		return cases.Suggestions.Record(ctx, proposal)
	}); err != nil {
		t.Fatalf("recording the proposal: %v", err)
	}

	before := deniedEntries(ctx, t, w.tenant, w.administrator, w.item)
	_, err = suggestion.DismissSuggestion{Cases: cases}.Execute(ctx, owner, proposal.ID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the owner was answered %v, want not found", err)
	}

	if got := deniedEntries(ctx, t, w.tenant, w.administrator, w.item) - before; got != 1 {
		t.Errorf("the trail gained %d DENIED entries about the entry, want one", got)
	}
}

// audit.md §7: accepting a proposal applies it through the write use case inside the answer's
// read-write transaction, and that use case's refusal rolls the transaction back. Its DENIED entry
// used to go with it. A workspace member who only views the private hub may answer proposals as far
// as the workspace goes and may read the entry, but UpdateWorkItem refuses her the change.
func TestARefusedAcceptanceIsInTheTrail(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	viewer := freshID(t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Vera Viewer')`,
			[]any{viewer.String(), w.tenant.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		  VALUES ($1, $2, $3, 'TENANT', 'MEMBER')`,
			[]any{freshID(t).String(), w.tenant.String(), viewer.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		  VALUES ($1, $2, $3, 'HUB', $4, 'VIEWER')`,
			[]any{freshID(t).String(), w.tenant.String(), viewer.String(), w.hub.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the viewer: %v", err)
		}
	}

	catalogue := patchCatalogueFor(ctx, t)
	cases := suggestionCases(ctx, t)
	cases.Targets = suggestion.EntryTargets{Catalogue: catalogue}
	cases.Catalogue = catalogue
	actor := reader(w.tenant, viewer)
	actor.Scopes = append(actor.Scopes, "items:write")

	proposal, err := suggestiondomain.New(suggestiondomain.NewInput{
		ID: freshID(t), TenantID: w.tenant,
		TargetType: suggestiondomain.TargetWorkItem, TargetID: w.item,
		Kind:    suggestiondomain.KindFields,
		Payload: map[string]any{"title": "Buy oat milk"},
		Provenance: suggestiondomain.Provenance{
			Model: "a-model", PromptID: "suggest-fields", PromptVersion: "v1", ProducedAt: created,
		},
		InputDigest: suggestiondomain.Digest(w.title, ""),
		Now:         created,
	})
	if err != nil {
		t.Fatalf("building the proposal: %v", err)
	}
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		return cases.Suggestions.Record(ctx, proposal)
	}); err != nil {
		t.Fatalf("recording the proposal: %v", err)
	}

	before := deniedEntries(ctx, t, w.tenant, viewer, w.item)
	_, err = suggestion.AcceptSuggestion{Cases: cases}.Execute(ctx, actor, proposal.ID, nil)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the hub's viewer was answered %v, want forbidden", err)
	}

	if got := deniedEntries(ctx, t, w.tenant, viewer, w.item) - before; got != 1 {
		t.Errorf("the trail gained %d DENIED entries about the entry, want one", got)
	}
}
