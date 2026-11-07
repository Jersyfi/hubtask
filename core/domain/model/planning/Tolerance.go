// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// TimeToleranceKey is how many calendar days a plan's forecast end may slip past its agreed one
// before the plan is in exception (configuration.md §10, PRINCE2's time tolerance). Decided by the
// workspace, a hub or a collection, inherited below and lockable there, like every setting.
const TimeToleranceKey setting.Key = "planning.time_tolerance_days"

// MaxTimeToleranceDays bounds the tolerance: a slip of more than a year is no tolerance at all.
const MaxTimeToleranceDays = 365

// TimeToleranceDefinition is the setting's declaration. Without a value at any level a plan has
// no tolerance and raises no exception.
func TimeToleranceDefinition() setting.Definition {
	zero, most := 0, MaxTimeToleranceDays
	return setting.Definition{
		Key: TimeToleranceKey, Kind: setting.KindCount, Minimum: &zero, Maximum: &most,
		Levels:   []setting.Level{setting.LevelWorkspace, setting.LevelHub, setting.LevelCollection},
		Lockable: true, May: setting.MayReplace,
		Writers: map[setting.Level]string{
			setting.LevelWorkspace: "STRUCTURE", setting.LevelHub: "STRUCTURE", setting.LevelCollection: "STRUCTURE",
		},
	}
}

// TimeToleranceOf reads the resolved tolerance; false when no level set one.
func TimeToleranceOf(r setting.Resolution) (int, bool) {
	switch v := r.Effective.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	}
	return 0, false
}

// Exception is a plan's standing against its time tolerance: how far the forecast end lies past
// the agreed one, and whether that is beyond what the tolerance allows.
type Exception struct {
	Agreed    Day
	Forecast  Day
	SlipDays  int
	Tolerance int
	Exceeded  bool
}

// TimeException compares the forecast end with the agreed one; a forecast before the agreed end
// slips by nothing.
func TimeException(agreed, forecast Day, tolerance int) Exception {
	slip := max(int(forecast-agreed), 0)
	return Exception{Agreed: agreed, Forecast: forecast, SlipDays: slip, Tolerance: tolerance, Exceeded: slip > tolerance}
}
