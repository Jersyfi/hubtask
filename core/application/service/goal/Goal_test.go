// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	goalrepo "github.com/Jersyfi/hubtask/core/application/repository/goal"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenantID = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000001")
	ana      = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000002")
	ben      = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000003")
	admin    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000004")
	hubA     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000010")
	secret   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000011")
	listA    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000012")
	listS    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000013")
	entry1   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000101")
	entry2   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000102")
	entryS   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000103")
	viewID   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000201")
)

var noon = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

type inline struct{}

func (inline) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (inline) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

// gate: Ben reads nothing of the secret hub; only the admin holds STRUCTURE.
type gate struct{ refused []access.Request }

func allows(actor shared.ID, p service.Permission, path []identity.Scope) bool {
	if actor == ben {
		for _, s := range path {
			if s.ID == secret || s.ID == listS {
				return false
			}
		}
	}
	if p == service.PermissionManageCosts {
		return actor == ana
	}
	return p != service.PermissionStructure || actor == admin
}

func (g *gate) Authorize(_ context.Context, actor appshared.ActorContext, r access.Request) error {
	if !allows(actor.AccountID, r.Permission, r.Path) {
		g.refused = append(g.refused, r)
		return shared.ErrForbidden.WithDetail("access.denied")
	}
	return nil
}

func (g *gate) Permitted(_ context.Context, actor appshared.ActorContext, r access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i, p := range paths {
		out[i] = allows(actor.AccountID, r.Permission, p)
	}
	return out, nil
}

type sink struct{ entries []audit.Entry }

func (s *sink) Append(_ context.Context, e audit.Entry) error {
	s.entries = append(s.entries, e)
	return nil
}

type ids struct{ n int }

func (i *ids) NewID() shared.ID {
	i.n++
	return shared.MustParseID("01936f2a-7c1e-7000-9000-" + pad(i.n))
}

func pad(n int) string {
	s := itoa(n)
	for len(s) < 12 {
		s = "0" + s
	}
	return s
}

type fixed struct{}

func (fixed) Now() time.Time { return noon }

// refs knows the entries and containers.
type refs struct {
	entries map[shared.ID]workrepo.EntrySummary
}

func (r refs) Entries(_ context.Context, list []shared.ID) ([]workrepo.EntrySummary, error) {
	var out []workrepo.EntrySummary
	for _, id := range list {
		if e, ok := r.entries[id]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}

var containers = map[shared.ID]workrepo.ContainerSummary{
	hubA:   {ID: hubA, Kind: work.ContainerHub, Name: "Product"},
	secret: {ID: secret, Kind: work.ContainerHub, Name: "Board"},
	listA:  {ID: listA, Kind: work.ContainerCollection, ParentID: hubA, Name: "Launch"},
	listS:  {ID: listS, Kind: work.ContainerCollection, ParentID: secret, Name: "Merger"},
}

func (refs) Containers(_ context.Context, list []shared.ID) ([]workrepo.ContainerSummary, error) {
	var out []workrepo.ContainerSummary
	for _, id := range list {
		if c, ok := containers[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (refs) Backlinks(context.Context, reference.Reference, int) ([]workrepo.Backlink, error) {
	return nil, nil
}

type hubs struct{ workrepo.Containers }

func (hubs) Find(_ context.Context, id shared.ID) (work.Container, error) {
	c, ok := containers[id]
	if !ok {
		return work.Container{}, shared.ErrNotFound
	}
	return work.Container{ID: c.ID, Type: c.Kind, ParentID: c.ParentID, Name: c.Name}, nil
}

// summaries answers a collection's completion and a view's total as a summary would.
type summaries struct{ calls []usecase.Input }

func (s *summaries) Invoke(_ context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	s.calls = append(s.calls, in)
	if in.String("view_id") != "" {
		return usecase.Output{"data": []usecase.Output{{"count": 30, "value": nil}}}, nil
	}
	if in.String("scope_container_id") == listS.String() && actor.AccountID == ben {
		return nil, shared.ErrForbidden.WithDetail("access.denied")
	}
	return usecase.Output{"data": []usecase.Output{
		{"group": "true", "count": 3}, {"group": "false", "count": 1},
	}, "truncated": false}, nil
}

func newSteward(t *testing.T) (Steward, *memory, *gate, *summaries) {
	t.Helper()
	store := &memory{
		periods: map[shared.ID]domain.Period{}, goals: map[shared.ID]domain.Goal{}, links: map[shared.ID][]shared.ID{},
		portfolios: map[shared.ID]domain.Portfolio{}, dashboards: map[shared.ID]domain.Dashboard{},
		digests: map[shared.ID]domain.Digest{},
	}
	g, sum := &gate{}, &summaries{}
	return Steward{
		Store: store, References: refs{entries: map[shared.ID]workrepo.EntrySummary{
			entry1: {ID: entry1, CollectionID: listA, HubID: hubA, Title: "a", Completed: true},
			entry2: {ID: entry2, CollectionID: listA, HubID: hubA, Title: "b"},
			entryS: {ID: entryS, CollectionID: listS, HubID: secret, Title: "s", Completed: true},
		}},
		Containers: hubs{}, Summaries: sum, ViewSummaries: sum, Authorizer: g, Reader: g, Audit: &sink{},
		UnitOfWork: inline{}, Clock: fixed{}, IDs: &ids{},
	}, store, g, sum
}

func as(id shared.ID) appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenantID, AccountID: id, Kind: shared.ActorUser,
		Scopes: []string{itemsRead, itemsWrite, containersRead, containersWrite}}
}

func call(t *testing.T, d usecase.Descriptor, actor shared.ID, in usecase.Input) (usecase.Output, error) {
	t.Helper()
	return d.Handler.Invoke(context.Background(), as(actor), in)
}

func ok(t *testing.T, d usecase.Descriptor, actor shared.ID, in usecase.Input) usecase.Output {
	t.Helper()
	out, err := call(t, d, actor, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return out
}

func code(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func near(t *testing.T, what string, got any, want float64) {
	t.Helper()
	v, ok := number(got)
	if !ok || math.Abs(v-want) > 1e-9 {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func period(t *testing.T, s Steward) string {
	t.Helper()
	out := ok(t, DefineGoalPeriod{s}.Descriptor(), admin, usecase.Input{
		"name": "2026 Q4", "start_on": "2026-10-01", "end_on": "2026-12-31",
	})
	return out.String("id")
}

// UC-GOL-01 checks 1 to 4: a goal's progress is the mean of its key results, its child goals
// and its linked entries' share completed, counted for the reader.
func TestAGoalsProgressComesFromItsWork(t *testing.T) {
	s, _, _, _ := newSteward(t)
	p := period(t, s)
	parent := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{
		"period_id": p, "title": "Win the market",
		"key_results": []any{map[string]any{"title": "Customers", "kind": "NUMBER", "start": 0.0, "target": 100.0, "current": 50.0}},
	})
	near(t, "the parent's first progress", parent["progress"], 0.5)
	child := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{
		"period_id": p, "hub_id": hubA.String(), "parent_id": parent.String("id"), "title": "Ship the launch",
	})
	ok(t, LinkGoalEntries{s}.Descriptor(), ana, usecase.Input{
		"goal_id": child.String("id"), "entry_ids": []any{entry1.String(), entry2.String()},
	})
	read := ok(t, ReadGoal{s}.Descriptor(), ana, usecase.Input{"goal_id": child.String("id")})
	near(t, "the child's progress", read["progress"], 0.5)
	if len(read["linked_entries"].([]usecase.Output)) != 2 {
		t.Fatalf("linked entries = %v", read["linked_entries"])
	}
	near(t, "the parent's rolled-up progress", ok(t, ReadGoal{s}.Descriptor(), ana, usecase.Input{"goal_id": parent.String("id")})["progress"], 0.5)

	// A key result fed by a summary view takes the view's total for its reader.
	fed := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{
		"period_id": p, "title": "Answer support",
		"key_results": []any{map[string]any{"title": "Tickets solved", "kind": "NUMBER", "start": 0.0, "target": 60.0, "source": "VIEW", "view_id": viewID.String()}},
	})
	results := fed["key_results"].([]usecase.Output)
	if results[0]["live"] != true {
		t.Fatalf("the view's value is not live: %v", results[0])
	}
	near(t, "the view-fed progress", fed["progress"], 0.5)
}

// UC-GOL-01 check 5: a goal in a hub the reader does not read is not found, and a child goal or a
// linked entry there is no part of a goal they do read.
func TestAGoalOutOfReachIsNoPartOfTheNumber(t *testing.T) {
	s, _, _, _ := newSteward(t)
	p := period(t, s)
	parent := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "title": "Grow"})
	hidden := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{
		"period_id": p, "hub_id": secret.String(), "parent_id": parent.String("id"), "title": "Merge",
	})
	ok(t, LinkGoalEntries{s}.Descriptor(), ana, usecase.Input{"goal_id": parent.String("id"), "entry_ids": []any{entryS.String(), entry2.String()}})
	if _, err := call(t, ReadGoal{s}.Descriptor(), ben, usecase.Input{"goal_id": hidden.String("id")}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Ben reads the hidden goal: %v", err)
	}
	asBen := ok(t, ReadGoal{s}.Descriptor(), ben, usecase.Input{"goal_id": parent.String("id")})
	if asBen["child_count"] != 0 || asBen["entries"].(usecase.Output)["total"] != 1 {
		t.Fatalf("Ben sees what he may not: %v %v", asBen["child_count"], asBen["entries"])
	}
	near(t, "Ben's progress", asBen["progress"], 0)
	listed := ok(t, ListGoals{s}.Descriptor(), ben, usecase.Input{"period_id": p})
	if len(listed["data"].([]usecase.Output)) != 1 {
		t.Fatalf("Ben lists %v", listed["data"])
	}
}

// UC-GOL-01 checks 4 and 5: an entry lists the goals it serves that its reader may read, and an
// entry the reader may not read is not found.
func TestAnEntryListsTheGoalsItServes(t *testing.T) {
	s, _, _, _ := newSteward(t)
	p := period(t, s)
	open := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "title": "Grow"})
	hidden := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "hub_id": secret.String(), "title": "Merge"})
	for _, g := range []usecase.Output{open, hidden} {
		ok(t, LinkGoalEntries{s}.Descriptor(), ana, usecase.Input{"goal_id": g.String("id"), "entry_ids": []any{entry2.String()}})
	}
	asAna := ok(t, ListEntryGoals{s}.Descriptor(), ana, usecase.Input{"item_id": entry2.String()})
	if len(asAna["data"].([]usecase.Output)) != 2 {
		t.Fatalf("Ana sees %v", asAna["data"])
	}
	asBen := ok(t, ListEntryGoals{s}.Descriptor(), ben, usecase.Input{"item_id": entry2.String()})
	data := asBen["data"].([]usecase.Output)
	if len(data) != 1 || data[0]["id"] != open.String("id") {
		t.Fatalf("Ben sees %v", data)
	}
	if _, err := call(t, ListEntryGoals{s}.Descriptor(), ben, usecase.Input{"item_id": entryS.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Ben reads the goals of an entry he may not read: %v", err)
	}
}

// UC-GOL-01 check 6: a goal never sits under itself or one of its own.
func TestAGoalIsNoParentOfItsParent(t *testing.T) {
	s, _, _, _ := newSteward(t)
	p := period(t, s)
	a := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "title": "A"})
	b := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "title": "B", "parent_id": a.String("id")})
	_, err := call(t, UpdateGoal{s}.Descriptor(), ana, usecase.Input{
		"goal_id": a.String("id"), "expected_version": 1, "period_id": p, "title": "A", "parent_id": b.String("id"),
	})
	if code(err) != "goals.parent_cycle" {
		t.Fatalf("a cycle was accepted: %v", err)
	}
}

// UC-GOL-02 checks 1 to 4: a check-in sets the goal's status, confidence and reported values and
// is kept as history; a closed period grades its goals and they take no more check-ins.
func TestACheckInIsKeptAndAClosedPeriodGrades(t *testing.T) {
	s, store, g, _ := newSteward(t)
	p := period(t, s)
	created := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{
		"period_id": p, "title": "Grow",
		"key_results": []any{map[string]any{"title": "Revenue", "kind": "NUMBER", "start": 0.0, "target": 10.0}},
	})
	result := created["key_results"].([]usecase.Output)[0].String("id")
	after := ok(t, CheckInGoal{s}.Descriptor(), ana, usecase.Input{
		"goal_id": created.String("id"), "status": "AT_RISK", "confidence": 4, "note": "slow quarter",
		"values": []any{map[string]any{"key_result_id": result, "value": 3.0}},
	})
	if after["status"] != "AT_RISK" || after["confidence"] != 4 {
		t.Fatalf("the check-in did not apply: %v", after)
	}
	near(t, "progress after the check-in", after["progress"], 0.3)
	history := ok(t, ListGoalCheckIns{s}.Descriptor(), ana, usecase.Input{"goal_id": created.String("id")})
	if rows := history["data"].([]usecase.Output); len(rows) != 1 || rows[0]["note"] != "slow quarter" {
		t.Fatalf("history = %v", history)
	}
	for _, e := range s.Audit.(*sink).entries {
		if strings.Contains(fmt.Sprint(e.Changes), "slow quarter") {
			t.Fatal("a note reached the audit trail")
		}
	}
	if _, err := call(t, CloseGoalPeriod{s}.Descriptor(), ana, usecase.Input{"period_id": p, "expected_version": 1}); !errors.Is(err, shared.ErrForbidden) || len(g.refused) == 0 {
		t.Fatalf("a member closed the period: %v", err)
	}
	ok(t, CloseGoalPeriod{s}.Descriptor(), admin, usecase.Input{"period_id": p, "expected_version": 1})
	graded := store.goals[shared.ID(created.String("id"))]
	if graded.Grade == nil || math.Abs(*graded.Grade-0.3) > 1e-9 {
		t.Fatalf("grade = %v", graded.Grade)
	}
	if _, err := call(t, CheckInGoal{s}.Descriptor(), ana, usecase.Input{"goal_id": created.String("id"), "status": "ON_TRACK", "confidence": 8}); code(err) != "goals.graded" {
		t.Fatalf("a graded goal took a check-in: %v", err)
	}
}

// UC-GOL-03 checks 1 to 5: a portfolio rolls its projects up through the reader's own summary;
// a project the reader may not read is left out, and kept when they change the portfolio; a
// status update sets its health.
func TestAPortfolioRollsUpWhatItsReaderSees(t *testing.T) {
	s, store, _, sum := newSteward(t)
	created := ok(t, CreatePortfolio{s}.Descriptor(), ana, usecase.Input{
		"name": "Programme", "items": []any{
			map[string]any{"container_id": listA.String(), "start_on": "2026-10-01", "end_on": "2026-12-31", "status": "ON_TRACK"},
			map[string]any{"container_id": listS.String()},
		},
	})
	id := created.String("id")
	read := ok(t, ReadPortfolio{s}.Descriptor(), ana, usecase.Input{"portfolio_id": id})
	near(t, "Ana's progress", read["progress"], 0.75)
	if len(sum.calls) == 0 || sum.calls[0]["aggregate"] == nil {
		t.Fatal("the roll-up did not run the summary")
	}
	asBen := ok(t, ReadPortfolio{s}.Descriptor(), ben, usecase.Input{"portfolio_id": id})
	if rows := asBen["items"].([]usecase.Output); len(rows) != 1 || rows[0]["container_id"] != listA.String() {
		t.Fatalf("Ben sees %v", asBen["items"])
	}
	ok(t, UpdatePortfolio{s}.Descriptor(), ben, usecase.Input{
		"portfolio_id": id, "expected_version": 1, "name": "Programme",
		"items": []any{map[string]any{"container_id": listA.String()}},
	})
	if items := store.portfolios[shared.ID(id)].Items; len(items) != 2 {
		t.Fatalf("Ben dropped a project he cannot see: %v", items)
	}
	ok(t, ReportPortfolioStatus{s}.Descriptor(), ana, usecase.Input{"portfolio_id": id, "status": "AT_RISK", "summary": "late"})
	if store.portfolios[shared.ID(id)].Status != domain.StatusAtRisk {
		t.Fatal("the update did not set the health")
	}
	if _, err := call(t, CreatePortfolio{s}.Descriptor(), ben, usecase.Input{
		"name": "Spy", "items": []any{map[string]any{"container_id": listS.String()}},
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Ben added a collection he cannot read: %v", err)
	}
}

// budgets answers the time module's budget of the Launch collection in euros; Merger has none.
type budgets struct{ calls int }

func (b *budgets) Invoke(_ context.Context, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	b.calls++
	if in.String("plan_id") != listA.String() {
		return usecase.Output{"budgeted": false}, nil
	}
	return usecase.Output{
		"budgeted": true, "budget_minutes": int64(6000), "budget_amount_minor": int64(500000), "currency": "EUR",
		"minutes": int64(4500), "amount_minor": int64(375000), "unpriced_minutes": int64(0), "over_time": false, "over_money": false,
	}, nil
}

// UC-GOL-03 check 9: the portfolio's budget is its projects' budgets rolled up from the time module's,
// shown only to whoever manages costs there.
func TestAPortfolioRollsUpItsBudgetsForWhoeverManagesCosts(t *testing.T) {
	s, _, _, _ := newSteward(t)
	b := &budgets{}
	s.Budgets = b
	created := ok(t, CreatePortfolio{s}.Descriptor(), ana, usecase.Input{
		"name": "Programme", "items": []any{
			map[string]any{"container_id": listA.String()}, map[string]any{"container_id": listS.String()},
		},
	})
	read := ok(t, ReadPortfolio{s}.Descriptor(), ana, usecase.Input{"portfolio_id": created.String("id")})
	total, ok1 := read["budget"].(usecase.Output)
	if !ok1 || total["projects"] != 1 || total["budget_minutes"] != int64(6000) || total["amount_minor"] != int64(375000) || total["currency"] != "EUR" {
		t.Fatalf("Ana's budget roll-up %v", read["budget"])
	}
	rows := read["items"].([]usecase.Output)
	if rows[0]["budget"] == nil || rows[1]["budget"] != nil {
		t.Fatalf("the projects' budgets %v / %v", rows[0]["budget"], rows[1]["budget"])
	}
	calls := b.calls
	asBen := ok(t, ReadPortfolio{s}.Descriptor(), ben, usecase.Input{"portfolio_id": created.String("id")})
	if asBen["budget"] != nil || b.calls != calls {
		t.Fatalf("Ben, who manages no costs, sees %v", asBen["budget"])
	}
	var sum budgetSum
	sum.add(usecase.Output{"budget_amount_minor": int64(1), "amount_minor": int64(1), "currency": "EUR"})
	sum.add(usecase.Output{"budget_amount_minor": int64(1), "amount_minor": int64(1), "currency": "USD"})
	if out := sum.output(); out["mixed_currencies"] != true || out["amount_minor"] != nil {
		t.Fatalf("two currencies summed: %v", out)
	}
}

// UC-GOL-04 checks 1 to 5: a widget runs the query's own summary for its reader; a private
// dashboard is its owner's; a shared one needs STRUCTURE and shows each reader their numbers,
// a widget they cannot run carrying its error in place.
func TestADashboardDrawsEachReadersOwnNumbers(t *testing.T) {
	s, _, g, _ := newSteward(t)
	widgets := []any{
		map[string]any{"title": "Launch", "chart": "PIE", "source": "QUERY", "query": map[string]any{
			"scope_container_id": listA.String(), "aggregate": map[string]any{"function": "COUNT", "group_by": "is_completed"},
		}},
		map[string]any{"title": "Merger", "chart": "NUMBER", "source": "QUERY", "x": 6.0, "query": map[string]any{
			"scope_container_id": listS.String(), "aggregate": map[string]any{"function": "COUNT"},
		}},
	}
	if _, err := call(t, CreateDashboard{s}.Descriptor(), ana, usecase.Input{"name": "Team", "sharing": "SCOPE", "widgets": widgets}); !errors.Is(err, shared.ErrForbidden) || len(g.refused) == 0 {
		t.Fatalf("a member shared a dashboard: %v", err)
	}
	if _, err := call(t, CreateDashboard{s}.Descriptor(), ana, usecase.Input{"name": "Bad", "widgets": []any{
		map[string]any{"title": "x", "chart": "BAR", "source": "QUERY", "query": map[string]any{"scope_container_id": listA.String(), "aggregate": map[string]any{"function": "MEDIAN"}}},
	}}); code(err) != "query.aggregate_function_unknown" {
		t.Fatalf("a malformed summary was stored: %v", err)
	}
	private := ok(t, CreateDashboard{s}.Descriptor(), ana, usecase.Input{"name": "Mine", "widgets": widgets})
	if _, err := call(t, ReadDashboard{s}.Descriptor(), ben, usecase.Input{"dashboard_id": private.String("id")}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Ben reads Ana's private dashboard: %v", err)
	}
	shared := ok(t, CreateDashboard{s}.Descriptor(), admin, usecase.Input{"name": "Team", "sharing": "SCOPE", "widgets": widgets})
	figures := ok(t, ReadDashboardFigures{s}.Descriptor(), ben, usecase.Input{"dashboard_id": shared.String("id")})
	rows := figures["widgets"].([]usecase.Output)
	if len(rows) != 2 || rows[0]["error"] != nil || rows[1]["error"] != "access.denied" {
		t.Fatalf("Ben's figures = %v", rows)
	}
	listed := ok(t, ListDashboards{s}.Descriptor(), ben, usecase.Input{})
	if len(listed["data"].([]usecase.Output)) != 1 {
		t.Fatalf("Ben lists %v", listed["data"])
	}
}

type groups map[shared.ID][]shared.ID

func (g groups) InGroup(_ context.Context, account, group shared.ID) (bool, error) {
	return slices.Contains(g[group], account), nil
}

// UC-GOL-04 check 14: a dashboard shared with groups is seen by their members who read its place,
// and by nobody else but its owner; sharing it asks STRUCTURE as sharing with the place does.
func TestADashboardSharedWithGroupsReachesTheirMembers(t *testing.T) {
	s, _, _, _ := newSteward(t)
	team := shared.MustParseID("018f0000-0000-7000-8000-00000000c001")
	s.Groups = groups{team: {ben}}
	if _, err := call(t, CreateDashboard{s}.Descriptor(), ana, usecase.Input{"name": "Team", "sharing": "GROUPS", "group_ids": []any{team.String()}}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member shared a dashboard with groups: %v", err)
	}
	board := ok(t, CreateDashboard{s}.Descriptor(), admin, usecase.Input{"name": "Team", "sharing": "GROUPS", "group_ids": []any{team.String()}})
	if got := board["group_ids"]; !slices.Equal(got.([]string), []string{team.String()}) {
		t.Fatalf("group_ids = %v", got)
	}
	if _, err := call(t, ReadDashboard{s}.Descriptor(), ben, usecase.Input{"dashboard_id": board.String("id")}); err != nil {
		t.Fatalf("a member of the group: %v", err)
	}
	if _, err := call(t, ReadDashboard{s}.Descriptor(), ana, usecase.Input{"dashboard_id": board.String("id")}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("somebody outside the group: %v", err)
	}
	s.Groups = nil
	if _, err := call(t, ReadDashboard{s}.Descriptor(), ben, usecase.Input{"dashboard_id": board.String("id")}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("without a membership answer: %v", err)
	}
}

// memory is the store in memory.
type memory struct {
	periods    map[shared.ID]domain.Period
	goals      map[shared.ID]domain.Goal
	order      []shared.ID
	links      map[shared.ID][]shared.ID
	checkIns   []domain.CheckIn
	portfolios map[shared.ID]domain.Portfolio
	updates    []domain.StatusUpdate
	dashboards map[shared.ID]domain.Dashboard
	digests    map[shared.ID]domain.Digest
}

var _ goalrepo.Store = (*memory)(nil)

func conflict() error { return shared.ErrVersionConflict.WithDetail("goals.version_conflict") }

func (m *memory) Periods(context.Context, int) ([]domain.Period, error) {
	var out []domain.Period
	for _, p := range m.periods {
		out = append(out, p)
	}
	return out, nil
}

func (m *memory) Period(_ context.Context, id shared.ID) (domain.Period, error) {
	p, ok := m.periods[id]
	if !ok {
		return domain.Period{}, shared.ErrNotFound
	}
	return p, nil
}

func (m *memory) InsertPeriod(_ context.Context, p domain.Period, _ shared.ID, _ time.Time) error {
	p.Version, p.State = 1, domain.PeriodOpen
	m.periods[p.ID] = p
	return nil
}

func (m *memory) UpdatePeriod(_ context.Context, p domain.Period, expected int, _ time.Time) error {
	if m.periods[p.ID].Version != expected {
		return conflict()
	}
	p.Version, p.State = expected+1, domain.PeriodOpen
	m.periods[p.ID] = p
	return nil
}

func (m *memory) ClosePeriod(_ context.Context, id shared.ID, expected int, grades map[shared.ID]float64, _ shared.ID, _ time.Time) error {
	p := m.periods[id]
	if p.Version != expected || p.State != domain.PeriodOpen {
		return conflict()
	}
	p.State, p.Version = domain.PeriodClosed, p.Version+1
	m.periods[id] = p
	for gid, grade := range grades {
		g := m.goals[gid]
		v := grade
		g.Grade = &v
		m.goals[gid] = g
	}
	return nil
}

func (m *memory) Goals(_ context.Context, f goalrepo.GoalFilter) ([]domain.Goal, error) {
	var out []domain.Goal
	for _, id := range m.order {
		g, ok := m.goals[id]
		if ok && (f.PeriodID.IsZero() || g.PeriodID == f.PeriodID) {
			out = append(out, g)
		}
	}
	return out, nil
}

func (m *memory) Goal(_ context.Context, id shared.ID) (domain.Goal, error) {
	g, ok := m.goals[id]
	if !ok {
		return domain.Goal{}, shared.ErrNotFound.WithDetail("goals.not_found")
	}
	return g, nil
}

func (m *memory) InsertGoal(_ context.Context, g domain.Goal, _ shared.ID, _ time.Time) error {
	g.Version = 1
	m.goals[g.ID] = g
	m.order = append(m.order, g.ID)
	return nil
}

func (m *memory) UpdateGoal(_ context.Context, g domain.Goal, expected int, _ shared.ID, _ time.Time) error {
	if m.goals[g.ID].Version != expected {
		return conflict()
	}
	g.Version = expected + 1
	m.goals[g.ID] = g
	return nil
}

func (m *memory) DeleteGoal(_ context.Context, g domain.Goal, _ int) error {
	delete(m.goals, g.ID)
	return nil
}

func (m *memory) LinkedEntries(_ context.Context, goals []shared.ID) (map[shared.ID][]shared.ID, error) {
	out := map[shared.ID][]shared.ID{}
	for _, g := range goals {
		if l := m.links[g]; len(l) > 0 {
			out[g] = l
		}
	}
	return out, nil
}

func (m *memory) GoalsOfEntry(_ context.Context, entry shared.ID, _ int) ([]shared.ID, error) {
	var out []shared.ID
	for g, entries := range m.links {
		for _, e := range entries {
			if e == entry {
				out = append(out, g)
			}
		}
	}
	return out, nil
}

func (m *memory) LinkEntries(_ context.Context, goal shared.ID, entries []shared.ID, _ shared.ID, _ time.Time) error {
	m.links[goal] = append(m.links[goal], entries...)
	return nil
}

func (m *memory) UnlinkEntry(_ context.Context, goal, entry shared.ID) (bool, error) {
	for i, e := range m.links[goal] {
		if e == entry {
			m.links[goal] = append(m.links[goal][:i], m.links[goal][i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (m *memory) AddCheckIn(_ context.Context, c domain.CheckIn) error {
	m.checkIns = append(m.checkIns, c)
	return nil
}

func (m *memory) CheckIns(_ context.Context, goal shared.ID, _ int) ([]domain.CheckIn, error) {
	var out []domain.CheckIn
	for _, c := range m.checkIns {
		if c.GoalID == goal {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memory) Portfolios(context.Context, int) ([]domain.Portfolio, error) {
	var out []domain.Portfolio
	for _, p := range m.portfolios {
		out = append(out, p)
	}
	return out, nil
}

func (m *memory) Portfolio(_ context.Context, id shared.ID) (domain.Portfolio, error) {
	p, ok := m.portfolios[id]
	if !ok {
		return domain.Portfolio{}, shared.ErrNotFound
	}
	return p, nil
}

func (m *memory) InsertPortfolio(_ context.Context, p domain.Portfolio, _ shared.ID, _ time.Time) error {
	p.Version = 1
	m.portfolios[p.ID] = p
	return nil
}

func (m *memory) UpdatePortfolio(_ context.Context, p domain.Portfolio, expected int, _ shared.ID, _ time.Time) error {
	if m.portfolios[p.ID].Version != expected {
		return conflict()
	}
	p.Version = expected + 1
	m.portfolios[p.ID] = p
	return nil
}

func (m *memory) DeletePortfolio(_ context.Context, p domain.Portfolio, _ int) error {
	delete(m.portfolios, p.ID)
	for id, child := range m.portfolios {
		if child.ParentID == p.ID {
			child.ParentID = p.ParentID
			m.portfolios[id] = child
		}
	}
	return nil
}

func (m *memory) AddStatusUpdate(_ context.Context, u domain.StatusUpdate) error {
	m.updates = append(m.updates, u)
	return nil
}

func (m *memory) StatusUpdates(context.Context, shared.ID, int) ([]domain.StatusUpdate, error) {
	return m.updates, nil
}

func (m *memory) Dashboards(context.Context, int) ([]domain.Dashboard, error) {
	var out []domain.Dashboard
	for _, d := range m.dashboards {
		out = append(out, d)
	}
	return out, nil
}

func (m *memory) Dashboard(_ context.Context, id shared.ID) (domain.Dashboard, error) {
	d, ok := m.dashboards[id]
	if !ok {
		return domain.Dashboard{}, shared.ErrNotFound
	}
	return d, nil
}

func (m *memory) InsertDashboard(_ context.Context, d domain.Dashboard, _ time.Time) error {
	m.dashboards[d.ID] = d
	return nil
}

func (m *memory) UpdateDashboard(_ context.Context, d domain.Dashboard, expected int, _ shared.ID, _ time.Time) error {
	if m.dashboards[d.ID].Version != expected {
		return conflict()
	}
	d.Version = expected + 1
	m.dashboards[d.ID] = d
	return nil
}

func (m *memory) DeleteDashboard(_ context.Context, id shared.ID, _ int) error {
	delete(m.dashboards, id)
	return nil
}
