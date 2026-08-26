// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The statements a restore writes a tenant through (E-06): the three outcomes of a collision, the
// tenant the row lands in whatever the archive says, and the boundary it may not cross (SG-3).

func importRepo() postgres.BackupImportRepository { return postgres.NewBackupImportRepository() }

// containerRow is one container as the archive carries it: the row with `tenant_id` taken out.
func containerRow(id shared.ID, author shared.ID, name string) map[string]any {
	return map[string]any{
		"id": id.String(), "type": "HUB", "name": name, "order_key": "m",
		"policies": map[string]any{}, "created_by": author.String(),
		"created_at": created.Format(time.RFC3339Nano),
		"updated_at": created.Format(time.RFC3339Nano),
		"version":    1,
	}
}

func containerName(ctx context.Context, t *testing.T, tenant, id shared.ID) string {
	t.Helper()
	var name string
	err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT name FROM container WHERE tenant_id = $1 AND id = $2`,
		tenant.String(), id.String()).Scan(&name)
	if err != nil {
		t.Fatalf("reading back the container: %v", err)
	}
	return name
}

func TestAnImportedRowLandsInTheTenantOfTheTransaction(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	id := freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "container", containerRow(id, authorA, "Imported"), false)
		return err
	}); err != nil {
		t.Fatalf("importing: %v", err)
	}

	if name := containerName(ctx, t, tenantA, id); name != "Imported" {
		t.Fatalf("the container came back as %q", name)
	}
}

// The three outcomes of a collision, which is the whole of the conflict rule: a row the tenant does
// not have is written, one it has is left alone under SKIP, and replaced under OVERWRITE.
func TestACollisionIsSettledByTheOverwriteFlag(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	id := freshID(t)

	var first, second, third bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if first, err = importRepo().Write(ctx, "container", containerRow(id, authorA, "Original"), false); err != nil {
			return err
		}
		if second, err = importRepo().Write(ctx, "container", containerRow(id, authorA, "Skipped"), false); err != nil {
			return err
		}
		third, err = importRepo().Write(ctx, "container", containerRow(id, authorA, "Overwritten"), true)
		return err
	}); err != nil {
		t.Fatalf("importing: %v", err)
	}

	if !first {
		t.Error("the first write reported nothing written")
	}
	if second {
		t.Error("a collision under SKIP reported a write")
	}
	if !third {
		t.Error("a collision under OVERWRITE reported no write")
	}
	if name := containerName(ctx, t, tenantA, id); name != "Overwritten" {
		t.Fatalf("the container is %q, so the overwrite did not take", name)
	}
}

// The question the dry run asks, and the one the execution asks again. It has to be answerable
// without writing anything, or §8.3's "a dry run with a report" is not possible.
func TestHoldsAnswersWithoutWriting(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	id := freshID(t)

	var before, after bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if before, err = importRepo().Holds(ctx, "container", containerRow(id, authorA, "x")); err != nil {
			return err
		}
		if _, err = importRepo().Write(ctx, "container", containerRow(id, authorA, "x"), false); err != nil {
			return err
		}
		after, err = importRepo().Holds(ctx, "container", containerRow(id, authorA, "x"))
		return err
	}); err != nil {
		t.Fatalf("importing: %v", err)
	}

	if before {
		t.Error("the tenant held a row it had never been given")
	}
	if !after {
		t.Error("the tenant does not hold a row it was just given")
	}
}

// A row of an entity whose key is made of references, which is what every join table is. It has no
// identity of its own and the import has to find it by both parts.
func TestARowWithACompositeKeyIsFoundByBothParts(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	group, account := freshID(t), authorA
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, 'Group')`,
		group.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the group: %v", err)
	}

	row := map[string]any{"group_id": group.String(), "account_id": account.String()}
	var held bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := importRepo().Write(ctx, "account_group_member", row, false); err != nil {
			return err
		}
		var err error
		held, err = importRepo().Holds(ctx, "account_group_member", row)
		return err
	}); err != nil {
		t.Fatalf("importing: %v", err)
	}
	if !held {
		t.Fatal("the membership was written and is not found by its key")
	}
}

// Gate SG-3, and BK-10 at the layer where it cannot be forgotten: the statements take no tenant,
// so a restore cannot write into another one even deliberately.
func TestAnImportCannotReachAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	id := freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "container", containerRow(id, authorA, "A's"), false)
		return err
	}); err != nil {
		t.Fatalf("importing: %v", err)
	}

	// B asks whether it has the row A just wrote, and clears its own containers. Neither may see
	// or touch A's.
	var heldByB bool
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		if heldByB, err = importRepo().Holds(ctx, "container", containerRow(id, authorB, "A's")); err != nil {
			return err
		}
		_, err = importRepo().Clear(ctx, "container")
		return err
	}); err != nil {
		t.Fatalf("clearing B: %v", err)
	}

	if heldByB {
		t.Error("tenant B was told it holds tenant A's row")
	}
	if name := containerName(ctx, t, tenantA, id); name != "A's" {
		t.Fatalf("tenant B's clear reached tenant A's container (%q)", name)
	}
}

// REPLACE_TENANT is made of this, and it may not be made of anything wider: a clear empties the
// table inside the tenant and nowhere else.
func TestClearEmptiesTheTenantAndAnswersHowMuchWent(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	for range 3 {
		id := freshID(t)
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			_, err := importRepo().Write(ctx, "container", containerRow(id, authorA, freshName(t)), false)
			return err
		}); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	var removed int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		removed, err = importRepo().Clear(ctx, "container")
		return err
	}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if removed < 3 {
		t.Fatalf("the clear removed %d rows, and three had just been written", removed)
	}

	var left int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM container WHERE tenant_id = $1`, tenantA.String()).Scan(&left); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if left != 0 {
		t.Fatalf("%d containers survived the clear", left)
	}
}

// The reference graph the DUPLICATE rule remaps through has to be the one the database actually
// has. A foreign key nobody declared is a duplicate that keeps pointing at the original; a
// declaration with no foreign key behind it is a remap of a column that means something else.
func TestEveryForeignKeyBetweenArchivedEntitiesIsDeclared(t *testing.T) {
	ctx := context.Background()

	rows, err := adminPool(ctx, t).Query(ctx, `
		SELECT c.conrelid::regclass::text, a.attname, c.confrelid::regclass::text
		FROM pg_constraint c
		JOIN unnest(c.conkey) WITH ORDINALITY AS k(attnum, ord) ON true
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
		WHERE c.contype = 'f'`)
	if err != nil {
		t.Fatalf("reading the foreign keys: %v", err)
	}
	defer rows.Close()

	declared := map[string]bool{}
	for _, entity := range archive.Entities() {
		for _, reference := range entity.References {
			declared[entity.Table+"."+reference.Field] = true
		}
	}

	for rows.Next() {
		var source, column, target string
		if err := rows.Scan(&source, &column, &target); err != nil {
			t.Fatalf("reading a foreign key: %v", err)
		}
		// The tenant is not a reference a restore remaps: it comes from the scope.
		if column == "tenant_id" {
			continue
		}
		if _, archived := archive.FindEntityByTable(source); !archived {
			continue
		}
		if _, archived := archive.FindEntityByTable(target); !archived {
			continue
		}
		if !declared[source+"."+column] {
			t.Errorf("%s.%s points at %s and is not declared in the archive's entity list",
				source, column, target)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the foreign keys: %v", err)
	}
}

// The round trip that matters most, on the table that makes it interesting: `work_item` carries a
// generated column, so the archive holds a value the insert may not be given. Exporting a real row
// and importing it back is the check that the two statements agree about which columns those are -
// and about how the row survives being turned into JSON and back.
func TestARealRowSurvivesTheRoundTripThroughTheArchivesShape(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := freshID(t)

	original := taskIn(tenantA, authorA, collection, id, "Weekly shop", "a0")
	original.Notes = "milk, bread, and something for Sunday"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, original)
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	// Out through the export, which is what an archive carries.
	var exportedRow map[string]any
	for _, row := range exported(ctx, t, tenantA, 100, "work_item", time.Time{}) {
		if row.ID == id.String() {
			exportedRow = row.Data
		}
	}
	if exportedRow == nil {
		t.Fatal("the item was not exported")
	}
	if _, generated := exportedRow["search_vector"]; !generated {
		t.Fatal("the export no longer carries the generated column, so this test proves nothing")
	}

	// The row is removed and put back from what the archive holds.
	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM work_item WHERE tenant_id = $1 AND id = $2`,
		tenantA.String(), id.String()); err != nil {
		t.Fatalf("removing the original: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "work_item", exportedRow, false)
		return err
	}); err != nil {
		t.Fatalf("importing the row back: %v", err)
	}

	// Every column, not the two the test happens to think of. An import statement that forgot a
	// column would drop it silently on every restore, and a check that named the columns it
	// expected would forget the same one.
	restored := exportedShape(ctx, t, tenantA, id)
	for column, before := range exportedRow {
		if column == "search_vector" {
			// Generated: the database rewrites it from the columns it is derived from, so a
			// difference here is the derivation working rather than a column being lost.
			continue
		}
		if after := restored[column]; !sameJSON(before, after) {
			t.Errorf("%s came back as %#v, want %#v", column, after, before)
		}
	}
}

// exportedShape reads one row back through the export, which is the same shape the archive carries.
func exportedShape(ctx context.Context, t *testing.T, tenant, id shared.ID) map[string]any {
	t.Helper()
	for _, row := range exported(ctx, t, tenant, 100, "work_item", time.Time{}) {
		if row.ID == id.String() {
			return row.Data
		}
	}
	t.Fatalf("%s is not in the export", id)
	return nil
}

// sameJSON compares two values as the archive carries them: everything has been through JSON, so
// comparing the encodings is comparing what a restore would actually write.
func sameJSON(a, b any) bool {
	first, err := json.Marshal(a)
	if err != nil {
		return false
	}
	second, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(first) == string(second)
}
