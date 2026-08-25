// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// What a service test of the query language has an opinion about is the specification that leaves
// the use case: whether the scope was resolved into an anchor, whether the placeholders were
// replaced, whether the page was clamped, and who was asked for permission. What that specification
// becomes in SQL is the compiler's business (infrastructure/postgres/query), and what it finds is
// the database's - test/integration.

func queryFixture() (*items, *containers, *unitOfWork) {
	store, containerStore := readFixture()
	store.result = repository.ItemQueryResult{
		Items: []domain.WorkItem{itemFixture(readItemID, readCollectionID, "Buy milk")},
	}
	return store, containerStore, &unitOfWork{}
}

func queryHandler(store *items, containerStore *containers, uow *unitOfWork, guard *authorizer) QueryItems {
	return QueryItems{
		Items: store, ItemLabels: &itemLabels{}, Containers: containerStore,
		Authorizer: guard, UnitOfWork: uow, Clock: clock.Fixed(now),
	}
}

func TestQueryItemsAnchorsOnWhatTheScopeNames(t *testing.T) {
	tests := []struct {
		name  string
		input usecase.Input
		want  repository.Anchor
	}{
		{
			"a collection, whole",
			usecase.Input{"scope_container_id": readCollectionID.String()},
			repository.Anchor{
				Kind: repository.AnchorCollection, CollectionID: readCollectionID,
				IncludeDescendants: true,
			},
		},
		{
			"a collection, one level",
			usecase.Input{
				"scope_container_id": readCollectionID.String(), "include_descendants": false,
			},
			repository.Anchor{Kind: repository.AnchorCollection, CollectionID: readCollectionID},
		},
		{
			// A hub holds no entries of its own, so the flag says nothing about it: what it scopes
			// is everything in its collections either way.
			"a hub",
			usecase.Input{"scope_container_id": hubID.String(), "include_descendants": false},
			repository.Anchor{Kind: repository.AnchorHub, HubID: hubID, IncludeDescendants: true},
		},
		{
			"an entry's subtree",
			usecase.Input{"scope_item_id": readItemID.String()},
			repository.Anchor{
				Kind: repository.AnchorItem, CollectionID: readCollectionID, ItemID: readItemID,
				PathPrefix: domain.RootPath(readItemID), IncludeDescendants: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, containerStore, uow := queryFixture()
			containerStore.stored[hubID] = hubFixture(hubID, "Home", "a0")

			if _, err := queryHandler(store, containerStore, uow, &authorizer{}).
				invoke(t.Context(), actorFixture(), test.input); err != nil {
				t.Fatalf("querying: %v", err)
			}

			if len(store.searched) != 1 {
				t.Fatalf("%d queries ran, want 1", len(store.searched))
			}
			if got := store.searched[0].Anchor; got != test.want {
				t.Errorf("anchored on %+v, want %+v", got, test.want)
			}
			if uow.writes != 0 {
				t.Errorf("a read opened %d write transactions", uow.writes)
			}
		})
	}
}

// One permission question for the whole result, asked about the container the client named - the
// same shape ListWorkItems has, and for the same reason: a refusal is a refusal rather than an
// empty page.
func TestQueryItemsAsksOncePerQueryAboutTheScope(t *testing.T) {
	store, containerStore, uow := queryFixture()
	guard := &authorizer{}

	if _, err := queryHandler(store, containerStore, uow, guard).invoke(
		t.Context(), actorFixture(), usecase.Input{"scope_container_id": readCollectionID.String()},
	); err != nil {
		t.Fatalf("querying: %v", err)
	}

	if len(guard.requests) != 1 {
		t.Fatalf("%d permission questions asked, want 1", len(guard.requests))
	}
	request := guard.requests[0]
	assertPath(t, request.Path, []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(readCollectionID),
	})
	if request.Permission != service.PermissionRead || request.TokenScope != itemsRead {
		t.Errorf("asked for %s / %q", request.Permission, request.TokenScope)
	}
}

func TestQueryItemsRefusedReadsNothing(t *testing.T) {
	store, containerStore, uow := queryFixture()
	guard := &authorizer{err: shared.ErrForbidden.WithDetail("access.not_permitted")}

	_, err := queryHandler(store, containerStore, uow, guard).invoke(
		t.Context(), actorFixture(), usecase.Input{"scope_container_id": readCollectionID.String()})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a refused query answered %v", err)
	}
	if len(store.searched) != 0 {
		t.Errorf("a refused query still ran %d searches", len(store.searched))
	}
}

func TestQueryItemsReportsAMissingScopeAsNotFound(t *testing.T) {
	store, containerStore, uow := queryFixture()
	missing := shared.MustParseID("0192f000-0000-7000-8000-0000000009ff")

	for field, id := range map[string]shared.ID{
		"scope_container_id": missing, "scope_item_id": missing,
	} {
		_, err := queryHandler(store, containerStore, uow, &authorizer{}).invoke(
			t.Context(), actorFixture(), usecase.Input{field: id.String()})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%s answered %v", field, err)
		}
	}
}

// The placeholders are the whole reason the use case holds a clock: a saved filter has to mean the
// same thing next week, and only the server knows who is asking and when their day began.
func TestQueryItemsResolvesThePlaceholders(t *testing.T) {
	store, containerStore, uow := queryFixture()
	actor := actorFixture()
	actor.TimeZone = "Europe/Berlin"

	_, err := queryHandler(store, containerStore, uow, &authorizer{}).invoke(
		t.Context(), actor, usecase.Input{
			"scope_container_id": readCollectionID.String(),
			"filter": map[string]any{"op": "AND", "nodes": []any{
				map[string]any{"field": "created_by", "op": "EQ", "value": "@me"},
				map[string]any{"field": "created_at", "op": "GTE", "value": "@today"},
				map[string]any{"field": "due_at", "op": "LTE", "value": "@today+P3D"},
			}},
		})
	if err != nil {
		t.Fatalf("querying: %v", err)
	}

	filter := store.searched[0].Spec.Filter
	if filter == nil || len(filter.Nodes) != 3 {
		t.Fatalf("the filter arrived as %+v", filter)
	}
	if got := filter.Nodes[0].Values[0]; got.IsPlaceholder() || got.ID != accountID {
		t.Errorf("@me resolved to %+v", got)
	}
	// Midnight in Berlin on the day the fixed clock says, which is an hour earlier in UTC than
	// midnight would be there.
	want := time.Date(2026, 8, 17, 0, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	if got := filter.Nodes[1].Values[0]; got.IsPlaceholder() || !got.Time.Equal(want) {
		t.Errorf("@today resolved to %s, want %s", got.Time, want.UTC())
	}
	// The lift B-12's acceptance anticipated (D-01): "due in the next three days" is a calendar
	// offset from that same midnight, in the actor's zone - not seventy-two clock hours.
	if got := filter.Nodes[2].Values[0]; got.IsPlaceholder() || !got.Time.Equal(want.AddDate(0, 0, 3)) {
		t.Errorf("@today+P3D resolved to %s, want %s", got.Time, want.AddDate(0, 0, 3).UTC())
	}
}

func TestQueryItemsRefusesWhatTheGrammarRefuses(t *testing.T) {
	tests := []struct {
		name  string
		input usecase.Input
		code  string
	}{
		{"no scope", usecase.Input{}, "query.scope_required"},
		{
			"two scopes",
			usecase.Input{
				"scope_container_id": readCollectionID.String(),
				"scope_item_id":      readItemID.String(),
			},
			"query.scope_ambiguous",
		},
		{
			"a field this installation does not serve",
			usecase.Input{
				"scope_container_id": readCollectionID.String(),
				"filter": map[string]any{
					"field": "recurrence_rule_id", "op": "EQ",
					"value": "0192f000-0000-7000-8000-000000000001",
				},
			},
			"query.field_unknown",
		},
		{
			"an estimate nothing produces",
			usecase.Input{"scope_container_id": readCollectionID.String(), "count": "estimated"},
			"query.count_not_supported",
		},
		{
			"a cursor into a grouped result",
			usecase.Input{
				"scope_container_id": readCollectionID.String(),
				"group_by":           map[string]any{"field": "bucket_id"},
				"cursor":             "opaque",
			},
			"query.cursor_not_grouped",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, containerStore, uow := queryFixture()

			_, err := queryHandler(store, containerStore, uow, &authorizer{}).
				invoke(t.Context(), actorFixture(), test.input)

			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != test.code {
				t.Fatalf("answered %v, want %s", err, test.code)
			}
			if len(store.searched) != 0 {
				t.Errorf("a refused query still ran a search")
			}
		})
	}
}

// A size beyond the ceiling is clamped rather than refused, as everywhere else that pages: a client
// asking for more than it can have wants as many as it can get (api-guidelines.md §4).
func TestQueryItemsClampsThePage(t *testing.T) {
	for requested, want := range map[int]int{0: DefaultPageSize, 500: MaxPageSize, 25: 25} {
		store, containerStore, uow := queryFixture()

		in := usecase.Input{"scope_container_id": readCollectionID.String()}
		if requested != 0 {
			in["size"] = requested
		}
		if _, err := queryHandler(store, containerStore, uow, &authorizer{}).
			invoke(t.Context(), actorFixture(), in); err != nil {
			t.Fatalf("querying: %v", err)
		}
		if got := store.searched[0].Spec.Size; got != want {
			t.Errorf("a size of %d became %d, want %d", requested, got, want)
		}
	}
}

// Both shapes are always in the answer, and the one that does not apply is empty: a client reads
// either unconditionally rather than checking which kind of query it sent.
func TestQueryItemsAnswersBothShapes(t *testing.T) {
	t.Run("ungrouped", func(t *testing.T) {
		store, containerStore, uow := queryFixture()

		out, err := queryHandler(store, containerStore, uow, &authorizer{}).invoke(
			t.Context(), actorFixture(), usecase.Input{
				"scope_container_id": readCollectionID.String(),
			})
		if err != nil {
			t.Fatalf("querying: %v", err)
		}
		if rows, _ := out["data"].([]usecase.Output); len(rows) != 1 {
			t.Errorf("data holds %v", out["data"])
		}
		if groups, ok := out["groups"].([]usecase.Output); !ok || len(groups) != 0 {
			t.Errorf("groups holds %v, want an empty list", out["groups"])
		}
		if out["total"] != nil {
			t.Errorf("a query that asked for no count answered total %v", out["total"])
		}
	})

	t.Run("grouped and counted", func(t *testing.T) {
		store, containerStore, uow := queryFixture()
		store.result = repository.ItemQueryResult{
			Total: 3,
			Groups: []repository.ItemGroup{
				{
					Key:   "0192f000-0000-7000-8000-000000000401",
					Items: []domain.WorkItem{itemFixture(readItemID, readCollectionID, "Buy milk")},
					Total: 2,
				},
				{Absent: true, Total: 1},
			},
		}

		out, err := queryHandler(store, containerStore, uow, &authorizer{}).invoke(
			t.Context(), actorFixture(), usecase.Input{
				"scope_container_id": readCollectionID.String(),
				"group_by":           map[string]any{"field": "bucket_id", "limit_per_group": 10},
				"count":              "exact",
			})
		if err != nil {
			t.Fatalf("querying: %v", err)
		}
		if rows, ok := out["data"].([]usecase.Output); !ok || len(rows) != 0 {
			t.Errorf("data holds %v, want an empty list", out["data"])
		}

		groups, _ := out["groups"].([]usecase.Output)
		if len(groups) != 2 {
			t.Fatalf("groups holds %v", out["groups"])
		}
		if groups[0]["key"] != "0192f000-0000-7000-8000-000000000401" || groups[0]["count"] != 2 {
			t.Errorf("the first group reads %v", groups[0])
		}
		// The entries with no value at all are a group too: a board draws that column.
		if groups[1]["key"] != nil {
			t.Errorf("the group with no value reads key %v", groups[1]["key"])
		}
		if out["total"] != 3 {
			t.Errorf("total is %v", out["total"])
		}
		if got := store.searched[0].Spec.GroupBy.LimitPerGroup; got != 10 {
			t.Errorf("the column size arrived as %d", got)
		}
	})
}

// The declaration is what MCP and the automation engine build their surface from, so the fields a
// query needs have to be in it - and the two document fields are what a schema of scalars cannot
// express (ADR-0012).
func TestQueryItemsDeclaresItsInput(t *testing.T) {
	declared := map[string]usecase.Kind{}
	for _, field := range (QueryItems{}).Descriptor().Input {
		declared[field.Name] = field.Kind
	}

	for name, kind := range map[string]usecase.Kind{
		"scope_container_id": usecase.KindID,
		"scope_item_id":      usecase.KindID,
		"filter":             usecase.KindObject,
		"sort":               usecase.KindList,
		"group_by":           usecase.KindObject,
		"count":              usecase.KindString,
		"size":               usecase.KindInt,
	} {
		if declared[name] != kind {
			t.Errorf("%s is declared as %q, want %q", name, declared[name], kind)
		}
	}

	if descriptor := (QueryItems{}).Descriptor(); !descriptor.ReadOnly ||
		descriptor.TokenScope != itemsRead {
		t.Errorf("a query is a read of items, and declares %+v", descriptor)
	}
}

// The grammar is the domain's, so nothing above it may add a field of its own: a name this
// installation does not serve has to be refused wherever it arrives, and the catalogue's own
// unknown-field check is what does it for the input as a whole.
func TestQueryItemsRefusesAnUnknownInputField(t *testing.T) {
	err := (QueryItems{}).Descriptor().ValidateInput(usecase.Input{
		"scope_container_id": readCollectionID.String(), "filters": map[string]any{},
	})
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "usecase.input_invalid" {
		t.Fatalf("answered %v", err)
	}
}

// A document where a scalar belongs, and the other way round: the catalogue checks the shape before
// a handler sees it, which is what keeps the grammar from having to.
func TestQueryItemsRefusesADocumentOfTheWrongShape(t *testing.T) {
	for name, value := range map[string]any{
		"filter": []any{}, "sort": map[string]any{}, "group_by": "bucket_id",
	} {
		err := (QueryItems{}).Descriptor().ValidateInput(usecase.Input{
			"scope_container_id": readCollectionID.String(), name: value,
		})
		if err == nil {
			t.Errorf("%s accepted a %T", name, value)
		}
	}
}
