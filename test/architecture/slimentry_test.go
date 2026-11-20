// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The slim entry's contract half (versioning-release.md §4, "Scheduled contract steps") drops the
// eight columns `work_item_side_rows` mirrors. It can land only once no query names them, so the
// queries that still do are held to this list: a new one fails, and a moved one must leave it.
// The writers still on it move in the first release after the suite (v0.10.1).
//
// What does not count as naming an entry's column: a name after AS (an alias of a side table's
// column), a name inside a string literal (the archive's record keeps the columns' names as its
// keys), and the side tables' own columns that share a name with the entry's (`flow_*`,
// `*_announced_at`) - qualified by that side table, its alias or EXCLUDED, or unqualified in a
// query that names that side table. Such a query qualifies every column of the entry it names.
var mirroredColumnNamers = map[string][]string{
	"BackupImport.sql ImportWorkItem": {"due_soon_announced_at", "flow_finished_at", "flow_started_at",
		"overdue_announced_at", "retention_action", "retention_blocked_by", "retention_pending_until", "retention_rule_id"},
	"Work.sql SetWorkItemDueDate":  {"due_soon_announced_at", "overdue_announced_at"},
	"Work.sql ClaimDueSoonItems":   {"due_soon_announced_at"},
	"Work.sql ClaimOverdueItems":   {"overdue_announced_at"},
	"Work.sql NextDueAnnouncement": {"due_soon_announced_at", "overdue_announced_at"},
}

// sideTableColumns are the side tables' own columns of the same name as the entry's.
var sideTableColumns = map[string][]string{
	"work_item_announcement": {"due_soon_announced_at", "overdue_announced_at"},
	"work_item_flow":         {"flow_started_at", "flow_finished_at"},
}

var (
	mirroredColumn = regexp.MustCompile(`(?i)(\bAS\s+)?(?:\b([a-z_][a-z0-9_]*)\.)?\b(due_soon_announced_at|` +
		`overdue_announced_at|flow_started_at|flow_finished_at|retention_pending_until|retention_rule_id|` +
		`retention_action|retention_blocked_by|search_vector)\b`)
	stringLiteral = regexp.MustCompile(`'[^']*'`)
	sideTableRef  = regexp.MustCompile(`(?i)\b(work_item_announcement|work_item_flow)\b(?:\s+(?:AS\s+)?([a-z_][a-z0-9_]*))?`)
	notAnAlias    = map[string]bool{"on": true, "where": true, "using": true, "set": true, "left": true,
		"right": true, "inner": true, "join": true, "cross": true, "full": true, "group": true, "order": true,
		"limit": true, "returning": true, "values": true, "select": true, "from": true, "and": true, "or": true,
		"do": true, "with": true}
)

// mirroredColumnsNamed answers the entry's mirrored columns one query's text names.
func mirroredColumnsNamed(query string) []string {
	query = stringLiteral.ReplaceAllString(query, "''")
	// For each column a side table shares with the entry: the qualifiers that mean the side table.
	sideQualifiers := map[string]map[string]bool{}
	for _, m := range sideTableRef.FindAllStringSubmatch(query, -1) {
		table := strings.ToLower(m[1])
		for _, col := range sideTableColumns[table] {
			if sideQualifiers[col] == nil {
				sideQualifiers[col] = map[string]bool{"excluded": true}
			}
			sideQualifiers[col][table] = true
			if alias := strings.ToLower(m[2]); alias != "" && !notAnAlias[alias] {
				sideQualifiers[col][alias] = true
			}
		}
	}
	named := map[string]bool{}
	for _, m := range mirroredColumn.FindAllStringSubmatch(query, -1) {
		if m[1] != "" {
			continue
		}
		col, qualifier := strings.ToLower(m[3]), strings.ToLower(m[2])
		side, shared := sideQualifiers[col]
		if shared && (qualifier == "" || side[qualifier]) {
			continue
		}
		named[col] = true
	}
	cols := make([]string, 0, len(named))
	for c := range named {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	return cols
}

func TestNoNewQueryNamesTheSlimEntrysMirroredColumns(t *testing.T) {
	name := regexp.MustCompile(`^--\s*name:\s*(\w+)`)

	files, err := filepath.Glob("../../db/queries/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no queries found: %v", err)
	}
	found := map[string][]string{}
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		query, body := "", []string{}
		flush := func() {
			if query == "" {
				return
			}
			if cols := mirroredColumnsNamed(strings.Join(body, "\n")); len(cols) > 0 {
				found[filepath.Base(path)+" "+query] = cols
			}
		}
		for _, line := range strings.Split(string(content), "\n") {
			if m := name.FindStringSubmatch(line); m != nil {
				flush()
				query, body = m[1], nil
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(line), "--") || query == "" {
				continue
			}
			body = append(body, line)
		}
		flush()
	}

	for query, cols := range found {
		want, held := mirroredColumnNamers[query]
		if !held {
			t.Errorf("%s names the slim entry's mirrored columns %v: read and write the side tables "+
				"instead (versioning-release.md §4, \"Scheduled contract steps\")", query, cols)
			continue
		}
		if !reflect.DeepEqual(cols, want) {
			t.Errorf("%s names %v, the list holds %v: a column moved off leaves the list, none joins it", query, cols, want)
		}
	}
	for query := range mirroredColumnNamers {
		if _, still := found[query]; !still {
			t.Errorf("%s no longer names a mirrored column: remove it from the list", query)
		}
	}
}

func TestTheMatcherTellsTheEntrysColumnsFromTheSideTables(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"UPDATE work_item SET due_soon_announced_at = NULL", []string{"due_soon_announced_at"}},
		{"SELECT w.flow_started_at FROM work_item w LEFT JOIN work_item_flow f ON f.item_id = w.id",
			[]string{"flow_started_at"}},
		{"SELECT f.flow_started_at FROM work_item w LEFT JOIN work_item_flow f ON f.item_id = w.id", []string{}},
		{"SELECT a.due_soon_announced_at FROM work_item_announcement AS a", []string{}},
		{"INSERT INTO work_item_announcement (item_id, overdue_announced_at) VALUES ($1, $2) " +
			"ON CONFLICT (item_id) DO UPDATE SET overdue_announced_at = EXCLUDED.overdue_announced_at", []string{}},
		{"SELECT rt.pending_until AS retention_pending_until FROM work_item_retention rt", []string{}},
		{"SELECT j->>'retention_action' FROM t", []string{}},
		{"SELECT retention_action FROM work_item_retention", []string{"retention_action"}},
		{"SELECT due_soon_announced_at FROM work_item_flow", []string{"due_soon_announced_at"}},
	} {
		if got := mirroredColumnsNamed(tc.query); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q names %v, want %v", tc.query, got, tc.want)
		}
	}
}

// releasesSinceSlimExpand names how many releases already shipped migration 0235; `make release-tag`
// sets it from the tags, so the contract steps cannot be forgotten: the release after the suite's
// is refused while a writer still names the columns, the one after that while the trigger stands.
const releasesSinceSlimExpand = "HUBTASK_RELEASES_SINCE_SLIM_EXPAND"

var dropsTheMirror = regexp.MustCompile(`(?i)DROP\s+TRIGGER\s+(?:IF\s+EXISTS\s+)?work_item_side_rows\b`)
var createsTheMirror = regexp.MustCompile(`(?i)CREATE\s+(?:OR\s+REPLACE\s+)?TRIGGER\s+work_item_side_rows\b`)

// slimEntryStepsDue answers the contract steps a release is cut without, given how many releases
// already shipped the expand half, the queries still naming the columns and whether a migration
// after 0235 drops the mirror.
func slimEntryStepsDue(releases int, namers map[string][]string, mirrorDropped bool) []string {
	var due []string
	if releases >= 1 {
		for query := range namers {
			due = append(due, query+" still names the mirrored columns: move it to the side tables")
		}
		sort.Strings(due)
	}
	if releases >= 2 && !mirrorDropped {
		due = append(due, "no migration drops work_item_side_rows and the eight columns")
	}
	return due
}

func TestTheSlimEntrysContractStepsAreDueOnTime(t *testing.T) {
	releases := 0
	if v := os.Getenv(releasesSinceSlimExpand); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			t.Fatalf("%s=%q is not a count of releases", releasesSinceSlimExpand, v)
		}
		releases = n
	}
	files, err := filepath.Glob("../../db/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	dropped := false
	for _, path := range files {
		if filepath.Base(path) <= "0235" {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if dropsTheMirror.Match(content) && !createsTheMirror.Match(content) {
			dropped = true
		}
	}
	for _, step := range slimEntryStepsDue(releases, mirroredColumnNamers, dropped) {
		t.Errorf("release %d after the slim entry's expand half: %s (versioning-release.md §4, "+
			"\"Scheduled contract steps\")", releases, step)
	}
}

func TestTheReleaseTagCountsTheReleasesSinceTheSlimExpand(t *testing.T) {
	makefile, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	recipe := string(makefile)
	start := strings.Index(recipe, "\nrelease-tag:")
	if start < 0 {
		t.Fatal("the Makefile has no release-tag target")
	}
	recipe = recipe[start:]
	if end := strings.Index(recipe, "\n\n"); end > 0 {
		recipe = recipe[:end]
	}
	tag := strings.Index(recipe, "git tag -s")
	check := strings.Index(recipe, releasesSinceSlimExpand+"=")
	if check < 0 || tag < 0 || check > tag {
		t.Errorf("release-tag must set %s and run TestTheSlimEntrysContractStepsAreDueOnTime before it tags",
			releasesSinceSlimExpand)
	}
}

func TestSlimEntryStepsDue(t *testing.T) {
	namers := map[string][]string{"Work.sql SetWorkItemDueDate": {"overdue_announced_at"}}
	for _, tc := range []struct {
		name     string
		releases int
		namers   map[string][]string
		dropped  bool
		want     int
	}{
		{"the suite's own release ships with the mirror", 0, namers, false, 0},
		{"the next release is refused while a writer names the columns", 1, namers, false, 1},
		{"the next release passes once the writers moved", 1, map[string][]string{}, false, 0},
		{"the release after is refused while the mirror stands", 2, map[string][]string{}, false, 1},
		{"the release after passes once the mirror is dropped", 2, map[string][]string{}, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := slimEntryStepsDue(tc.releases, tc.namers, tc.dropped); len(got) != tc.want {
				t.Errorf("due %v, want %d", got, tc.want)
			}
		})
	}
}
