// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// Art. 18 as a technical state (E-10, data-protection.md §4): readable, not processed - and not a
// lockout, which is the distinction the whole use case turns on.

// subjectStore keeps the statuses an erasure and a restriction write.
type subjectStore struct {
	statuses map[shared.ID]string
	missing  bool
}

func newSubjectStore() *subjectStore {
	return &subjectStore{statuses: map[shared.ID]string{}}
}

func (s *subjectStore) SetStatus(
	_ context.Context, id shared.ID, status string, _ time.Time,
) (bool, error) {
	if s.missing {
		return false, nil
	}
	s.statuses[id] = status
	return true, nil
}

func (s *subjectStore) Tenants(context.Context, string) ([]shared.ID, error) { return nil, nil }

func newRestriction(subjects *subjectStore, authorizer *authorizerDouble, sink *auditSink) RestrictProcessing {
	return RestrictProcessing{
		Subjects: subjects, Authorizer: authorizer, Audit: sink,
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
	if err == nil {
		t.Fatal("an account that is not there was restricted")
	}
	if len(sink.entries) != 0 {
		t.Error("a restriction nobody could write was recorded")
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
