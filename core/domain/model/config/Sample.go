// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"regexp"
	"slices"
	"strings"
)

// SampleKind is what a mark of the workspace's sample content names (user-experience.md §3.6).
type SampleKind string

const (
	SampleContainer  SampleKind = "CONTAINER"
	SampleEntry      SampleKind = "ENTRY"
	SamplePage       SampleKind = "PAGE"
	SampleWhiteboard SampleKind = "WHITEBOARD"
	SampleGoal       SampleKind = "GOAL"
	SamplePortfolio  SampleKind = "PORTFOLIO"
	SampleDashboard  SampleKind = "DASHBOARD"
	SampleTimeRecord SampleKind = "TIME_RECORD"
)

// removalOrder is the order one act removes the samples in. The kinds without a trash go first,
// while their hub still stands to be read through; then the containers, which take what lies in
// them along; then whatever is left, which finds itself gone with its hub or is trashed alone.
var removalOrder = []SampleKind{
	SampleTimeRecord, SampleDashboard, SamplePortfolio, SampleGoal,
	SampleContainer, SampleWhiteboard, SamplePage, SampleEntry,
}

var sampleObjectKey = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,99}$`)

// ValidSampleKey reports whether a key can mark sample content: a lower-case word, dotted.
func ValidSampleKey(key string) bool { return sampleObjectKey.MatchString(key) }

// Known reports whether the kind is one a mark can name.
func (k SampleKind) Known() bool { return slices.Contains(removalOrder, k) }

// Trashed reports whether removing an object of the kind moves it into the trash. One that is not
// is deleted, and only while it stands at the version it was marked with.
func (k SampleKind) Trashed() bool {
	switch k {
	case SampleContainer, SampleEntry, SamplePage, SampleWhiteboard:
		return true
	default:
		return false
	}
}

// SampleMark is one mark: its key, what it names and the version it was made at.
type SampleMark struct {
	Key      string
	Kind     SampleKind
	ObjectID string
	Version  *int64
}

// RemovalOrder sorts marks into the order one act removes them in, by key within a kind. A mark
// of a kind nobody knows is left out: it names nothing removing could reach. Of a kind deleted at
// its version, a deeper key goes first: a child goal keyed under its parent ("goal.a.b") goes
// before the parent, whose deletion would otherwise move the child's version.
func RemovalOrder(marks []SampleMark) []SampleMark {
	out := make([]SampleMark, 0, len(marks))
	for _, kind := range removalOrder {
		start := len(out)
		for _, m := range marks {
			if m.Kind == kind {
				out = append(out, m)
			}
		}
		slices.SortFunc(out[start:], func(a, b SampleMark) int {
			if !kind.Trashed() {
				if depth := strings.Count(b.Key, ".") - strings.Count(a.Key, "."); depth != 0 {
					return depth
				}
			}
			return strings.Compare(a.Key, b.Key)
		})
	}
	return out
}
