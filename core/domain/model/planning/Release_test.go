// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func release(t *testing.T, fields ReleaseFields) (Release, error) {
	t.Helper()
	return NewRelease(NewReleaseInput{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000a1"), TenantID: shared.ID("01936f2a-7c1e-7000-8000-0000000000a2"),
		HubID: shared.ID("01936f2a-7c1e-7000-8000-0000000000a3"), Fields: fields, Now: time.Unix(0, 0),
	})
}

// A release is checked as a person names it (UC-PLN-08 check 1).
func TestAReleaseIsCheckedAsItIsPlanned(t *testing.T) {
	day := DayFrom(2026, 11, 30)
	for _, tc := range []struct {
		name   string
		fields ReleaseFields
		code   string
	}{
		{"whole", ReleaseFields{Name: " 2.4 ", Description: "Checkout", ReleaseOn: day}, ""},
		{"no name", ReleaseFields{Name: " ", ReleaseOn: day}, "releases.name_required"},
		{"two lines", ReleaseFields{Name: "a\nb", ReleaseOn: day}, "releases.name_invalid"},
		{"too long", ReleaseFields{Name: "a", Description: strings.Repeat("x", MaxReleaseDescriptionLength+1), ReleaseOn: day}, "releases.description_too_long"},
		{"no day", ReleaseFields{Name: "a"}, "releases.date_required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := release(t, tc.fields)
			if tc.code == "" {
				if err != nil || r.Name != "2.4" || r.State != ReleasePlanned {
					t.Fatalf("%+v %v", r, err)
				}
				return
			}
			if !errors.Is(err, shared.ErrValidation) || !strings.Contains(err.Error(), tc.code) {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
		})
	}
}

// A released release is history: it changes no more and is released once (UC-PLN-08 check 3).
func TestAReleasedReleaseChangesNoMore(t *testing.T) {
	r, err := release(t, ReleaseFields{Name: "2.4", ReleaseOn: DayFrom(2026, 11, 30)})
	if err != nil {
		t.Fatal(err)
	}
	page := shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")
	if err := r.Release(page, time.Unix(10, 0)); err != nil || r.State != ReleaseReleased || r.NotesPageID != page || r.ReleasedAt == nil {
		t.Fatalf("%+v %v", r, err)
	}
	for _, err := range []error{
		r.Release("", time.Unix(20, 0)),
		r.Change(ReleaseFields{Name: "2.5", ReleaseOn: DayFrom(2026, 12, 1)}, time.Unix(20, 0), nil),
		r.Changeable(),
	} {
		if !errors.Is(err, shared.ErrConflict) {
			t.Errorf("want a conflict, got %v", err)
		}
	}
}

// The notes are the description and the done entries under their type's name, each a mention -
// words the workspace wrote, none of the backend's (UC-PLN-08 check 4, rule 8).
func TestTheNotesMentionTheDoneEntriesByType(t *testing.T) {
	r, _ := release(t, ReleaseFields{Name: "2.4", Description: "The checkout release.", ReleaseOn: DayFrom(2026, 11, 30)})
	notes := Notes(r, []NoteEntry{
		{ID: "01936f2a-7c1e-7000-8000-000000000001", Title: "Pay [by] card", TypeName: "Story"},
		{ID: "01936f2a-7c1e-7000-8000-000000000002", Title: "Crash on save", TypeName: "Bug"},
		{ID: "01936f2a-7c1e-7000-8000-000000000003", Title: "Docs", TypeName: ""},
	})
	want := "The checkout release.\n\n" +
		"- [Docs](hubtask:entry/01936f2a-7c1e-7000-8000-000000000003)\n\n" +
		"## Bug\n\n- [Crash on save](hubtask:entry/01936f2a-7c1e-7000-8000-000000000002)\n\n" +
		"## Story\n\n- [Pay \\[by\\] card](hubtask:entry/01936f2a-7c1e-7000-8000-000000000001)\n"
	if notes != want {
		t.Fatalf("notes:\n%q\nwant:\n%q", notes, want)
	}
}
