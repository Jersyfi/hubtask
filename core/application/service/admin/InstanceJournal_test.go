// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"slices"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/quota"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// The journal names every default and lock that moved (UC-INS-12 check 3) - the localisation
// defaults and the quota ceilings as well as the sign-in switches - and never a value.
func TestMovingALocalisationDefaultOrACeilingIsNamedInTheJournal(t *testing.T) {
	before := identityrepo.InstanceLevel{
		Quotas: identity.QuotaDefaults{
			Limits: map[string]int64{quota.Items: 500, quota.ExportJobs: 3},
			Locks:  map[string]bool{},
		},
	}
	after := identityrepo.InstanceLevel{
		Localisation: identity.LocalisationDefaults{Locale: "de", WeekStart: identity.WeekStartSunday},
		Quotas: identity.QuotaDefaults{
			Limits: map[string]int64{quota.Items: 600, quota.ExportJobs: 3},
			Locks:  map[string]bool{quota.ExportJobs: true},
		},
	}
	moved := movedSwitches(before, after)
	for _, want := range []any{"localisation.locale", "localisation.week_start", "quotas.items", "quotas.export_jobs"} {
		if !slices.Contains(moved, want) {
			t.Errorf("the journal does not name %v: %v", want, moved)
		}
	}
	for _, unmoved := range []any{"localisation.time_zone", "quotas.media_bytes"} {
		if slices.Contains(moved, unmoved) {
			t.Errorf("the journal names %v, which did not move", unmoved)
		}
	}
	for _, name := range moved {
		if name == "de" || name == "SUNDAY" {
			t.Errorf("the journal carries a value: %v", moved)
		}
	}
}

// Deciding a switch at the product's own default is a change of the installation's defaults even
// though the value in force stays the same: from then on a workspace cannot go below it. So the
// journal names a switch whose decided-ness moves, both ways, and not one that stays undecided.
func TestDecidingASwitchAtTheProductsDefaultIsNamedInTheJournal(t *testing.T) {
	zero, off, none := 0, false, identity.MfaRequirement("NONE")
	decided := identityrepo.InstanceLevel{Policy: identity.PolicyLayer{Patch: identity.PolicyPatch{
		MinSymbols: &zero, BreachCheck: &off, MfaRequiredFor: &none,
	}}}
	open := identityrepo.InstanceLevel{}

	for direction, pair := range map[string][2]identityrepo.InstanceLevel{
		"starting to decide": {open, decided},
		"ceasing to decide":  {decided, open},
	} {
		moved := movedSwitches(pair[0], pair[1])
		for _, want := range []any{"min_symbols", "breach_check", "mfa_required_for"} {
			if !slices.Contains(moved, want) {
				t.Errorf("%s: the journal does not name %v: %v", direction, want, moved)
			}
		}
		if slices.Contains(moved, any("min_length")) {
			t.Errorf("%s: the journal names min_length, which nobody decided: %v", direction, moved)
		}
	}
}
