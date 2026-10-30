// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// A plan is in exception once its forecast end slips past the agreed one by more than its
// tolerance (UC-PLN-04 check 7, standards.md P12).
func TestAPlanIsInExceptionBeyondItsTolerance(t *testing.T) {
	agreed := DayFrom(2026, 12, 1)
	for _, tc := range []struct {
		name      string
		forecast  Day
		tolerance int
		slip      int
		exceeded  bool
	}{
		{"early", agreed - 3, 0, 0, false},
		{"on the day", agreed, 0, 0, false},
		{"within", agreed + 5, 5, 5, false},
		{"beyond", agreed + 6, 5, 6, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := TimeException(agreed, tc.forecast, tc.tolerance)
			if got.SlipDays != tc.slip || got.Exceeded != tc.exceeded {
				t.Fatalf("%+v", got)
			}
		})
	}
	if err := TimeToleranceDefinition().Validate(); err != nil {
		t.Fatal(err)
	}
	if _, ok := TimeToleranceOf(setting.Resolution{}); ok {
		t.Error("an unset tolerance reads as one")
	}
	if days, ok := TimeToleranceOf(setting.Resolution{Effective: 5}); !ok || days != 5 {
		t.Errorf("tolerance = %d %v", days, ok)
	}
}
