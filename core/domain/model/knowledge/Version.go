// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// SessionGap is how long a writer may pause and still be in the same editing session: writes
// closer together than this, by the same person, fold into one version (UC-DOC-06).
const SessionGap = 10 * time.Minute

// MaxVersionName bounds a version's name ("Sent to legal").
const MaxVersionName = 120

// Version is one state of a page that its history keeps: its title and body as they stood, who
// wrote them, and the name a person gave it, if any.
type Version struct {
	PageID   shared.ID
	Seq      int
	Title    string
	Body     string
	AuthorID shared.ID
	Name     string
	// Restored is the version this one brought back, zero for a version written by editing.
	Restored  int
	CreatedAt time.Time
}

// Snapshot answers the version the write leaves behind: a new one when there is none yet, when
// somebody else wrote the last one, when the last one is named, or when the session paused longer
// than SessionGap; otherwise the last one carries the new state (fold true). A restore never folds:
// it is a version of its own (P-03).
func Snapshot(last *Version, page Page, author shared.ID, now time.Time) (version Version, fold bool) {
	next := Version{PageID: page.ID, Seq: 1, Title: page.Title, Body: page.Body, AuthorID: author, CreatedAt: now}
	if last == nil {
		return next, false
	}
	if last.AuthorID == author && last.Name == "" && last.Restored == 0 && now.Sub(last.CreatedAt) < SessionGap {
		folded := *last
		folded.Title, folded.Body, folded.CreatedAt = page.Title, page.Body, now
		return folded, true
	}
	next.Seq = last.Seq + 1
	return next, false
}

// CheckVersionName brings a version's name to its stored form; empty takes the name away.
func CheckVersionName(raw string, form text.Normalizer) (string, error) {
	name, err := shared.NFC(strings.TrimSpace(raw), form)
	if err != nil {
		return "", err
	}
	if utf8.RuneCountInString(name) > MaxVersionName || strings.ContainsAny(name, "\r\n") {
		return "", invalid("/name", "pages.version_name_malformed").WithParams(map[string]string{"maximum": "120"})
	}
	return name, nil
}
