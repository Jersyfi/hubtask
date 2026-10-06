// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// ADR-0078 §3: an operator opens the password for one workspace, 24 hours by default and at most
// seven days, with the requester and the reason recorded.

var openedAt = time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)

func TestAnOpeningIsDatedFromNowAndBoundedToAWeek(t *testing.T) {
	cases := []struct {
		name  string
		hours int
		code  string
	}{
		{"the default day", PasswordOpeningDefaultHours, ""},
		{"one hour", 1, ""},
		{"the whole week", PasswordOpeningMaximumHours, ""},
		{"no time at all", 0, "admin.password_opening_hours"},
		{"a negative time", -1, "admin.password_opening_hours"},
		{"an hour past the week", PasswordOpeningMaximumHours + 1, "admin.password_opening_hours"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opening, err := NewOpening(c.hours, "TICKET-4711", "the directory is down", openedAt, text.Composing{})
			if c.code != "" {
				if got := shared.AsError(err).DetailCode; got != c.code {
					t.Fatalf("answer %v, want %s", err, c.code)
				}
				if fields := shared.AsError(err).Fields; len(fields) != 1 || fields[0].Path != "/hours" {
					t.Errorf("the refusal is not at the field: %+v", fields)
				}
				return
			}
			if err != nil {
				t.Fatalf("a valid opening was refused: %v", err)
			}
			if want := openedAt.Add(time.Duration(c.hours) * time.Hour); !opening.Until.Equal(want) {
				t.Errorf("ends %v, want %v", opening.Until, want)
			}
		})
	}
}

func TestAnOpeningSaysWhoAskedAndWhy(t *testing.T) {
	cases := []struct {
		name, requester, reason, code, path string
	}{
		{"nobody asked", "  ", "the directory is down", "admin.password_opening_requester_required", "/requester"},
		{"no reason", "TICKET-4711", "", "admin.password_opening_reason_required", "/reason"},
		{"a requester past its length", strings.Repeat("r", maxOpeningRequester+1), "down",
			"admin.password_opening_requester_too_long", "/requester"},
		{"a reason past its length", "TICKET-4711", strings.Repeat("r", maxOpeningReason+1),
			"admin.password_opening_reason_too_long", "/reason"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewOpening(24, c.requester, c.reason, openedAt, text.Composing{})
			refused := shared.AsError(err)
			if refused == nil || refused.DetailCode != c.code {
				t.Fatalf("answer %v, want %s", err, c.code)
			}
			if len(refused.Fields) != 1 || refused.Fields[0].Path != c.path {
				t.Errorf("the refusal is not at %s: %+v", c.path, refused.Fields)
			}
		})
	}

	opening, err := NewOpening(24, "  Ticket Büro-12 ", " Entra answers 500 ", openedAt, text.Composing{})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if opening.Requester != "Ticket Büro-12" || opening.Reason != "Entra answers 500" {
		t.Errorf("the texts were not trimmed and composed: %q, %q", opening.Requester, opening.Reason)
	}
}

func TestAnOpeningEndsOnItsOwn(t *testing.T) {
	opening, err := NewOpening(2, "TICKET-4711", "the directory is down", openedAt, text.Composing{})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if !opening.InForce(openedAt) || !opening.InForce(openedAt.Add(time.Hour)) {
		t.Error("an opening is not in force within its time")
	}
	if opening.InForce(openedAt.Add(2 * time.Hour)) {
		t.Error("an opening outlived its end")
	}
	if (PasswordOpening{}).InForce(openedAt) {
		t.Error("no opening is in force")
	}
}
