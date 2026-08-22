// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"testing"
	"time"

	// The zone database, embedded so that this test proves the same thing on a machine that has no
	// system tzdata - which is every container this project ships (i18n-l10n.md §2).
	_ "time/tzdata"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var actor = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")

func berlin(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("the zone database is unreadable: %v", err)
	}
	return location
}

func TestParsePlaceholderRefuses(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value string
		code  string
	}{
		{"an anchor nobody defined", "created_at", "@tomorrow", "query.placeholder_unknown"},
		{"a moment on an identifier", "created_by", "@today", "query.placeholder_not_applicable"},
		{"an account on a moment", "created_at", "@me", "query.placeholder_not_applicable"},
		{"an offset on @me", "created_by", "@me+P1D", "query.placeholder_offset_not_allowed"},
		{"an offset that is not a duration", "created_at", "@today+3D", "query.offset_malformed"},
		{"a duration with no components", "created_at", "@today+P", "query.offset_malformed"},
		{"a duration of nothing", "created_at", "@today+PT", "query.offset_malformed"},
		{"a designator that is not one", "created_at", "@today+P1X", "query.offset_malformed"},
		{"a component with no amount", "created_at", "@today+PD", "query.offset_malformed"},
		{"a fraction", "created_at", "@today+P1.5D", "query.offset_malformed"},
		{"a shift beyond any calendar", "created_at", "@today+P99999D", "query.offset_malformed"},
		{"an empty time half", "created_at", "@today+P1DT", "query.offset_malformed"},
		{"a minute before the T", "created_at", "@today+PT1D", "query.offset_malformed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseFilter(leaf(test.field, "EQ", test.value), filterPath)
			if code := detailOf(t, err); code != test.code {
				t.Errorf("detail code = %q, want %q", code, test.code)
			}
		})
	}
}

func TestParseOffset(t *testing.T) {
	tests := []struct {
		text string
		want Offset
	}{
		{"+P3D", Offset{Days: 3}},
		{"P3D", Offset{Days: 3}},
		{"-P1W", Offset{Days: 7, Negative: true}},
		{"P1Y", Offset{Years: 1}},
		{"P1M15D", Offset{Months: 1, Days: 15}},
		{"P1DT12H", Offset{Days: 1, Clock: 12 * time.Hour}},
		{"PT30M", Offset{Clock: 30 * time.Minute}},
		{"+PT1H30M", Offset{Clock: 90 * time.Minute}},
		{"PT45S", Offset{Clock: 45 * time.Second}},
		{"P2W3DT4H", Offset{Days: 17, Clock: 4 * time.Hour}},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			got, err := parseOffset(test.text, "/filter/value")
			if err != nil {
				t.Fatalf("unexpected refusal: %v", err)
			}
			if got != test.want {
				t.Errorf("read as %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestResolveTheAnchors(t *testing.T) {
	location := berlin(t)
	// A Wednesday, so that the week anchors have something to move.
	now := time.Date(2026, 8, 19, 14, 30, 0, 0, location)
	at := Resolution{Now: now, Location: location, ActorID: actor}

	tests := []struct {
		placeholder string
		want        time.Time
	}{
		{"@now", now},
		{"@today", time.Date(2026, 8, 19, 0, 0, 0, 0, location)},
		{"@end_of_day", time.Date(2026, 8, 20, 0, 0, 0, 0, location).Add(-time.Microsecond)},
		{"@start_of_week", time.Date(2026, 8, 17, 0, 0, 0, 0, location)},
		{"@end_of_week", time.Date(2026, 8, 24, 0, 0, 0, 0, location).Add(-time.Microsecond)},
		{"@start_of_month", time.Date(2026, 8, 1, 0, 0, 0, 0, location)},
		{"@end_of_month", time.Date(2026, 9, 1, 0, 0, 0, 0, location).Add(-time.Microsecond)},
		{"@today+P3D", time.Date(2026, 8, 22, 0, 0, 0, 0, location)},
		{"@start_of_week-P1W", time.Date(2026, 8, 10, 0, 0, 0, 0, location)},
		{"@end_of_month+P1M", time.Date(2026, 10, 1, 0, 0, 0, 0, location).Add(-time.Microsecond)},
		{"@end_of_month-P1M", time.Date(2026, 8, 1, 0, 0, 0, 0, location).Add(-time.Microsecond)},
		{"@now+PT90M", now.Add(90 * time.Minute)},
	}

	for _, test := range tests {
		t.Run(test.placeholder, func(t *testing.T) {
			got := resolveValue(t, "created_at", test.placeholder, at)
			if !got.Time.Equal(test.want) {
				t.Errorf("resolved to %s, want %s", got.Time, test.want.UTC())
			}
			if got.Kind != KindTimestamp {
				t.Errorf("resolved to a %s", got.Kind)
			}
		})
	}
}

// The reason the calendar parts and the clock parts are applied separately: a day is not always
// twenty-four hours, and somebody filtering by date means the date.
func TestADayIsADayAcrossADaylightSavingBoundary(t *testing.T) {
	location := berlin(t)
	// Central European Summer Time begins in the small hours of Sunday, 29 March 2026.
	at := Resolution{
		Now:      time.Date(2026, 3, 28, 10, 0, 0, 0, location),
		Location: location,
		ActorID:  actor,
	}

	day := resolveValue(t, "created_at", "@today+P2D", at)
	if want := time.Date(2026, 3, 30, 0, 0, 0, 0, location); !day.Time.Equal(want) {
		t.Errorf("two days on is %s, want midnight: %s", day.Time, want.UTC())
	}

	clock := resolveValue(t, "created_at", "@today+PT48H", at)
	if want := time.Date(2026, 3, 30, 1, 0, 0, 0, location); !clock.Time.Equal(want) {
		t.Errorf("forty-eight hours on is %s, want %s", clock.Time, want.UTC())
	}
}

func TestResolveTheActor(t *testing.T) {
	at := Resolution{Now: time.Now(), ActorID: actor}

	got := resolveValue(t, "created_by", "@me", at)
	if got.ID != actor || got.Kind != KindID {
		t.Errorf("@me resolved to %+v", got)
	}

	node, err := ParseFilter(leaf("created_by", "EQ", "@me"), filterPath)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	if _, err := node.Resolve(Resolution{Now: at.Now}, filterPath); detailOf(t, err) != "query.placeholder_needs_actor" {
		t.Errorf("without an actor @me has no answer, got %v", err)
	}
}

// "my items" is the query api-guidelines.md §3 writes out in its own example, and half of it is a
// set: `members CONTAINS @me`. The placeholder stands for one identifier either way - whether the
// field holds one of them or several is the operator's business rather than the value's.
func TestTheActorResolvesOnASetOfIdentifiersToo(t *testing.T) {
	at := Resolution{Now: time.Now(), ActorID: actor}

	// Not through resolveValue: that one asks with EQ, and a set does not answer EQ - which is
	// exactly the pairing this test is about.
	node, err := ParseFilter(leaf("members", "CONTAINS", "@me"), filterPath)
	if err != nil {
		t.Fatalf("@me on a set was refused: %v", err)
	}
	resolved, err := node.Resolve(at, filterPath)
	if err != nil {
		t.Fatalf("@me on a set did not resolve: %v", err)
	}

	got := resolved.Values[0]
	if got.ID != actor || got.Kind != KindID {
		t.Errorf("@me on a set resolved to %+v", got)
	}
}

// A moment on a set of identifiers is as wrong as a moment on one, and is refused by the same line.
func TestAMomentOnASetOfIdentifiersIsRefused(t *testing.T) {
	node, err := ParseFilter(leaf("members", "CONTAINS", "@today"), filterPath)
	if err == nil {
		t.Fatalf("@today was accepted on a set: %+v", node)
	}
	if detail := detailOf(t, err); detail != "query.placeholder_not_applicable" {
		t.Errorf("detail %q", detail)
	}
}

// Without a zone the server has to pick one, and it must not be the machine's: two replicas of one
// deployment would otherwise disagree about when today began.
func TestResolveFallsBackToUTC(t *testing.T) {
	at := Resolution{Now: time.Date(2026, 8, 19, 23, 30, 0, 0, time.UTC), ActorID: actor}

	got := resolveValue(t, "created_at", "@today", at)
	if want := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC); !got.Time.Equal(want) {
		t.Errorf("resolved to %s, want %s", got.Time, want)
	}
}

// Resolving rebuilds the tree rather than writing into it: a saved view holds the parsed filter,
// and one resolve must not turn "this week" into the week it was first opened in.
func TestResolveLeavesTheParsedFilterAlone(t *testing.T) {
	node, err := ParseFilter(map[string]any{"op": "AND", "nodes": []any{
		leaf("created_at", "GTE", "@today"),
		leaf("created_by", "EQ", "@me"),
	}}, filterPath)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}

	if _, err := node.Resolve(Resolution{Now: time.Now(), ActorID: actor}, filterPath); err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	for index, child := range node.Nodes {
		if !child.Values[0].IsPlaceholder() {
			t.Errorf("node %d lost its placeholder to the resolve", index)
		}
	}
}

func resolveValue(t *testing.T, field, placeholder string, at Resolution) Value {
	t.Helper()

	node, err := ParseFilter(leaf(field, "EQ", placeholder), filterPath)
	if err != nil {
		t.Fatalf("%s was refused: %v", placeholder, err)
	}
	resolved, err := node.Resolve(at, filterPath)
	if err != nil {
		t.Fatalf("%s did not resolve: %v", placeholder, err)
	}
	if resolved.Values[0].IsPlaceholder() {
		t.Fatalf("%s stayed a placeholder", placeholder)
	}
	return resolved.Values[0]
}
