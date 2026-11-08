// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type starStore struct{ lists map[shared.ID]domain.StarList }

func (s *starStore) Find(_ context.Context, id shared.ID) (domain.StarList, error) {
	return s.lists[id], nil
}

func (s *starStore) Replace(_ context.Context, id shared.ID, stars []domain.Star, expected int, _ time.Time) error {
	if s.lists[id].Version != expected {
		return shared.ErrVersionConflict
	}
	s.lists[id] = domain.StarList{Stars: stars, Version: expected + 1}
	return nil
}

// starReader answers the reads of the kinds as the catalogue would: an id in hidden is refused, a
// container answers its type.
type starReader struct {
	hidden map[string]bool
	types  map[string]string
	asked  []string
}

func (r *starReader) Invoke(_ context.Context, name string, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	for _, v := range in {
		id, _ := v.(string)
		r.asked = append(r.asked, name)
		if r.hidden[id] {
			return nil, shared.ErrNotFound
		}
		return usecase.Output{"id": id, "type": r.types[id]}, nil
	}
	return nil, shared.ErrNotFound
}

// starrer is the person with a session's scopes for their own account.
func starrer() appshared.ActorContext {
	actor := admin()
	actor.Scopes = []string{accountsRead, accountsWrite}
	return actor
}

const (
	starHub     = shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")
	starPage    = shared.ID("01936f2a-7c1e-7000-8000-0000000000b2")
	starPrivate = shared.ID("01936f2a-7c1e-7000-8000-0000000000b3")
	somebody    = shared.ID("01936f2a-7c1e-7000-8000-0000000000b4")
)

func starsFixture(t *testing.T) (Stars, *starStore, *auditSink, *authorizer) {
	t.Helper()
	store := &starStore{lists: map[shared.ID]domain.StarList{}}
	sink := &auditSink{}
	auth := &authorizer{}
	return Stars{
		Stars: store, Accounts: newAccounts(settled(t)), Authorizer: auth, Audit: sink,
		Reader: &starReader{
			hidden: map[string]bool{starPrivate.String(): true},
			types:  map[string]string{starHub.String(): "HUB"},
		},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}, store, sink, auth
}

// UC-WRK-37 checks 1-4: the whole ordered list is written and read back in its order, a stale
// version is a conflict, an unreadable star is neither answered nor kept, and the trail holds the
// count, never the places.
func TestStarsAreWrittenWholeReadInOrderAndFiltered(t *testing.T) {
	stars, store, sink, _ := starsFixture(t)
	list, err := ReplaceStars{Stars: stars}.Execute(t.Context(), starrer(), ReplaceStarsCommand{Stars: []domain.Star{
		{Kind: domain.StarPage, ID: starPage}, {Kind: domain.StarHub, ID: starHub}, {Kind: domain.StarPage, ID: starPrivate},
	}})
	if err != nil {
		t.Fatalf("writing the stars: %v", err)
	}
	if len(list.Stars) != 2 || list.Stars[0].ID != starPage || list.Stars[1].ID != starHub || list.Version != 1 {
		t.Fatalf("written %+v, want the page then the hub at version 1, the unreadable one dropped", list)
	}
	if got := store.lists[adminID]; len(got.Stars) != 2 {
		t.Errorf("stored %+v, want the unreadable star not kept", got)
	}
	entry := sink.entries[len(sink.entries)-1]
	if entry.Action != StarsChangedAction || len(entry.Changes) != 1 {
		t.Errorf("trail %+v, want account.stars_changed with one change", entry)
	}
	if text := fmt.Sprint(entry.Changes); !strings.Contains(text, "2") || strings.Contains(text, starPage.String()) || strings.Contains(text, starHub.String()) {
		t.Errorf("trail changes %s, want the count and no starred place", text)
	}

	_, err = ReplaceStars{Stars: stars}.Execute(t.Context(), starrer(), ReplaceStarsCommand{ExpectedVersion: 7})
	if shared.AsError(err).Code != "version_conflict" {
		t.Errorf("a stale version: %v, want version_conflict", err)
	}

	// A star that was readable when written and is not now is not answered.
	store.lists[adminID] = domain.StarList{Stars: []domain.Star{{Kind: domain.StarPage, ID: starPrivate}, {Kind: domain.StarHub, ID: starHub}}, Version: 3}
	read, err := ListStars{Stars: stars}.Execute(t.Context(), starrer(), "")
	if err != nil || len(read.Stars) != 1 || read.Stars[0].ID != starHub || read.Version != 3 {
		t.Errorf("read %+v, %v; want only the hub at version 3", read, err)
	}
}

// A star of one kind is not the other: a hub starred as a collection is not answered.
func TestAStarOfTheWrongContainerKindIsNotAnswered(t *testing.T) {
	stars, store, _, _ := starsFixture(t)
	store.lists[adminID] = domain.StarList{Stars: []domain.Star{{Kind: domain.StarCollection, ID: starHub}}, Version: 1}
	read, err := ListStars{Stars: stars}.Execute(t.Context(), starrer(), "")
	if err != nil || len(read.Stars) != 0 {
		t.Errorf("read %+v, %v; want nothing", read, err)
	}
}

// UC-WRK-37 check 1: another account's stars are asked as another account's preferences are.
func TestSomebodyElsesStarsNeedTheMemberPermission(t *testing.T) {
	stars, _, _, auth := starsFixture(t)
	auth.refuse = shared.ErrForbidden
	if _, err := (ListStars{Stars: stars}).Execute(t.Context(), starrer(), somebody); shared.AsError(err).Code != "forbidden" {
		t.Errorf("another account's stars: %v, want forbidden", err)
	}
	if _, err := (ReplaceStars{Stars: stars}).Execute(t.Context(), starrer(), ReplaceStarsCommand{AccountID: somebody}); shared.AsError(err).Code != "forbidden" {
		t.Errorf("writing another account's stars: %v, want forbidden", err)
	}
}

// UC-WRK-37 check 2 through the catalogue: an unknown kind and more than 100 are invalid.
func TestTheStarsInputIsCheckedThroughTheCatalogue(t *testing.T) {
	stars, _, _, _ := starsFixture(t)
	registry, err := usecase.NewRegistry(nil, ReplaceStars{Stars: stars}.Descriptor(), ListStars{Stars: stars}.Descriptor())
	if err != nil {
		t.Fatalf("the catalogue refused the entries: %v", err)
	}
	_, err = registry.Invoke(t.Context(), ReplaceStarsName, starrer(), usecase.Input{
		"stars": []any{map[string]any{"kind": "FOLDER", "id": starPage.String()}},
	})
	if shared.AsError(err).DetailCode != "stars.kind_invalid" {
		t.Errorf("an unknown kind: %v, want stars.kind_invalid", err)
	}
	many := make([]any, 101)
	for i := range many {
		many[i] = map[string]any{"kind": "PAGE", "id": starPage.String()}
	}
	_, err = registry.Invoke(t.Context(), ReplaceStarsName, starrer(), usecase.Input{"stars": many})
	if shared.AsError(err).DetailCode != "stars.too_many" {
		t.Errorf("101 stars: %v, want stars.too_many", err)
	}
	out, err := registry.Invoke(t.Context(), ReplaceStarsName, starrer(), usecase.Input{
		"stars": []any{map[string]any{"kind": "PAGE", "id": starPage.String()}},
	})
	if rows, _ := out["stars"].([]usecase.Output); err != nil || len(rows) != 1 || out["version"] != 1 {
		t.Errorf("answer %v, %v", out, err)
	}
}
