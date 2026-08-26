// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"bytes"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func upsert(id string) Record {
	return Record{ID: id, Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"title_length": 4}}
}

func lines(records ...Record) func(func(Record) error) error {
	return func(yield func(Record) error) error {
		for _, record := range records {
			if err := yield(record); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestRecordsSurviveTheRoundTripInOrder(t *testing.T) {
	written := []Record{
		upsert("a"),
		{ID: "b", Op: OpDelete, UpdatedAt: snapshot()},
		{ID: "c", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"x": "y"},
			Blobs: []Blob{{Digest: strings.Repeat("ab", 32), Bytes: 12}}},
	}

	var file bytes.Buffer
	count, err := WriteRecords(&file, lines(written...))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if count != 3 {
		t.Fatalf("count %d, want 3", count)
	}

	var read []Record
	if err := ReadRecords(&file, func(r Record) error { read = append(read, r); return nil }); err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(read) != 3 {
		t.Fatalf("read %d records", len(read))
	}
	for i := range written {
		if read[i].ID != written[i].ID || read[i].Op != written[i].Op {
			t.Fatalf("record %d: %+v, want %+v", i, read[i], written[i])
		}
	}
	if read[2].Blobs[0].Digest != written[2].Blobs[0].Digest {
		t.Fatalf("blob lost: %+v", read[2])
	}
}

// One line per record is the property the whole format rests on: a writer never holds the file.
func TestOneRecordIsOneLine(t *testing.T) {
	var file bytes.Buffer
	if _, err := WriteRecords(&file, lines(upsert("a"), upsert("b"))); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := strings.Count(strings.TrimRight(file.String(), "\n"), "\n"); got != 1 {
		t.Fatalf("%d newlines between two records:\n%s", got, file.String())
	}
	if strings.HasPrefix(file.String(), "[") {
		t.Fatal("the file is a JSON array, not JSON Lines")
	}
}

// A tombstone carries no content, and the reason is not tidiness: the fields of a row being
// deleted are exactly the personal data an erasure was meant to remove.
func TestATombstoneCarriesNoContent(t *testing.T) {
	withData := Record{ID: "a", Op: OpDelete, UpdatedAt: snapshot(), Data: map[string]any{"title": "buy milk"}}

	if err := withData.Validate(); err == nil {
		t.Fatal("a tombstone carrying a payload was accepted")
	}
	if got := detail(t, withData.Validate()); got != CodeRecordInvalid {
		t.Fatalf("detail code %q", got)
	}

	withBlob := Record{ID: "a", Op: OpDelete, UpdatedAt: snapshot(), Blobs: []Blob{{Digest: strings.Repeat("a", 64)}}}
	if err := withBlob.Validate(); err == nil {
		t.Fatal("a tombstone referencing a medium was accepted")
	}
}

func TestValidateRefusesWhatCannotBeApplied(t *testing.T) {
	cases := map[string]Record{
		"id":         {Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{}},
		"op":         {ID: "a", Op: "MAYBE", UpdatedAt: snapshot(), Data: map[string]any{}},
		"updated_at": {ID: "a", Op: OpUpsert, Data: map[string]any{}},
		"data":       {ID: "a", Op: OpUpsert, UpdatedAt: snapshot()},
		"blobs.sha256": {ID: "a", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{},
			Blobs: []Blob{{Digest: "../../etc/passwd"}}},
	}

	for reason, record := range cases {
		t.Run(reason, func(t *testing.T) {
			if err := record.Validate(); err == nil {
				t.Fatalf("%s was accepted", reason)
			}
		})
	}
}

// A digest becomes a key at somebody else's storage. It is checked rather than trusted, for the
// reason the storage port refuses a key that tries to leave its namespace.
func TestADigestIsSixtyFourHexCharactersOrItIsNotAnAddress(t *testing.T) {
	for _, bad := range []string{"", "AB" + strings.Repeat("c", 62), strings.Repeat("a", 63),
		strings.Repeat("a", 65), "../" + strings.Repeat("a", 61)} {
		if looksLikeDigest(bad) {
			t.Fatalf("%q was accepted as a content address", bad)
		}
	}
	if !looksLikeDigest(strings.Repeat("0123456789abcdef", 4)) {
		t.Fatal("a real digest was refused")
	}
}

func TestAnUnknownFieldInALineIsRefused(t *testing.T) {
	file := strings.NewReader(`{"id":"a","op":"UPSERT","updated_at":"2026-08-26T03:00:00Z","smuggled":1}` + "\n")

	err := ReadRecords(file, func(Record) error { return nil })
	if err == nil {
		t.Fatal("an unknown field was accepted")
	}
	if got := detail(t, err); got != CodeRecordUnreadable {
		t.Fatalf("detail code %q", got)
	}
}

func TestAWriteThatFailsValidationStopsAtTheBadLine(t *testing.T) {
	var file bytes.Buffer

	count, err := WriteRecords(&file, lines(upsert("a"), Record{ID: "", Op: OpUpsert, UpdatedAt: time.Now()}))
	if err == nil {
		t.Fatal("an invalid record was written")
	}
	if count != 1 {
		t.Fatalf("count %d, want the one good line", count)
	}
}

// The list of entities and the list of exclusions have to be total over the schema. A table added
// by a later migration turns this red until somebody decides which of the two it is - "nobody
// thought about it" is how a restore quietly loses a feature's data.
func TestEveryTenantScopedTableIsEitherArchivedOrDeliberatelyNot(t *testing.T) {
	schema, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	archived := map[string]bool{}
	for _, entity := range Entities() {
		if archived[entity.Table] {
			t.Errorf("%s is archived twice", entity.Table)
		}
		archived[entity.Table] = true
	}

	for _, table := range tenantScopedTables(t, string(schema)) {
		_, isExcluded := excluded[table]
		switch {
		case archived[table] && isExcluded:
			t.Errorf("%s is both archived and excluded - decide", table)
		case !archived[table] && !isExcluded:
			t.Errorf("%s is under row level security and in neither list: archive it, or say in "+
				"excluded why an archive leaves it out", table)
		}
	}

	// And nothing in either list that the schema does not have, which is how a table renamed by a
	// migration is caught rather than silently skipped.
	all := tenantScopedTables(t, string(schema))
	for table := range excluded {
		if !slices.Contains(all, table) {
			t.Errorf("excluded names %s, which is not a tenant-scoped table", table)
		}
	}
	for _, entity := range Entities() {
		if !slices.Contains(all, entity.Table) {
			t.Errorf("%s names %s, which is not a tenant-scoped table", entity, entity.Table)
		}
	}
}

// tenantScopedTables reads what the schema itself puts under row level security: the array
// 0001_init loops over, plus the handful that declare their own policy because a loop cannot -
// the partitioned ones, and the ones whose policy is not a plain comparison against the tenant.
//
// Reading the schema rather than keeping a second list is the point: the second list is the one
// that goes stale, and a stale list here means a table that is silently in neither.
func tenantScopedTables(t *testing.T, schema string) []string {
	t.Helper()

	block := regexp.MustCompile(`(?s)FOREACH t IN ARRAY ARRAY\[(.*?)\]`).FindStringSubmatch(schema)
	if block == nil {
		t.Fatal("the row level security loop is no longer recognisable in db/schema.sql")
	}
	var tables []string
	for _, quoted := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(block[1], -1) {
		tables = append(tables, quoted[1])
	}
	for _, named := range regexp.MustCompile(`ALTER TABLE ([a-z_]+) ENABLE ROW LEVEL SECURITY`).
		FindAllStringSubmatch(schema, -1) {
		if !slices.Contains(tables, named[1]) {
			tables = append(tables, named[1])
		}
	}
	if len(tables) < 20 {
		t.Fatalf("only %d tenant-scoped tables found - the pattern no longer matches", len(tables))
	}
	return tables
}

// Restore order is part of the format: a row's parents come first, so that a restore applies the
// files in sequence without deferring foreign keys.
func TestParentsComeBeforeChildren(t *testing.T) {
	position := map[string]int{}
	for i, entity := range Entities() {
		position[entity.Name] = i
	}

	for child, parent := range map[string]string{
		"work_items":       "containers",
		"buckets":          "containers",
		"comments":         "work_items",
		"item_labels":      "labels",
		"item_members":     "accounts",
		"item_attachments": "media_objects",
		"reminders":        "work_items",
		"memberships":      "accounts",
		"containers":       "tenants",
	} {
		if position[parent] >= position[child] {
			t.Errorf("%s (%d) is written before its parent %s (%d)",
				child, position[child], parent, position[parent])
		}
	}
}

func TestOnlyTheAuditTrailIsOptional(t *testing.T) {
	for _, entity := range Entities() {
		if entity.Optional && entity.Name != "audit" {
			t.Errorf("%s is optional; only the audit trail is (backup-restore.md §7)", entity)
		}
	}
	audit, found := FindEntity("audit")
	if !found || !audit.Optional {
		t.Fatal("the audit trail is not the optional entity")
	}
	if last := Entities()[len(Entities())-1]; last.Name != "audit" {
		t.Fatalf("the optional file is not written last: %s", last)
	}
}

func TestEveryExclusionSaysWhy(t *testing.T) {
	for table, reason := range ExcludedTables() {
		if len(reason) < 20 {
			t.Errorf("%s is excluded with %q, which is not a reason", table, reason)
		}
	}
}
