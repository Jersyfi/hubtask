// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package searchtest

import (
	"context"
	"errors"
	"testing"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The fakes stand in for searchrepo.Names, the unit of work and the access reader in every finder's
// tests; these hold them to what the real ones promise, so a finder test cannot pass on a fake that
// answers what the real port never would.
var (
	_ searchrepo.Names = Names{}
)

// EntryByKey answers at most one living entry, matched on the key's word whatever its case, and
// only in the hub where one is given (searchrepo.Names).
func TestEntryByKeyAnswersTheKeyedEntryInItsHubAlone(t *testing.T) {
	entry := Row("0193a0c0-0000-7000-8000-0000000000e1", HubA, "Login fails")
	names := Names{Keys: map[string]searchrepo.Row{"sa-12": entry}}
	tests := []struct {
		name string
		key  searchrepo.Key
		hub  shared.ID
		want int
	}{
		{"the key in any hub", searchrepo.Key{Word: "SA", Number: 12}, "", 1},
		{"the key in its own hub", searchrepo.Key{Word: "sa", Number: 12}, HubA, 1},
		{"the key in another hub", searchrepo.Key{Word: "SA", Number: 12}, HubB, 0},
		{"another number", searchrepo.Key{Word: "SA", Number: 13}, "", 0},
		{"another word", searchrepo.Key{Word: "SB", Number: 12}, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := names.EntryByKey(t.Context(), tt.key, tt.hub)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != tt.want || (tt.want == 1 && rows[0].ID != entry.ID) {
				t.Fatalf("rows %+v, want %d of %s", rows, tt.want, entry.ID)
			}
		})
	}
}

// Every kind answers the same candidates, and each question is kept in the order asked.
func TestEveryKindAnswersTheCandidatesAndKeepsTheQuestion(t *testing.T) {
	var asks []searchrepo.Ask
	rows := []searchrepo.Row{Row("0193a0c0-0000-7000-8000-0000000000e2", HubA, "Plan")}
	names := Names{Rows: rows, Asks: &asks}
	kinds := map[string]func(context.Context, searchrepo.Ask) ([]searchrepo.Row, error){
		"whiteboards": names.Whiteboards, "goals": names.Goals, "dashboards": names.Dashboards,
		"cis": names.Cis, "portfolios": names.Portfolios, "releases": names.Releases,
		"sprints": names.Sprints, "people": names.People, "entry files": names.EntryFiles,
		"entry comments": names.EntryComments, "page files": names.PageFiles,
		"page comments": names.PageComments, "whiteboard files": names.WhiteboardFiles,
		"whiteboard comments": names.WhiteboardComments,
	}
	for kind, find := range kinds {
		before := len(asks)
		ask := searchrepo.Ask{Terms: []string{kind}, HubID: HubA, Batch: 7}
		got, err := find(t.Context(), ask)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if len(got) != 1 || got[0].ID != rows[0].ID {
			t.Fatalf("%s answered %+v, want the candidates", kind, got)
		}
		if len(asks) != before+1 || asks[before].Terms[0] != kind || asks[before].Batch != 7 {
			t.Fatalf("%s kept %+v, want its question last", kind, asks)
		}
	}

	// Without a place to keep them, the questions are simply not kept.
	if got, err := (Names{Rows: rows}).Goals(t.Context(), searchrepo.Ask{}); err != nil || len(got) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// The unit of work runs the function once and hands back its error, read-only or not.
func TestTheUnitOfWorkCallsThrough(t *testing.T) {
	failure := errors.New("refused")
	for name, within := range map[string]func(context.Context, persistence.Scope, func(context.Context) error) error{
		"within": UnitOfWork{}.Within, "read only": UnitOfWork{}.WithinReadOnly,
	} {
		calls := 0
		err := within(t.Context(), persistence.Scope{}, func(context.Context) error { calls++; return failure })
		if calls != 1 || !errors.Is(err, failure) {
			t.Fatalf("%s: %d calls, %v; want one call and its error", name, calls, err)
		}
	}
}

// The reader answers per path as the access service does: the workspace alone for a one-step path,
// otherwise the entry it ends on where entries are named, else the hub; every path is kept.
func TestTheReaderAnswersPerPath(t *testing.T) {
	entry := shared.ID("0193a0c0-0000-7000-8000-0000000000e3")
	workspace := identity.TenantScope()
	inHub := func(hub shared.ID) []identity.Scope {
		return []identity.Scope{workspace, identity.HubScope(hub)}
	}
	onEntry := append(inHub(HubA), identity.ItemScope(entry))
	request := access.Request{TokenScope: "items:read"}
	tests := []struct {
		name   string
		reader HubReader
		paths  [][]identity.Scope
		want   []bool
	}{
		{"the hubs it names", HubReader{Hubs: map[shared.ID]bool{HubA: true}}, [][]identity.Scope{inHub(HubA), inHub(HubB)}, []bool{true, false}},
		{"the workspace when set", HubReader{Workspace: true}, [][]identity.Scope{{workspace}, inHub(HubA)}, []bool{true, false}},
		{"not the workspace otherwise", HubReader{Hubs: map[shared.ID]bool{HubA: true}}, [][]identity.Scope{{workspace}}, []bool{false}},
		{"the entries it names, not their hub", HubReader{Hubs: map[shared.ID]bool{HubA: true}, Entries: map[shared.ID]bool{entry: true}}, [][]identity.Scope{onEntry, inHub(HubA)}, []bool{true, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var asked [][]identity.Scope
			tt.reader.Asked = &asked
			got, err := tt.reader.Permitted(t.Context(), Actor("items:read"), request, tt.paths)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
			if len(asked) != len(tt.paths) {
				t.Fatalf("kept %d paths, want %d", len(asked), len(tt.paths))
			}
		})
	}
}

// The token scope refuses the whole question, as access.Service.Permitted does, and an
// anonymous caller is refused before anything.
func TestTheReaderHoldsTheTokenScopeAsTheAccessServiceDoes(t *testing.T) {
	reader := HubReader{Workspace: true}
	path := [][]identity.Scope{{identity.TenantScope()}}
	tests := []struct {
		name    string
		actor   appshared.ActorContext
		scope   string
		refusal error
	}{
		{"the scope held", Actor("items:read"), "items:read", nil},
		{"the scope missing", Actor("containers:read"), "items:read", shared.ErrForbidden},
		{"nobody signed in", appshared.ActorContext{}, "items:read", shared.ErrUnauthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reader.Permitted(t.Context(), tt.actor, access.Request{TokenScope: tt.scope}, path)
			if tt.refusal == nil {
				if err != nil || len(got) != 1 || !got[0] {
					t.Fatalf("got %v, %v; want the workspace readable", got, err)
				}
				return
			}
			if !errors.Is(err, tt.refusal) {
				t.Fatalf("err %v, want %v", err, tt.refusal)
			}
		})
	}
}

// Permits answers whether the reader is a member, nothing else.
func TestPermitsAnswersMembership(t *testing.T) {
	for _, member := range []bool{true, false} {
		got, err := HubReader{Member: member}.Permits(t.Context(), Actor(), access.Request{})
		if err != nil || got != member {
			t.Fatalf("got %v, %v; want %v", got, err, member)
		}
	}
}

// Row places a titled candidate in its hub; Actor is a person of the workspace with the scopes.
func TestRowAndActorAreWhatTheFindersAssume(t *testing.T) {
	row := Row("0193a0c0-0000-7000-8000-0000000000e4", HubB, "Roadmap")
	if row.HubID != HubB || row.Title != "Roadmap" {
		t.Fatalf("row %+v", row)
	}
	actor := Actor("items:read")
	if !actor.IsAuthenticated() || actor.TenantID != Tenant || actor.AccountID != Account || !actor.HasScope("items:read") {
		t.Fatalf("actor %+v", actor)
	}
}
