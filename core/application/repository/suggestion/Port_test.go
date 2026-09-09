// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion_test

import (
	"reflect"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
)

// The two interfaces are separate on purpose, and this is the pin: what the *retention engine* can
// do to a suggestion is count what is due and remove a batch of it, and nothing else. A sweep that
// could read one, write one or decide one would be a second path into the record, reachable from a
// job with no actor behind it.
func TestTheRetentionSliceCanOnlyCountAndRemove(t *testing.T) {
	expiring := reflect.TypeOf((*repository.Expiring)(nil)).Elem()

	want := map[string]bool{"CountExpired": true, "DeleteExpired": true}
	if expiring.NumMethod() != len(want) {
		t.Fatalf("the retention slice has %d methods, want exactly %d",
			expiring.NumMethod(), len(want))
	}
	for i := range expiring.NumMethod() {
		if name := expiring.Method(i).Name; !want[name] {
			t.Errorf("the retention slice can %s; it should only count and remove", name)
		}
	}
}

// And the store's own surface, which is the other half of the same statement: recording, reading
// and deciding are the whole of what the application layer asks of it.
func TestTheStoreSurfaceIsTheFourThingsTheUseCasesNeed(t *testing.T) {
	store := reflect.TypeOf((*repository.Suggestions)(nil)).Elem()

	want := map[string]bool{"Record": true, "Find": true, "List": true, "Decide": true}
	if store.NumMethod() != len(want) {
		t.Fatalf("the store has %d methods, want %d", store.NumMethod(), len(want))
	}
	for i := range store.NumMethod() {
		if name := store.Method(i).Name; !want[name] {
			t.Errorf("the store can %s, which no use case asks for", name)
		}
	}
}

// A query with no status is "what is still standing", which is what a reader almost always wants -
// an inbox of proposals rather than a history of them. The zero value has to express that, so the
// field is a plain value and its empty state is meaningful.
func TestAQueryWithNoStatusIsTheStandingOnes(t *testing.T) {
	var query repository.Query

	if query.Status != "" {
		t.Errorf("the zero query narrows to %q", query.Status)
	}
	if reflect.TypeOf(query.Status).Kind() != reflect.String {
		t.Error("the status is not a plain value, so \"unset\" and \"any\" cannot be one state")
	}
}
