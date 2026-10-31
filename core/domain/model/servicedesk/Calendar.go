// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package servicedesk is the service desk's building blocks (M5): a workspace's business calendars
// - working hours, holidays, a time zone under a key - and its SLA policies, the response and
// resolution promises an entry of a type is measured against on one of those calendars
// (domain-model.md, business calendars and SLAs).
//
// The clocks themselves are durable timers (package calendar); a policy decides only which timers
// an entry gets, how long each runs, and in which statuses it stands still.
package servicedesk

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// keyPattern is a building block's stable key, as the configuration document names it.
var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// MaxNameLength bounds a calendar's or a policy's name.
const MaxNameLength = 120

// Calendar is a workspace's business calendar under a key: what an SLA policy counts working time
// on. Changing it moves no clock already running - a timer keeps the calendar it was armed on.
//
// A hub may keep its own version of one (HubID set): its entries count on it wherever a policy
// names the key, the others on the workspace's.
type Calendar struct {
	ID       shared.ID
	TenantID shared.ID
	// HubID is the hub whose own version this is; empty for the workspace's calendar.
	HubID     shared.ID
	Key       string
	Name      string
	Calendar  calendar.BusinessCalendar
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// NewCalendarInput is what defining a calendar takes.
type NewCalendarInput struct {
	ID, TenantID shared.ID
	Key, Name    string
	Spec         calendar.Spec
	Now          time.Time
	Text         text.Normalizer
}

// NewCalendar checks a calendar: a key, a name on one line, and a spec the calendar package
// accepts.
func NewCalendar(in NewCalendarInput) (Calendar, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return Calendar{}, shared.ErrInternal.WithDetail("calendars.identity_incomplete")
	}
	checked, err := NewInstallationCalendar(in.Key, in.Name, in.Spec, in.Text)
	if err != nil {
		return Calendar{}, err
	}
	checked.ID, checked.TenantID = in.ID, in.TenantID
	checked.CreatedAt, checked.UpdatedAt, checked.Version = in.Now, in.Now, 1
	return checked, nil
}

// NewInstallationCalendar checks a calendar the instance file offers every workspace
// (`business_calendars`) as NewCalendar checks a workspace's: it has no identity, because it is no
// workspace's row, and a workspace calendar of the same key stands in front of it.
func NewInstallationCalendar(key, name string, spec calendar.Spec, normalizer text.Normalizer) (Calendar, error) {
	key = strings.TrimSpace(key)
	if !keyPattern.MatchString(key) {
		return Calendar{}, invalid("/key", "calendars.key_malformed")
	}
	checkedName, err := checkName(name, normalizer, "calendars")
	if err != nil {
		return Calendar{}, err
	}
	cal, err := calendar.New(spec)
	if err != nil {
		return Calendar{}, err
	}
	return Calendar{Key: key, Name: checkedName, Calendar: cal}, nil
}

// Change answers the calendar with a new name and spec, its key kept.
func (c Calendar) Change(name string, spec calendar.Spec, now time.Time, normalizer text.Normalizer) (Calendar, error) {
	changed, err := NewCalendar(NewCalendarInput{
		ID: c.ID, TenantID: c.TenantID, Key: c.Key, Name: name, Spec: spec, Now: now, Text: normalizer,
	})
	if err != nil {
		return Calendar{}, err
	}
	changed.CreatedAt, changed.Version, changed.HubID = c.CreatedAt, c.Version, c.HubID
	return changed, nil
}

func checkName(raw string, normalizer text.Normalizer, area string) (string, error) {
	name := strings.TrimSpace(raw)
	if normalizer != nil {
		name = normalizer.NFC(name)
	}
	switch {
	case name == "":
		return "", invalid("/name", area+".name_required")
	case utf8.RuneCountInString(name) > MaxNameLength || strings.ContainsAny(name, "\r\n"):
		return "", invalid("/name", area+".name_invalid")
	}
	return name, nil
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}
