// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// FigureClockBound is how long a kept count of a kind the clock moves may answer: FLOW's seven days
// roll forward with every read, and RHYTHM counts up to the moment of counting. The kinds the
// entries alone decide answer for as long as the collection is unchanged (migration 0292).
const FigureClockBound = time.Minute

// FigureKeptLife is how old a kept count of any kind may be before it is counted again: the recount
// job's bound on what it walks, and a day is longer than any reader keeps one basis.
const FigureKeptLife = 24 * time.Hour

// KeptFor is how long a kept count of the kind may answer, zero where the collection's changes
// alone decide.
func (f Figure) KeptFor() time.Duration {
	switch f {
	case FigureFlow, FigureRhythm:
		return FigureClockBound
	}
	return 0
}

// Recountable reports whether a kept count of the kind can be checked against a fresh count of
// its own basis: true where the clock moves nothing.
func (f Figure) Recountable() bool { return f.KeptFor() == 0 }

// RecountableFigures are the kinds the recount job checks.
func RecountableFigures() []Figure {
	return []Figure{FigureOpen, FigureFieldSum, FigureDue, FigureProgress}
}

// FigureBasis is everything a count depends on besides the collection's entries: what the
// structure offers, and the parts of the reader's day and week the kind reads. Two readers with the
// same basis are answered the same count.
type FigureBasis struct {
	Offered  []ItemType
	Dated    []ItemType
	FieldKey string
	// Today and DayEnd are DUE's, Zone and WeekStart RHYTHM's; empty where the kind reads none.
	Today     string
	DayEnd    time.Time
	Zone      string
	WeekStart time.Time
}

// Basis answers the request's basis in a window. Only what the kind reads enters it, so that a
// reader in another zone shares a progress count rather than keeping one of their own.
func (r FigureRequest) Basis(window FigureWindow) FigureBasis {
	basis := FigureBasis{
		Offered: slices.Clone(r.Offered), Dated: slices.Clone(r.Dated), FieldKey: r.FieldKey,
	}
	switch r.Kind {
	case FigureDue:
		basis.Today, basis.DayEnd = window.Today, window.DayEnd.UTC()
	case FigureRhythm:
		if window.Location != nil {
			basis.Zone = window.Location.String()
		}
		basis.WeekStart = window.WeekStart.UTC()
	}
	return basis
}

// Key is the basis as one comparable text: equal for equal bases, and never equal otherwise,
// since no part may hold the separator (types are names, a field key `[a-z0-9_]`, a zone a path).
func (b FigureBasis) Key() string {
	parts := []string{JoinTypes(b.Offered), JoinTypes(b.Dated), b.FieldKey, b.Today, "", b.Zone, ""}
	if !b.DayEnd.IsZero() {
		parts[4] = b.DayEnd.UTC().Format(time.RFC3339)
	}
	if !b.WeekStart.IsZero() {
		parts[6] = b.WeekStart.UTC().Format(time.RFC3339)
	}
	return strings.Join(parts, "|")
}

// Request is the request the basis was taken from, for a recount of the same figure.
func (b FigureBasis) Request(collectionID shared.ID, kind Figure) FigureRequest {
	return FigureRequest{
		CollectionID: collectionID, Kind: kind,
		Offered: slices.Clone(b.Offered), Dated: slices.Clone(b.Dated), FieldKey: b.FieldKey,
	}
}

// Window is the part of a window the basis holds, at a moment: enough for the kinds the entries
// alone decide, whose recount must count exactly what the kept count did.
func (b FigureBasis) Window(now time.Time) FigureWindow {
	return FigureWindow{Now: now, Location: time.UTC, Today: b.Today, DayEnd: b.DayEnd, WeekStart: b.WeekStart}
}

// SameCounts reports whether two counts answer the same figure.
func SameCounts(a, b FigureCounts) bool {
	if a.Open != b.Open || a.Summed != b.Summed || a.Due != b.Due || a.Done != b.Done ||
		a.Total != b.Total || a.Completed != b.Completed || a.OverLimit != b.OverLimit {
		return false
	}
	if (a.Sum == nil) != (b.Sum == nil) || (a.Sum != nil && *a.Sum != *b.Sum) {
		return false
	}
	if (a.AverageSeconds == nil) != (b.AverageSeconds == nil) ||
		(a.AverageSeconds != nil && *a.AverageSeconds != *b.AverageSeconds) {
		return false
	}
	days := func(c FigureCounts) []string {
		out := slices.Clone(c.Days)
		slices.Sort(out)
		return slices.Compact(out)
	}
	return slices.Equal(days(a), days(b))
}
