// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package service_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var (
	roleID   = shared.MustParseID("0192f000-0000-7000-8000-000000000101")
	tenantID = shared.MustParseID("0192f000-0000-7000-8000-000000000102")
	otherID  = shared.MustParseID("0192f000-0000-7000-8000-000000000103")
)

func custom(scope identity.Scope, key string, permissions ...service.Permission) identity.Membership {
	names := make([]string, len(permissions))
	for i, p := range permissions {
		names[i] = string(p)
	}
	return identity.Membership{AccountID: account, Scope: scope, Custom: key, Permissions: names}
}

func TestTheCatalogueKnowsItsPermissionsAndRefusesOthers(t *testing.T) {
	cases := []struct {
		name  string
		known bool
	}{
		{"READ", true},
		{"WRITE_ASSIGNED", true},
		{"REQUEST", true},
		{"APPROVE_TIME", true},
		{"FIELD_WRITE:cost", true},
		{"FIELD_READ:cost", true},
		{"FIELD_READ:", false},
		{"FIELD_READ:FIELD_WRITE:cost", false},
		{"TAKE_STEP:change/approve", true},
		{"FIELD_WRITE:", false},
		{"FIELD_WRITE:Cost", false},
		{"TAKE_STEP:change", false},
		{"TAKE_STEP:/approve", false},
		{"EVERYTHING", false},
		{"read", false},
	}
	for _, c := range cases {
		_, err := service.ParsePermission(c.name)
		if (err == nil) != c.known {
			t.Errorf("%s: known = %v, want %v", c.name, err == nil, c.known)
		}
	}
}

func TestEveryCatalogueEntryParsesAndNamesItsShippedHolders(t *testing.T) {
	for _, entry := range service.PermissionCatalogue() {
		if entry.Kind == service.KindBase {
			if _, err := service.ParsePermission(string(entry.Permission)); err != nil {
				t.Errorf("%s does not parse", entry.Permission)
			}
		}
		for _, role := range entry.Shipped {
			if entry.Kind == service.KindBase && !service.RoleAllows(role, entry.Permission) {
				t.Errorf("%s names %s, which does not hold it", entry.Permission, role)
			}
		}
	}
}

func TestDefineRoleRefusesAnUnknownPermissionAndAnEmptyOne(t *testing.T) {
	if _, err := service.DefineRole(roleID, tenantID, "service_lead", "Service lead", []string{"READ", "FLY"}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("unknown permission: %v", err)
	}
	if _, err := service.DefineRole(roleID, tenantID, "service_lead", "Service lead", nil); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("no permission: %v", err)
	}
	if _, err := service.DefineRole(roleID, tenantID, "ADMIN", "Admin", []string{"READ"}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a built-in's spelling: %v", err)
	}
	role, err := service.DefineRole(roleID, tenantID, "service_lead", "Service lead", []string{"STRUCTURE", "READ", "READ"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(role.Permissions, []string{"READ", "STRUCTURE"}) {
		t.Fatalf("permissions = %v", role.Permissions)
	}
}

func TestACustomRoleAllowsWhatItNamesAndNothingElse(t *testing.T) {
	held := []identity.Membership{custom(identity.HubScope(hub), "service_lead", service.PermissionRead, service.PermissionStructure)}
	if !service.Allows(held, collectionPth, service.SharedPath, service.PermissionStructure) {
		t.Fatal("STRUCTURE named and not allowed")
	}
	if service.Allows(held, collectionPth, service.SharedPath, service.PermissionManageMembers) {
		t.Fatal("MANAGE_MEMBERS not named and allowed")
	}
	if service.Allows(held, []identity.Scope{identity.TenantScope(), identity.HubScope(otherHub)}, service.SharedPath, service.PermissionRead) {
		t.Fatal("a hub's custom role reaches its neighbour")
	}
	if _, found := service.EffectiveRole(held, collectionPth, service.SharedPath); found {
		t.Fatal("a custom role is a rung on the ladder")
	}
	if !service.HoldsAnything(held, collectionPth, service.SharedPath) {
		t.Fatal("a custom role holds nothing")
	}
}

func TestACustomRolesItemReachIsTheShippedRowsSpelledAsPermissions(t *testing.T) {
	cases := []struct {
		permissions []service.Permission
		action      service.ItemAction
		want        service.ItemAccess
	}{
		{[]service.Permission{service.PermissionRead}, service.ItemRead, service.AccessAll},
		{[]service.Permission{service.PermissionRead}, service.ItemChange, service.AccessNone},
		{[]service.Permission{service.PermissionWriteItems}, service.ItemChange, service.AccessAll},
		{[]service.Permission{service.PermissionWriteAssigned}, service.ItemCreate, service.AccessAll},
		{[]service.Permission{service.PermissionWriteAssigned}, service.ItemChange, service.AccessAssigned},
		{[]service.Permission{service.PermissionWriteAssigned}, service.ItemComment, service.AccessAssigned},
		{[]service.Permission{service.PermissionComment}, service.ItemComment, service.AccessAll},
		{[]service.Permission{service.PermissionComment}, service.ItemChange, service.AccessNone},
		{[]service.Permission{service.PermissionStructure}, service.ItemRead, service.AccessNone},
	}
	for _, c := range cases {
		names := make([]string, len(c.permissions))
		for i, p := range c.permissions {
			names[i] = string(p)
		}
		if got := service.CustomItemAccess(names, c.action); got != c.want {
			t.Errorf("%v %s = %s, want %s", c.permissions, c.action, got, c.want)
		}
	}
}

func TestItemReachIsTheWidestOfWhatIsHeld(t *testing.T) {
	held := []identity.Membership{
		grant(identity.TenantScope(), identity.RoleViewer),
		custom(identity.HubScope(hub), "helper", service.PermissionWriteAssigned),
	}
	verdict, found := service.AllowsItemActionAlong(held, collectionPth, service.SharedPath, service.ItemChange, account, otherID)
	if !found || verdict != service.ItemRefusedByAssignment {
		t.Fatalf("somebody else's entry: %v %v", verdict, found)
	}
	verdict, _ = service.AllowsItemActionAlong(held, collectionPth, service.SharedPath, service.ItemChange, account, account)
	if verdict != service.ItemPermitted {
		t.Fatalf("their own entry: %v", verdict)
	}
	if _, found := service.AllowsItemActionAlong(nil, collectionPth, service.SharedPath, service.ItemRead, account, account); found {
		t.Fatal("nothing held and found")
	}
}

func TestFieldAndStepPermissionsAreTheShapersAndTheCustomRolesThatNameThem(t *testing.T) {
	cost := service.FieldWrite("cost")
	approve := service.TakeStep("change", "approve")
	for _, role := range identity.Roles() {
		want := role == identity.RoleOwner || role == identity.RoleAdmin
		if service.RoleAllows(role, cost) != want || service.RoleAllows(role, approve) != want {
			t.Errorf("%s: field/step = %v/%v, want %v", role, service.RoleAllows(role, cost), service.RoleAllows(role, approve), want)
		}
	}
	held := []identity.Membership{custom(identity.TenantScope(), "approver", service.PermissionRead, approve)}
	if !service.Allows(held, collectionPth, service.SharedPath, approve) || service.Allows(held, collectionPth, service.SharedPath, cost) {
		t.Fatal("a custom role's step permission")
	}
	if !slices.Contains(service.Held(held, collectionPth, service.SharedPath), approve) {
		t.Fatal("the step permission is not reported as held")
	}
}

func TestAFieldRestrictedForReadingIsSeenWithItsReadOrWritePermission(t *testing.T) {
	cases := []struct {
		name string
		held []service.Permission
		sees bool
	}{
		{"nothing", nil, false},
		{"its read permission", []service.Permission{service.FieldRead("cost")}, true},
		{"its write permission", []service.Permission{service.FieldWrite("cost")}, true},
		{"another field's", []service.Permission{service.FieldRead("salary"), service.FieldWrite("band")}, false},
		{"reading the entry", []service.Permission{service.PermissionRead, service.PermissionWriteItems}, false},
	}
	for _, c := range cases {
		got := service.SeesField(func(p service.Permission) bool { return slices.Contains(c.held, p) }, "cost")
		if got != c.sees {
			t.Errorf("%s: sees = %v, want %v", c.name, got, c.sees)
		}
	}
	for _, role := range identity.Roles() {
		want := role == identity.RoleOwner || role == identity.RoleAdmin
		if got := service.SeesField(func(p service.Permission) bool { return service.RoleAllows(role, p) }, "cost"); got != want {
			t.Errorf("%s sees a restricted field: %v, want %v", role, got, want)
		}
	}
	if service.FieldRead("cost").Kind() != service.KindField {
		t.Fatal("FIELD_READ is not of the field family")
	}
}

func TestNobodyHandsOutAPermissionTheyDoNotHold(t *testing.T) {
	admin := []identity.Membership{grant(identity.TenantScope(), identity.RoleAdmin)}
	if service.MayHandOut(admin, []identity.Scope{identity.TenantScope()}, service.SharedPath, []string{"DELETE_CONTAINER"}) {
		t.Fatal("an administrator hands out DELETE_CONTAINER")
	}
	if !service.MayHandOut(admin, []identity.Scope{identity.TenantScope()}, service.SharedPath, []string{"STRUCTURE", "FIELD_WRITE:cost"}) {
		t.Fatal("an administrator may not hand out what they hold")
	}
	if got := service.Lacking(admin, []identity.Scope{identity.TenantScope()}, service.SharedPath, []string{"READ", "DELETE_CONTAINER"}); !slices.Equal(got, []string{"DELETE_CONTAINER"}) {
		t.Fatalf("lacking = %v", got)
	}
	member := []identity.Membership{grant(identity.TenantScope(), identity.RoleMember)}
	if service.MayHandOut(member, []identity.Scope{identity.TenantScope()}, service.SharedPath, []string{"READ"}) {
		t.Fatal("a member hands out a role without MANAGE_MEMBERS")
	}
}

// D-05's property: over random schemes, what a grant under MayHandOut gives the grantee never
// reaches beyond what the granter holds there - no scheme is a ladder to more rights.
func TestNoRandomSchemeLetsAGrantReachBeyondTheGranter(t *testing.T) {
	permissions := []service.Permission{}
	for _, entry := range service.PermissionCatalogue() {
		if entry.Kind == service.KindBase {
			permissions = append(permissions, entry.Permission)
		}
	}
	permissions = append(permissions, service.FieldWrite("cost"), service.TakeStep("change", "approve"))
	scopes := []identity.Scope{identity.TenantScope(), identity.HubScope(hub), identity.CollectionScope(collectionID), identity.HubScope(otherHub)}

	seed := uint64(20261029)
	next := func(n int) int {
		seed = seed*6364136223846793005 + 1442695040888963407
		return int((seed >> 33) % uint64(n))
	}
	pick := func() []string {
		var names []string
		for _, p := range permissions {
			if next(3) == 0 {
				names = append(names, string(p))
			}
		}
		return names
	}

	for round := range 2000 {
		var granter []identity.Membership
		for range next(3) + 1 {
			scope := scopes[next(len(scopes))]
			if next(2) == 0 {
				granter = append(granter, grant(scope, identity.Roles()[next(len(identity.Roles()))]))
			} else {
				granter = append(granter, identity.Membership{AccountID: account, Scope: scope, Custom: "r", Permissions: pick()})
			}
		}
		at := scopes[next(3)]
		path := collectionPth[:slices.Index(collectionPth, at)+1]
		offered := pick()
		if !service.MayHandOut(granter, path, service.SharedPath, offered) {
			continue
		}
		grantee := []identity.Membership{{AccountID: otherID, Scope: at, Custom: "given", Permissions: offered}}
		for _, p := range permissions {
			if service.Allows(grantee, path, service.SharedPath, p) && !service.Allows(granter, path, service.SharedPath, p) {
				t.Fatalf("round %d: the grant reaches %s, which the granter lacks at %v", round, p, at)
			}
		}
	}
}

func TestARequesterRaisesRequestsAndReadsNothing(t *testing.T) {
	requester := []identity.Membership{custom(identity.TenantScope(), "requester", service.PermissionRequest)}
	if !service.IsRequester(requester, collectionPth, service.SharedPath) {
		t.Fatal("REQUEST alone is not a requester")
	}
	if service.Allows(requester, collectionPth, service.SharedPath, service.PermissionRead) {
		t.Fatal("a requester reads the team's work")
	}
	member := append(requester, grant(identity.HubScope(hub), identity.RoleMember))
	if service.IsRequester(member, collectionPth, service.SharedPath) {
		t.Fatal("a member of the team is a requester")
	}
}
