// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	revokedTenant = shared.ID("01936f2a-7c1e-7000-8000-0000000000e1")
	anna          = shared.ID("01936f2a-7c1e-7000-8000-0000000000e2")
	bert          = shared.ID("01936f2a-7c1e-7000-8000-0000000000e3")
	team          = shared.ID("01936f2a-7c1e-7000-8000-0000000000e4")
	hubA          = shared.ID("01936f2a-7c1e-7000-8000-0000000000e5")
	hubB          = shared.ID("01936f2a-7c1e-7000-8000-0000000000e6")
	collectionA   = shared.ID("01936f2a-7c1e-7000-8000-0000000000e7")
	collectionB   = shared.ID("01936f2a-7c1e-7000-8000-0000000000ea")
	itemA         = shared.ID("01936f2a-7c1e-7000-8000-0000000000e8")
)

// stillReads answers the permission question by account and by the last scope of the path: the
// set of (account, scope identifier) pairs that still read.
type stillReads struct {
	kept  map[string]bool
	asked []string
}

func key(account, scope shared.ID) string { return account.String() + "@" + scope.String() }

func (p *stillReads) Permits(_ context.Context, actor appshared.ActorContext, request Request) (bool, error) {
	last := request.Path[len(request.Path)-1]
	p.asked = append(p.asked, key(actor.AccountID, last.ID))
	return p.kept[key(actor.AccountID, last.ID)], nil
}

type grantReader struct {
	at      map[identity.Scope][]identity.Grant
	ofGroup map[shared.ID][]identity.Grant
}

func (g grantReader) ListAt(_ context.Context, scope identity.Scope, _ identityrepo.Page) (identityrepo.GrantPage, error) {
	return identityrepo.GrantPage{Grants: g.at[scope]}, nil
}

func (g grantReader) OfGroup(_ context.Context, groupID shared.ID) ([]identity.Grant, error) {
	return g.ofGroup[groupID], nil
}

type groupMembers map[shared.ID][]shared.ID

func (g groupMembers) Members(_ context.Context, groupID shared.ID) ([]shared.ID, error) {
	return g[groupID], nil
}

type containerReader struct{ containers map[shared.ID]work.Container }

func (c containerReader) Find(_ context.Context, id shared.ID) (work.Container, error) {
	container, found := c.containers[id]
	if !found {
		return work.Container{}, shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return container, nil
}

func (c containerReader) List(_ context.Context, query workrepo.ContainerQuery) (workrepo.ContainerPage, error) {
	var found []work.Container
	for _, container := range c.containers {
		if container.Type == query.Type && container.ParentID == query.ParentID {
			found = append(found, container)
		}
	}
	slices.SortFunc(found, func(a, b work.Container) int { return strings.Compare(string(a.ID), string(b.ID)) })
	return workrepo.ContainerPage{Containers: found}, nil
}

type itemReader struct{ items map[shared.ID]work.WorkItem }

func (i itemReader) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	item, found := i.items[id]
	if !found {
		return work.WorkItem{}, shared.ErrNotFound.WithDetail("items.not_found")
	}
	return item, nil
}

type recorded struct{ changes []changelog.Change }

func (r *recorded) Record(_ context.Context, change changelog.Change) error {
	r.changes = append(r.changes, change)
	return nil
}

type ticking struct{ n uint32 }

func (t *ticking) Next() shared.HLC {
	t.n++
	reading, _ := shared.NewHLC(time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC), t.n, "server")
	return reading
}

type revocationFixture struct {
	revocations Revocations
	permits     *stillReads
	changes     *recorded
}

func revoking(t *testing.T, kept ...string) revocationFixture {
	t.Helper()
	permits := &stillReads{kept: map[string]bool{}}
	for _, k := range kept {
		permits.kept[k] = true
	}
	changes := &recorded{}
	return revocationFixture{
		permits: permits, changes: changes,
		revocations: Revocations{
			Permits: permits,
			Grants: grantReader{
				at: map[identity.Scope][]identity.Grant{
					identity.HubScope(hubA): {
						{ID: shared.ID("g1"), AccountID: anna, Scope: identity.HubScope(hubA)},
						{ID: shared.ID("g2"), GroupID: team, Scope: identity.HubScope(hubA)},
					},
				},
				ofGroup: map[shared.ID][]identity.Grant{
					team: {
						{ID: shared.ID("g2"), GroupID: team, Scope: identity.HubScope(hubA)},
						{ID: shared.ID("g3"), GroupID: team, Scope: identity.CollectionScope(collectionA)},
					},
				},
			},
			Groups: groupMembers{team: {bert}},
			Containers: containerReader{containers: map[shared.ID]work.Container{
				hubA:        {ID: hubA, Type: work.ContainerHub},
				hubB:        {ID: hubB, Type: work.ContainerHub},
				collectionA: {ID: collectionA, Type: work.ContainerCollection, ParentID: hubA},
				collectionB: {ID: collectionB, Type: work.ContainerCollection, ParentID: hubA},
			}},
			Items:   itemReader{items: map[shared.ID]work.WorkItem{itemA: {ID: itemA, CollectionID: collectionA}}},
			Changes: changes,
			HLC:     &ticking{},
		},
	}
}

func (f revocationFixture) announced() []string {
	var out []string
	for _, change := range f.changes.changes {
		out = append(out, key(change.ActorID, change.ContainerID))
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The record is written where the effective access ends: an account that may no longer read the
// hub is told, at the hub; one that still reads it another way is told nothing.
func TestARevokedGrantIsAnnouncedWhereTheAccessEnds(t *testing.T) {
	cases := map[string]struct {
		grant identity.Grant
		kept  []string
		want  []string
	}{
		"a direct grant whose account reads nothing else": {
			grant: identity.Grant{AccountID: anna, Scope: identity.HubScope(hubA)},
			want:  []string{key(anna, hubA)},
		},
		"a direct grant whose account keeps a second path": {
			grant: identity.Grant{AccountID: anna, Scope: identity.HubScope(hubA)},
			kept:  []string{key(anna, hubA)},
		},
		"a group grant is announced to each member": {
			grant: identity.Grant{GroupID: team, Scope: identity.CollectionScope(collectionA)},
			want:  []string{key(bert, collectionA)},
		},
		"a workspace grant is announced per hub": {
			grant: identity.Grant{AccountID: anna, Scope: identity.TenantScope()},
			kept:  []string{key(anna, hubB)},
			want:  []string{key(anna, hubA)},
		},
		"a share is announced for the entry under its collection": {
			grant: identity.Grant{AccountID: anna, Scope: identity.ItemScope(itemA)},
			want:  []string{key(anna, collectionA)},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := revoking(t, tc.kept...)
			loss, err := f.revocations.AfterGrantRevoked(t.Context(), tc.grant)
			if err != nil {
				t.Fatalf("describing the loss: %v", err)
			}
			if err := f.revocations.Announce(t.Context(), revokedTenant, loss); err != nil {
				t.Fatalf("announcing: %v", err)
			}
			if got := f.announced(); !same(got, tc.want) {
				t.Errorf("announced %v, want %v", got, tc.want)
			}
			for _, change := range f.changes.changes {
				if change.Op != changelog.AccessRevoked || change.TenantID != revokedTenant || change.HLC.IsZero() {
					t.Errorf("the record is not an addressed revocation: %+v", change)
				}
				if change.Payload != nil {
					t.Errorf("a revocation carries content: %+v", change.Payload)
				}
			}
		})
	}
}

// A hub lost by somebody who still reads one of its collections on their own is announced as the
// collections they lost, one by one - a revocation at the hub would take the kept one with it.
func TestAHubLostIsNarrowedToTheCollectionsLostWhenOneIsKept(t *testing.T) {
	f := revoking(t, key(anna, collectionA))
	loss := Loss{Accounts: []shared.ID{anna}, Scope: identity.HubScope(hubA)}
	if err := f.revocations.Announce(t.Context(), revokedTenant, loss); err != nil {
		t.Fatalf("announcing: %v", err)
	}
	if got := f.announced(); !same(got, []string{key(anna, collectionB)}) {
		t.Errorf("announced %v, want the collection lost alone", got)
	}
}

// The share's record names the entry, not the collection: a device drops the one entry it was
// shown, not the collection it never held.
func TestAShareRevokedNamesTheEntry(t *testing.T) {
	f := revoking(t)
	loss, _ := f.revocations.AfterGrantRevoked(t.Context(), identity.Grant{AccountID: anna, Scope: identity.ItemScope(itemA)})
	if err := f.revocations.Announce(t.Context(), revokedTenant, loss); err != nil {
		t.Fatalf("announcing: %v", err)
	}
	if len(f.changes.changes) != 1 || f.changes.changes[0].Entity != "item" || f.changes.changes[0].EntityID != itemA {
		t.Errorf("recorded %+v, want the entry", f.changes.changes)
	}
	// And the question was put with the entry on the path, where a share is held.
	if len(f.permits.asked) != 1 || f.permits.asked[0] != key(anna, itemA) {
		t.Errorf("asked %v, want the entry's own scope", f.permits.asked)
	}
}

// A member taken out of a group loses each of the group's grants, and is asked about each.
func TestAMemberTakenOutOfAGroupLosesWhatTheGroupHeld(t *testing.T) {
	f := revoking(t, key(bert, hubA))
	losses, err := f.revocations.AfterGroupLoss(t.Context(), team, []shared.ID{bert})
	if err != nil {
		t.Fatalf("describing the losses: %v", err)
	}
	if len(losses) != 2 {
		t.Fatalf("described %d losses, want one per grant of the group", len(losses))
	}
	if err := f.revocations.Announce(t.Context(), revokedTenant, losses...); err != nil {
		t.Fatalf("announcing: %v", err)
	}
	// Bert keeps the hub through a direct grant and loses the collection.
	if got := f.announced(); !same(got, []string{key(bert, collectionA)}) {
		t.Errorf("announced %v, want the collection alone", got)
	}
}

func TestNobodyTakenOutOfAGroupIsNoLoss(t *testing.T) {
	f := revoking(t)
	losses, err := f.revocations.AfterGroupLoss(t.Context(), team, nil)
	if err != nil || losses != nil {
		t.Errorf("described %v, %v", losses, err)
	}
}

// A collection moved under another hub is lost by whoever read it through the hub it left - the
// direct holders and the group's members - unless they still read it where it now is.
func TestAMoveAsksEverybodyWhoHeldTheHubItLeft(t *testing.T) {
	f := revoking(t, key(anna, collectionA))
	from := work.Container{ID: collectionA, Type: work.ContainerCollection, ParentID: hubA}
	to := work.Container{ID: collectionA, Type: work.ContainerCollection, ParentID: hubB}

	loss, err := f.revocations.AfterContainerMoved(t.Context(), from, to)
	if err != nil {
		t.Fatalf("describing the loss: %v", err)
	}
	if err := f.revocations.Announce(t.Context(), revokedTenant, loss); err != nil {
		t.Fatalf("announcing: %v", err)
	}
	if got := f.announced(); !same(got, []string{key(bert, collectionA)}) {
		t.Errorf("announced %v, want Bert alone - Anna still reads it", got)
	}
}

func TestAMoveThatChangesNoParentIsNoLoss(t *testing.T) {
	f := revoking(t)
	same := work.Container{ID: collectionA, Type: work.ContainerCollection, ParentID: hubA}
	loss, err := f.revocations.AfterContainerMoved(t.Context(), same, same)
	if err != nil || len(loss.Accounts) != 0 {
		t.Errorf("described %v, %v", loss, err)
	}
}

// A root that is gone - the collection in the trash by the time the grant goes - is nothing to
// announce: its deletion is what the device applies.
func TestARootThatIsGoneIsNothingToAnnounce(t *testing.T) {
	f := revoking(t)
	gone := shared.ID("01936f2a-7c1e-7000-8000-0000000000e9")
	loss := Loss{Accounts: []shared.ID{anna}, Scope: identity.CollectionScope(gone)}
	if err := f.revocations.Announce(t.Context(), revokedTenant, loss); err != nil {
		t.Fatalf("announcing: %v", err)
	}
	if len(f.changes.changes) != 0 {
		t.Errorf("announced %+v for a container that is not there", f.changes.changes)
	}
}
