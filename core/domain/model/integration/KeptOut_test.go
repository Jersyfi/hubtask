// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration_test

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	kimID = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	liID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
)

func keptKim() domain.KeptOut {
	return domain.NewKeptOut([]domain.KeptPerson{
		{AccountID: kimID, Reason: domain.KeptOutObjected, Names: []string{"Kim Keeper", "kimk", "Kim.Keeper@Example.org"}},
		{AccountID: liID, Reason: domain.KeptOutObjected, Names: []string{"Li Ng", "", ""}},
	})
}

// A kept-out person's name becomes the placeholder wherever it is written - whole, in parts, as a
// sign-in name or an address - and nothing that merely contains it does.
func TestScrubReplacesEverySpellingAndNothingElse(t *testing.T) {
	kept := keptKim()
	for _, testCase := range []struct{ in, want string }{
		{"Ask Kim Keeper about it", "Ask [person] about it"},
		{"kim said yes, KEEPER agreed", "[person] said yes, [person] agreed"},
		{"mail kim.keeper@example.org today", "mail [person] today"},
		{"@kimk please", "@[person] please"},
		{"Kimberley and Kimkat stay", "Kimberley and Kimkat stay"},
		{"Kim's plan", "[person]'s plan"},
		{"Li and Ng are too short to stand alone", "Li and Ng are too short to stand alone"},
		{"Li Ng wrote", "[person] wrote"},
		{"Übermorgen mit Kim", "Übermorgen mit [person]"},
		{"", ""},
	} {
		if got := kept.Scrub(testCase.in); got != testCase.want {
			t.Errorf("Scrub(%q) = %q, want %q", testCase.in, got, testCase.want)
		}
	}
}

func TestTheExclusionKnowsItsPeopleAndTheirAddress(t *testing.T) {
	kept := keptKim()
	if !kept.Has(kimID) || kept.Has(shared.MustParseID("0192f000-0000-7000-8000-0000000000b9")) || kept.Has("") {
		t.Error("Has answers wrongly")
	}
	if !kept.HasAddress(" kim.keeper@example.org ") || kept.HasAddress("ada@example.org") {
		t.Error("HasAddress answers wrongly")
	}
	if kept.ReasonOf(kimID) != domain.KeptOutObjected {
		t.Errorf("reason %q", kept.ReasonOf(kimID))
	}
	var nobody domain.KeptOut
	if !nobody.Empty() || nobody.Has(kimID) || nobody.Scrub("Kim Keeper") != "Kim Keeper" {
		t.Error("the zero exclusion keeps somebody out")
	}
}

// One answer for nothing to work with, whatever the reason; the objector's own is another.
func TestTheRefusalsAreFixedCodes(t *testing.T) {
	if got := shared.AsError(domain.NothingToWorkWith()); got == nil || got.DetailCode != "ai.nothing_to_work_with" ||
		got.Category != shared.CategoryValidation || len(got.Params) != 0 {
		t.Errorf("nothing to work with: %v", got)
	}
	if got := shared.AsError(domain.KeptOutOfAi()); got == nil || got.DetailCode != "ai.kept_out" || got.Category != shared.CategoryForbidden {
		t.Errorf("kept out: %v", got)
	}
}

// A restriction keeps out what is about the person as well as what they wrote; an objection only
// what they wrote. Each asker is told their own reason.
func TestARestrictionKeepsOutWhatIsAssignedAndSaysSo(t *testing.T) {
	restrictedID := shared.MustParseID("0192f000-0000-7000-8000-0000000000b3")
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000b9")
	kept := domain.NewKeptOut([]domain.KeptPerson{
		{AccountID: restrictedID, Reason: domain.KeptOutRestricted, Names: []string{"Rae Rest", "", ""}},
		// Named twice: the first reason wins, which is why the restriction's source comes first.
		{AccountID: restrictedID, Reason: domain.KeptOutObjected},
		{AccountID: kimID, Reason: domain.KeptOutObjected, Names: []string{"Kim Keeper"}},
	})
	for _, testCase := range []struct {
		name               string
		createdBy, assigns shared.ID
		want               bool
	}{
		{"created by the restricted person", restrictedID, "", true},
		{"assigned to the restricted person", other, restrictedID, true},
		{"created by the objector", kimID, other, true},
		{"assigned to the objector", other, kimID, false},
		{"nobody's", other, "", false},
	} {
		if got := kept.Excludes(testCase.createdBy, testCase.assigns); got != testCase.want {
			t.Errorf("%s: %v, want %v", testCase.name, got, testCase.want)
		}
	}
	if got := kept.Restricted(); len(got) != 1 || got[0] != restrictedID {
		t.Errorf("restricted %v", got)
	}
	if got := shared.AsError(kept.Refusal(restrictedID)); got == nil || got.DetailCode != "ai.processing_restricted" ||
		got.Category != shared.CategoryForbidden {
		t.Errorf("the restricted person is told %v", got)
	}
	if got := shared.AsError(kept.Refusal(kimID)); got == nil || got.DetailCode != "ai.kept_out" {
		t.Errorf("the objector is told %v", got)
	}
	if kept.Refusal(other) != nil {
		t.Error("somebody else is refused")
	}
}
