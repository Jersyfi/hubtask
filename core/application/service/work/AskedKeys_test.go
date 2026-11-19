// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// reachFake answers the time module's question: whether the asker approves time in each collection.
type reachFake struct {
	approves bool
	sumsOnly bool
	asked    []shared.ID
}

func (r *reachFake) Reach(_ context.Context, _ appshared.ActorContext, collections []domain.Container) ([]bool, bool, error) {
	shown := make([]bool, len(collections))
	for i, c := range collections {
		r.asked = append(r.asked, c.ID)
		shown[i] = r.approves
	}
	return shown, r.sumsOnly, nil
}

type criticalFake struct{ path []shared.ID }

func (c criticalFake) Critical(context.Context, appshared.ActorContext, shared.ID) ([]shared.ID, error) {
	return c.path, nil
}

type groupsFake struct{ people map[string][]shared.ID }

func (g groupsFake) People(_ context.Context, _ appshared.ActorContext, wanted []view.Placeholder) (map[string][]shared.ID, error) {
	out := map[string][]shared.ID{}
	for _, p := range wanted {
		people, known := g.people[p.GroupKey()]
		if !known {
			return nil, shared.ErrValidation.WithDetail("query.group_unknown")
		}
		out[p.GroupKey()] = people
	}
	return out, nil
}

func askedQuery(t *testing.T, h QueryItems, filter map[string]any) (*view.Node, error) {
	t.Helper()
	store := h.Items.(*items)
	_, err := h.invoke(t.Context(), actorFixture(), usecase.Input{"scope_container_id": readCollectionID.String(), "filter": filter})
	if err != nil {
		return nil, err
	}
	return store.searched[len(store.searched)-1].Spec.Filter, nil
}

// UC-TIM-04 through time.logged: a member counts only their own time, a lead who approves time in
// the collection everybody's, and the decision is the time module's, asked of the collection.
func TestTimeLoggedCountsWhatTheTimeModuleShowsTheAsker(t *testing.T) {
	filter := map[string]any{"field": "time_logged", "op": "GT", "value": 0}
	for _, c := range []struct {
		name     string
		reach    reachFake
		shown    []shared.ID
		withhold bool
	}{
		{"a member", reachFake{}, []shared.ID{}, false},
		{"a lead", reachFake{approves: true}, []shared.ID{readCollectionID}, false},
		{"a lead where only sums are shown", reachFake{approves: true, sumsOnly: true}, []shared.ID{readCollectionID}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			store, containerStore, uow := queryFixture()
			h := queryHandler(store, containerStore, uow, &authorizer{})
			h.Time = &c.reach
			node, err := askedQuery(t, h, filter)
			if err != nil {
				t.Fatal(err)
			}
			if node.Asked == nil {
				t.Fatal("time.logged reached the statement unanswered")
			}
			sight := node.Asked.Sight
			if sight.Reader != accountID || !sameShared(sight.Shown, c.shown) || sight.Withhold != c.withhold {
				t.Errorf("sight %+v", sight)
			}
			if !sameShared(c.reach.asked, []shared.ID{readCollectionID}) {
				t.Errorf("the time module was asked about %v", c.reach.asked)
			}
		})
	}
}

func TestPlanCriticalIsTheSchedulersAnswer(t *testing.T) {
	store, containerStore, uow := queryFixture()
	h := queryHandler(store, containerStore, uow, &authorizer{})
	filter := map[string]any{"field": "critical", "op": "EQ", "value": true}
	if _, err := askedQuery(t, h, filter); !isDetail(err, "query.key_not_here") {
		t.Fatalf("unwired: %v", err)
	}
	h.Critical = criticalFake{path: []shared.ID{readItemID}}
	node, err := askedQuery(t, h, filter)
	if err != nil {
		t.Fatal(err)
	}
	if node.Asked == nil || !sameShared(node.Asked.Critical, []shared.ID{readItemID}) {
		t.Fatalf("answered %+v", node.Asked)
	}
}

func TestTheGroupWordsAreReadForTheAsker(t *testing.T) {
	store, containerStore, uow := queryFixture()
	h := queryHandler(store, containerStore, uow, &authorizer{})
	filter := map[string]any{"field": "assignee_id", "op": "EQ", "value": "@my_groups"}
	if _, err := askedQuery(t, h, filter); !isDetail(err, "query.groups_not_here") {
		t.Fatalf("unwired: %v", err)
	}
	h.Groups = groupsFake{people: map[string][]shared.ID{"@my_groups": {accountID}}}
	node, err := askedQuery(t, h, filter)
	if err != nil {
		t.Fatal(err)
	}
	if node.Op != view.OpIn || len(node.Values) != 1 || node.Values[0].ID != accountID {
		t.Fatalf("resolved %+v", node)
	}
	if _, err := askedQuery(t, h, map[string]any{"field": "assignee_id", "op": "EQ", "value": "@group:Nobody"}); !isDetail(err, "query.group_unknown") {
		t.Fatalf("an unknown group: %v", err)
	}
}

func sameShared(a, b []shared.ID) bool {
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

func isDetail(err error, code string) bool {
	var fault *shared.Error
	return errors.As(err, &fault) && fault.DetailCode == code
}
