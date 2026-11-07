// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The one question's scale (UC-SVC-07): 1 is very dissatisfied, 5 very satisfied.
const (
	MinSatisfaction = 1
	MaxSatisfaction = 5
)

// Rating is how satisfied the requester was with a resolved request, one per request; rating
// again replaces it. It names nobody: the rater is always who raised the request, and the words
// they add go into the discussion as a comment of theirs.
type Rating struct {
	ID       shared.ID
	TenantID shared.ID
	ItemID   shared.ID
	Score    int
	RatedAt  time.Time
	Version  int
}

// NewRating checks a score on the scale.
func NewRating(id, tenantID, itemID shared.ID, score int, now time.Time) (Rating, error) {
	if id.IsZero() || tenantID.IsZero() || itemID.IsZero() {
		return Rating{}, shared.ErrInternal.WithDetail("satisfaction.identity_incomplete")
	}
	if score < MinSatisfaction || score > MaxSatisfaction {
		return Rating{}, shared.ErrValidation.WithDetail("satisfaction.score_invalid").
			WithParams(map[string]string{"min": strconv.Itoa(MinSatisfaction), "max": strconv.Itoa(MaxSatisfaction)}).
			WithFields(shared.FieldError{Path: "/score", Code: "satisfaction.score_invalid"})
	}
	return Rating{ID: id, TenantID: tenantID, ItemID: itemID, Score: score, RatedAt: now, Version: 1}, nil
}
