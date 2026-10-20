// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-ID-20 check 5 and identity.md §13 against PostgreSQL, with the real authoriser: a new way in
// - a start password, an address for somebody else, an invitation link, a service account's token -
// is issued by an owner for anybody and by an administrator only for an account holding no role
// above their own. The roles are the stored ones, a group's included; a refusal is the authoriser's
// `access.role_above_own`, its DENIED entry is in the trail, and the proof is not spent.

// countingStepUp accepts every proof and counts what it was shown.
type countingStepUp struct{ judged *int }

func (s countingStepUp) Available() bool { return true }
func (s countingStepUp) Satisfied(context.Context, shared.ID, string) (bool, error) {
	*s.judged++
	return true, nil
}
func (s countingStepUp) Methods(context.Context, shared.ID, shared.ID) ([]stepup.Method, error) {
	return nil, nil
}

type wayInWorld struct {
	clara, olga                 appshared.ActorContext
	ownerTarget, memberTarget   shared.ID
	ownerInvited, memberInvited shared.ID
	ownerMachine, memberMachine shared.ID
}

// seedWayInWorld gives tenant A an administrator (Clara) and an owner (Olga), and for each door an
// account holding OWNER - on a hub, through a group - and one holding MEMBER in its own right.
func seedWayInWorld(ctx context.Context, t *testing.T) wayInWorld {
	t.Helper()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	person := func(name, role string) shared.ID {
		id := freshID(t)
		exec(`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, $3)`, id.String(), tenantA.String(), name)
		exec(`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', $4)`,
			freshID(t).String(), tenantA.String(), id.String(), role)
		return id
	}
	actor := func(id shared.ID, name string) appshared.ActorContext {
		return appshared.ActorContext{
			Kind: shared.ActorUser, TenantID: tenantA, AccountID: id, AccountName: name,
			Scopes: []string{"members:write", "accounts:write"},
		}
	}
	w := wayInWorld{
		clara: actor(person("Clara", "ADMIN"), "Clara"),
		olga:  actor(person("Olga", "OWNER"), "Olga"),
	}

	owners := freshID(t)
	exec(`INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, $3)`,
		owners.String(), tenantA.String(), "Hub owners "+owners.String())
	exec(`INSERT INTO membership (id, tenant_id, group_id, scope_type, scope_id, role) VALUES ($1, $2, $3, 'HUB', $4, 'OWNER')`,
		freshID(t).String(), tenantA.String(), owners.String(), membershipHub.String())
	t.Cleanup(func() {
		if _, err := admin.Exec(context.WithoutCancel(ctx), `DELETE FROM account_group WHERE id = $1`, owners.String()); err != nil {
			t.Errorf("removing the group: %v", err)
		}
	})
	asOwner := func(id shared.ID) {
		exec(`INSERT INTO account_group_member (tenant_id, group_id, account_id) VALUES ($1, $2, $3)`,
			tenantA.String(), owners.String(), id.String())
	}
	asMember := func(id shared.ID) {
		exec(`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role) VALUES ($1, $2, $3, 'HUB', $4, 'MEMBER')`,
			freshID(t).String(), tenantA.String(), id.String(), membershipHub.String())
	}

	managed := func() shared.ID {
		account, err := identity.NewManagedAccount(freshID(t), tenantA, freshSignInName(t), "Lena", nil)
		if err != nil {
			t.Fatalf("building the account: %v", err)
		}
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return postgres.NewAccountRepository().InsertManaged(ctx, account, "argon2id-hash", created)
		}); err != nil {
			t.Fatalf("writing the account: %v", err)
		}
		return account.ID
	}
	w.ownerTarget, w.memberTarget = managed(), managed()
	invited := func() shared.ID {
		id := freshID(t)
		exec(`INSERT INTO account (id, tenant_id, kind, email, display_name, status) VALUES ($1, $2, 'USER', $3, 'Invited', 'INVITED')`,
			id.String(), tenantA.String(), "invited-"+id.String()+"@example.org")
		return id
	}
	w.ownerInvited, w.memberInvited = invited(), invited()
	machine := func() shared.ID {
		id := freshID(t)
		exec(`INSERT INTO account (id, tenant_id, kind, display_name, status) VALUES ($1, $2, 'SERVICE_ACCOUNT', 'Export', 'ACTIVE')`,
			id.String(), tenantA.String())
		return id
	}
	w.ownerMachine, w.memberMachine = machine(), machine()
	for _, id := range []shared.ID{w.ownerTarget, w.ownerInvited, w.ownerMachine} {
		asOwner(id)
	}
	for _, id := range []shared.ID{w.memberTarget, w.memberInvited, w.memberMachine} {
		asMember(id)
	}
	return w
}

// deniedFor counts the trail's refusals of the action against the target.
func deniedFor(ctx context.Context, t *testing.T, action string, target shared.ID) int {
	t.Helper()
	var denied int
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		WHERE tenant_id = $1 AND outcome = 'DENIED' AND action = $2 AND target_id = $3`,
		tenantA.String(), action, target.String()).Scan(&denied); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	return denied
}

func refusedByTheCeiling(t *testing.T, door string, err error) {
	t.Helper()
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "access.role_above_own" ||
		refusal.Params["role"] != "OWNER" {
		t.Errorf("%s answered %v, want access.role_above_own for OWNER", door, err)
	}
}

func TestANewWayInIsHeldToTheCeilingAsTheDatabaseResolvesIt(t *testing.T) {
	ctx := context.Background()
	w := seedWayInWorld(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	authoriser := realAuthoriser(ctx, t)
	held := postgres.NewMembershipRepository()
	sink := postgres.NewAuditSink(generator{t})
	ids := clockadapter.NewUUIDv7(clockadapter.System{})

	t.Run("a start password", func(t *testing.T) {
		judged := 0
		issue := identityservice.IssueStartPassword{
			Accounts: postgres.NewAccountRepository(), StartPasswords: postgres.NewAccountRepository(),
			Sessions: postgres.NewSessionRepository(), Authorizer: authoriser, Held: held,
			StepUp: countingStepUp{judged: &judged}, Passwords: plainHasher{}, Entropy: clockadapter.CryptoRandom{},
			Audit: sink, UnitOfWork: uow, Clock: clockadapter.System{},
		}
		_, err := issue.Execute(ctx, w.clara, w.ownerTarget, "proof")
		refusedByTheCeiling(t, "Clara for the owner", err)
		if judged != 0 {
			t.Error("the proof was judged before the ceiling refused")
		}
		if deniedFor(ctx, t, "account.start_password_issued", w.ownerTarget) != 1 {
			t.Error("the trail holds no refusal of Clara's start password")
		}
		if _, err := issue.Execute(ctx, w.clara, w.memberTarget, "proof"); err != nil {
			t.Errorf("Clara for the member: %v", err)
		}
		for _, target := range []shared.ID{w.ownerTarget, w.memberTarget} {
			if _, err := issue.Execute(ctx, w.olga, target, "proof"); err != nil {
				t.Errorf("Olga for %s: %v", target, err)
			}
		}
	})

	t.Run("an address for somebody else", func(t *testing.T) {
		request := identityservice.RequestAddress{
			Requests: addressStoreOf(), Authorizer: authoriser, Held: held,
			Notifier: postgres.NewQueue(ids, clockadapter.System{}), Audit: sink, UnitOfWork: uow,
			Clock: clockadapter.System{}, IDs: ids,
		}
		_, err := request.Execute(ctx, w.clara, w.ownerTarget, freshSignInName(t)+"@example.org")
		refusedByTheCeiling(t, "Clara for the owner", err)
		if deniedFor(ctx, t, "account.address_requested", w.ownerTarget) != 1 {
			t.Error("the trail holds no refusal of Clara's address request")
		}
		if _, err := request.Execute(ctx, w.clara, w.memberTarget, freshSignInName(t)+"@example.org"); err != nil {
			t.Errorf("Clara for the member: %v", err)
		}
		// Ten minutes between two requests for one account: Olga asks for the owner only.
		if _, err := request.Execute(ctx, w.olga, w.ownerTarget, freshSignInName(t)+"@example.org"); err != nil {
			t.Errorf("Olga for the owner: %v", err)
		}
	})

	t.Run("an invitation link", func(t *testing.T) {
		_, _, signIn, _ := sessionStores(ctx, t)
		renew := identityservice.RenewInvitation{
			Accounts: postgres.NewAccountRepository(), Authorizer: authoriser, Held: held,
			Notifier: postgres.NewQueue(ids, clockadapter.System{}), Audit: sink, UnitOfWork: uow,
			Clock: clockadapter.System{},
			Links: identityservice.InvitationLinks{
				Minter: identityservice.MintRedemptionToken{
					Accounts: signIn, UnitOfWork: uow, Clock: clockadapter.System{}, Entropy: clockadapter.CryptoRandom{},
				},
				BaseURL: "https://home.example",
			},
		}
		_, err := renew.Execute(ctx, w.clara, identityservice.RenewInvitationCommand{AccountID: w.ownerInvited})
		refusedByTheCeiling(t, "Clara for the invited owner", err)
		if deniedFor(ctx, t, "account.invitation_renewed", w.ownerInvited) != 1 {
			t.Error("the trail holds no refusal of Clara's renewal")
		}
		if _, err := renew.Execute(ctx, w.clara, identityservice.RenewInvitationCommand{AccountID: w.memberInvited}); err != nil {
			t.Errorf("Clara for the invited member: %v", err)
		}
		for _, target := range []shared.ID{w.ownerInvited, w.memberInvited} {
			if _, err := renew.Execute(ctx, w.olga, identityservice.RenewInvitationCommand{AccountID: target}); err != nil {
				t.Errorf("Olga for %s: %v", target, err)
			}
		}
	})

	t.Run("a service account's token", func(t *testing.T) {
		tokens, _ := tokenRepository(ctx, t)
		mint := identityservice.CreateAccessToken{Writer: identityservice.AccessTokenWriter{
			Tokens: tokens, Accounts: postgres.NewAccountRepository(), Authorizer: authoriser, Held: held,
			Audit: sink, UnitOfWork: uow, Clock: clockadapter.System{}, IDs: ids,
			Entropy: clockadapter.CryptoRandom{}, KnownScopes: []string{"items:read"},
		}}
		command := func(owner shared.ID) identityservice.CreateAccessTokenCommand {
			return identityservice.CreateAccessTokenCommand{
				AccountID: owner, Name: "the nightly export", Scopes: []string{"items:read"},
				ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
			}
		}
		_, err := mint.Execute(ctx, w.clara, command(w.ownerMachine))
		refusedByTheCeiling(t, "Clara for the owning machine", err)
		if deniedFor(ctx, t, "access.token_created", w.ownerMachine) != 1 {
			t.Error("the trail holds no refusal of Clara's mint")
		}
		if _, err := mint.Execute(ctx, w.clara, command(w.memberMachine)); err != nil {
			t.Errorf("Clara for the member machine: %v", err)
		}
		for _, target := range []shared.ID{w.ownerMachine, w.memberMachine} {
			if _, err := mint.Execute(ctx, w.olga, command(target)); err != nil {
				t.Errorf("Olga for %s: %v", target, err)
			}
		}
	})
}
