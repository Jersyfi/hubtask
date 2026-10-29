// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// SessionGap is how long a board may rest and its drawing still be one session: the saves of a
// session fold into one version (UC-CNV-06).
const SessionGap = 10 * time.Minute

// MaxVersionName bounds a version's name ("Before the workshop").
const MaxVersionName = 120

// Version is one state of a board its history keeps: the title and the scene as they stood, who
// saved them last, and the name a person gave it, if any.
type Version struct {
	BoardID   shared.ID
	Seq       int
	Title     string
	Scene     Scene
	AuthorID  shared.ID
	Name      string
	CreatedAt time.Time
}

// Snapshot answers the version a save leaves behind: a new one when there is none yet, when the
// last one is named, or when the board rested longer than SessionGap; otherwise the last one carries
// the new state (fold true). Unlike a page, a board is drawn by several people at once, each device
// saving the whole scene, so a session is the board's, not one writer's: the version names whoever
// saved last.
func Snapshot(last *Version, board Board, author shared.ID, now time.Time) (version Version, fold bool) {
	next := Version{BoardID: board.ID, Seq: 1, Title: board.Title, Scene: board.Scene, AuthorID: author, CreatedAt: now}
	if last == nil {
		return next, false
	}
	if last.Name == "" && now.Sub(last.CreatedAt) < SessionGap {
		folded := *last
		folded.Title, folded.Scene, folded.AuthorID, folded.CreatedAt = board.Title, board.Scene, author, now
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
		return "", invalid("/name", "whiteboards.version_name_malformed").WithParams(map[string]string{"maximum": "120"})
	}
	return name, nil
}
