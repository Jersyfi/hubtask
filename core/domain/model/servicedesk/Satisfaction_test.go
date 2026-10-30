// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-SVC-07 check 2: a score is a whole number from 1 to 5.
func TestARatingIsOnTheScale(t *testing.T) {
	id := shared.ID("11111111-1111-4111-8111-111111111111")
	for score, ok := range map[int]bool{0: false, 1: true, 3: true, 5: true, 6: false, -1: false} {
		_, err := NewRating(id, id, id, score, time.Unix(0, 0))
		var refused *shared.Error
		switch {
		case ok && err != nil:
			t.Errorf("score %d refused: %v", score, err)
		case !ok && (!errors.As(err, &refused) || refused.DetailCode != "satisfaction.score_invalid"):
			t.Errorf("score %d: %v, want satisfaction.score_invalid", score, err)
		}
	}
}
