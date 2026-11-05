// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres_test

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// notImported is every column of a restored table that its import statement leaves out on
// purpose, with the reason. The export writes the whole row (`to_jsonb`), so a column missing
// from an import's column list is a value a restore drops without a word; this list is where
// leaving one out is a decision somebody wrote down.
var notImported = map[string]string{
	"account.ai_consent": "not exported; dropped by a later contract migration (tenant-export.md §11)",
	// The installation's state of the workspace at the moment of the restore, not the archive's:
	// the grace clock of a deletion (without it nothing is purged), the epoch the restore itself
	// advances, the plan and the platform's reference, an operator's password opening, the last
	// archive download.
	"tenant.purge_after":               "the installation's deletion clock",
	"tenant.purge_started_at":          "the installation's deletion clock",
	"tenant.sync_epoch":                "advanced by the restore itself",
	"tenant.plan_id":                   "the installation's plan mapping",
	"tenant.external_reference":        "not exported: the platform's mapping",
	"tenant.password_opened_until":     "an operator's opening, bounded in time",
	"tenant.password_opened_requester": "an operator's opening, bounded in time",
	"tenant.password_opened_reason":    "an operator's opening, bounded in time",
	"tenant.archive_downloaded_at":     "not exported: the installation's window",
	"page.search_document":             "not exported: the trigger builds it",
	"work_item.search_configuration":   "the trigger sets it with the document",
	"media_object.unreferenced_since":  "the recount's clock; NULL holds the sweep off until it counts",
	"entry_status_change.seq":          "drawn anew: the identity is installation-wide, and a carried one could meet the next drawn",
}

var (
	createTable = regexp.MustCompile(`^CREATE TABLE (?:IF NOT EXISTS )?([a-z_][a-z0-9_]*) \($`)
	columnLine  = regexp.MustCompile(`^  ([a-z_][a-z0-9_]*)\s+\S`)
	alterTable  = regexp.MustCompile(`^ALTER TABLE ([a-z_][a-z0-9_]*) ADD COLUMN (?:IF NOT EXISTS )?([a-z_][a-z0-9_]*)`)
	addColumn   = regexp.MustCompile(`^\s+ADD COLUMN (?:IF NOT EXISTS )?([a-z_][a-z0-9_]*)`)
	sqlComment  = regexp.MustCompile(`--[^\n]*`)
	insertInto  = regexp.MustCompile(`(?s)INSERT INTO ([a-z_][a-z0-9_]*) \(([^)]*)\)`)
)

var notAColumn = map[string]bool{
	"constraint": true, "primary": true, "unique": true, "foreign": true, "check": true,
	"exclude": true, "like": true,
}

// schemaColumns reads db/schema.sql into table → writable columns. A generated column cannot be
// written and is not counted.
func schemaColumns(t *testing.T) map[string][]string {
	t.Helper()
	file, err := os.Open("../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	out := map[string][]string{}
	table, altering := "", ""
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case createTable.MatchString(line):
			table, altering = createTable.FindStringSubmatch(line)[1], ""
			continue
		case alterTable.MatchString(line):
			match := alterTable.FindStringSubmatch(line)
			table, altering = "", match[1]
			out[altering] = append(out[altering], match[2])
			continue
		}
		if altering != "" {
			if match := addColumn.FindStringSubmatch(line); match != nil {
				out[altering] = append(out[altering], match[1])
				continue
			}
			altering = ""
		}
		if table == "" {
			continue
		}
		if strings.HasPrefix(line, ")") {
			table = ""
			continue
		}
		match := columnLine.FindStringSubmatch(line)
		if match == nil || notAColumn[match[1]] || strings.Contains(line, "GENERATED ALWAYS AS (") ||
			strings.HasSuffix(strings.TrimSpace(line), "GENERATED ALWAYS AS") {
			continue
		}
		out[table] = append(out[table], match[1])
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// importedColumns reads db/queries/BackupImport.sql into table → the columns its INSERT writes.
func importedColumns(t *testing.T) map[string][]string {
	t.Helper()
	raw, err := os.ReadFile("../../db/queries/BackupImport.sql")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	text := sqlComment.ReplaceAllString(string(raw), "")
	for _, match := range insertInto.FindAllStringSubmatch(text, -1) {
		for _, column := range strings.Split(match[2], ",") {
			out[match[1]] = append(out[match[1]], strings.TrimSpace(column))
		}
	}
	return out
}

// Every column of a restored table is written back by its import, or is on notImported with a
// reason: a restore that drops a hub's logo or a rule's pause is corruption that looks like success.
func TestARestoreWritesBackEveryColumnTheSchemaHolds(t *testing.T) {
	schema := schemaColumns(t)
	imported := importedColumns(t)
	used := map[string]bool{}
	for _, table := range postgres.ImportableTables() {
		columns, known := schema[table]
		if !known {
			t.Errorf("%s is restored and db/schema.sql has no such table", table)
			continue
		}
		written, found := imported[table]
		if !found {
			t.Errorf("%s is restored and BackupImport.sql has no INSERT INTO it", table)
			continue
		}
		for _, column := range columns {
			key := table + "." + column
			if _, excluded := notImported[key]; excluded {
				used[key] = true
				continue
			}
			if !slices.Contains(written, column) {
				t.Errorf("%s is in the schema and a restore does not write it back", key)
			}
		}
	}
	for key := range notImported {
		if !used[key] {
			t.Errorf("%s is listed as not imported and is no column of a restored table", key)
		}
	}
}
