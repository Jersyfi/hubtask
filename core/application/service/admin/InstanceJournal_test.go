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
