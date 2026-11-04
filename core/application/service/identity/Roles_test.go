// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	customRoleID = shared.ID("01936f2a-7c1e-7000-8000-0000000000e1")
	otherGrantID = shared.ID("01936f2a-7c1e-7000-8000-0000000000e2")
	tenantGrant  = shared.ID("01936f2a-7c1e-7000-8000-0000000000e3")
)

// roleStore is the custom role tables, in memory.
type roleStore struct {
	roles  map[string]repository.CustomRoleRow
	grants map[shared.ID]repository.CustomGrantRow
	// onLock runs once at Lock, as another transaction committing while this one waited for it.
	onLock func()
}

func newRoleStore(roles ...domain.CustomRole) *roleStore {
	store := &roleStore{roles: map[string]repository.CustomRoleRow{}, grants: map[shared.ID]repository.CustomGrantRow{}}
	for _, role := range roles {
		store.roles[role.Key] = repository.CustomRoleRow{Role: role, Version: 1}
	}
	return store
}

func (s *roleStore) List(context.Context) ([]repository.CustomRoleRow, error) {
	out := []repository.CustomRoleRow{}
	for _, row := range s.roles {
		out = append(out, row)
	}
	return out, nil
}

func (s *roleStore) Find(_ context.Context, key string) (repository.CustomRoleRow, error) {
	row, found := s.roles[key]
	if !found {
		return repository.CustomRoleRow{}, shared.ErrNotFound.WithDetail("roles.not_found")
	}
	return row, nil
}

func (s *roleStore) Lock(_ context.Context, id shared.ID) (repository.CustomRoleRow, error) {
	if hook := s.onLock; hook != nil {
		s.onLock = nil
		hook()
	}
	for _, row := range s.roles {
		if row.Role.ID == id {
			return row, nil
		}
	}
	return repository.CustomRoleRow{}, shared.ErrNotFound.WithDetail("roles.not_found")
}

func (s *roleStore) Insert(_ context.Context, role domain.CustomRole, at time.Time) error {
	if _, taken := s.roles[role.Key]; taken {
		return shared.ErrConflict.WithDetail("roles.key_taken")
	}
	s.roles[role.Key] = repository.CustomRoleRow{Role: role, CreatedAt: at, UpdatedAt: at, Version: 1}
	return nil
}

func (s *roleStore) Update(_ context.Context, role domain.CustomRole, version int, at time.Time) (bool, error) {
	row := s.roles[role.Key]
	if row.Version != version {
		return false, nil
	}
	s.roles[role.Key] = repository.CustomRoleRow{Role: role, CreatedAt: row.CreatedAt, UpdatedAt: at, Version: version + 1}
	return true, nil
}

func (s *roleStore) Delete(_ context.Context, id shared.ID) (bool, error) {
	for key, row := range s.roles {
		if row.Role.ID == id {
			delete(s.roles, key)
			for grantID, grant := range s.grants {
				if grant.Grant.RoleID == id {
					delete(s.grants, grantID)
				}
			}
			return true, nil
		}
	}
	return false, nil
}

func (s *roleStore) Grant(_ context.Context, grant domain.CustomGrant, _ time.Time) (bool, error) {
	for _, row := range s.roles {
		if row.Role.ID == grant.RoleID {
			s.grants[grant.ID] = repository.CustomGrantRow{Grant: grant, RoleKey: row.Role.Key}
			return true, nil
		}
	}
	return false, shared.ErrNotFound
}

func (s *roleStore) FindGrant(_ context.Context, id shared.ID) (repository.CustomGrantRow, error) {
	row, found := s.grants[id]
	if !found {
		return repository.CustomGrantRow{}, shared.ErrNotFound
	}
	return row, nil
}

func (s *roleStore) Revoke(_ context.Context, id shared.ID) (bool, error) {
	_, found := s.grants[id]
	delete(s.grants, id)
	return found, nil
}

func (s *roleStore) Grants(_ context.Context, filter repository.GrantFilter, after shared.ID, limit int) ([]repository.CustomGrantRow, error) {
	out := []repository.CustomGrantRow{}
	for _, row := range s.grants {
		if (filter.RoleID != "" && row.Grant.RoleID != filter.RoleID) ||
			(filter.GroupID != "" && row.Grant.GroupID != filter.GroupID) || row.Grant.ID <= after {
			continue
		}
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b repository.CustomGrantRow) int {
		return strings.Compare(string(a.Grant.ID), string(b.Grant.ID))
	})
	return out[:min(limit, len(out))], nil
}

var _ repository.CustomRoles = (*roleStore)(nil)

// heldBy answers a fixed holding for whoever is asked about.
type heldBy struct {
	held  access.Held
	asked []shared.ID
}

func (h *heldBy) HeldBy(_ context.Context, _ appshared.ActorContext, accountID shared.ID, _ []domain.Scope) (access.Held, error) {
	h.asked = append(h.asked, accountID)
	return h.held, nil
}

// reach answers the unrecorded question for every scope but the hidden ones - a private hub the
// caller holds nothing in, as the authorisation service would.
type reach struct{ hidden map[shared.ID]bool }

func (r reach) Permits(_ context.Context, _ appshared.ActorContext, request access.Request) (bool, error) {
	return !r.hidden[request.Path[len(request.Path)-1].ID], nil
}

func rolesWith(store *roleStore, auth *authorizer, sink *auditSink, held *heldBy) Roles {
	return Roles{
		Store: store, Authorizer: auth, Reach: reach{}, Held: held, Revocations: &revoker{}, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: customRoleID},
	}
}

func serviceLead(t *testing.T) domain.CustomRole {
	t.Helper()
	role, err := service.DefineRole(customRoleID, tenant, "service_lead", "Service lead", []string{"READ", "STRUCTURE"})
	if err != nil {
		t.Fatal(err)
	}
	return role
}

// UC-ID-26 check 1: a role is defined from the catalogue, the ceiling asked for every permission
// it carries, and the definition recorded with its key and permissions and never its name.
func TestARoleIsDefinedFromTheCatalogueUnderTheCeiling(t *testing.T) {
	store, auth, sink := newRoleStore(), &authorizer{}, &auditSink{}
	row, err := CreateRole{Roles: rolesWith(store, auth, sink, &heldBy{})}.Execute(t.Context(), admin(), RoleCommand{
		Key: "service_lead", Name: "Service lead", Permissions: []string{"STRUCTURE", "READ"},
	})
	if err != nil {
		t.Fatalf("defining: %v", err)
	}
	if !slices.Equal(row.Role.Permissions, []string{"READ", "STRUCTURE"}) {
		t.Fatalf("permissions = %v", row.Role.Permissions)
	}
	if len(auth.requests) != 1 || !slices.Equal(auth.requests[0].HandsOut, []string{"READ", "STRUCTURE"}) ||
		auth.requests[0].Permission != service.PermissionManageMembers {
		t.Fatalf("asked %+v", auth.requests)
	}
	if len(sink.entries) != 1 || sink.entries[0].Action != RoleCreatedAction {
		t.Fatalf("trail %+v", sink.entries)
	}
	if _, named := sink.entries[0].Changes["name"]; named {
		t.Fatal("the role's name - user content - is in the trail")
	}
}

func TestARoleIsRefusedAnUnknownPermissionAndAShippedRolesKey(t *testing.T) {
	roles := rolesWith(newRoleStore(), &authorizer{}, &auditSink{}, &heldBy{})
	_, err := CreateRole{Roles: roles}.Execute(t.Context(), admin(), RoleCommand{Key: "fly", Name: "Fly", Permissions: []string{"FLY"}})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("unknown permission: %v", err)
	}
	_, err = CreateRole{Roles: roles}.Execute(t.Context(), admin(), RoleCommand{Key: "OWNER", Name: "Owner", Permissions: []string{"READ"}})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a shipped role's key: %v", err)
	}
}

// D-05: a change asks the ceiling for what it adds and what it takes away, not for what stays.
func TestAChangeAsksTheCeilingForWhatItAddsAndTakesAway(t *testing.T) {
	store, auth := newRoleStore(serviceLead(t)), &authorizer{}
	_, err := ChangeRole{Roles: rolesWith(store, auth, &auditSink{}, &heldBy{})}.Execute(t.Context(), admin(), RoleCommand{
		Key: "service_lead", Name: "Service lead", Permissions: []string{"READ", "AUTOMATION"},
	})
	if err != nil {
		t.Fatalf("changing: %v", err)
	}
	got := auth.requests[0].HandsOut
	slices.Sort(got)
	if !slices.Equal(got, []string{"AUTOMATION", "STRUCTURE"}) {
		t.Fatalf("handed out %v, want AUTOMATION and STRUCTURE", got)
	}
}

// UC-ID-26 check 2: the grant asks MANAGE_MEMBERS at its own place and every permission of the
// role, and is recorded as a grant.
func TestACustomRoleIsGrantedUnderTheCeilingAtItsPlace(t *testing.T) {
	store, auth, sink := newRoleStore(serviceLead(t)), &authorizer{}, &auditSink{}
	grant := GrantRole{
		Roles:  rolesWith(store, auth, sink, &heldBy{}),
		Grants: GrantMembership{Accounts: newAccounts(invitedAccount(t)), Groups: newGroups(), Hubs: &hubLocks{}},
	}
	row, err := grant.Execute(t.Context(), admin(), GrantRoleCommand{
		Role: "service_lead", AccountID: invitedID, Scope: domain.HubScope(hubID),
	})
	if err != nil {
		t.Fatalf("granting: %v", err)
	}
	if row.RoleKey != "service_lead" || len(store.grants) != 1 {
		t.Fatalf("granted %+v", row)
	}
	request := auth.requests[0]
	if request.Path[len(request.Path)-1] != domain.HubScope(hubID) || !slices.Equal(request.HandsOut, []string{"READ", "STRUCTURE"}) {
		t.Fatalf("asked %+v", request)
	}
	if len(sink.entries) != 1 || sink.entries[0].Action != MembershipGrantedAction {
		t.Fatalf("trail %+v", sink.entries)
	}
	if _, err := grant.Execute(t.Context(), admin(), GrantRoleCommand{
		Role: "service_lead", AccountID: invitedID, Scope: domain.ItemScope(hubID),
	}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a custom role on one entry: %v", err)
	}
}

// A role in use is refused with its counts and keeps every grant: a rollback recreates a role,
// never who held it where (configuration.md §2).
func TestARoleInUseIsNotDeleted(t *testing.T) {
	store := newRoleStore(serviceLead(t))
	store.grants[membershipID] = repository.CustomGrantRow{RoleKey: "service_lead", Grant: domain.CustomGrant{
		ID: membershipID, TenantID: tenant, AccountID: invitedID, Scope: domain.HubScope(hubID), RoleID: customRoleID,
	}}
	roles := rolesWith(store, &authorizer{}, &auditSink{}, &heldBy{})
	err := (DeleteRole{Roles: roles}).Execute(t.Context(), admin(), "service_lead")
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "roles.in_use" || refusal.Params["grants"] != "1" {
		t.Fatalf("deleting a held role: %v", err)
	}
	if len(store.roles) != 1 || len(store.grants) != 1 {
		t.Fatal("the refused deletion took the role or its grant")
	}

	delete(store.grants, membershipID)
	roles.Rules = namingRules{"service_lead": 3}
	if err := (DeleteRole{Roles: roles}).Execute(t.Context(), admin(), "service_lead"); shared.AsError(err).Params["rules"] != "3" {
		t.Fatalf("deleting a role three rules address: %v", err)
	}
	roles.Rules = namingRules{}
	if err := (DeleteRole{Roles: roles}).Execute(t.Context(), admin(), "service_lead"); err != nil {
		t.Fatalf("deleting a role nobody holds: %v", err)
	}
	if len(store.roles) != 0 {
		t.Fatal("the role stayed")
	}
}

type namingRules map[string]int

func (n namingRules) Role(_ context.Context, key string) (int, error) { return n[key], nil }

// A6, A11: a person may ask about themselves without any permission; about somebody else it takes
// MANAGE_MEMBERS or READ_CONFIGURATION, and the answer names the role that carries the asked one.
func TestExplainAccessAnswersWhoHoldsWhatAndThroughWhichRole(t *testing.T) {
	held := &heldBy{held: access.Held{
		Roles:       []string{"MEMBER"},
		Permissions: []service.Permission{service.PermissionRead, service.PermissionWriteItems},
		Sources:     []access.Source{{Role: "MEMBER", Scope: domain.HubScope(hubID)}},
	}}
	auth := &authorizer{}
	explain := ExplainAccess{Roles: rolesWith(newRoleStore(), auth, &auditSink{}, held)}

	self, err := explain.Execute(t.Context(), admin(), shared.ID(""), domain.HubScope(hubID), "WRITE_ITEMS", "")
	if err != nil || !self.Holds || len(self.HeldThrough) != 1 || len(auth.requests) != 0 {
		t.Fatalf("about oneself: %+v %v %d", self, err, len(auth.requests))
	}
	other, err := explain.Execute(t.Context(), admin(), invitedID, domain.HubScope(hubID), "STRUCTURE", "")
	if err != nil || other.Holds {
		t.Fatalf("about somebody else: %+v %v", other, err)
	}
	if len(auth.requests) != 1 || auth.requests[0].Alternative != service.PermissionReadConfiguration {
		t.Fatalf("asked %+v", auth.requests)
	}
	auth.refuse = shared.ErrForbidden
	if _, err := explain.Execute(t.Context(), admin(), invitedID, domain.TenantScope(), "", ""); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("refused: %v", err)
	}
}

// UC-ID-16: a workspace administrator reads the custom-role grants of every place they reach and
// none inside a private hub they hold nothing in.
func TestListRolesLeavesOutTheGrantsOfAPrivateHubTheCallerCannotReach(t *testing.T) {
	store := newRoleStore(serviceLead(t))
	for id, scope := range map[shared.ID]domain.Scope{
		membershipID: domain.HubScope(hubID),
		otherGrantID: domain.HubScope(otherHubID),
		tenantGrant:  domain.TenantScope(),
	} {
		store.grants[id] = repository.CustomGrantRow{RoleKey: "service_lead", Grant: domain.CustomGrant{
			ID: id, TenantID: tenant, AccountID: invitedID, Scope: scope, RoleID: customRoleID,
		}}
	}
	roles := rolesWith(store, &authorizer{}, &auditSink{}, &heldBy{})
	roles.Reach = reach{hidden: map[shared.ID]bool{otherHubID: true}}

	catalogue, err := ListRoles{Roles: roles}.Execute(t.Context(), reader())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	got := map[shared.ID]bool{}
	for _, row := range catalogue.Grants {
		got[row.Grant.ID] = true
	}
	if got[otherGrantID] {
		t.Fatal("a grant inside a private hub the caller cannot reach is listed")
	}
	if !got[membershipID] || !got[tenantGrant] || len(got) != 2 {
		t.Fatalf("listed %v, want the shared hub's and the workspace's grant", got)
	}
}

// UC-ID-16: what somebody else holds inside a private hub the caller cannot reach is not found, in
// the words a missing hub produces, and nothing is read for it.
func TestExplainAccessDoesNotLookIntoAPrivateHubTheCallerCannotReach(t *testing.T) {
	held := &heldBy{held: access.Held{
		Roles:   []string{"MEMBER"},
		Sources: []access.Source{{Role: "service_lead", Custom: true, Scope: domain.HubScope(otherHubID)}},
	}}
	roles := rolesWith(newRoleStore(), &authorizer{}, &auditSink{}, held)
	roles.Reach = reach{hidden: map[shared.ID]bool{otherHubID: true}}

	_, err := ExplainAccess{Roles: roles}.Execute(t.Context(), reader(), invitedID, domain.HubScope(otherHubID), "", "")
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("about somebody else in a hidden private hub: %v, want not found", err)
	}
	if len(held.asked) != 0 {
		t.Fatalf("read what %v holds in a hub the caller cannot reach", held.asked)
	}
}

// A role deleted while somebody grants it: the deletion holds the role before it counts its grants,
// so a grant committed meanwhile refuses the deletion instead of vanishing with the role by
// cascade after it was announced and recorded as granted.
func TestARoleGrantedWhileItIsDeletedStaysWithItsGrant(t *testing.T) {
	store := newRoleStore(serviceLead(t))
	store.onLock = func() {
		store.grants[membershipID] = repository.CustomGrantRow{RoleKey: "service_lead", Grant: domain.CustomGrant{
			ID: membershipID, TenantID: tenant, AccountID: invitedID, Scope: domain.HubScope(hubID), RoleID: customRoleID,
		}}
	}
	err := (DeleteRole{Roles: rolesWith(store, &authorizer{}, &auditSink{}, &heldBy{})}).Execute(t.Context(), admin(), "service_lead")
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "roles.in_use" {
		t.Fatalf("a role granted meanwhile was deleted: %v", err)
	}
	if len(store.roles) != 1 || len(store.grants) != 1 {
		t.Fatal("the grant went with the role")
	}
}

// A grant holds the role it hands out: one deleted meanwhile is not found, and one whose
// permissions changed meanwhile is asked of the ceiling again as it now stands.
func TestAGrantHoldsTheRoleItHandsOut(t *testing.T) {
	store, auth := newRoleStore(serviceLead(t)), &authorizer{}
	grant := GrantRole{
		Roles:  rolesWith(store, auth, &auditSink{}, &heldBy{}),
		Grants: GrantMembership{Accounts: newAccounts(invitedAccount(t)), Groups: newGroups(), Hubs: &hubLocks{}},
	}
	command := GrantRoleCommand{Role: "service_lead", AccountID: invitedID, Scope: domain.HubScope(hubID)}
	store.onLock = func() { delete(store.roles, "service_lead") }
	if _, err := grant.Execute(t.Context(), admin(), command); shared.AsError(err) == nil || shared.AsError(err).DetailCode != "roles.not_found" {
		t.Fatalf("a role deleted meanwhile was granted: %v", err)
	}
	if len(store.grants) != 0 {
		t.Fatal("a grant of a deleted role was kept")
	}

	store.roles["service_lead"] = repository.CustomRoleRow{Role: serviceLead(t), Version: 1}
	store.onLock = func() {
		widened := store.roles["service_lead"]
		widened.Role.Permissions = []string{"MANAGE_MEMBERS", "READ", "STRUCTURE"}
		widened.Version = 2
		store.roles["service_lead"] = widened
	}
	auth.requests = nil
	if _, err := grant.Execute(t.Context(), admin(), command); err != nil {
		t.Fatalf("granting: %v", err)
	}
	last := auth.requests[len(auth.requests)-1]
	if !slices.Equal(last.HandsOut, []string{"MANAGE_MEMBERS", "READ", "STRUCTURE"}) {
		t.Fatalf("the grant was judged on the permissions read before: %+v", auth.requests)
	}
}

type administration []shared.ID

func (a administration) Administrators(context.Context, []domain.Scope) ([]shared.ID, error) {
	return a, nil
}

type namedAccounts map[shared.ID]domain.Account

func (n namedAccounts) Find(_ context.Context, id shared.ID) (domain.Account, error) {
	if account, ok := n[id]; ok {
		return account, nil
	}
	return domain.Account{}, shared.ErrNotFound
}

// user-experience.md §5.5: a person who may not create at a place is told whom to ask - up to three
// of its administrators by name, active ones only, and only about oneself.
func TestExplainAccessNamesWhomToAskAtAPlace(t *testing.T) {
	ben, cleo, dan, eve := shared.ID("01936f2a-7c1e-7000-8000-0000000000c2"), shared.ID("01936f2a-7c1e-7000-8000-0000000000c3"),
		shared.ID("01936f2a-7c1e-7000-8000-0000000000c4"), shared.ID("01936f2a-7c1e-7000-8000-0000000000c5")
	explain := ExplainAccess{
		Roles:          rolesWith(newRoleStore(), &authorizer{}, &auditSink{}, &heldBy{}),
		Administration: administration{ben, cleo, dan, eve, invitedID},
		Accounts: namedAccounts{
			ben:  {DisplayName: "Ben Ott", Status: domain.AccountActive},
			cleo: {DisplayName: "Anna Keller", Status: domain.AccountActive},
			dan:  {DisplayName: "Dan Roe", Status: domain.AccountDisabled},
			eve:  {DisplayName: "Eve Lund", Status: domain.AccountActive},
		},
	}
	self, err := explain.Execute(t.Context(), admin(), shared.ID(""), domain.HubScope(hubID), "", "")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, one := range self.Administrators {
		names = append(names, one.Name)
	}
	if !slices.Equal(names, []string{"Anna Keller", "Ben Ott", "Eve Lund"}) {
		t.Fatalf("named %v", names)
	}
	out := AccessExplainedOutput(self)["administrators"].([]map[string]any)
	if len(out) != 3 || out[0]["name"] != "Anna Keller" || out[0]["account_id"] != cleo.String() {
		t.Fatalf("output %v", out)
	}
	other, err := explain.Execute(t.Context(), admin(), invitedID, domain.HubScope(hubID), "", "")
	if err != nil || len(other.Administrators) != 0 {
		t.Fatalf("about somebody else: %v %v", other.Administrators, err)
	}
	workspace, err := explain.Execute(t.Context(), admin(), shared.ID(""), domain.TenantScope(), "", "")
	if err != nil || len(workspace.Administrators) != 3 {
		t.Fatalf("at the workspace: %v %v", workspace.Administrators, err)
	}
}
