// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The UID a calendar client knows an entry by (P-07, issue #721). What the domain holds it to is
// what a path segment can carry, because the tree answers the entry at `{uid}.ics`; what it says
// about the value is nothing else - the string is the client's and opaque.

func TestAnEntryKeepsTheUIDACalendarClientChose(t *testing.T) {
	for _, uid := range []string{
		"6BA7B810-9DAD-11D1-80B4-00C04FD430C8",    // Reminders: an uppercase UUID
		"b1c9c2f0-3a7e-4f2e-9b3d-2f1a5c6d7e8f",    // Thunderbird: a lowercase one
		"20260917T101500Z-4711@laptop.example",    // RFC 5545's recommended shape
		"x!$&'()*+,;=:@-._~",                      // the whole of pchar
		strings.Repeat("a", MaxCalendarUIDLength), // exactly the bound
	} {
		in := taskInput()
		in.CalendarUID = uid
		item, err := NewWorkItem(in)
		if err != nil {
			t.Fatalf("%q: %v", uid, err)
		}
		if item.CalendarUID != uid {
			t.Errorf("%q came back as %q", uid, item.CalendarUID)
		}
	}
}

// Empty is the ordinary case: an entry no calendar client made carries no address of its own.
func TestAnEntryMadeAnywhereElseHasNoCalendarUID(t *testing.T) {
	item, err := NewWorkItem(taskInput())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if item.CalendarUID != "" {
		t.Errorf("an entry nobody addressed carries %q", item.CalendarUID)
	}
}

func TestACalendarUIDAnAddressCannotCarryIsRefusedByName(t *testing.T) {
	for name, uid := range map[string]string{
		"a slash":      "a/b",
		"a space":      "two words",
		"a percent":    "a%20b",
		"a question":   "a?b",
		"a hash":       "a#b",
		"a control":    "a\tb",
		"a non-ASCII":  "café",
		"one too long": strings.Repeat("a", MaxCalendarUIDLength+1),
	} {
		in := taskInput()
		in.CalendarUID = uid
		_, err := NewWorkItem(in)
		if !errors.Is(err, shared.ErrValidation) {
			t.Fatalf("%s: answered %v", name, err)
		}
		var domainErr *shared.Error
		if !errors.As(err, &domainErr) || domainErr.DetailCode != "items.calendar_uid_invalid" {
			t.Fatalf("%s: the detail code is not items.calendar_uid_invalid: %v", name, err)
		}
		if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != "/calendar_uid" {
			t.Errorf("%s: the error does not name the field: %+v", name, domainErr.Fields)
		}
	}
}
