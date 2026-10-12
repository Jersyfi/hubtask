// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	privacydomain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The exclusion over its first reason (UC-PRV-05 checks 7, 8, 9): an objection takes effect only
// where the workspace offers it and more than one person works here.
func TestAnObjectionKeepsContentOutOnlyWhereItTakesEffect(t *testing.T) {
	book := &consentBook{}
	book.withdraw(objectorID, providerClock{}.Now())
	for _, testCase := range []struct {
		name     string
		people   int
		position string
		kept     bool
	}{
		{"offered, two people", 2, "", true},
		{"offered, alone", 1, "", false},
		{"AI part of everybody's work", 2, "NOT_OFFERED", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			world := &sourceWorld{}
			world.workspace.Settings.AiPersonOptOut = testCase.position
			world.workspace.Settings.AiOptOutBasis = "WORKS_AGREEMENT"
			rules := &AiRules{Instance: world, Workspaces: sourceWorkspaces{world}, Offered: sourceOffered{world}, Choices: sourceChoices{world}}
			exclusion := Exclusion{Sources: []KeptOutSource{
				ObjectionSource{Rules: rules, People: headcount(testCase.people), Consents: book},
			}}
			kept, err := exclusion.Read(t.Context(), providerActor().TenantID)
			if err != nil {
				t.Fatalf("reading: %v", err)
			}
			if kept.Has(objectorID) != testCase.kept || kept.Has(colleagueID) {
				t.Errorf("kept out: objector %v, colleague %v", kept.Has(objectorID), kept.Has(colleagueID))
			}
			if testCase.kept && kept.Scrub("ask Kim Keeper") != "ask "+domain.Placeholder {
				t.Errorf("the name was not replaced: %q", kept.Scrub("ask Kim Keeper"))
			}
		})
	}
}

type restrictedBook []privacydomain.RestrictedPerson

func (b restrictedBook) RestrictedPeople(context.Context) ([]privacydomain.RestrictedPerson, error) {
	return b, nil
}

// The second reason (UC-PRV-03 check 10, UC-PRV-04 check 5): a restriction keeps a person out
// where an objection would not take effect - alone, or where AI is part of everybody's work - and
// wins over an objection of the same person.
func TestARestrictionKeepsContentOutWhereverItStands(t *testing.T) {
	book := &consentBook{}
	book.withdraw(objectorID, providerClock{}.Now())
	for _, testCase := range []struct {
		name     string
		people   int
		position string
	}{
		{"offered, two people", 2, ""},
		{"alone", 1, ""},
		{"AI part of everybody's work", 2, "NOT_OFFERED"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			world := &sourceWorld{}
			world.workspace.Settings.AiPersonOptOut = testCase.position
			world.workspace.Settings.AiOptOutBasis = "WORKS_AGREEMENT"
			rules := &AiRules{Instance: world, Workspaces: sourceWorkspaces{world}, Offered: sourceOffered{world}, Choices: sourceChoices{world}}
			exclusion := Exclusion{Sources: []KeptOutSource{
				RestrictionSource{Restrictions: restrictedBook{{AccountID: objectorID, DisplayName: "Kim Keeper"}}},
				ObjectionSource{Rules: rules, People: headcount(testCase.people), Consents: book},
			}}
			kept, err := exclusion.Read(t.Context(), providerActor().TenantID)
			if err != nil {
				t.Fatalf("reading: %v", err)
			}
			if !kept.Has(objectorID) || kept.ReasonOf(objectorID) != domain.KeptOutRestricted || kept.Has(colleagueID) {
				t.Errorf("kept out: %v (%q), colleague %v", kept.Has(objectorID), kept.ReasonOf(objectorID), kept.Has(colleagueID))
			}
			if kept.Scrub("ask Kim Keeper") != "ask "+domain.Placeholder {
				t.Errorf("the name was not replaced: %q", kept.Scrub("ask Kim Keeper"))
			}
		})
	}
	// Unwired, it keeps nobody out rather than failing.
	if people, err := (RestrictionSource{}).KeptOut(t.Context(), ""); err != nil || len(people) != 0 {
		t.Errorf("an unwired source answered %v, %v", people, err)
	}
}

type failingSource struct{}

func (failingSource) KeptOut(context.Context, shared.ID) ([]domain.KeptPerson, error) {
	return nil, errors.New("the query failed")
}

// A source that cannot answer fails the read: content must not reach a model because a query
// could not say whose it is.
func TestAFailingSourceFailsTheRead(t *testing.T) {
	if _, err := (Exclusion{Sources: []KeptOutSource{failingSource{}}}).Read(t.Context(), ""); err == nil {
		t.Error("a failing source answered an exclusion")
	}
	// An unwired source keeps nobody out rather than failing.
	kept, err := (Exclusion{Sources: []KeptOutSource{ObjectionSource{}}}).Read(t.Context(), "")
	if err != nil || !kept.Empty() {
		t.Errorf("an unwired source answered %v, %v", kept, err)
	}
}
