// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const (
	groupID  = shared.ID("01936f2a-7c1e-7000-8000-0000000000g1")
	tenantID = shared.ID("01936f2a-7c1e-7000-8000-00000000000a")
)

func validInput() NewGroupInput {
	return NewGroupInput{ID: groupID, TenantID: tenantID, Name: "Design"}
}

func TestAGroupIsCreatedWithItsInvariantsChecked(t *testing.T) {
	group, err := NewGroup(validInput())
	if err != nil {
		t.Fatalf("creating the group: %v", err)
	}
	if group.Name != "Design" {
		t.Errorf("name %q", group.Name)
	}
	if group.Version != 1 {
		t.Errorf("version %d, want 1 - the optimistic lock starts somewhere", group.Version)
	}
}

// The name is what a person picks the group out of a list by, so what is stored is what they would
// recognise: trimmed, on one line, and never only whitespace.
func TestTheNameIsCheckedAndNormalised(t *testing.T) {
	cases := []struct {
		name  string
		given string
		want  string
		code  string
	}{
		{"surrounding space is not part of the name", "  Design  ", "Design", ""},
		{"whitespace alone is empty", "   ", "", "groups.name_empty"},
		{"empty is empty", "", "", "groups.name_empty"},
		{"a name that spans lines breaks every list", "Design\nTeam", "", "groups.name_malformed"},
		{"longer than the column allows", strings.Repeat("a", maxGroupName+1), "", "groups.name_too_long"},
		{"exactly the maximum still fits", strings.Repeat("a", maxGroupName), strings.Repeat("a", maxGroupName), ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := validInput()
			in.Name = c.given
			group, err := NewGroup(in)

			if c.code == "" {
				if err != nil {
					t.Fatalf("error %v, want the name accepted", err)
				}
				if group.Name != c.want {
					t.Errorf("name %q, want %q", group.Name, c.want)
				}
				return
			}
			if err == nil || shared.AsError(err).DetailCode != c.code {
				t.Fatalf("error %v, want %s", err, c.code)
			}
			if !errors.Is(err, shared.ErrValidation) {
				t.Errorf("category %v, want a validation error - the caller can fix this", err)
			}
		})
	}
}

func TestADescriptionIsBounded(t *testing.T) {
	in := validInput()
	in.Description = strings.Repeat("d", maxGroupDescription+1)

	if _, err := NewGroup(in); err == nil ||
		shared.AsError(err).DetailCode != "groups.description_too_long" {
		t.Fatalf("error %v, want the description refused", err)
	}
}

// An identifier the caller failed to supply is a programming error, not a bad request: the
// generator produces it, and the tenant comes from authentication.
func TestAGroupWithoutItsIdentityIsAnInternalError(t *testing.T) {
	for _, missing := range []string{"id", "tenant"} {
		t.Run(missing, func(t *testing.T) {
			in := validInput()
			if missing == "id" {
				in.ID = ""
			} else {
				in.TenantID = ""
			}

			_, err := NewGroup(in)
			if err == nil || shared.AsError(err).DetailCode != "groups.identity_incomplete" {
				t.Fatalf("error %v, want the group refused", err)
			}
			if !errors.Is(err, shared.ErrInternal) {
				t.Errorf("category %v, want internal - nobody outside can fix this", err)
			}
		})
	}
}

// Rename and Describe return a copy. A caller that ignores the result changes nothing, which is
// what makes a half-applied change impossible.
func TestRenamingReturnsACopyRatherThanMutating(t *testing.T) {
	group, err := NewGroup(validInput())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	renamed, err := group.Rename("  Platform  ")
	if err != nil {
		t.Fatalf("renaming: %v", err)
	}
	if renamed.Name != "Platform" {
		t.Errorf("renamed to %q", renamed.Name)
	}
	if group.Name != "Design" {
		t.Errorf("the original changed to %q - Rename mutated its receiver", group.Name)
	}

	if _, err := group.Rename(""); err == nil {
		t.Error("an empty rename was accepted")
	}
}

func TestDescribingChecksTheSameBound(t *testing.T) {
	group, _ := NewGroup(validInput())

	described, err := group.Describe("  The people who own the design system  ")
	if err != nil {
		t.Fatalf("describing: %v", err)
	}
	if described.Description != "The people who own the design system" {
		t.Errorf("description %q", described.Description)
	}
	if _, err := group.Describe(strings.Repeat("d", maxGroupDescription+1)); err == nil {
		t.Error("an oversized description was accepted")
	}
}
