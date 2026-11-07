// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import "testing"

func TestSampleKindsAndKeys(t *testing.T) {
	for _, tc := range []struct {
		kind           SampleKind
		known, trashed bool
	}{
		{SampleContainer, true, true}, {SampleEntry, true, true}, {SamplePage, true, true},
		{SampleWhiteboard, true, true}, {SampleGoal, true, false}, {SamplePortfolio, true, false},
		{SampleDashboard, true, false}, {SampleTimeRecord, true, false}, {"CI", false, false}, {"", false, false},
	} {
		if tc.kind.Known() != tc.known || tc.kind.Trashed() != tc.trashed {
			t.Errorf("%q: known %v trashed %v", tc.kind, tc.kind.Known(), tc.kind.Trashed())
		}
	}
	for key, valid := range map[string]bool{
		"hub.scrum": true, "entry.scrum.login_wp1": true, "a": true, "": false, "Hub": false,
		"1hub": false, "hub-scrum": false, "hub scrum": false,
	} {
		if ValidSampleKey(key) != valid {
			t.Errorf("%q: valid %v", key, !valid)
		}
	}
}

// What has no trash goes while its hub still stands; containers before what lies in them.
func TestRemovalOrderPutsTheUntrashedFirstAndTheContainersBeforeTheirContent(t *testing.T) {
	marks := []SampleMark{
		{Key: "entry.b", Kind: SampleEntry}, {Key: "hub.a", Kind: SampleContainer},
		{Key: "goal.a", Kind: SampleGoal}, {Key: "entry.a", Kind: SampleEntry},
		{Key: "time.a", Kind: SampleTimeRecord}, {Key: "page.a", Kind: SamplePage}, {Key: "ci.a", Kind: "CI"},
	}
	got := RemovalOrder(marks)
	want := []string{"time.a", "goal.a", "hub.a", "page.a", "entry.a", "entry.b"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, key := range want {
		if got[i].Key != key {
			t.Errorf("%d: %s, want %s", i, got[i].Key, key)
		}
	}
}

// A child goal keyed under its parent goes before it: deleting the parent first would move the
// child's version and keep it.
func TestRemovalOrderPutsAChildBeforeItsParent(t *testing.T) {
	got := RemovalOrder([]SampleMark{{Key: "goal.company", Kind: SampleGoal}, {Key: "goal.company.desk", Kind: SampleGoal},
		{Key: "goal.build", Kind: SampleGoal}, {Key: "hub.a", Kind: SampleContainer}, {Key: "hub.a.b", Kind: SampleContainer}})
	want := []string{"goal.company.desk", "goal.build", "goal.company", "hub.a", "hub.a.b"}
	for i, key := range want {
		if got[i].Key != key {
			t.Errorf("%d: %s, want %s", i, got[i].Key, key)
		}
	}
}
