// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// operatorRegister is the register, in memory. An empty one answers true, which is the private
// installation - and the test that asserts it is the one that matters most here.
type operatorRegister struct {
	accounts map[shared.ID]bool
	empty    bool
}

func (r *operatorRegister) Resolve(context.Context, string, string) (shared.ID, error) {
	// Nothing in this package registers anybody; the door exists so the port is satisfied.
	return shared.ID(""), nil
}

func (r *operatorRegister) Holds(_ context.Context, accountID shared.ID) (bool, error) {
	if r.empty {
		return true, nil
	}
	return r.accounts[accountID], nil
}

func (r *operatorRegister) List(context.Context) ([]repository.Operator, error) {
	register := make([]repository.Operator, 0, len(r.accounts))
	for id, held := range r.accounts {
		if held {
			register = append(register, repository.Operator{TenantID: tenant, AccountID: id})
		}
	}
	return register, nil
}

func (r *operatorRegister) Add(_ context.Context, accountID, _ shared.ID) (bool, error) {
	if r.accounts[accountID] {
		return false, nil
	}
	r.accounts[accountID] = true
	return true, nil
}

func (r *operatorRegister) Remove(_ context.Context, accountID shared.ID) (bool, error) {
	if len(r.accounts) <= 1 {
		return false, nil
	}
	delete(r.accounts, accountID)
	return true, nil
}

func newElevationFixture(t *testing.T) (*sessionFixture, *operatorRegister) {
	t.Helper()
	fixture := newRecoveryFixture(t)
	register := &operatorRegister{accounts: map[shared.ID]bool{account: true}}
	return fixture, register
}

// An operator with a fresh proof raises their own session for an hour, and the answer says how
// long is left.
func TestAnOperatorRaisesTheirOwnSessionForAnHour(t *testing.T) {
	fixture, register := newElevationFixture(t)

	elevation, err := ElevateSession{
		Writer: fixture.writer, Operators: register,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &idSequence{},
	}.Execute(t.Context(), recoveryActor(), fixture.proof)
	if err != nil {
		t.Fatalf("the elevation was refused: %v", err)
	}

	if want := now.Add(domain.ElevationLifetime).UTC(); !elevation.Until.Equal(want) {
		t.Errorf("elevated until %v, want %v", elevation.Until, want)
	}
	if elevation.Remaining != domain.ElevationLifetime {
		t.Errorf("remaining %v", elevation.Remaining)
	}
	if len(fixture.sessions.elevated) != 1 ||
		fixture.sessions.elevated[0].sessionID != sessionRowID {
		t.Errorf("the store was asked for %v", fixture.sessions.elevated)
	}
	if len(fixture.audit.entries) != 1 || fixture.audit.entries[0].Action != SessionElevatedAction {
		t.Errorf("the trail holds %v", fixture.audit.entries)
	}
}

// Somebody the register does not name is refused, and learns it before being asked for a password.
func TestSomebodyOutsideTheRegisterCannotRaiseTheirSession(t *testing.T) {
	fixture, register := newElevationFixture(t)
	register.accounts = map[shared.ID]bool{}

	_, err := ElevateSession{
		Writer: fixture.writer, Operators: register,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &idSequence{},
	}.Execute(t.Context(), recoveryActor(), fixture.proof)

	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a stranger answered %v", err)
	}
	if len(fixture.sessions.elevated) != 0 {
		t.Error("a session was raised anyway")
	}
}

// Without a fresh proof there is no elevation: a session that could raise itself would be a tab
// that could, and the hour is exactly what a stolen tab must not get.
func TestRaisingASessionDemandsAFreshProof(t *testing.T) {
	fixture, register := newElevationFixture(t)

	_, err := ElevateSession{
		Writer: fixture.writer, Operators: register,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &idSequence{},
	}.Execute(t.Context(), recoveryActor(), "")

	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("an elevation without a proof answered %v", err)
	}
}

// The elevation is a window on the session and nothing else, so it ends by the clock rather than by
// anything having to remember to end it.
func TestAnElevationEndsByTheClock(t *testing.T) {
	raised := domain.Session{ElevatedUntil: now.Add(domain.ElevationLifetime)}

	if !raised.IsElevated(now) {
		t.Error("a session raised a moment ago does not read as raised")
	}
	if !raised.IsElevated(now.Add(59 * time.Minute)) {
		t.Error("a session raised fifty-nine minutes ago does not read as raised")
	}
	if raised.IsElevated(now.Add(domain.ElevationLifetime)) {
		t.Error("the hour did not end")
	}
	if (domain.Session{}).IsElevated(now) {
		t.Error("a session nobody raised reads as raised")
	}
}
