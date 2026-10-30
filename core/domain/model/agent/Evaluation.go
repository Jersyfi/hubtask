// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The bounds of an evaluation set.
const (
	MaxCases        = 500
	MaxCaseText     = 20000
	MaxExpectedKeys = 20
)

// Case is one golden case of an agent's evaluation set (G7): a ticket's words and what the agent
// should answer for them - a collection, labels, a CI. The words are the workspace's data, written
// by whoever keeps the set, and never leave it but to the agent's own model.
type Case struct {
	ID       shared.ID
	AgentID  shared.ID
	Title    string
	Notes    string
	Expected map[string]any
	// CreatedBy names who wrote it.
	CreatedBy shared.ID
	CreatedAt time.Time
}

// CheckCase checks a case's fields.
func CheckCase(c Case, normalizer text.Normalizer) (Case, error) {
	c.Title = strings.TrimSpace(c.Title)
	c.Notes = strings.TrimSpace(c.Notes)
	if normalizer != nil {
		c.Title, c.Notes = normalizer.NFC(c.Title), normalizer.NFC(c.Notes)
	}
	if c.Title == "" || utf8.RuneCountInString(c.Title) > 500 {
		return Case{}, invalid("/title", "agent_cases.title_invalid")
	}
	if utf8.RuneCountInString(c.Notes) > MaxCaseText {
		return Case{}, invalid("/notes", "agent_cases.notes_too_long")
	}
	if len(c.Expected) == 0 || len(c.Expected) > MaxExpectedKeys {
		return Case{}, invalid("/expected", "agent_cases.expected_invalid")
	}
	return c, nil
}

// Run is one evaluation of an agent against its set, on one revision of its model: how many cases
// it ran and how many it answered as expected. Who started it and when; no answer is kept.
type Run struct {
	ID            shared.ID
	AgentID       shared.ID
	ModelRevision int
	Cases         int
	Passed        int
	By            shared.ID
	At            time.Time
}

// Accuracy is the share of cases passed, in percent, rounded down.
func (r Run) Accuracy() int {
	if r.Cases == 0 {
		return 0
	}
	return r.Passed * 100 / r.Cases
}

// PassedAt reports a run that reached the minimum: at least one case, and the share of them.
func (r Run) PassedAt(min int) bool { return r.Cases > 0 && r.Passed*100 >= min*r.Cases }

// Matches reports whether an answer meets a case's expectation: every expected key answered with
// the same value, a list compared as a set. A key the case does not name is not judged.
func Matches(expected, answer map[string]any) bool {
	for key, want := range expected {
		got, found := answer[key]
		if !found || !same(want, got) {
			return false
		}
	}
	return true
}

func same(want, got any) bool {
	wantList, wantIsList := want.([]any)
	gotList, gotIsList := got.([]any)
	if wantIsList || gotIsList {
		if !wantIsList || !gotIsList || len(wantList) != len(gotList) {
			return false
		}
		a, b := strs(wantList), strs(gotList)
		slices.Sort(a)
		slices.Sort(b)
		return slices.Equal(a, b)
	}
	return fmt.Sprint(want) == fmt.Sprint(got)
}

func strs(values []any) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = fmt.Sprint(v)
	}
	return out
}
