// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// ReleaseState is where a release stands: planned, or out (PLN-08).
type ReleaseState string

const (
	ReleasePlanned  ReleaseState = "PLANNED"
	ReleaseReleased ReleaseState = "RELEASED"
)

// The bounds of a release: its description and its entries. A release's notes mention each done
// entry once, so its entries stay within what one page's references hold.
const (
	MaxReleaseDescriptionLength = 2000
	MaxReleaseEntries           = 200
)

// Release is a version a hub ships: a name, the day it is meant to go out, and the entries in it.
// An entry is in at most one release. Releasing it records the moment and, when asked, the page
// its notes were written to; a released release changes no more.
type Release struct {
	ID          shared.ID
	TenantID    shared.ID
	HubID       shared.ID
	Name        string
	Description string
	ReleaseOn   Day
	State       ReleaseState
	ReleasedAt  *time.Time
	NotesPageID shared.ID
	CreatedAt   time.Time
	CreatedBy   shared.ID
	UpdatedAt   time.Time
	Version     int
}

// ReleaseFields are the fields a person sets on a release.
type ReleaseFields struct {
	Name        string
	Description string
	ReleaseOn   Day
}

// NewReleaseInput is what planning a release takes.
type NewReleaseInput struct {
	ID, TenantID, HubID, CreatedBy shared.ID
	Fields                         ReleaseFields
	Now                            time.Time
	Text                           text.Normalizer
}

// NewRelease checks a planned release.
func NewRelease(in NewReleaseInput) (Release, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.HubID.IsZero() {
		return Release{}, shared.ErrInternal.WithDetail("releases.identity_incomplete")
	}
	r := Release{
		ID: in.ID, TenantID: in.TenantID, HubID: in.HubID, State: ReleasePlanned,
		CreatedAt: in.Now, CreatedBy: in.CreatedBy, UpdatedAt: in.Now, Version: 1,
	}
	if err := r.set(in.Fields, in.Text); err != nil {
		return Release{}, err
	}
	return r, nil
}

// Change sets a planned release's name, description or day; a released one is history.
func (r *Release) Change(fields ReleaseFields, now time.Time, normalizer text.Normalizer) error {
	if r.State == ReleaseReleased {
		return shared.ErrConflict.WithDetail("releases.released")
	}
	if err := r.set(fields, normalizer); err != nil {
		return err
	}
	r.UpdatedAt = now
	return nil
}

// Changeable refuses a change to a released release's entries.
func (r Release) Changeable() error {
	if r.State == ReleaseReleased {
		return shared.ErrConflict.WithDetail("releases.released")
	}
	return nil
}

func (r *Release) set(f ReleaseFields, normalizer text.Normalizer) error {
	name, description := strings.TrimSpace(f.Name), strings.TrimSpace(f.Description)
	if normalizer != nil {
		name, description = normalizer.NFC(name), normalizer.NFC(description)
	}
	switch {
	case name == "":
		return invalid("/name", "releases.name_required")
	case utf8.RuneCountInString(name) > MaxNameLength || strings.ContainsAny(name, "\r\n"):
		return invalid("/name", "releases.name_invalid")
	case utf8.RuneCountInString(description) > MaxReleaseDescriptionLength:
		return shared.ErrValidation.WithDetail("releases.description_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxReleaseDescriptionLength)}).
			WithFields(shared.FieldError{Path: "/description", Code: "releases.description_too_long"})
	case f.ReleaseOn == 0:
		return invalid("/release_on", "releases.date_required")
	}
	r.Name, r.Description, r.ReleaseOn = name, description, f.ReleaseOn
	return nil
}

// Release marks a planned release as out at a moment, with the page its notes went to, if any.
func (r *Release) Release(notesPage shared.ID, now time.Time) error {
	if r.State == ReleaseReleased {
		return shared.ErrConflict.WithDetail("releases.released")
	}
	r.State, r.ReleasedAt, r.NotesPageID, r.UpdatedAt = ReleaseReleased, &now, notesPage, now
	return nil
}

// NoteEntry is one entry as a release's notes name it: its title and the name of its type, both
// the workspace's own words.
type NoteEntry struct {
	ID       shared.ID
	Title    string
	TypeName string
}

// Notes writes a release's notes as Markdown: the release's description, then its done entries
// grouped under their type's name - the workspace's vocabulary, an entry without a type first -
// each a mention of the entry, so the page links to the work and follows its title. The words are
// the workspace's own; the backend adds none of its own (rule 8).
func Notes(r Release, done []NoteEntry) string {
	groups := map[string][]NoteEntry{}
	names := []string{}
	for _, entry := range done {
		if _, ok := groups[entry.TypeName]; !ok {
			names = append(names, entry.TypeName)
		}
		groups[entry.TypeName] = append(groups[entry.TypeName], entry)
	}
	sort.Strings(names)
	var out strings.Builder
	if r.Description != "" {
		out.WriteString(r.Description)
		out.WriteString("\n\n")
	}
	for _, name := range names {
		if name != "" {
			out.WriteString("## " + escapeLine(name) + "\n\n")
		}
		entries := groups[name]
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Title < entries[j].Title })
		for _, entry := range entries {
			out.WriteString("- [" + escapeLabel(entry.Title) + "](hubtask:entry/" + entry.ID.String() + ")\n")
		}
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n") + "\n"
}

// escapeLabel keeps a title inside its link's label: the brackets and backslashes it holds are
// escaped, and it stays on one line.
func escapeLabel(s string) string {
	return strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`, "\r", " ", "\n", " ").Replace(s)
}

func escapeLine(s string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(s) }
