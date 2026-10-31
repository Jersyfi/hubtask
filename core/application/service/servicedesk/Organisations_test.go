// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"slices"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var colleagueID = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f9")

type organisationStore struct {
	rows    map[string]domain.Organisation
	members map[string][]shared.ID
}

func (s *organisationStore) Insert(_ context.Context, o domain.Organisation) error {
	for _, held := range s.rows {
		for _, d := range o.Domains {
			if slices.Contains(held.Domains, d) {
				return shared.ErrConflict.WithDetail("organisations.domain_taken")
			}
		}
	}
	s.rows[o.Key] = o
	return nil
}
func (s *organisationStore) Update(_ context.Context, o domain.Organisation, _ int) error {
	s.rows[o.Key] = o
	return nil
}
func (s *organisationStore) Find(_ context.Context, key string) (domain.Organisation, error) {
	o, ok := s.rows[key]
	if !ok {
		return domain.Organisation{}, shared.ErrNotFound
	}
	return o, nil
}
func (s *organisationStore) List(context.Context) ([]domain.Organisation, error) { return nil, nil }
func (s *organisationStore) Count(context.Context) (int, error)                  { return len(s.rows), nil }
func (s *organisationStore) Delete(_ context.Context, key string) error {
	delete(s.rows, key)
	return nil
}
func (s *organisationStore) OfDomain(_ context.Context, d string) (domain.Organisation, error) {
	for _, o := range s.rows {
		if slices.Contains(o.Domains, d) {
			return o, nil
		}
	}
	return domain.Organisation{}, shared.ErrNotFound
}
func (s *organisationStore) Members(_ context.Context, domains []string, _ int) ([]shared.ID, error) {
	var out []shared.ID
	for _, d := range domains {
		out = append(out, s.members[d]...)
	}
	return out, nil
}

type addressBook struct {
	identityrepo.Accounts
	emails map[shared.ID]string
}

func (b addressBook) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	email, ok := b.emails[id]
	if !ok {
		return identity.Account{}, shared.ErrNotFound
	}
	return identity.Account{ID: id, Email: email}, nil
}

type queriedItems struct {
	itemStore
	searched []workrepo.ItemSearch
}

func (q *queriedItems) Query(_ context.Context, search workrepo.ItemSearch) (workrepo.ItemQueryResult, error) {
	q.searched = append(q.searched, search)
	return workrepo.ItemQueryResult{Items: []work.WorkItem{q.item}}, nil
}

type commentList struct{ workrepo.Comments }

func (commentList) List(context.Context, shared.ID, workrepo.Page) (workrepo.CommentPage, error) {
	return workrepo.CommentPage{}, nil
}

type noRecords struct{ recordStore }

// raisers is every account a filter names, walking its OR of IN leaves.
func raisers(node *view.Node) []shared.ID {
	if node == nil {
		return nil
	}
	var out []shared.ID
	for _, v := range node.Values {
		out = append(out, v.ID)
	}
	for i := range node.Nodes {
		out = append(out, raisers(&node.Nodes[i])...)
	}
	return out
}

func organisationPortal(t *testing.T, shares bool) (Portal, *queriedItems, *organisationStore) {
	t.Helper()
	store := &organisationStore{rows: map[string]domain.Organisation{}, members: map[string][]shared.ID{
		"acme.example": {requesterID, colleagueID},
	}}
	define := DefineRequesterOrganisation{Desk: Desk{
		Organisations: store, Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{},
		Clock: at(monday), IDs: &seqIDs{}, Text: text.Composing{},
	}}
	if _, err := define.Execute(t.Context(), reader(), DefineRequesterOrganisationCommand{
		Key: "acme", Name: "Acme", Domains: []string{"Acme.Example"}, SharesRequests: shares,
	}); err != nil {
		t.Fatal(err)
	}
	items := &queriedItems{itemStore: itemStore{item: work.WorkItem{ID: itemID, CreatedBy: colleagueID, CollectionID: itemID}}}
	return Portal{
		Items: items, Containers: containerStore{}, Comments: commentList{},
		Accounts: addressBook{emails: map[shared.ID]string{
			requesterID: "mia@acme.example", colleagueID: "jonas@ACME.example",
		}},
		Desk: Desk{
			Organisations: store, Records: &noRecords{}, Ratings: &ratingStore{rows: map[shared.ID]domain.Rating{}},
			Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday),
		},
	}, items, store
}

// UC-SVC-11 check 1: an organisation is defined by its mail domains, sharing off unless set, and a
// domain another organisation holds is refused.
func TestAnOrganisationIsDefinedByItsMailDomains(t *testing.T) {
	_, _, store := organisationPortal(t, false)
	if got := store.rows["acme"]; got.SharesRequests || !slices.Equal(got.Domains, []string{"acme.example"}) {
		t.Fatalf("defined %+v", got)
	}
	define := DefineRequesterOrganisation{Desk: Desk{
		Organisations: store, Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{},
		Clock: at(monday), IDs: &seqIDs{}, Text: text.Composing{},
	}}
	_, err := define.Execute(context.Background(), reader(), DefineRequesterOrganisationCommand{
		Key: "other", Name: "Other", Domains: []string{"acme.example"},
	})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "organisations.domain_taken" {
		t.Fatalf("a domain of another organisation: %v", err)
	}
}

// UC-SVC-11 checks 3 and 4: where the organisation shares its requests, a member lists everybody's
// of it and reads a colleague's - without the colleague's score and never answering it; where it
// does not, the list is refused and the colleague's request is not found.
func TestAnOrganisationsRequestsAreSeenWhereItSharesThem(t *testing.T) {
	ctx := context.Background()
	t.Run("shared", func(t *testing.T) {
		portal, items, _ := organisationPortal(t, true)
		listed, err := ListMyRequests{Portal: portal}.Execute(ctx, requester(), itemID, RequestsOrganisation, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := raisers(items.searched[0].Spec.Filter); !slices.Contains(got, requesterID) || !slices.Contains(got, colleagueID) {
			t.Errorf("the list asked for %v", got)
		}
		if listed.Organisation == nil || listed.Organisation.Name != "Acme" {
			t.Errorf("the organisation named %+v", listed.Organisation)
		}
		portal.Desk.Ratings.(*ratingStore).rows[itemID] = domain.Rating{ItemID: itemID, Score: 2}
		read, err := ReadMyRequest{Portal: portal}.Execute(ctx, requester(), itemID)
		if err != nil || read.Rating != nil {
			t.Errorf("a colleague's request read: %+v %v", read.Rating, err)
		}
		if out := requestOutputFor(read.Item, requesterID); out["is_own"] != false {
			t.Errorf("a colleague's request said it was the reader's own")
		}
		if _, err := portal.ownRequest(ctx, requester(), itemID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a colleague's request was answerable: %v", err)
		}
	})
	t.Run("not shared", func(t *testing.T) {
		portal, items, _ := organisationPortal(t, false)
		_, err := ListMyRequests{Portal: portal}.Execute(ctx, requester(), itemID, RequestsOrganisation, "", 0)
		var refusal *shared.Error
		if !errors.As(err, &refusal) || refusal.DetailCode != "requests.organisation_not_shared" {
			t.Errorf("an unshared organisation's list: %v", err)
		}
		mine, err := ListMyRequests{Portal: portal}.Execute(ctx, requester(), itemID, "", "", 0)
		if err != nil || !slices.Equal(raisers(items.searched[len(items.searched)-1].Spec.Filter), []shared.ID{requesterID}) || mine.Organisation == nil {
			t.Errorf("the caller's own list: %v", err)
		}
		if _, err := (ReadMyRequest{Portal: portal}).Execute(ctx, requester(), itemID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("an unshared colleague's request was read: %v", err)
		}
	})
	t.Run("moved on", func(t *testing.T) {
		portal, _, _ := organisationPortal(t, true)
		portal.Accounts.(addressBook).emails[colleagueID] = "jonas@elsewhere.example"
		if _, err := (ReadMyRequest{Portal: portal}).Execute(ctx, requester(), itemID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the request of somebody whose address moved on was read: %v", err)
		}
	})
}

// UC-SVC-02 check 9: a policy names organisations the workspace holds, and reads them back.
func TestAPolicyNamesOrganisationsTheWorkspaceHolds(t *testing.T) {
	define := DefineSLAPolicy{Desk: Desk{
		Policies: policyStore{}, Organisations: &organisationStore{rows: map[string]domain.Organisation{"acme": {Key: "acme"}}},
		Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: &seqIDs{},
		Text: text.Composing{},
	}}
	command := DefineSLAPolicyCommand{
		Key: "key_accounts", Name: "Key accounts", EntryTypes: []string{"incident"}, Enabled: true,
		Organisations: []string{"acme", "initech"},
		Goals:         []domain.SLAGoal{{Kind: domain.GoalFirstResponse, Minutes: 30}},
	}
	if _, err := define.Execute(context.Background(), reader(), command); shared.AsError(err).DetailCode != "sla_policies.organisation_unknown" {
		t.Errorf("an organisation nobody holds: %v", err)
	}
	command.Organisations = []string{"acme"}
	policy, err := define.Execute(context.Background(), reader(), command)
	if err != nil {
		t.Fatal(err)
	}
	if out := PolicyOutput(policy); !slices.Equal(out["organisations"].([]string), []string{"acme"}) {
		t.Errorf("output %v", out["organisations"])
	}
}
