// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// Fifth answers 6: an administrator resets the second factor of a person who lost the app and the
// codes - behind their own proof, within the role ceiling, never their own - and the app and every
// code go, every session ends, the person is told and the trail holds it.

type factorResetFixture struct {
	*renewalFixture
	enrollments *enrollmentsStore
	recovery    *recoveryStore
	told        *toldOf
}

func newFactorResetFixture(t *testing.T) *factorResetFixture {
	t.Helper()
	renewal := newRenewalFixture(t)
	f := &factorResetFixture{renewalFixture: renewal, enrollments: newEnrollments(), recovery: newRecovery(), told: &toldOf{}}
	f.enrollments.rows[bertID] = &repository.MfaEnrollment{AccountID: bertID, ConfirmedAt: now.Add(-time.Hour)}
	f.enrollments.rows[carlaID] = &repository.MfaEnrollment{AccountID: carlaID} // begun, never armed
	f.recovery.codes["abcd-efgh"] = true
	registry, err := usecase.NewRegistry(nil, ResetSecondFactor{
		Accounts: renewal.accounts, Enrollments: f.enrollments, Recovery: f.recovery,
		Sessions: renewal.sessions, Pending: renewal.store,
		Authorizer: renewal.authorizer, Held: renewal.held, StepUp: renewal.stepUp,
		Notices: f.told, Audit: renewal.audit, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	f.registry = registry
	return f
}

func (f *factorResetFixture) reset(t *testing.T, actor appshared.ActorContext, id shared.ID) error {
	t.Helper()
	_, err := f.registry.Invoke(t.Context(), ResetSecondFactorName, actor,
		usecase.Input{"account_id": id.String(), "step_up_token": "hbt_sup_fresh"})
	return err
}

func (f *factorResetFixture) wrote() int {
	return len(f.sessions.ended) + len(f.store.spent) + len(f.audit.entries) + len(f.told.notices) +
		f.recovery.replacements
}

func TestAnAdministratorResetsAPersonsSecondFactor(t *testing.T) {
	f := newFactorResetFixture(t)

	if err := f.reset(t, admin(), bertID); err != nil {
		t.Fatalf("resetting: %v", err)
	}
	if _, armed := f.enrollments.rows[bertID]; armed {
		t.Error("the authenticator app is still enrolled")
	}
	if len(f.recovery.codes) != 0 || f.recovery.replacements != 1 {
		t.Errorf("recovery codes left: %v", f.recovery.codes)
	}
	if len(f.sessions.ended) != 1 || f.sessions.ended[0] != bertID {
		t.Errorf("sessions ended for %v, want Bert's", f.sessions.ended)
	}
	if len(f.store.spent) != 1 || f.store.spent[0] != bertID {
		t.Errorf("pending steps spent for %v, want Bert's", f.store.spent)
	}
	if len(f.stepUp.presented) != 1 {
		t.Errorf("the proof was asked %d times, want once", len(f.stepUp.presented))
	}
	if len(f.audit.entries) != 1 {
		t.Fatalf("the trail holds %v", f.audit.entries)
	}
	entry := f.audit.entries[0]
	if entry.Action != SecondFactorResetAction || entry.Severity != audit.SeverityWarning ||
		entry.TargetID != bertID || entry.ActorID != adminID {
		t.Errorf("the trail entry reads %+v", entry)
	}
	if len(f.told.notices) != 1 || f.told.notices[0].account != bertID ||
		f.told.notices[0].change.Event != domain.WayInSecondFactorReset {
		t.Errorf("told %+v, want Bert of the reset", f.told.notices)
	}
}

// Nothing is written, and the proof is not spent on a refused target, for each refusal.
func TestAResetOfASecondFactorIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	cases := []struct {
		name    string
		target  shared.ID
		arrange func(f *factorResetFixture, actor *appshared.ActorContext)
		want    string
		proved  bool
	}{
		{"one's own", adminID, nil, "accounts.factor_reset_own", false},
		{"no app armed", lenaID, nil, "auth.mfa_not_enrolled", false},
		{"an enrolment never armed", carlaID, nil, "auth.mfa_not_enrolled", false},
		{"a service account", robotID, nil, "auth.mfa_not_enrolled", false},
		{"nobody here", "01936f2a-7c1e-7000-8000-0000000000ff", nil, "accounts.not_found", false},
		{"the actor may not manage people", bertID, func(f *factorResetFixture, _ *appshared.ActorContext) {
			f.authorizer.refuse = shared.ErrForbidden.WithDetail("access.denied")
		}, "access.denied", false},
		{"an automation rule runs it", bertID, func(_ *factorResetFixture, actor *appshared.ActorContext) {
			actor.Kind, actor.RunsForRule = shared.ActorServiceAccount, "01936f2a-7c1e-7000-8000-0000000000e1"
		}, "accounts.factor_reset_needs_a_person", false},
		{"no fresh proof", bertID, func(f *factorResetFixture, _ *appshared.ActorContext) {
			f.stepUp.satisfied = false
		}, "auth.step_up_required", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFactorResetFixture(t)
			actor := admin()
			if c.arrange != nil {
				c.arrange(f, &actor)
			}
			err := f.reset(t, actor, c.target)
			if got := detailOf(err); got != c.want {
				t.Fatalf("answered %q (%v), want %s", got, err, c.want)
			}
			if !c.proved && len(f.stepUp.presented) != 0 {
				t.Error("the proof was spent on a refused target")
			}
			if f.wrote() != 0 {
				t.Error("something was written")
			}
			if _, armed := f.enrollments.rows[bertID]; !armed {
				t.Error("Bert's app went although the reset was refused")
			}
		})
	}
}

// The role ceiling (identity.md §13): an administrator never resets an owner's factor, an owner
// resets another owner's.
func TestAResetOfASecondFactorIsHeldToTheRoleCeiling(t *testing.T) {
	for _, c := range ceilingCases {
		t.Run(c.name, func(t *testing.T) {
			f := newFactorResetFixture(t)
			f.authorizer.own = c.own
			f.held[bertID] = c.held

			err := f.reset(t, admin(), bertID)
			if c.issued {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if got := detailOf(err); got != "access.role_above_own" {
				t.Fatalf("answered %q (%v), want access.role_above_own", got, err)
			}
			if len(f.stepUp.presented) != 0 || f.wrote() != 0 {
				t.Error("the proof was looked at or something written before the ceiling refused")
			}
		})
	}
}
