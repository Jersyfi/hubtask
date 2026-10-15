// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// The erasures of a rewound period, carried out again (backup-restore.md §8.5 step 5). What is
// under test is what the recovered workspace lets the operator's input do: a case it holds must be
// the same erasure, one it lacks must be one a rewind could have taken, and nothing runs without a
// fresh step-up.

// recoveredStore is the recovered workspace's cases and accounts, sharing the cases with the
// erasure's own store.
type recoveredStore struct {
	cases    *requestStore
	accounts map[shared.ID]bool
}

func (r *recoveredStore) InsertAsItWas(_ context.Context, request domain.Request) (bool, error) {
	if _, held := r.cases.stored[request.ID]; held {
		return false, nil
	}
	r.cases.stored[request.ID] = request
	return true, nil
}

func (r *recoveredStore) CompleteIfOpen(
	_ context.Context, id shared.ID, mode domain.ErasureMode, completedAt time.Time, subject shared.ID,
) (bool, error) {
	held, found := r.cases.stored[id]
	if !found || (held.Status != domain.StatusReceived && held.Status != domain.StatusInProgress) {
		return false, nil
	}
	held.Status, held.ErasureMode, held.CompletedAt, held.SubjectAccountID =
		domain.StatusCompleted, mode, completedAt, subject
	r.cases.stored[id] = held
	return true, nil
}

func (r *recoveredStore) SubjectExists(_ context.Context, accountID shared.ID) (bool, error) {
	return r.accounts[accountID], nil
}

type workspaceThere struct{ gone bool }

func (w workspaceThere) Find(context.Context) (adminrepo.TenantRecord, error) {
	if w.gone {
		return adminrepo.TenantRecord{}, shared.ErrNotFound
	}
	return adminrepo.TenantRecord{ID: tenantID}, nil
}

// proof accepts one token.
type proof struct{ consumed int }

func (p *proof) Available() bool { return true }

func (p *proof) Methods(context.Context, shared.ID, shared.ID) ([]stepup.Method, error) {
	return []stepup.Method{stepup.MethodPassword}, nil
}

func (p *proof) Satisfied(_ context.Context, _ shared.ID, token string) (bool, error) {
	if token != "fresh" {
		return false, nil
	}
	p.consumed++
	return true, nil
}

var (
	recoveredTo  = now.Add(-72 * time.Hour)
	reappliedID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	strangerID   = shared.MustParseID("0192f000-0000-7000-8000-0000000000b9")
	operatorHome = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
)

type reapplyFixture struct {
	harness   *erasureHarness
	cases     *requestStore
	recovered *recoveredStore
	proof     *proof
	handler   ReapplyErasures
}

func newReapplyFixture() *reapplyFixture {
	f := &reapplyFixture{harness: newErasureHarness(), cases: newRequestStore(), proof: &proof{}}
	f.recovered = &recoveredStore{cases: f.cases, accounts: map[shared.ID]bool{subjectID: true}}
	f.handler = ReapplyErasures{
		Requests: f.cases, Recovered: f.recovered, Eraser: f.harness.eraserFor(f.cases),
		Workspaces: workspaceThere{}, StepUp: f.proof, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
	return f
}

// operatorOf is the operator's credential: from another workspace, with the control plane's scope.
func operatorOf() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: operatorHome, AccountID: accountID,
		AccountName: "Operator", Scopes: []string{reapplyScope},
	}
}

// lostCase is a full deletion received and completed after the recovery point.
func lostCase(mode domain.ErasureMode) domain.Request {
	return domain.Request{
		ID: reappliedID, ErasureMode: mode, SubjectAccountID: subjectID,
		ReceivedAt: recoveredTo.Add(time.Hour), DueAt: recoveredTo.Add(30 * 24 * time.Hour),
		CompletedAt: recoveredTo.Add(2 * time.Hour), Notes: "asked by mail",
	}
}

func (f *reapplyFixture) run(t *testing.T, records ...domain.Request) []Reapplied {
	t.Helper()
	results, err := f.handler.Execute(context.Background(), operatorOf(), ReapplyErasuresCommand{
		TenantID: tenantID, RecoveryPoint: recoveredTo, Erasures: records, StepUpToken: "fresh",
	})
	if err != nil {
		t.Fatalf("re-applying: %v", err)
	}
	if len(results) != len(records) {
		t.Fatalf("answered %d cases for %d", len(results), len(records))
	}
	return results
}

// A case the rewind took is written again, its erasure runs, and it is completed at its own moment;
// a second run finds it completed and does nothing.
func TestAnErasureTheRewindTookIsCarriedOutAgain(t *testing.T) {
	f := newReapplyFixture()

	got := f.run(t, lostCase(domain.ModeFullDelete))
	if got[0].Outcome != ErasureReapplied || got[0].Kept {
		t.Fatalf("answered %+v", got[0])
	}
	if !f.harness.storage.deleted {
		t.Error("the account was not removed again")
	}
	held := f.cases.stored[reappliedID]
	if held.Status != domain.StatusCompleted || !held.CompletedAt.Equal(lostCase("").CompletedAt) ||
		!held.SubjectAccountID.IsZero() || held.Notes != "asked by mail" {
		t.Errorf("the case reads %+v", held)
	}
	if len(f.harness.audit.entries) != 1 {
		t.Fatalf("%d trail entries", len(f.harness.audit.entries))
	}
	entry := f.harness.audit.entries[0]
	if entry.Action != ErasureReappliedAction || entry.TenantID != tenantID || entry.TargetID != reappliedID ||
		entry.ActorID != accountID {
		t.Errorf("the entry is %+v", entry)
	}
	if _, recorded := entry.Changes["recovery_point"]; !recorded {
		t.Error("the entry does not name the recovery it belongs to")
	}

	again := f.run(t, lostCase(domain.ModeFullDelete))
	if again[0].Outcome != ErasureAlreadyCompleted || len(f.harness.audit.entries) != 1 {
		t.Errorf("a second run answered %+v and wrote %d entries", again[0], len(f.harness.audit.entries))
	}
}

// A case the recovered workspace holds open - received before the point - is carried out and
// completed; one it holds rejected is a person's to decide.
func TestAHeldCaseIsCarriedOutOrLeftToItsOwner(t *testing.T) {
	f := newReapplyFixture()
	open := lostCase(domain.ModeAnonymize)
	open.Kind, open.Status, open.Scope = domain.KindErasure, domain.StatusInProgress, domain.ScopeTenant
	open.ReceivedAt, open.CompletedAt = recoveredTo.Add(-time.Hour), time.Time{}
	f.cases.stored[open.ID] = open

	record := lostCase(domain.ModeAnonymize)
	record.ReceivedAt = open.ReceivedAt
	got := f.run(t, record)
	if got[0].Outcome != ErasureReapplied || !f.harness.storage.anonymised {
		t.Fatalf("answered %+v, anonymised %v", got[0], f.harness.storage.anonymised)
	}
	if held := f.cases.stored[open.ID]; held.SubjectAccountID != subjectID {
		t.Errorf("an anonymised person's case stopped naming them: %+v", held)
	}

	g := newReapplyFixture()
	rejected := open
	rejected.Status = domain.StatusRejected
	g.cases.stored[rejected.ID] = rejected
	if got := g.run(t, record); got[0].Outcome != ErasureCaseRejected || g.harness.storage.anonymised {
		t.Errorf("a rejected case answered %+v", got[0])
	}
}

// What the recovered workspace contradicts, or a rewind could not have taken, is left alone -
// the guard against a crafted file erasing a person who never asked.
func TestACaseTheWorkspaceContradictsIsLeftAlone(t *testing.T) {
	held := lostCase(domain.ModeFullDelete)
	held.Kind, held.Status, held.Scope = domain.KindErasure, domain.StatusInProgress, domain.ScopeTenant
	held.ReceivedAt = recoveredTo.Add(-time.Hour)

	cases := map[string]struct {
		held   *domain.Request
		record func(domain.Request) domain.Request
	}{
		"another subject than the held case": {&held, func(r domain.Request) domain.Request {
			r.SubjectAccountID = strangerID
			return r
		}},
		"another mode than the held case": {&held, func(r domain.Request) domain.Request {
			r.ErasureMode = domain.ModeAnonymize
			return r
		}},
		"a held case of another kind": {func() *domain.Request {
			access := held
			access.Kind = domain.KindAccess
			return &access
		}(), func(r domain.Request) domain.Request { return r }},
		"a lost case received before the point": {nil, func(r domain.Request) domain.Request {
			r.ReceivedAt = recoveredTo.Add(-time.Minute)
			return r
		}},
		"a case completed before the point": {nil, func(r domain.Request) domain.Request {
			r.CompletedAt = recoveredTo.Add(-time.Minute)
			return r
		}},
		"a case completed in the future": {nil, func(r domain.Request) domain.Request {
			r.CompletedAt = now.Add(time.Minute)
			return r
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newReapplyFixture()
			if c.held != nil {
				f.cases.stored[c.held.ID] = *c.held
			}
			got := f.run(t, c.record(lostCase(domain.ModeFullDelete)))
			if got[0].Outcome != ErasureCaseMismatch || got[0].Code != CodeErasureRecordMismatch {
				t.Errorf("answered %+v", got[0])
			}
			if f.harness.storage.deleted || len(f.harness.audit.entries) != 0 {
				t.Error("a contradicted case was erased")
			}
		})
	}
}

// An account the recovered workspace lacks has nothing to erase; the case is answered.
func TestAnAbsentSubjectCompletesTheCaseWithNothingErased(t *testing.T) {
	f := newReapplyFixture()
	record := lostCase(domain.ModeFullDelete)
	record.SubjectAccountID = strangerID

	got := f.run(t, record)
	if got[0].Outcome != ErasureSubjectAbsent || f.harness.storage.deleted {
		t.Fatalf("answered %+v", got[0])
	}
	if held := f.cases.stored[reappliedID]; held.Status != domain.StatusCompleted || !held.SubjectAccountID.IsZero() {
		t.Errorf("the case reads %+v", held)
	}
}

// The erasure's own refusal - a rule still runs as the person - leaves that case open and the
// others going.
func TestABlockedErasureLeavesItsCaseOpenAndTheOthersGoOn(t *testing.T) {
	f := newReapplyFixture()
	f.harness.storage.runningRules = 1
	other := lostCase(domain.ModeFullDelete)
	other.ID, other.SubjectAccountID = shared.MustParseID("0192f000-0000-7000-8000-0000000000d2"), strangerID

	got := f.run(t, lostCase(domain.ModeFullDelete), other)
	if got[0].Outcome != ErasureBlocked || got[0].Code != domain.CodeErasureBlockedByRule {
		t.Errorf("the blocked case answered %+v", got[0])
	}
	if held := f.cases.stored[reappliedID]; held.Status != domain.StatusInProgress {
		t.Errorf("the blocked case reads %+v", held)
	}
	if got[1].Outcome != ErasureSubjectAbsent {
		t.Errorf("the next case answered %+v", got[1])
	}
}

// Through the registry: the step-up, the scope, the input refusals - nothing read or written
// before them.
func TestReapplyingErasuresIsGuardedAtTheDoor(t *testing.T) {
	record := map[string]any{
		"id": reappliedID.String(), "erasure_mode": "FULL_DELETE", "subject_account_id": subjectID.String(),
		"received_at":  recoveredTo.Add(time.Hour).Format(time.RFC3339Nano),
		"due_at":       recoveredTo.Add(30 * 24 * time.Hour).Format(time.RFC3339Nano),
		"completed_at": recoveredTo.Add(2 * time.Hour).Format(time.RFC3339Nano),
	}
	input := func(token string, erasures []any) usecase.Input {
		in := usecase.Input{
			"tenant_id": tenantID.String(), "recovery_point": recoveredTo.Format(time.RFC3339Nano),
			"erasures": erasures,
		}
		if token != "" {
			in["step_up_token"] = token
		}
		return in
	}
	cases := map[string]struct {
		actor appshared.ActorContext
		in    usecase.Input
		code  string
	}{
		"no step-up":       {operatorOf(), input("", []any{record}), stepup.CodeRequired},
		"a stale step-up":  {operatorOf(), input("stale", []any{record}), stepup.CodeRequired},
		"no operator":      {actor(), input("fresh", []any{record}), ""},
		"no erasures":      {operatorOf(), input("fresh", []any{}), codeErasuresRequired},
		"a partial record": {operatorOf(), input("fresh", []any{map[string]any{"id": reappliedID.String()}}), codeErasureRecordInvalid},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newReapplyFixture()
			registry, err := usecase.NewRegistry(nil, f.handler.Descriptor())
			if err != nil {
				t.Fatal(err)
			}
			_, err = registry.Invoke(context.Background(), ReapplyErasuresName, c.actor, c.in)
			if err == nil {
				t.Fatal("accepted")
			}
			if c.code != "" && shared.AsError(err).DetailCode != c.code {
				t.Errorf("refused with %q, want %q", shared.AsError(err).DetailCode, c.code)
			}
			if c.code == "" && !errors.Is(err, shared.ErrForbidden) {
				t.Errorf("refused with %v, want forbidden", err)
			}
			if len(f.cases.stored) != 0 || f.harness.storage.deleted {
				t.Error("a refused call wrote")
			}
		})
	}

	f := newReapplyFixture()
	registry, err := usecase.NewRegistry(nil, f.handler.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	out, err := registry.Invoke(context.Background(), ReapplyErasuresName, operatorOf(), input("fresh", []any{record}))
	if err != nil {
		t.Fatalf("re-applying: %v", err)
	}
	rows, _ := out["erasures"].([]any)
	if len(rows) != 1 || rows[0].(usecase.Output)["outcome"] != string(ErasureReapplied) || f.proof.consumed != 1 {
		t.Errorf("answered %v, %d proofs consumed", out, f.proof.consumed)
	}
}
