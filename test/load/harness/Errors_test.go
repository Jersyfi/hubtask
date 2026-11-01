// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"strings"
	"testing"
)

func TestServerErrorsAreCountedPerMessage(t *testing.T) {
	log := strings.Join([]string{
		`{"time":"t","level":"INFO","msg":"ready"}`,
		`{"time":"t","level":"WARN","msg":"a slow query","ms":812}`,
		`=== RUN   TestCapacityAtScale`,
		`{"time":"t","level":"WARN","msg":"a slow query","ms":907}`,
		`{"time":"t","level":"ERROR","msg":"the pool is exhausted"}`,
	}, "\n")
	got, err := ServerErrors(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (LogError{Source: "server", Level: "WARN", Message: "a slow query", Count: 2}) ||
		got[1].Level != "ERROR" || got[1].Count != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestPostgresErrorsAreCountedWithTheirValuesMasked(t *testing.T) {
	log := strings.Join([]string{
		`2026-10-31 13:00:00.000 UTC [61] LOG:  checkpoint starting: time`,
		`2026-10-31 13:00:01.000 UTC [62] ERROR:  deadlock detected on relation 16384`,
		`2026-10-31 13:00:02.000 UTC [63] ERROR:  deadlock detected on relation 16999`,
		`2026-10-31 13:00:03.000 UTC [64] FATAL:  terminating connection due to administrator command`,
	}, "\n")
	got, err := PostgresErrors(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Count != 2 || got[0].Message != "deadlock detected on relation …" || got[1].Level != "FATAL" {
		t.Errorf("got %+v", got)
	}
}

// The migration tool's first look at an empty database is its own probe, not the product's error;
// any other missing relation still counts.
func TestTheMigrationToolsFirstProbeIsNotAnError(t *testing.T) {
	log := strings.Join([]string{
		`2026-10-31 23:52:03.706 UTC [79] ERROR:  relation "goose_db_version" does not exist at character 36`,
		`2026-10-31 23:52:04.706 UTC [80] ERROR:  relation "work_item" does not exist at character 15`,
	}, "\n")
	got, err := PostgresErrors(strings.NewReader(log))
	if err != nil || len(got) != 1 || got[0].Count != 1 {
		t.Fatalf("rows %+v (%v)", got, err)
	}
}

// A cancelled statement is named by the query it cancelled, so finding 4 of the 10⁷ run can be told
// apart by path; the statement's own text, and anything after its name, never reaches the row.
func TestAnErrorIsNamedByTheQueryItsStatementRan(t *testing.T) {
	log := strings.Join([]string{
		`2026-11-01 10:00:00.000 UTC [70] ERROR:  canceling statement due to user request`,
		`2026-11-01 10:00:00.000 UTC [70] STATEMENT:  -- name: ListWorkItems :many`,
		`	SELECT wi.id FROM work_item wi WHERE wi.title = 'secret'`,
		`2026-11-01 10:00:01.000 UTC [71] ERROR:  canceling statement due to user request`,
		`2026-11-01 10:00:01.000 UTC [71] STATEMENT:  -- name: ListWorkItems :many`,
		`2026-11-01 10:00:02.000 UTC [72] ERROR:  canceling statement due to user request`,
		`2026-11-01 10:00:02.000 UTC [72] STATEMENT:  SELECT 1`,
		`2026-11-01 10:00:03.000 UTC [73] ERROR:  canceling statement due to user request`,
	}, "\n")
	got, err := PostgresErrors(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	want := []LogError{
		{Source: "postgres", Level: "ERROR", Message: "canceling statement due to user request", Count: 2},
		{Source: "postgres", Level: "ERROR", Message: "canceling statement due to user request (in ListWorkItems)", Count: 2},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %+v", got)
	}
}

// auto_explain's plans are kept per query, the slowest of each, with how often the query was slow:
// the dataset is gone after the run, and a plan read on a new seed may be another plan.
func TestSlowPlansKeepTheSlowestPlanPerQuery(t *testing.T) {
	log := strings.Join([]string{
		`2026-11-01 10:00:00.000 UTC [70] LOG:  duration: 120.500 ms  plan:`,
		`	Query Text: -- name: ListWorkItems :many`,
		`	SELECT wi.id FROM work_item wi`,
		`	Limit  (cost=0.1..9.0 rows=51 width=8) (actual rows=51 loops=1)`,
		`	  ->  Index Scan using wi_level_order_idx on work_item wi`,
		`2026-11-01 10:00:01.000 UTC [71] LOG:  duration: 480.250 ms  plan:`,
		`	Query Text: -- name: ListWorkItems :many`,
		`	SELECT wi.id FROM work_item wi`,
		`	Limit  (cost=0.1..9.0 rows=51 width=8) (actual rows=51 loops=1)`,
		`	  ->  Sort`,
		`2026-11-01 10:00:02.000 UTC [72] LOG:  checkpoint starting: time`,
		`2026-11-01 10:00:03.000 UTC [73] LOG:  duration: 300.000 ms  plan:`,
		`	Query Text: SELECT count(*) FROM work_item WHERE title = 'secret'`,
		`	Result  (cost=0.00..0.01 rows=1 width=4) (actual rows=1 loops=1)`,
		`2026-11-01 10:00:04.000 UTC [74] LOG:  duration: 200.000 ms  plan:`,
		`	Query Text: SELECT wi.id FROM work_item wi WHERE wi.collection_id = $1`,
		`	Result  (cost=0.00..0.01 rows=1 width=4) (actual rows=1 loops=1)`,
	}, "\n")
	got, err := SlowPlans(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("plans %+v", got)
	}
	first := got[0]
	if first.Query != "ListWorkItems" || first.Count != 2 || first.SlowestMS != 480.25 ||
		!strings.Contains(first.Plan, "->  Sort") || strings.Contains(first.Plan, "Index Scan") ||
		strings.Contains(first.Plan, "SELECT") {
		t.Errorf("first %+v", first)
	}
	// The query DSL's statements carry no name: they are told apart by their opening, its values
	// masked.
	if !strings.HasPrefix(got[1].Query, "SELECT count(*) FROM work_item WHERE title = … #") || got[1].Count != 1 ||
		!strings.HasPrefix(got[2].Query, "SELECT wi.id FROM work_item wi WHERE wi.collection_id = $… #") {
		t.Errorf("unnamed %+v", got[1:])
	}
}
