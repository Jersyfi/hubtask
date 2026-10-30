// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"math"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// EstimateUnit is what an estimate counts (PLN-07).
type EstimateUnit string

const (
	// Points are story points, a team's relative size of a piece of work.
	Points EstimateUnit = "POINTS"
	// Hours are the hours a piece of work is expected to take.
	Hours EstimateUnit = "HOURS"
	// Size is a T-shirt size; it counts as the points of its size (sizePoints), so that a team
	// estimating in sizes still has a velocity.
	Size EstimateUnit = "SIZE"
)

// Valid reports whether the unit is one of the three.
func (u EstimateUnit) Valid() bool { return u == Points || u == Hours || u == Size }

// MaxEstimate is the largest value one estimate holds.
const MaxEstimate = 10000

// sizePoints are the points a T-shirt size counts as: the Fibonacci steps most teams map them to.
var sizePoints = map[string]float64{"XS": 1, "S": 2, "M": 3, "L": 5, "XL": 8, "XXL": 13}

// Sizes are the T-shirt sizes, smallest first.
var Sizes = []string{"XS", "S", "M", "L", "XL", "XXL"}

// Estimate is how big one entry is: a first-class value, so that a sprint's commitment and a
// team's velocity add up without guessing which custom field holds it.
type Estimate struct {
	EntryID   shared.ID
	Unit      EstimateUnit
	Value     float64
	Size      string
	UpdatedAt time.Time
	UpdatedBy shared.ID
	Version   int
}

// NewEstimate checks an estimate. A size names its value; points and hours carry at most two
// decimals and are at most MaxEstimate.
func NewEstimate(entryID shared.ID, unit EstimateUnit, value float64, size string) (Estimate, error) {
	if entryID.IsZero() {
		return Estimate{}, shared.ErrInternal.WithDetail("estimates.identity_incomplete")
	}
	if !unit.Valid() {
		return Estimate{}, invalid("/unit", "estimates.unit_unknown")
	}
	if unit == Size {
		points, ok := sizePoints[size]
		if !ok {
			return Estimate{}, invalid("/size", "estimates.size_unknown")
		}
		return Estimate{EntryID: entryID, Unit: Size, Value: points, Size: size}, nil
	}
	if size != "" {
		return Estimate{}, invalid("/size", "estimates.size_without_unit")
	}
	if math.IsNaN(value) || value < 0 || value > MaxEstimate || math.Abs(value*100-math.Round(value*100)) > 1e-6 {
		return Estimate{}, shared.ErrValidation.WithDetail("estimates.value_out_of_range").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxEstimate)}).
			WithFields(shared.FieldError{Path: "/value", Code: "estimates.value_out_of_range"})
	}
	return Estimate{EntryID: entryID, Unit: unit, Value: math.Round(value*100) / 100}, nil
}

// CountsIn answers what the estimate adds to a sum kept in a unit: points take sizes too, hours
// only hours, and an estimate in another unit adds nothing.
func (e Estimate) CountsIn(unit EstimateUnit) (float64, bool) {
	switch {
	case unit == Hours && e.Unit == Hours:
		return e.Value, true
	case unit == Points && (e.Unit == Points || e.Unit == Size):
		return e.Value, true
	}
	return 0, false
}
