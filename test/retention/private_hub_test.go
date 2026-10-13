// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package retention

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	lifecycleDomain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-ID-16 check 5 (ADR-0073 §5): a retention rule is not a reader, so it removes a completed
// entry inside a private hub exactly as it removes one beside it - and a legal hold keeps it,
// whether it names the workspace or the private hub itself. The holds are placed by the
// workspace's owner, who is not a member of the hub, through the real authoriser: the controller's
// instrument has to reach what the controller cannot read.
func TestRetentionAndLegalHoldsApplyToAPrivateHub(t *testing.T) {
	for _, tc := range []struct {
		name string
		// hold names what the owner holds; empty places none.
		hold lifecycleDomain.HoldScope
	}{
		{"a rule removes the entry", ""},
		{"a hold on the workspace keeps it", lifecycleDomain.HoldTenant},
		{"a hold on the private hub keeps it", lifecycleDomain.HoldContainer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSuite(t, 24*time.Hour)
			owner := s.privateHubOwner(t)
			hub, collection := s.privateHub(t)
			inside := s.completedItem(t, collection, 400)
			beside := s.completedItem(t, s.collection(t), 400)
			// Work beside it, so the rule is not the broad one the activation switch announces
			// instead of acting on.
			elsewhere := s.collection(t)
			for range 60 {
				s.completedItem(t, elsewhere, 1)
			}

			s.createRule(t, lifecycle.CreateRetentionPolicyCommand{
				Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeTenant},
				DataKind: lifecycleDomain.KindCompletedItem, RetainDays: 365,
				Action: lifecycleDomain.ActionHardDelete, GraceDays: graceOf(0),
			})

			if tc.hold != "" {
				var target shared.ID // a hold on the workspace names nothing below it
				if tc.hold == lifecycleDomain.HoldContainer {
					target = hub
				}
				if _, err := (lifecycle.PlaceLegalHold{Holds: s.holdsAuthorisedForReal()}).Execute(
					s.ctx, s.ownerActor(owner), lifecycle.PlaceLegalHoldCommand{
						Scope: tc.hold, ScopeID: target, Reason: "Pending litigation, ref. 4 O 128/26",
					}); err != nil {
					t.Fatalf("the workspace's owner could not place the hold: %v", err)
				}
			}

			// Two passes, because the rule announces before it acts.
			s.sweep(t)
			s.sweepAt(t, now.Add(time.Minute))

			if tc.hold == "" {
				if s.exists(t, inside) {
					t.Error("the rule left a completed entry in a private hub that it removed beside it")
				}
				if s.exists(t, beside) {
					t.Error("the rule did not act at all")
				}
				return
			}
			if !s.exists(t, inside) {
				t.Error("an entry in a private hub under a legal hold was deleted")
			}
			if _, reason := s.itemRetention(t, inside); reason != lifecycleDomain.BlockedByLegalHold {
				t.Errorf("the entry says %q is stopping it, want the legal hold", reason)
			}
		})
	}
}

// privateHub seeds a private hub with a collection in it, and a member of the hub other than the
// workspace's owner.
func (s *suite) privateHub(t *testing.T) (hub, collection shared.ID) {
	t.Helper()
	hub, collection = freshID(t), freshID(t)
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by, private)
		VALUES ($1, $2, 'HUB', NULL, $3, 'a0', $5, true),
		       ($4, $2, 'COLLECTION', $1, 'Presents', 'a0', $5, false)`,
		hub.String(), s.tenant.String(), "Private "+hub.String(), collection.String(),
		s.author.String()); err != nil {
		t.Fatalf("seeding the private hub: %v", err)
	}
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		VALUES ($1, $2, $3, 'HUB', $4, 'OWNER')`,
		freshID(t).String(), s.tenant.String(), s.author.String(), hub.String()); err != nil {
		t.Fatalf("seeding the hub's member: %v", err)
	}
	return hub, collection
}

// privateHubOwner seeds the workspace's owner, who holds nothing on the private hub.
func (s *suite) privateHubOwner(t *testing.T) shared.ID {
	t.Helper()
	owner := s.account(t)
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
		freshID(t).String(), s.tenant.String(), owner.String()); err != nil {
		t.Fatalf("seeding the workspace's owner: %v", err)
	}
	return owner
}

func (s *suite) ownerActor(owner shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: s.tenant, AccountID: owner,
		AccountName: "Bert", Locale: "en", TimeZone: "UTC", Scopes: []string{"retention:manage"},
	}
}

// holdsAuthorisedForReal is holds() with the authoriser the server wires instead of allowAll,
// because who may place a hold on a private hub is the question.
func (s *suite) holdsAuthorisedForReal() lifecycle.Holds {
	holds := s.holds()
	holds.Authorizer = access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: s.uow,
		Audit: postgres.NewAuditSink(ids), Clock: clock.Fixed(now),
	}
	return holds
}
