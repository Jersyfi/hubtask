// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	backupdomain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The statements a restore writes a tenant through (E-06): the three outcomes of a collision, the
// tenant the row lands in whatever the archive says, and the boundary it may not cross (SG-3).

func importRepo() postgres.BackupImportRepository { return postgres.NewBackupImportRepository() }

// containerRow is one container as the archive carries it: the row with `tenant_id` taken out.
func containerRow(id shared.ID, author shared.ID, name string) map[string]any {
	return map[string]any{
		// `a1` rather than `m`: the hub level is the whole tenant's, and the keys the ordering
		// service produces are a letter head declaring how many digits follow. A test that
		// committed a malformed one left it there for every other test in this database to rank
		// a new hub against, and they answered `ordering.key_malformed`.
		"id": id.String(), "type": "HUB", "name": name, "order_key": "a1",
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

// A row that collides with a living one by name rather than by identity - a second collection
// called what the hub already holds - is a conflict in the container's own words, never a
// database error the queue would retry into the same name (issue 766). The index is per tenant and
// parent, case- and accent-insensitive; two hubs at the top level meet it as two collections under
// one hub would.
func TestASecondCollectionUnderATakenNameIsAConflict(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	first, second := freshID(t), freshID(t)

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := importRepo().Write(ctx, "container", containerRow(first, authorA, "Errands"), false); err != nil {
			return err
		}
		_, err := importRepo().Write(ctx, "container", containerRow(second, authorA, "errands"), false)
		return err
	})
	if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).DetailCode != "containers.name_taken" {
		t.Fatalf("the second write answered %v, want the container's own conflict", err)
	}
	if got := shared.AsError(err).Params["name"]; got != "errands" {
		t.Errorf("the conflict names %q, want the colliding name", got)
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

// The freshness question a NEW_TENANT restore asks under real row level security (#206): a tenant
// sees exactly its own row in `tenant`, so a minted identifier answers "not held" and a living one
// answers "held" - which is what turns a run row naming a living tenant into a refusal rather than
// a write into somebody's workspace.
func TestAFreshTenantScopeHoldsNoTenantRow(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	minted := freshID(t)

	var mintedHeld, livingHeld bool
	if err := write(ctx, t, minted, func(ctx context.Context) error {
		var err error
		mintedHeld, err = importRepo().Holds(ctx, "tenant", map[string]any{"id": minted.String()})
		return err
	}); err != nil {
		t.Fatalf("asking the minted scope: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		livingHeld, err = importRepo().Holds(ctx, "tenant", map[string]any{"id": tenantA.String()})
		return err
	}); err != nil {
		t.Fatalf("asking the living scope: %v", err)
	}

	if mintedHeld {
		t.Error("a tenant that was never created answered as held")
	}
	if !livingHeld {
		t.Error("a living tenant answered as not held, so a NEW_TENANT restore would write into it")
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

// The rename against the index it exists for (#790).
//
// The applier's own tests answer with a store that has no unique index, so they prove the copy is
// renamed and not that the renamed copy lands. This writes both rows the way a DUPLICATE restore
// writes them: the living hub, then the copy under a minted identity - once carrying the name it
// used to carry, which is the bug, and once carrying the name the rule gives it.
func TestADuplicatedHubLandsUnderTheNameTheRuleGivesIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	living, run := freshID(t), freshID(t)
	name := freshName(t)

	// This one commits, and the hub level belongs to the whole tenant: a hub left behind is a
	// sibling every other test in this database ranks against.
	t.Cleanup(func() {
		done := context.Background()
		if _, err := adminPool(done, t).Exec(done,
			`DELETE FROM container WHERE tenant_id = $1 AND id = ANY($2)`,
			tenantA.String(), []string{living.String(),
				backupdomain.DuplicateID(run, "containers", living.String()).String()}); err != nil {
			t.Errorf("clearing up the hubs this test committed: %v", err)
		}
	})

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "container", containerRow(living, authorA, name), false)
		return err
	}); err != nil {
		t.Fatalf("seeding the living hub: %v", err)
	}
	copyID := backupdomain.DuplicateID(run, "containers", living.String())

	// What `mint` did on its own: a new identity and the name it was copied from.
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "container", containerRow(copyID, authorA, name), false)
		return err
	})
	if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).DetailCode != "containers.name_taken" {
		t.Fatalf("a copy under the living name was answered %v, and the whole of #790 is that the "+
			"index refuses it", err)
	}

	// What the rule gives it.
	renamed := backupdomain.DuplicatedName(run, created, name)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "container", containerRow(copyID, authorA, renamed), false)
		return err
	}); err != nil {
		t.Fatalf("the renamed copy was refused too, so the rule does not settle the index: %v", err)
	}
	if got := containerName(ctx, t, tenantA, copyID); got != renamed {
		t.Errorf("the copy came back as %q, want %q", got, renamed)
	}
	if got := containerName(ctx, t, tenantA, living); got != name {
		t.Errorf("the living hub came back as %q, want the name it had", got)
	}
}

// The uniquenesses a DUPLICATE has to settle have to be the ones the database actually insists
// on. `mint` gave a copy an identity and changed nothing else, so a duplicated collection arrived
// under the living one's name and landed nothing (#790) - and the identity was never the only
// unique index on those tables. A uniqueness nobody declared is that bug again, on a column
// somebody adds next year.
func TestEveryUniquenessADuplicateWouldMeetIsDeclared(t *testing.T) {
	ctx := context.Background()
	pool := adminPool(ctx, t)

	columns := map[string][]string{}
	columnRows, err := pool.Query(ctx, `
		SELECT table_name, column_name FROM information_schema.columns WHERE table_schema = 'public'`)
	if err != nil {
		t.Fatalf("reading the columns: %v", err)
	}
	for columnRows.Next() {
		var table, column string
		if err := columnRows.Scan(&table, &column); err != nil {
			columnRows.Close()
			t.Fatalf("reading a column: %v", err)
		}
		columns[table] = append(columns[table], column)
	}
	columnRows.Close()
	if err := columnRows.Err(); err != nil {
		t.Fatalf("reading the columns: %v", err)
	}

	// The plain columns of each unique index, and the expression of an index built on one -
	// `container_name_uq` is `lower(imm_unaccent(name))`, so the column it turns on appears
	// nowhere in `indkey`.
	rows, err := pool.Query(ctx, `
		SELECT c.relname,
		       ix.indexrelid::regclass::text,
		       coalesce(array_agg(a.attname) FILTER (WHERE a.attname IS NOT NULL), '{}'),
		       coalesce(pg_get_expr(ix.indexprs, ix.indrelid), '')
		FROM pg_index ix
		JOIN pg_class c ON c.oid = ix.indrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN unnest(ix.indkey) AS k(attnum) ON true
		LEFT JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = k.attnum
		WHERE ix.indisunique AND n.nspname = 'public'
		GROUP BY c.relname, ix.indexrelid, ix.indexprs, ix.indrelid`)
	if err != nil {
		t.Fatalf("reading the unique indexes: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var table, index, expression string
		var indexed []string
		if err := rows.Scan(&table, &index, &indexed, &expression); err != nil {
			t.Fatalf("reading a unique index: %v", err)
		}
		entity, archived := archive.FindEntityByTable(table)
		if !archived || !entity.Duplicable {
			continue
		}

		// What the index turns on: its own columns, plus the columns named inside its expression.
		touched := map[string]bool{}
		for _, column := range indexed {
			touched[column] = true
		}
		for _, column := range columns[table] {
			if regexp.MustCompile(`\b` + regexp.QuoteMeta(column) + `\b`).MatchString(expression) {
				touched[column] = true
			}
		}
		// The tenant comes from the scope a restore runs in rather than from the archive.
		delete(touched, "tenant_id")

		if coversAll(touched, entity.Keys) && changesIdentity(entity) {
			// The identity, possibly with something beside it - `activity_entry_pkey` is
			// (id, occurred_at), and `set_element_pkey` spans the set's name. Either mint gives
			// the row a new identity or the remap moves one of the columns the key is made of, so
			// the whole tuple is new.
			continue
		}
		if subsetOfReferences(touched, entity) {
			// Every column it turns on is remapped at the copy, so the copy's value differs from
			// the original's by construction - a recurrence rule's source item, a join row's ends.
			continue
		}

		declared := false
		for _, unique := range entity.Unique {
			if touched[unique.Field] {
				declared = true
				break
			}
		}
		if !declared {
			t.Errorf("%s is unique on %v and no rule says what a copy does with it - a DUPLICATE "+
				"of a row in %s would meet it and land nothing", index, keysOf(touched), table)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the unique indexes: %v", err)
	}
}

// Every column a duplicable entity declares as a name to change must be a column the table has,
// or the applier renames a key nothing reads.
func TestEveryDeclaredUniqueColumnExists(t *testing.T) {
	ctx := context.Background()

	for _, entity := range archive.Entities() {
		for _, unique := range entity.Unique {
			var exists bool
			if err := adminPool(ctx, t).QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2)`,
				entity.Table, unique.Field).Scan(&exists); err != nil {
				t.Fatalf("asking for %s.%s: %v", entity.Table, unique.Field, err)
			}
			if !exists {
				t.Errorf("%s declares %q as a column a copy has to change, and the table has no "+
					"such column", entity.Table, unique.Field)
			}
			if !entity.Duplicable {
				t.Errorf("%s declares %q and is not duplicable, so nothing would ever apply it",
					entity.Table, unique.Field)
			}

			for _, within := range unique.Within {
				if !slices.ContainsFunc(entity.References, func(r archive.Reference) bool {
					return r.Field == within
				}) {
					t.Errorf("%s scopes %q by %q, which is not a reference the archive remaps - "+
						"a column nothing remaps settles nothing",
						entity.Table, unique.Field, within)
					continue
				}
			}
		}
	}
}

func coversAll(touched map[string]bool, keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	for _, key := range keys {
		if !touched[key] {
			return false
		}
	}
	return true
}

// changesIdentity reports an entity whose key the copy cannot carry unchanged: mint draws it a new
// one, or the key is made of references the remap points at the other copies.
func changesIdentity(entity archive.Entity) bool {
	if entity.HasOwnIdentity() {
		return true
	}
	return slices.ContainsFunc(entity.Keys, func(key string) bool {
		return slices.ContainsFunc(entity.References, func(r archive.Reference) bool {
			return r.Field == key
		})
	})
}

func subsetOfReferences(touched map[string]bool, entity archive.Entity) bool {
	if len(touched) == 0 {
		return false
	}
	for column := range touched {
		if !slices.ContainsFunc(entity.References, func(r archive.Reference) bool {
			return r.Field == column
		}) {
			return false
		}
	}
	return true
}

func keysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
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
