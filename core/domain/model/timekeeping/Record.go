// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package timekeeping is the time module's domain (M7): the time a person records where the work
// is, by hand or with a timer; the week it falls in, handed in, approved and then locked; a
// workspace's kinds of work; its locked period and whether a lead sees a person's minutes or only
// a team's; and the sums a report, an export and an entry's estimate against its actuals are made
// of.
//
// A record is personal data about the person who worked (UC-TIM-01): it is theirs to write while
// its week is open, it is never changed once approved - a correction is a new, signed record in an
// open week - and nothing here scores a person (P-14).
package timekeeping

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Day is a civil date, as the planning module counts it.
type Day = planning.Day

// The bounds of a record.
const (
	// MaxMinutes is one day: a record is at most a day's time, a correction at most a day back.
	MaxMinutes = 1440
	// MaxNoteLength is a record's note, in characters.
	MaxNoteLength = 2000
)

// Source says how a record came to be.
type Source string

const (
	SourceManual     Source = "MANUAL"
	SourceTimer      Source = "TIMER"
	SourceCopy       Source = "COPY"
	SourceCorrection Source = "CORRECTION"
)

// Valid reports whether the source is one of the four.
func (s Source) Valid() bool {
	return s == SourceManual || s == SourceTimer || s == SourceCopy || s == SourceCorrection
}

// Record is time one person spent on one day, on an entry or on nothing in particular (a meeting,
// a training), of a kind of work, billable or not.
type Record struct {
	ID        shared.ID
	AccountID shared.ID
	// EntryID is the entry the time was spent on; zero for time on no entry.
	EntryID  shared.ID
	WorkType string
	Day      Day
	// StartedAt is when the work began, where it is known (a timer, or a start and an end given).
	StartedAt *time.Time
	// Minutes are positive; a correction's are signed and never zero.
	Minutes  int
	Note     string
	Billable bool
	Source   Source
	// Corrects is the approved record a correction corrects.
	Corrects  shared.ID
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// Correction reports whether the record corrects an approved one.
func (r Record) Correction() bool { return r.Source == SourceCorrection }

// Check holds a record to its bounds: an owner, a day, minutes within a day (signed only for a
// correction, which names what it corrects), a note of at most MaxNoteLength characters.
func (r Record) Check() error {
	if r.ID.IsZero() || r.AccountID.IsZero() {
		return shared.ErrInternal.WithDetail("time.identity_incomplete")
	}
	if !r.Source.Valid() {
		return invalid("/source", "time.source_unknown")
	}
	if r.Correction() {
		if r.Corrects.IsZero() {
			return invalid("/corrects", "time.correction_target_required")
		}
		if r.Minutes == 0 || r.Minutes < -MaxMinutes || r.Minutes > MaxMinutes {
			return minutesError()
		}
	} else {
		if !r.Corrects.IsZero() {
			return invalid("/corrects", "time.correction_target_required")
		}
		if r.Minutes < 1 || r.Minutes > MaxMinutes {
			return minutesError()
		}
	}
	return CheckNote(r.Note)
}

// CheckNote holds a note to its length, free of control characters other than a line break.
func CheckNote(note string) error {
	if utf8.RuneCountInString(note) > MaxNoteLength {
		return shared.ErrValidation.WithDetail("time.note_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxNoteLength)}).
			WithFields(shared.FieldError{Path: "/note", Code: "time.note_too_long"})
	}
	if strings.ContainsFunc(note, func(r rune) bool { return r < 0x20 && r != '\n' && r != '\t' || r == 0x7f }) {
		return invalid("/note", "time.note_invalid")
	}
	return nil
}

func minutesError() error {
	return shared.ErrValidation.WithDetail("time.minutes_out_of_range").
		WithParams(map[string]string{"maximum": strconv.Itoa(MaxMinutes)}).
		WithFields(shared.FieldError{Path: "/minutes", Code: "time.minutes_out_of_range"})
}

// Span answers the minutes between a start and an end, rounded up to the started minute: the end
// is after the start and at most a day later.
func Span(start, end time.Time) (int, error) {
	if !end.After(start) {
		return 0, invalid("/ended_at", "time.ends_before_start")
	}
	elapsed := end.Sub(start)
	minutes := int(elapsed / time.Minute)
	if elapsed%time.Minute != 0 {
		minutes++
	}
	if minutes > MaxMinutes {
		return 0, minutesError()
	}
	return minutes, nil
}

// ParseDay reads a day written YYYY-MM-DD.
func ParseDay(text string) (Day, bool) {
	at, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return 0, false
	}
	return planning.DayFrom(at.Year(), at.Month(), at.Day()), true
}

// WeekOf answers the Monday of a day's week (ISO 8601): a timesheet is a week from Monday.
func WeekOf(d Day) Day {
	// 1970-01-01, day 0, was a Thursday: three days after a Monday.
	offset := (int(d)%7 + 7 + 3) % 7
	return d - Day(offset)
}

// MonthOf answers the first day of a day's month.
func MonthOf(d Day) Day {
	y, m, _ := d.Date()
	return planning.DayFrom(y, m, 1)
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

func planningDay(at time.Time, zone *time.Location) Day {
	if zone == nil {
		zone = time.UTC
	}
	return planning.DayOf(at, zone)
}
