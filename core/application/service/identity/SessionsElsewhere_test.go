// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-ID-06 check 4: *Sign out everywhere else* leaves exactly the current session.

// thisSession puts the caller's own session into the fixture's store, as a signed-in caller has one.
func thisSession(fixture *sessionFixture) {
	fixture.sessions.sessions[sessionRowID] = repository.SessionCredential{
		Session: domain.Session{ID: sessionRowID, AccountID: account},
	}
}

func TestSigningOutEverywhereElseSparesThisSession(t *testing.T) {
	fixture := newSessionFixture(now)
	fixture.sessions.revokedAll = 2
	thisSession(fixture)

	if err := (RevokeOtherSessions{Writer: fixture.writer}).
		Execute(t.Context(), sessionActor(accountRead)); err != nil {
		t.Fatalf("signing out everywhere else: %v", err)
	}
	if len(fixture.sessions.keptByOthers) != 1 || fixture.sessions.keptByOthers[0] != sessionRowID {
		t.Fatalf("the store was asked to spare %v, want this session", fixture.sessions.keptByOthers)
	}
	if len(fixture.audit.entries) != 1 || fixture.audit.entries[0].Action != SessionsRevokedAction {
		t.Fatalf("audit %v, want one entry", fixture.audit.entries)
	}
	if fixture.audit.entries[0].TargetID != account {
		t.Errorf("the entry names %q, want the account, as signing out everywhere does", fixture.audit.entries[0].TargetID)
	}
}

// A personal access token has no session of its own: there is nothing to spare, and every session
// ends - what "everywhere else" means from a caller that is nowhere.
func TestATokenCallerHasNoSessionToSpare(t *testing.T) {
	fixture := newSessionFixture(now)
	fixture.sessions.revokedAll = 3
	actor := sessionActor(accountRead)
	actor.TokenID = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1") // a token row, no session
	fixture.sessions.sessions = nil

	if err := (RevokeOtherSessions{Writer: fixture.writer}).Execute(t.Context(), actor); err != nil {
		t.Fatalf("signing out everywhere else with a token: %v", err)
	}
	if len(fixture.sessions.keptByOthers) != 1 || !fixture.sessions.keptByOthers[0].IsZero() {
		t.Errorf("a token caller spared %v, want nothing", fixture.sessions.keptByOthers)
	}
}

// Nothing else was live: no entry, as for every revocation that changed nothing.
func TestNothingElseLiveWritesNothing(t *testing.T) {
	fixture := newSessionFixture(now)
	if err := (RevokeOtherSessions{Writer: fixture.writer}).
		Execute(t.Context(), sessionActor(accountRead)); err != nil {
		t.Fatalf("an empty revocation answered %v", err)
	}
	if len(fixture.audit.entries) != 0 {
		t.Error("an empty revocation wrote an entry")
	}
}

func TestSigningOutEverywhereElseIsReachableThroughTheRegistry(t *testing.T) {
	fixture := newSessionFixture(now)
	fixture.sessions.revokedAll = 1
	thisSession(fixture)
	registry, err := usecase.NewRegistry(nil, RevokeOtherSessions{Writer: fixture.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	if _, err := registry.Invoke(t.Context(), RevokeOtherSessionsName, sessionActor(accountRead), usecase.Input{}); err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if len(fixture.sessions.keptByOthers) != 1 || fixture.sessions.keptByOthers[0] != sessionRowID {
		t.Errorf("through the registry the store spared %v", fixture.sessions.keptByOthers)
	}
}
