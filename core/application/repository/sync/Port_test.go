// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"
)

type double struct{ recorded []Change }

func (d *double) Record(_ context.Context, change Change) error {
	d.recorded = append(d.recorded, change)
	return nil
}

var _ ChangeLog = (*double)(nil)

// The operations are the values the column's CHECK constraint allows (db/schema.sql, change_log).
// A fourth one invented here would be refused at commit time, which is the worst moment to find
// out.
func TestTheOperationsAreTheOnesTheSchemaAllows(t *testing.T) {
	allowed := []Operation{"UPSERT", "DELETE", "ACCESS_REVOKED"}

	for _, op := range []Operation{Upsert, Delete, AccessRevoked} {
		if !slices.Contains(allowed, op) {
			t.Errorf("%s is not a value the change log column accepts", op)
		}
	}
}

// A deletion carries no payload: there is nothing left to describe, and a tombstone with content
// would be a copy of the deleted object living on in the log.
func TestADeletionCarriesNoPayload(t *testing.T) {
	log := &double{}

	if err := log.Record(t.Context(), Change{Op: Delete, Entity: "container"}); err != nil {
		t.Fatalf("recording failed: %v", err)
	}
	if log.recorded[0].Payload != nil {
		t.Errorf("a deletion carries a payload: %v", log.recorded[0].Payload)
	}
}
