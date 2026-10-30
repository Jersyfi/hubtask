// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
)

const viewAsToken = "hbt_sat_view"

type viewAsFixture struct {
	registry   *usecase.Registry
	sessions   *sessionsStore
	authorizer *authorizer
	audit      *auditSink
	held       heldRoles
}

// newViewAsFixture holds Bert's own live session and the people of the renewal fixture as the
// accounts a view could join.
func newViewAsFixture(t *testing.T) *viewAsFixture {
	t.Helper()
	sessions := newSessionsStore()
	credential, _ := refreshCredential(now)
	sessions.sessions[sessionRowID] = repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: credential.Session, Account: credential.Account,
		TenantLocale: "en", TenantTimeZone: "UTC",
	}
	accounts := newAccounts()
	sessions.viewable = map[shared.ID]domain.Account{}
	sessions.inPrivateHub = map[shared.ID]bool{}
	for _, each := range []domain.Account{
		{ID: lenaID, TenantID: tenant, Kind: domain.AccountUser, DisplayName: "Lena", Status: domain.AccountActive, TimeZone: "Europe/Berlin"},
		{ID: robotID, TenantID: tenant, Kind: domain.AccountServiceAccount, DisplayName: "Robot", Status: domain.AccountActive},
		{ID: carlaID, TenantID: tenant, Kind: domain.AccountUser, DisplayName: "Carla", Status: domain.AccountInvited},
	} {
		accounts.byID[each.ID] = each
		sessions.viewable[each.ID] = each
	}
	f := &viewAsFixture{sessions: sessions, authorizer: &authorizer{own: domain.RoleAdmin}, audit: &auditSink{}, held: heldRoles{}}
	registry, err := usecase.NewRegistry(nil,
		StartViewAs{
			Accounts: accounts, Sessions: sessions, Held: f.held, Authorizer: f.authorizer,
			Audit: f.audit, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
		}.Descriptor(),
		GetViewAs{}.Descriptor(),
		EndViewAs{Sessions: sessions, Audit: f.audit, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now)}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	f.registry = registry
	return f
}

// viewer is Bert on his own session.
func viewer() appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: account, AccountName: "Bert", Kind: shared.ActorUser,
		TokenID: sessionRowID, Scopes: []string{"items:read", "items:write", "members:write"},
	}
}

func (f *viewAsFixture) authenticate(t *testing.T, at time.Time) appshared.ActorContext {
	t.Helper()
	signer := validatingSigner{token: viewAsToken, claims: cryptoport.SessionClaims{
		TenantID: tenant, SessionID: sessionRowID, AccountID: account, ExpiresAt: at.Add(10 * time.Minute),
	}}
	actor, err := sessionAuthenticator(f.sessions, signer, at).Execute(t.Context(),
		AuthenticateTokenCommand{Credential: viewAsToken})
	if err != nil {
		t.Fatalf("authenticating: %v", err)
	}
	return actor
}

// UC-ID-29 checks 1, 2 and 4: the session reads as the person, with the reading scopes only, no
// copy is filled, the start is a warning in the trail, and ending gives the session back.
func TestAViewShowsTheAppAsThePersonReadOnly(t *testing.T) {
	f := newViewAsFixture(t)
	out, err := f.registry.Invoke(t.Context(), StartViewAsName, viewer(), usecase.Input{"account_id": lenaID.String()})
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	if out["active"] != true || out["account_id"] != lenaID.String() || out["until"] != now.Add(30*time.Minute).UTC() {
		t.Errorf("answered %v, want Lena for half an hour", out)
	}
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != ViewedAsAction ||
		f.audit.entries[0].Severity != audit.SeverityWarning || f.audit.entries[0].TargetID != lenaID ||
		f.audit.entries[0].ActorID != account {
		t.Fatalf("trail %+v, want one warning naming Bert and Lena", f.audit.entries)
	}

	actor := f.authenticate(t, now.Add(time.Minute))
	if actor.AccountID != lenaID || actor.ViewedBy != account || !actor.IsViewingAs() {
		t.Fatalf("actor %+v, want Lena viewed by Bert", actor)
	}
	if !slices.Equal(actor.Scopes, []string{"items:read"}) {
		t.Errorf("scopes %v, want the reading ones only", actor.Scopes)
	}
	if actor.TimeZone != "Europe/Berlin" || actor.Locale != "en" {
		t.Errorf("zone %q locale %q, want Lena's day in the viewer's language", actor.TimeZone, actor.Locale)
	}
	if err := actor.RequireScope("items:write"); err == nil {
		t.Error("a write's scope is held while viewing")
	}
	if got := detailOf(actor.RefuseACopy()); got != "access.view_as_online_only" {
		t.Errorf("a copy answered %q, want access.view_as_online_only", got)
	}

	read, err := f.registry.Invoke(t.Context(), GetViewAsName, actor, usecase.Input{})
	if err != nil || read["active"] != true || read["display_name"] != "Lena" {
		t.Errorf("the running view read %v (%v)", read, err)
	}
	if _, err := f.registry.Invoke(t.Context(), EndViewAsName, actor, usecase.Input{}); err != nil {
		t.Fatalf("ending: %v", err)
	}
	if last := f.audit.entries[len(f.audit.entries)-1]; last.Action != ViewAsEndedAction || last.ActorID != account {
		t.Errorf("the end was recorded as %+v", last)
	}
	if own := f.authenticate(t, now.Add(2*time.Minute)); own.AccountID != account || own.IsViewingAs() {
		t.Errorf("after the end the session is %+v, want Bert's own", own)
	}
}

// UC-ID-29 check 3: the view lapses by itself after half an hour, and with it every reason it could
// no longer stand - the session is then its own person's again, never the person viewed.
func TestAViewLapsesToTheSessionsOwnPerson(t *testing.T) {
	cases := []struct {
		name  string
		after time.Duration
		alter func(*viewAsFixture)
	}{
		{"after half an hour", 31 * time.Minute, func(*viewAsFixture) {}},
		{"once the person joins a private hub", time.Minute, func(f *viewAsFixture) {
			c := f.sessions.sessions[sessionRowID]
			c.ViewedReachesPrivateHub = true
			f.sessions.sessions[sessionRowID] = c
		}},
		{"once the person is switched off", time.Minute, func(f *viewAsFixture) {
			c := f.sessions.sessions[sessionRowID]
			c.Viewed.Status = domain.AccountDisabled
			f.sessions.sessions[sessionRowID] = c
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newViewAsFixture(t)
			if _, err := f.registry.Invoke(t.Context(), StartViewAsName, viewer(), usecase.Input{"account_id": lenaID.String()}); err != nil {
				t.Fatalf("starting: %v", err)
			}
			c.alter(f)
			actor := f.authenticate(t, now.Add(c.after))
			if actor.AccountID != account || actor.IsViewingAs() || !slices.Contains(actor.Scopes, "items:write") {
				t.Errorf("actor %+v, want Bert's own session", actor)
			}
		})
	}
}

// UC-ID-29 check 5: who may not be viewed, and who may not view - refused before anything is
// written.
func TestAViewIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	agent := viewer()
	agent.OnBehalfOf = lenaID
	token := viewer()
	token.TokenID = ""
	cases := []struct {
		name   string
		actor  appshared.ActorContext
		target shared.ID
		alter  func(*viewAsFixture)
		want   string
	}{
		{"a person holding a role above the viewer's", viewer(), lenaID,
			func(f *viewAsFixture) { f.held[lenaID] = []domain.Role{domain.RoleOwner} }, "access.role_above_own"},
		{"a member of a private hub", viewer(), lenaID,
			func(f *viewAsFixture) { f.sessions.inPrivateHub[lenaID] = true }, "access.view_as_private_hub"},
		{"a service account", viewer(), robotID, func(*viewAsFixture) {}, "access.view_as_not_a_person"},
		{"a person not yet active", viewer(), carlaID, func(*viewAsFixture) {}, "access.view_as_not_a_person"},
		{"oneself", viewer(), account, func(*viewAsFixture) {}, "access.view_as_self"},
		{"an agent acting for somebody", agent, lenaID, func(*viewAsFixture) {}, "access.view_as_needs_a_session"},
		{"a credential that is no session", token, lenaID, func(*viewAsFixture) {}, "access.view_as_needs_a_session"},
		{"without MANAGE_MEMBERS", viewer(), lenaID,
			func(f *viewAsFixture) {
				f.authorizer.refuse = shared.ErrForbidden.WithDetail("access.permission_required")
			},
			"access.permission_required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newViewAsFixture(t)
			c.alter(f)
			_, err := f.registry.Invoke(t.Context(), StartViewAsName, c.actor, usecase.Input{"account_id": c.target.String()})
			if got := detailOf(err); got != c.want {
				t.Fatalf("answered %q (%v), want %q", got, err, c.want)
			}
			if len(f.audit.entries) != 0 {
				t.Errorf("trail %+v, want nothing recorded", f.audit.entries)
			}
			if c.want != "access.view_as_private_hub" && !f.sessions.sessions[sessionRowID].Session.ViewingAs.IsZero() {
				t.Error("the session was marked")
			}
		})
	}
}

// Ending where no view runs is not an error and records nothing.
func TestEndingWithoutAViewIsQuiet(t *testing.T) {
	f := newViewAsFixture(t)
	if _, err := f.registry.Invoke(t.Context(), EndViewAsName, viewer(), usecase.Input{}); err != nil {
		t.Fatalf("ending: %v", err)
	}
	read, err := f.registry.Invoke(t.Context(), GetViewAsName, viewer(), usecase.Input{})
	if err != nil || read["active"] != false || len(f.audit.entries) != 0 {
		t.Errorf("read %v (%v), trail %v", read, err, f.audit.entries)
	}
}
