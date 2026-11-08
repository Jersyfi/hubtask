// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"strconv"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// StarKind is what a star points at: a place a person returns to (navigation concept §6.2).
type StarKind string

const (
	StarHub        StarKind = "HUB"
	StarCollection StarKind = "COLLECTION"
	StarView       StarKind = "VIEW"
	StarPage       StarKind = "PAGE"
	StarWhiteboard StarKind = "WHITEBOARD"
	StarDashboard  StarKind = "DASHBOARD"
	StarPortfolio  StarKind = "PORTFOLIO"

	// MaxStars bounds one person's list: a list longer than this is no longer a short way back.
	MaxStars = 100
)

// StarKinds is the closed set, in the contract's order.
var StarKinds = []StarKind{StarHub, StarCollection, StarView, StarPage, StarWhiteboard, StarDashboard, StarPortfolio}

// Star is one starred place.
type Star struct {
	Kind StarKind
	ID   shared.ID
}

// StarList is a person's stars in their order, with the version a second device writes against.
type StarList struct {
	Stars   []Star
	Version int
}

// CheckStars checks a list as a person writes it whole: at most MaxStars, each of a known kind with
// an identifier; the same place twice is kept once, at its first place.
func CheckStars(raw []Star) ([]Star, error) {
	if len(raw) > MaxStars {
		return nil, shared.ErrValidation.WithDetail("stars.too_many").
			WithParams(map[string]string{"max": strconv.Itoa(MaxStars)}).
			WithFields(shared.FieldError{Path: "/stars", Code: "stars.too_many"})
	}
	stars := make([]Star, 0, len(raw))
	for i, star := range raw {
		if !slices.Contains(StarKinds, star.Kind) {
			return nil, shared.ErrValidation.WithDetail("stars.kind_invalid").
				WithParams(map[string]string{"value": string(star.Kind)}).
				WithFields(shared.FieldError{Path: "/stars/" + strconv.Itoa(i) + "/kind", Code: "stars.kind_invalid"})
		}
		if star.ID.IsZero() {
			return nil, shared.ErrValidation.WithDetail("stars.id_required").
				WithFields(shared.FieldError{Path: "/stars/" + strconv.Itoa(i) + "/id", Code: "stars.id_required"})
		}
		if slices.Contains(stars, star) {
			continue
		}
		stars = append(stars, star)
	}
	return stars, nil
}
