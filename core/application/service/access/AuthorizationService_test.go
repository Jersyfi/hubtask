// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenantID  = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	accountID = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	hubID     = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	now       = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
)

type unitOfWork struct {
	writes int
	reads  int
	err    error
}

func (u *unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	u.writes++
	if u.err != nil {
		return u.err
	}
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	u.reads++
	if u.err != nil {
		return u.err
	}
	return fn(ctx)
}

type memberships struct {
	held []identity.Membership
	path []identity.Scope
	err  error
}

func (m *memberships) Along(_ context.Context, _ shared.ID, path []identity.Scope) ([]identity.Membership, error) {
	m.path = path
	return m.held, m.err
}

type sink struct {
	entries []audit.Entry
	err     error
}

func (s *sink) Append(_ context.Context, entry audit.Entry) error {
	if s.err != nil {
		return s.err
	}
	s.entries = append(s.entries, entry)
	return nil
}

func actorWithScopes(scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID, Scopes: scopes,
	}
}

func request() Request {
	return Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)},
		Action:     "container.created",
		TokenScope: "containers:write",
		TargetType: "container",
		TargetID:   hubID,
	}
}

func serviceWith(held []identity.Membership) (Service, *memberships, *sink, *unitOfWork) {
	store := &memberships{held: held}
	trail := &sink{}
	uow := &unitOfWork{}
	return Service{Memberships: store, UnitOfWork: uow, Audit: trail, Clock: clock.Fixed(now)},
		store, trail, uow
}

func TestAnAdministratorMayCreateAContainer(t *testing.T) {
	authorize, store, trail, uow := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleAdmin},
	})

	if err := authorize.Authorize(context.Background(), actorWithScopes("containers:write"), request()); err != nil {
		t.Fatalf("an administrator was refused: %v", err)
	}
	if len(trail.entries) != 0 {
		t.Errorf("a permitted action produced an audit entry here: %+v", trail.entries)
	}
	if uow.writes != 0 || uow.reads != 1 {
		t.Errorf("unexpected transactions: %d read, %d write", uow.reads, uow.writes)
	}
	// The query is bounded by the path rather than reading everything the account holds.
	if len(store.path) != 2 {
		t.Errorf("the membership query was not bounded by the path: %+v", store.path)
	}
}

// Test AT-3: a refusal is recorded, with the actor and the action that was refused.
func TestARefusalIsRecorded(t *testing.T) {
	authorize, _, trail, uow := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleMember},
	})

	err := authorize.Authorize(context.Background(), actorWithScopes("containers:write"), request())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if got := shared.AsError(err).DetailCode; got != "access.not_permitted" {
		t.Errorf("detail code %s, want access.not_permitted", got)
	}

	if len(trail.entries) != 1 {
		t.Fatalf("%d audit entries, want 1", len(trail.entries))
	}
	entry := trail.entries[0]
	if entry.Outcome != audit.OutcomeDenied || entry.Action != "container.created" {
		t.Errorf("unexpected entry: %+v", entry)
	}
	if entry.ActorID != accountID || entry.ActorKind != shared.ActorUser || entry.TenantID != tenantID {
		t.Errorf("the entry does not name the actor: %+v", entry)
	}
	if entry.OccurredAt != now {
		t.Errorf("the entry did not take its time from the clock port: %v", entry.OccurredAt)
	}
	// The refusal is written in a transaction of its own. Inside the caller's, it would be rolled
	// back together with the refusal and the record would be missing.
	if uow.writes != 1 {
		t.Errorf("%d write transactions, want 1", uow.writes)
	}
}

// The token scope is the second bound: the role may allow it and the credential still may not
// (ADR-0005). It is checked first, because it needs no database at all.
func TestTheTokenScopeIsCheckedBeforeTheRole(t *testing.T) {
	authorize, store, trail, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleOwner},
	})

	err := authorize.Authorize(context.Background(), actorWithScopes("containers:read"), request())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if got := shared.AsError(err).DetailCode; got != "access.insufficient_scope" {
		t.Errorf("detail code %s, want access.insufficient_scope", got)
	}
	if store.path != nil {
		t.Error("a token without the scope still cost a membership lookup")
	}
	if len(trail.entries) != 1 || trail.entries[0].Outcome != audit.OutcomeDenied {
		t.Errorf("the refusal was not recorded: %+v", trail.entries)
	}
}

// An account with no membership at all is refused like any other - and recorded, because "who
// tried to reach what" is exactly the question the trail exists for.
func TestAnAccountWithoutAnyMembershipIsRefused(t *testing.T) {
	authorize, _, trail, _ := serviceWith(nil)

	if err := authorize.Authorize(context.Background(), actorWithScopes("containers:write"), request()); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if len(trail.entries) != 1 {
		t.Errorf("%d audit entries, want 1", len(trail.entries))
	}
}

func TestAnUnauthenticatedActorIsRefusedWithoutAnEntry(t *testing.T) {
	authorize, _, trail, uow := serviceWith(nil)

	err := authorize.Authorize(context.Background(), appshared.Anonymous("en", "UTC"), request())
	if !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("error %v, want unauthenticated", err)
	}
	// Nothing to record it against: an unauthenticated request has no tenant, and the credential
	// was already judged by authentication.
	if len(trail.entries) != 0 || uow.reads != 0 || uow.writes != 0 {
		t.Errorf("an anonymous request reached the database: %d reads, %d writes, %+v",
			uow.reads, uow.writes, trail.entries)
	}
}

// A database that cannot answer the question is not a refusal. Reporting it as forbidden would
// send a client off to fix a permission that is not the problem.
func TestAnUnreadableMembershipIsNotARefusal(t *testing.T) {
	authorize, store, trail, _ := serviceWith(nil)
	store.err = shared.ErrUnavailable.WithDetail("postgres.query_failed")

	err := authorize.Authorize(context.Background(), actorWithScopes("containers:write"), request())
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("error %v, want unavailable", err)
	}
	if len(trail.entries) != 0 {
		t.Errorf("a failed lookup was recorded as a refusal: %+v", trail.entries)
	}
}

// The refusal stands even when the trail cannot be written: the client is denied either way, and
// the gap in the evidence is an operational problem rather than a different answer.
func TestARefusalSurvivesAFailingTrail(t *testing.T) {
	authorize, _, trail, _ := serviceWith(nil)
	trail.err = shared.ErrUnavailable.WithDetail("postgres.query_failed")

	if err := authorize.Authorize(context.Background(), actorWithScopes("containers:write"), request()); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
}

// An operation that needs no particular scope skips that bound rather than inventing one.
func TestAnOperationWithoutATokenScope(t *testing.T) {
	authorize, _, _, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleViewer},
	})
	unscoped := request()
	unscoped.Permission, unscoped.TokenScope = service.PermissionRead, ""

	if err := authorize.Authorize(context.Background(), actorWithScopes(), unscoped); err != nil {
		t.Fatalf("a viewer was refused a read: %v", err)
	}
}

// RoleAlong is the qualifier question (C-03): which role the actor holds, for the rules the
// permission matrix cannot express - only the author or an administrator may change a comment.
func TestRoleAlongReportsTheHighestRoleAndItsAbsence(t *testing.T) {
	authorize, _, trail, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleViewer},
		{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleAdmin},
	})

	role, found, err := authorize.RoleAlong(context.Background(), actorWithScopes(),
		[]identity.Scope{identity.TenantScope(), identity.HubScope(hubID)})
	if err != nil || !found || role != identity.RoleAdmin {
		t.Fatalf("role = %v/%v/%v, want the highest along the path", role, found, err)
	}
	if len(trail.entries) != 0 {
		t.Errorf("a question produced an audit entry: %+v", trail.entries)
	}

	stranger, _, _, _ := serviceWith(nil)
	if _, found, err := stranger.RoleAlong(context.Background(), actorWithScopes(),
		[]identity.Scope{identity.TenantScope()}); err != nil || found {
		t.Fatalf("an account with no membership reports %v/%v", found, err)
	}
}
