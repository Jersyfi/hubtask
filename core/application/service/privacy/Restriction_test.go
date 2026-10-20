// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// Art. 18 as a technical state (data-protection.md §4): readable, not processed - and not a
// lockout, which is the distinction the whole use case turns on.

// subjectStore keeps the statuses an erasure and a restriction write. An account it holds no
// status for reads as active.
type subjectStore struct {
	statuses map[shared.ID]string
	missing  bool
	// invited are the accounts whose invitation nobody accepted.
	invited map[shared.ID]bool
	// calls is the order in which a restriction locked, asked the hold and wrote.
	calls []string
}

func newSubjectStore() *subjectStore {
	return &subjectStore{statuses: map[shared.ID]string{}, invited: map[shared.ID]bool{}}
}

func (s *subjectStore) SetStatus(
	_ context.Context, id shared.ID, status string, _ time.Time,
) (bool, error) {
	s.calls = append(s.calls, "write")
	if s.missing {
		return false, nil
	}
	s.statuses[id] = status
	return true, nil
}

func (s *subjectStore) LockStatus(_ context.Context, id shared.ID) (repository.LockedAccount, bool, error) {
	s.calls = append(s.calls, "lock")
	if s.missing {
		return repository.LockedAccount{}, false, nil
	}
	account := repository.LockedAccount{Status: identity.AccountActive, InvitationOpen: s.invited[id]}
	if status, held := s.statuses[id]; held {
		account.Status = identity.AccountStatus(status)
	}
	return account, true, nil
}

func (s *subjectStore) Tenants(context.Context, string) ([]shared.ID, error) { return nil, nil }

func newRestriction(subjects *subjectStore, authorizer *authorizerDouble, sink *auditSink) RestrictProcessing {
	return RestrictProcessing{
		Subjects: subjects, Statuses: subjects, Authorizer: authorizer, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
}

func TestARestrictionIsWrittenAndRecorded(t *testing.T) {
	subjects, authorizer, sink := newSubjectStore(), &authorizerDouble{}, &auditSink{}

	if err := newRestriction(subjects, authorizer, sink).
		Execute(context.Background(), actor(), RestrictCommand{
			AccountID: subjectID, Restricted: true, Reason: "Art. 18 request of 26 August",
		}); err != nil {
		t.Fatalf("restricting: %v", err)
	}

	if subjects.statuses[subjectID] != string(identity.AccountRestricted) {
		t.Errorf("the account is %q", subjects.statuses[subjectID])
	}
	// The administrator's line: it is a state of an account, and nothing is destroyed.
	if authorizer.requests[0].Permission != domainservice.PermissionManageMembers {
		t.Errorf("restricting asked for %s", authorizer.requests[0].Permission)
	}

	entry := sink.entries[0]
	if entry.Action != RestrictedAction || entry.Severity != audit.SeverityWarning {
		t.Errorf("the restriction was recorded as %s / %s", entry.Action, entry.Severity)
	}
	if entry.LegalBasis != "dsr.restriction" {
		t.Errorf("the entry names the occasion %q", entry.LegalBasis)
	}
	reason, _ := entry.Changes["reason"].(map[string]any)
	if reason["to"] != "Art. 18 request of 26 August" {
		t.Errorf("the entry carries %v", entry.Changes)
	}
}

// Lifting it is the same call, and it says so in the trail rather than looking like a restriction
// that was written twice.
func TestLiftingARestrictionIsItsOwnAction(t *testing.T) {
	subjects, authorizer, sink := newSubjectStore(), &authorizerDouble{}, &auditSink{}

	if err := newRestriction(subjects, authorizer, sink).
		Execute(context.Background(), actor(), RestrictCommand{
			AccountID: subjectID, Restricted: false, Reason: "Withdrawn by the person",
		}); err != nil {
		t.Fatalf("lifting: %v", err)
	}

	if subjects.statuses[subjectID] != string(identity.AccountActive) {
		t.Errorf("the account is %q", subjects.statuses[subjectID])
	}
	if sink.entries[0].Action != UnrestrictedAction {
		t.Errorf("lifting was recorded as %s", sink.entries[0].Action)
	}
}

type forgetter struct{ creators, assignees []shared.ID }

func (f *forgetter) Forget(_ context.Context, creators []shared.ID) (int, error) {
	f.creators = append(f.creators, creators...)
	return len(creators), nil
}

func (f *forgetter) ForgetAssigned(_ context.Context, assignees []shared.ID) (int, error) {
	f.assignees = append(f.assignees, assignees...)
	return len(assignees), nil
}

// UC-PRV-04 check 5: restricting forgets the vectors of what the person created and of what they
// are on, in the restricting transaction; lifting forgets nothing - the next pass embeds again.
func TestARestrictionForgetsThePersonsVectors(t *testing.T) {
	for _, restricted := range []bool{true, false} {
		vectors := &forgetter{}
		restriction := newRestriction(newSubjectStore(), &authorizerDouble{}, &auditSink{})
		restriction.Embeddings = vectors
		if err := restriction.Execute(context.Background(), actor(), RestrictCommand{
			AccountID: subjectID, Restricted: restricted,
		}); err != nil {
			t.Fatalf("restricted=%v: %v", restricted, err)
		}
		forgot := len(vectors.creators) == 1 && vectors.creators[0] == subjectID &&
			len(vectors.assignees) == 1 && vectors.assignees[0] == subjectID
		if restricted != forgot {
			t.Errorf("restricted=%v: forgot creators %v, assignees %v", restricted, vectors.creators, vectors.assignees)
		}
	}
}

// A restriction is not a lockout: the person keeps working, and what stops is the processing.
func TestARestrictedAccountMayStillAct(t *testing.T) {
	restricted := identity.Account{Status: identity.AccountRestricted}
	if err := restricted.Verify(); err != nil {
		t.Errorf("a restricted account may not act: %v", err)
	}
	if restricted.Status.ProcessingAllowed() {
		t.Error("a restricted account may still be processed automatically")
	}

	// And an anonymised one may not act at all: there is nobody left to act.
	anonymised := identity.Account{Status: identity.AccountAnonymized}
	if err := anonymised.Verify(); err == nil {
		t.Error("an anonymised account was allowed to act")
	}
	if anonymised.Status.ProcessingAllowed() {
		t.Error("an anonymised account may still be processed")
	}
}

func TestAnAccountThatIsNotThereIsNotFound(t *testing.T) {
	subjects, authorizer, sink := newSubjectStore(), &authorizerDouble{}, &auditSink{}
	subjects.missing = true

	err := newRestriction(subjects, authorizer, sink).
		Execute(context.Background(), actor(), RestrictCommand{AccountID: subjectID, Restricted: true})
	if !errors.Is(err, shared.ErrNotFound) || shared.AsError(err).DetailCode != "accounts.not_found" {
		t.Fatalf("an account that is not there answered %v", err)
	}
	if got := strings.Join(subjects.calls, ","); got != "lock" {
		t.Errorf("restricting an account that is not there went %s, want lock alone", got)
	}
	if len(sink.entries) != 0 {
		t.Error("a restriction nobody could write was recorded")
	}
}

// keptInOrder answers that no hold keeps the account, and notes that it was asked.
type keptInOrder struct {
	*keptStore
	calls *[]string
}

func (k keptInOrder) KeepsAccount(context.Context, shared.ID) (bool, error) {
	*k.calls = append(*k.calls, "kept")
	return false, nil
}

// The hold is asked once the row is locked: an erasure that keeps the account and commits in
// between then waits on the lock, instead of seeing its restriction lifted afterwards.
func TestLiftingAsksTheHoldAfterLockingTheAccount(t *testing.T) {
	subjects := newSubjectStore()
	subjects.statuses[subjectID] = string(identity.AccountRestricted)
	restriction := newRestriction(subjects, &authorizerDouble{}, &auditSink{})
	restriction.Kept = keptInOrder{keptStore: &keptStore{}, calls: &subjects.calls}

	if err := restriction.Execute(context.Background(), actor(), RestrictCommand{
		AccountID: subjectID, Restricted: false,
	}); err != nil {
		t.Fatalf("lifting: %v", err)
	}
	if got := strings.Join(subjects.calls, ","); got != "lock,kept,write" {
		t.Errorf("lifting went %s, want lock,kept,write", got)
	}
}

func TestTheRestrictionDescriptorTakesWhatTheControllerSends(t *testing.T) {
	descriptor := RestrictProcessing{}.Descriptor()

	if err := descriptor.ValidateInput(usecase.Input{
		"account_id": subjectID.String(), "restricted": true, "reason": "Art. 18",
	}); err != nil {
		t.Fatalf("the input the REST controller builds is refused: %v", err)
	}
	if descriptor.ReadOnly || !descriptor.Audit.Required {
		t.Error("the restriction is declared as a read, or without its audit obligation")
	}
}
