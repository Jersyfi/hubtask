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
// A name after AS is an alias of a side table's column and does not count; the side tables'
// own columns share two names with the entry's (`flow_*`, `*_announced_at`).
var mirroredColumnNamers = map[string][]string{
	// Writers, moved in the first release after the suite.
	"BackupImport.sql ImportWorkItem": {"due_soon_announced_at", "flow_finished_at", "flow_started_at",
		"overdue_announced_at", "retention_action", "retention_blocked_by", "retention_pending_until", "retention_rule_id"},
	"Retention.sql MarkItemsForRetention":     retentionColumns,
	"Retention.sql BlockItemsForRetention":    retentionColumns,
	"Retention.sql ClearItemRetention":        retentionColumns,
	"Retention.sql ClearRetentionMarksOfRule": retentionColumns,
	"Work.sql SetWorkItemDueDate":             {"due_soon_announced_at", "overdue_announced_at"},
	// The announcements' claim and next moment, which stay on the columns until that release.
	"Work.sql ClaimDueSoonItems":   {"due_soon_announced_at"},
	"Work.sql ClaimOverdueItems":   {"overdue_announced_at"},
	"Work.sql NextDueAnnouncement": {"due_soon_announced_at", "overdue_announced_at"},
	// Reads work_item_flow's own columns.
	"Figures.sql CountFigureFlow": {"flow_finished_at", "flow_started_at"},
}

// The one entry that stays: it reads work_item_flow's own columns, which share the entry's names.
var readsTheSideTablesOwnColumns = map[string]bool{"Figures.sql CountFigureFlow": true}

var retentionColumns = []string{"retention_action", "retention_blocked_by", "retention_pending_until", "retention_rule_id"}

func TestNoNewQueryNamesTheSlimEntrysMirroredColumns(t *testing.T) {
	column := regexp.MustCompile(`(?i)(?:\bAS\s+)?(?:\b[a-z_]+\.)?\b(due_soon_announced_at|overdue_announced_at|` +
		`flow_started_at|flow_finished_at|retention_pending_until|retention_rule_id|retention_action|` +
		`retention_blocked_by|search_vector)\b`)
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
		query, named := "", map[string]bool{}
		flush := func() {
			if len(named) == 0 {
				return
			}
			cols := make([]string, 0, len(named))
			for c := range named {
				cols = append(cols, c)
			}
			sort.Strings(cols)
			found[filepath.Base(path)+" "+query] = cols
		}
		for _, line := range strings.Split(string(content), "\n") {
			if m := name.FindStringSubmatch(line); m != nil {
				flush()
				query, named = m[1], map[string]bool{}
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(line), "--") || query == "" {
				continue
			}
			for _, m := range column.FindAllStringSubmatch(line, -1) {
				if !strings.HasPrefix(strings.ToUpper(m[0]), "AS") {
					named[strings.ToLower(m[1])] = true
				}
			}
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
			if !readsTheSideTablesOwnColumns[query] {
				due = append(due, query+" still names the mirrored columns: move it to the side tables")
			}
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
	namers := map[string][]string{"Work.sql SetWorkItemDueDate": {"overdue_announced_at"},
		"Figures.sql CountFigureFlow": {"flow_started_at"}}
	for _, tc := range []struct {
		name     string
		releases int
		namers   map[string][]string
		dropped  bool
		want     int
	}{
		{"the suite's own release ships with the mirror", 0, namers, false, 0},
		{"the next release is refused while a writer names the columns", 1, namers, false, 1},
		{"the next release passes once the writers moved", 1, map[string][]string{"Figures.sql CountFigureFlow": {"flow_started_at"}}, false, 0},
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
