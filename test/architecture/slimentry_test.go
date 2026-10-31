// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
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
