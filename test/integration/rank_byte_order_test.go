// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// errRollBack ends a transaction whose writes must not be kept.
var errRollBack = errors.New("roll back")

// The last rank at a level is the highest key in byte order, whatever the column's collation.
// "Zz" is the integer just below "a0" in the rank scheme, and a linguistic collation puts it
// above: on a database created under a glibc locale, a new container or item read "Zz" as the
// last rank and landed between the two.
//
// The integration image sorts its default collation like "C", so the real tables cannot show the
// defect. Each case shadows the table with a temporary one of the same name - pg_temp comes first
// on the search path - whose order_key carries the ICU collation of migration 0080, and runs the
// repository against it. The transaction is rolled back and the table is dropped with it, so
// nothing reaches the shared schema.
func TestTheLastRankIsReadInByteOrder(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	var linguistic bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT 'a0' COLLATE hubtask_name < 'Zz' COLLATE hubtask_name`).Scan(&linguistic); err != nil {
		t.Fatalf("comparing under hubtask_name: %v", err)
	}
	if !linguistic {
		t.Fatal(`hubtask_name sorts "a0" after "Zz" - this test could not tell byte order from it`)
	}

	cases := []struct {
		name   string
		shadow string
		insert string
		last   func(context.Context) (string, error)
	}{
		{
			name:   "container",
			shadow: `CREATE TEMPORARY TABLE container (parent_id uuid, order_key text COLLATE hubtask_name) ON COMMIT DROP`,
			insert: `INSERT INTO container (parent_id, order_key) VALUES (NULL, 'Zz'), (NULL, 'a0')`,
			last: func(ctx context.Context) (string, error) {
				return containerRepo().LastOrderKey(ctx, "")
			},
		},
		{
			name: "work item",
			shadow: `CREATE TEMPORARY TABLE work_item
			           (collection_id uuid, parent_id uuid, order_key text COLLATE hubtask_name) ON COMMIT DROP`,
			insert: `INSERT INTO work_item (collection_id, parent_id, order_key)
			         VALUES ('0191a1b2-0000-7000-8000-00000000c011', NULL, 'Zz'),
			                ('0191a1b2-0000-7000-8000-00000000c011', NULL, 'a0')`,
			last: func(ctx context.Context) (string, error) {
				return itemRepo().LastOrderKey(ctx, "0191a1b2-0000-7000-8000-00000000c011", "")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var last string
			err := write(ctx, t, tenantA, func(ctx context.Context) error {
				tx, err := postgres.FromContext(ctx)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, tc.shadow); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, tc.insert); err != nil {
					return err
				}
				if last, err = tc.last(ctx); err != nil {
					return err
				}
				return errRollBack
			})
			if !errors.Is(err, errRollBack) {
				t.Fatalf("reading the last rank: %v", err)
			}
			if last != "a0" {
				t.Errorf("the last rank is %q, want \"a0\" - the key highest in byte order", last)
			}
		})
	}
}
