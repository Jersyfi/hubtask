// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"strings"
	"testing"
	"time"
)

func TestServerErrorsAreCountedPerMessage(t *testing.T) {
	log := strings.Join([]string{
		`{"time":"t","level":"INFO","msg":"ready"}`,
		`{"time":"t","level":"WARN","msg":"a slow query","ms":812}`,
		`=== RUN   TestCapacityAtScale`,
		`{"time":"t","level":"WARN","msg":"a slow query","ms":907}`,
		`{"time":"t","level":"ERROR","msg":"the pool is exhausted"}`,
	}, "\n")
	got, err := ServerErrors(strings.NewReader(log), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !same(got[0], LogError{Source: "server", Level: "WARN", Message: "a slow query", Count: 2}) ||
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
	got, err := PostgresErrors(strings.NewReader(log), nil)
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
	got, err := PostgresErrors(strings.NewReader(log), nil)
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
	got, err := PostgresErrors(strings.NewReader(log), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []LogError{
		{Source: "postgres", Level: "ERROR", Message: "canceling statement due to user request", Count: 2},
		{Source: "postgres", Level: "ERROR", Message: "canceling statement due to user request (in ListWorkItems)", Count: 2},
	}
	if len(got) != 2 || !same(got[0], want[0]) || !same(got[1], want[1]) {
		t.Errorf("got %+v", got)
	}
}

// same compares what a row says, not when it was seen.
func same(a, b LogError) bool {
	return a.Source == b.Source && a.Level == b.Level && a.Message == b.Message && a.Count == b.Count
}

// Every row carries its first and last time and the scenarios whose windows any occurrence fell
// in, from the server's and PostgreSQL's log alike (test/load/PLAN.md, "How the errors are
// collected").
func TestARowIsPlacedInTheScenariosItFellIn(t *testing.T) {
	at := func(clock string) time.Time {
		parsed, err := time.Parse(time.RFC3339, "2026-11-05T"+clock+"Z")
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	scenarios := []Scenario{
		{Name: "S1", Start: at("10:00:00"), End: at("10:01:00")},
		{Name: "S2", Start: at("10:01:00"), End: at("10:02:00")},
		{Name: "S1", Start: at("10:02:00"), End: at("10:03:00")},
	}
	server := strings.Join([]string{
		`{"time":"2026-11-05T11:00:30+01:00","level":"WARN","msg":"a slow query"}`,
		`{"time":"2026-11-05T10:02:30Z","level":"WARN","msg":"a slow query"}`,
		`{"time":"2026-11-05T10:05:00Z","level":"ERROR","msg":"between scenarios"}`,
	}, "\n")
	rows, err := ServerErrors(strings.NewReader(server), scenarios)
	if err != nil {
		t.Fatal(err)
	}
	slow := rows[0]
	if slow.Message != "a slow query" || !slow.First.Equal(at("10:00:30")) || !slow.Last.Equal(at("10:02:30")) ||
		strings.Join(slow.Scenarios, ",") != "S1" {
		t.Errorf("the slow query's row %+v", slow)
	}
	if rows[1].Scenarios != nil || !rows[1].First.Equal(at("10:05:00")) {
		t.Errorf("the row outside every window %+v", rows[1])
	}

	postgres := strings.Join([]string{
		`2026-11-05 10:00:10.250 UTC [61] ERROR:  deadlock detected on relation 1`,
		`2026-11-05 10:01:10.000 UTC [62] ERROR:  deadlock detected on relation 2`,
	}, "\n")
	rows, err = PostgresErrors(strings.NewReader(postgres), scenarios)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || strings.Join(rows[0].Scenarios, ",") != "S1,S2" ||
		!rows[0].First.Equal(at("10:00:10").Add(250*time.Millisecond)) || !rows[0].Last.Equal(at("10:01:10")) {
		t.Errorf("PostgreSQL's row %+v", rows)
	}
}

// The figure deadline's cancelled counts are expected and counted apart, with their reason; the
// same message in another query, and the browser's rows of several walks merged, are findings.
func TestTheReportKeepsExpectedRowsOutOfTheFindings(t *testing.T) {
	scenarios := []Scenario{{Name: "S2"}}
	report := Report(scenarios, []LogError{
		{Source: "postgres", Level: "ERROR", Message: "canceling statement due to user request (in CountFigureProgress)", Count: 13, Scenarios: []string{"S2"}},
		{Source: "postgres", Level: "ERROR", Message: "canceling statement due to user request (in ListWorkItems)", Count: 2},
		{Source: "browser", Level: "ERROR", Message: "refused 403 Forbidden: GET /api/v1/x", Count: 2, Scenarios: []string{"S7"}},
		{Source: "browser", Level: "ERROR", Message: "refused 403 Forbidden: GET /api/v1/x", Count: 3, Scenarios: []string{"S7"}},
	})
	if len(report.Expected) != 1 || report.Expected[0].Count != 13 || report.Expected[0].Expected == "" {
		t.Errorf("expected %+v", report.Expected)
	}
	if len(report.Findings) != 2 || report.Findings[0].Count != 5 || report.Findings[0].Source != "browser" ||
		strings.Join(report.Findings[0].Scenarios, ",") != "S7" ||
		report.Findings[1].Message != "canceling statement due to user request (in ListWorkItems)" {
		t.Errorf("findings %+v", report.Findings)
	}
	if len(report.Scenarios) != 1 {
		t.Errorf("scenarios %+v", report.Scenarios)
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
