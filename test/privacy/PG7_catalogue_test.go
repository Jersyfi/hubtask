// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package privacy

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// PG-7: the data catalogue is consistent with the schema - every table with personal content is
// recorded (data-protection.md §10, ADR-0018 decision 2).
//
// The decisive one, with PG-2. `data-protection.md` says why in as many words: these two "prevent
// the data catalogue from drifting away from the code and stop deletion paths being forgotten when
// new tables are added - the usual route by which a clean deletion concept becomes untrue over two
// years".
//
// It reads a **migrated database** rather than the schema file, for the reason
// `schema_reference_test.go` does: what an installation runs is the migrations, and a document
// checked against another document proves the two documents agree.
//
// The catalogue's own rule 6 is honoured here rather than worked around: a partition is not a data
// category, so `audit_log_2026_08` resolves to `audit_log` and does not demand a row of its own.

// personalColumns are the column names that make a table hold somebody's data. A table with one of
// these and no entry in the catalogue is what this gate fails on.
//
// Names rather than types, because a `uuid` says nothing and `author_id` says everything. The list
// is the vocabulary this schema actually uses; a column that means a person and is spelled some
// other way is a column somebody has to add here, which is the moment they would also add the
// catalogue row.
var personalColumns = map[string]bool{
	"account_id": true, "actor_id": true, "author_id": true, "assignee_id": true,
	"created_by": true, "updated_by": true, "completed_by": true, "owner_id": true,
	"recipient_id": true, "handled_by": true, "placed_by": true, "released_by": true,
	"subject_account_id": true, "subject_email": true, "on_behalf_of_id": true,
	"run_as": true, "email": true, "sender": true, "actor_label": true,
	"display_name": true, "ip_truncated": true, "external_subject": true,
	"password_hash": true, "token_hash": true, "recipients": true,
	"user_agent": true, "ip_class": true, "subject_hash": true,
	"redemption_token_hash": true, "secret_enc": true, "code_hash": true,
}

// notADataCategory is what the catalogue deliberately does not have a row for, and why. Rule 6's
// partitions are resolved before this, so what is left here is the machinery: tables that hold the
// system's own bookkeeping about work rather than anybody's data.
var notADataCategory = map[string]string{
	"goose_db_version":        "the migration ledger: which migrations ran, and nothing about anybody",
	"item_capability_profile": "which fields a kind of entry may carry - the product's own shape",
	"leader_election":         "which process is the leader for a minute",
}

func TestPG7EveryTableWithPersonalContentIsInTheCatalogue(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AdminPool(ctx, t)

	rows, err := pool.Query(ctx, `
		SELECT c.relname, a.attname,
		       coalesce(parent.relname, '') AS parent
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		LEFT JOIN pg_inherits i ON i.inhrelid = c.oid
		LEFT JOIN pg_class parent ON parent.oid = i.inhparent
		WHERE c.relkind IN ('r', 'p')
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	defer rows.Close()

	// table -> the personal columns it carries, with partitions folded into their parent.
	personal := map[string][]string{}
	for rows.Next() {
		var table, column, parent string
		if err := rows.Scan(&table, &column, &parent); err != nil {
			t.Fatalf("reading the schema: %v", err)
		}
		if parent != "" {
			// Rule 6: a partition is recorded through its parent.
			table = parent
		}
		if personalColumns[column] {
			personal[table] = append(personal[table], column)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	if len(personal) == 0 {
		t.Fatal("PG-7 found no table with personal content at all - the column vocabulary no longer matches")
	}

	recorded := catalogueTables(t)
	var missing []string
	for table, columns := range personal {
		if recorded[table] || notADataCategory[table] != "" {
			continue
		}
		sort.Strings(columns)
		missing = append(missing, table+" ("+strings.Join(columns, ", ")+")")
	}
	sort.Strings(missing)

	for _, table := range missing {
		t.Errorf("%s holds personal content and has no entry in docs/privacy/data-catalog.md "+
			"(PG-7). Add the row - the category, the classification, the retention and the "+
			"deletion path - or say in notADataCategory why it is not one", table)
	}
	t.Logf("PG-7 reconciled %d tables holding personal content against the catalogue", len(personal))
}

// And the reverse, as a reading rather than a failure: a catalogue row naming a table this schema
// does not have is either a plan (`account_mfa` and `account_identity` arrive with the sign-in flow
// in 0.6.0) or a leftover. The gate says which names are unmatched and leaves the judgement to the
// person reading it - failing here would make the document unable to describe anything before it
// exists, which is what a record of processing activities is often written to do.
func TestPG7TheCatalogueNamesNoTableThatVanished(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AdminPool(ctx, t)

	rows, err := pool.Query(ctx, `
		SELECT c.relname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		WHERE c.relkind IN ('r', 'p')`)
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	defer rows.Close()

	existing := map[string]bool{}
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("reading the schema: %v", err)
		}
		existing[table] = true
	}

	var unmatched []string
	for table := range catalogueTables(t) {
		if !existing[table] {
			unmatched = append(unmatched, table)
		}
	}
	sort.Strings(unmatched)
	if len(unmatched) > 0 {
		t.Logf("the catalogue names %d tables this schema does not have yet: %s",
			len(unmatched), strings.Join(unmatched, ", "))
	}
}

// catalogueTables reads every backticked identifier out of the catalogue's "Table / location"
// columns.
//
// The document is the source the gate reads, which is what "one source" means: there is no second
// machine-readable copy to keep in step with it. A name that is not a table - "Object storage",
// "Prometheus" - simply matches nothing in the schema and is ignored.
func catalogueTables(t *testing.T) map[string]bool {
	t.Helper()

	content := readFile(t, "../../docs/privacy/data-catalog.md")
	identifier := regexp.MustCompile("`([a-z][a-z0-9_]*)`")

	tables := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		columns := strings.Split(line, "|")
		if len(columns) < 3 {
			continue
		}
		// The second column is "Table / location" in every section's table.
		for _, match := range identifier.FindAllStringSubmatch(columns[2], -1) {
			tables[match[1]] = true
		}
	}
	if len(tables) == 0 {
		t.Fatal("no table found in the data catalogue - the document's shape has changed")
	}
	return tables
}
