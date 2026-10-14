// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// grantStore answers the one grant a test gave.
type grantStore struct{ grant identity.Grant }

func (s grantStore) Find(_ context.Context, id shared.ID) (identity.Grant, error) {
	if id != s.grant.ID {
		return identity.Grant{}, shared.ErrNotFound.WithDetail("memberships.not_found")
	}
	return s.grant, nil
}

// placeStore answers the one hub or collection a test named.
type placeStore struct{ place work.Container }

func (s placeStore) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if id != s.place.ID {
		return work.Container{}, shared.ErrNotFound
	}
	return s.place, nil
}

// roleDelivery is the delivery with one pending record about a grant to Bert, given by Anna.
func roleDelivery(t *testing.T, grant identity.Grant) deliveryFixture {
	t.Helper()
	fixture := delivery(t, domain.CategoryMembership, true)
	record, err := domain.New(domain.NewInput{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e7"), TenantID: tenant, RecipientID: bert,
		Category: domain.CategoryMembership, Channel: domain.ChannelEmail,
		MembershipID: grant.ID, ActorID: anna, At: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.notifications.Insert(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	fixture.record = record
	fixture.delivery.Grants = grantStore{grant: grant}
	fixture.delivery.Places = placeStore{place: work.Container{ID: hubGranted, TenantID: tenant, Name: "Garden"}}
	return fixture
}

// UC-NOT-01 check 9 at the delivery: the mail names who gave the role, the role and the place it
// is on, and links to that place - a hub, a collection, an entry, or the installation for the
// whole workspace. The role and the place are rendered in the recipient's language (check 4).
func TestTheRoleMailNamesWhoTheRoleAndThePlace(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scope identity.Scope
		body  string
		want  []string
	}{
		{"a hub", identity.HubScope(hubGranted), bodyRoleGranted,
			[]string{"title=Garden", "place=" + placePrefix + "hub", "link=" + baseURL + "/hubs/" + hubGranted.String()}},
		{"a collection", identity.CollectionScope(hubGranted), bodyRoleGranted,
			[]string{"title=Garden", "place=" + placePrefix + "collection", "link=" + baseURL + "/collections/" + hubGranted.String()}},
		{"an entry", identity.ItemScope(itemID), bodyRoleGranted,
			[]string{"title=Review the quote", "place=" + placePrefix + "item", "link=" + baseURL + "/items/" + itemID.String()}},
		{"the workspace", identity.TenantScope(), bodyRoleGrantedWorkspace,
			[]string{"link=" + baseURL}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			grant := grantOn(tc.scope, bert, "")
			grant.Role = identity.RoleContributor
			fixture := roleDelivery(t, grant)

			if err := fixture.delivery.Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
				t.Fatalf("delivering: %v", err)
			}
			if len(fixture.mailbox.sent) != 1 {
				t.Fatalf("%d messages sent, want one", len(fixture.mailbox.sent))
			}
			body := fixture.mailbox.sent[0].Body
			if !strings.HasPrefix(body, tc.body+" ") {
				t.Errorf("the body %q is not %s", body, tc.body)
			}
			for _, want := range append(tc.want, "actor=Anna", "role="+rolePrefix+"contributor") {
				if !strings.Contains(body, want) {
					t.Errorf("the body %q lacks %q", body, want)
				}
			}
			if tc.scope.Type == identity.ScopeTenant && strings.Contains(body, "place=") {
				t.Errorf("the workspace's mail names a place: %q", body)
			}
			if saved, _ := fixture.notifications.Find(t.Context(), fixture.record.ID); saved.State != domain.StateSent {
				t.Errorf("record %s after the send, want sent", saved.State)
			}
		})
	}
}

// The title preference holds for the place's name as for an entry's, and a place that is gone
// leaves the same withheld sentence - the role and the link still travel.
func TestTheRoleMailWithholdsThePlaceName(t *testing.T) {
	for _, tc := range []struct {
		name     string
		prepare  func(*deliveryFixture)
		withheld bool
	}{
		{"titles kept out", func(f *deliveryFixture) {
			quiet := domain.DefaultPreference(tenant, bert, domain.CategoryMembership, domain.ChannelEmail)
			quiet.IncludeTitle = false
			_ = f.preferences.Save(context.Background(), quiet)
		}, true},
		{"the hub gone", func(f *deliveryFixture) { f.delivery.Places = placeStore{} }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := roleDelivery(t, grantOn(identity.HubScope(hubGranted), bert, ""))
			tc.prepare(&fixture)

			if err := fixture.delivery.Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
				t.Fatalf("delivering: %v", err)
			}
			body := fixture.mailbox.sent[0].Body
			if !strings.HasPrefix(body, bodyRoleGranted+withheldSuffix+" ") || strings.Contains(body, "Garden") {
				t.Errorf("the body %q, want the withheld sentence without the hub's name", body)
			}
			for _, want := range []string{"role=" + rolePrefix + "member", "link=" + baseURL + "/hubs/"} {
				if !strings.Contains(body, want) {
					t.Errorf("the body %q lacks %q", body, want)
				}
			}
		})
	}
}

// A role revoked before its mail went is not announced: the grant is gone, the job ends quietly.
func TestARevokedRoleIsNotMailed(t *testing.T) {
	fixture := roleDelivery(t, grantOn(identity.HubScope(hubGranted), bert, ""))
	fixture.delivery.Grants = grantStore{}

	if err := fixture.delivery.Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(fixture.mailbox.sent) != 0 {
		t.Errorf("%d messages about a role no longer held", len(fixture.mailbox.sent))
	}
}
