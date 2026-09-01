// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/repository/admin"
)

// The footprint is the evidence's vocabulary: one field per store the §5 table names, counted
// before the fall and recorded in the entry that outlives them. The pin is the list itself - a
// store added to the purge without a count is a deletion the evidence cannot attest, and this is
// the test that turns red first.
func TestTheFootprintNamesEveryStoreTheEvidenceRecords(t *testing.T) {
	want := []string{
		"AuditEntries", "Containers", "Items", "MediaBytes", "MediaObjects", "OutboxEvents",
	}

	shape := reflect.TypeOf(admin.Footprint{})
	got := make([]string, 0, shape.NumField())
	for i := 0; i < shape.NumField(); i++ {
		field := shape.Field(i)
		if field.Type.Kind() != reflect.Int64 {
			t.Errorf("%s is %s - a count is an int64, whatever the store", field.Name, field.Type)
		}
		got = append(got, field.Name)
	}
	sort.Strings(got)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("the footprint counts %v, the evidence records %v - change both together, and "+
			"the purge and audit.md §6 with them", got, want)
	}
}
