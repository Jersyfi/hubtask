// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package reference is the one way anything in Hubtask points at anything else (P-19, F-3): a
// kind, an identifier and an optional anchor inside the object. A mention, a card and a live view
// are the same reference drawn at three strengths; none of them carries a copy of what it points
// at, and the reader's permission decides what it shows.
package reference

import (
	"regexp"
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Kind is what a reference points at.
type Kind string

const (
	KindEntry      Kind = "ENTRY"
	KindHub        Kind = "HUB"
	KindCollection Kind = "COLLECTION"
	KindView       Kind = "VIEW"
	// The kinds of the suite's later modules. A reference to one is valid today and resolves as
	// not available until the module that holds it exists.
	KindPage       Kind = "PAGE"
	KindWhiteboard Kind = "WHITEBOARD"
	KindDashboard  Kind = "DASHBOARD"
	KindGoal       Kind = "GOAL"
	KindSprint     Kind = "SPRINT"
)

var kinds = []Kind{
	KindEntry, KindHub, KindCollection, KindView,
	KindPage, KindWhiteboard, KindDashboard, KindGoal, KindSprint,
}

// Kinds are every kind a reference may name.
func Kinds() []Kind { return slices.Clone(kinds) }

// Valid reports whether the kind is one of the known ones.
func (k Kind) Valid() bool { return slices.Contains(kinds, k) }

// Viewable reports whether the kind draws as a live view rather than as a card.
func (k Kind) Viewable() bool {
	return k == KindView || k == KindDashboard || k == KindWhiteboard
}

// Strength is how a reference is drawn where it stands.
type Strength string

const (
	// Mention is an inline link with the object's live title, key and state.
	Mention Strength = "MENTION"
	// Card is the object with its key fields, actionable where the reader may act.
	Card Strength = "CARD"
	// Embed is a view drawn in place, with its own layout.
	Embed Strength = "EMBED"
)

// Valid reports whether the strength is one of the three.
func (s Strength) Valid() bool { return s == Mention || s == Card || s == Embed }

// MaxAnchorLength bounds an anchor: a heading's slug, a block's identifier.
const MaxAnchorLength = 64

var anchorPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Reference points at one object, and optionally at a place inside it.
type Reference struct {
	Kind   Kind
	ID     shared.ID
	Anchor string
}

// New checks a reference's shape. Whether the object exists, and whether the reader may see it,
// is the resolver's question - never this one's, so that a malformed reference and one out of
// reach cannot be told apart by their error.
func New(kind Kind, id shared.ID, anchor string) (Reference, error) {
	if !kind.Valid() {
		return Reference{}, invalid("/kind", "references.kind_unknown")
	}
	if id.IsZero() {
		return Reference{}, invalid("/id", "references.id_required")
	}
	anchor = strings.TrimSpace(anchor)
	if anchor != "" && !anchorPattern.MatchString(anchor) {
		return Reference{}, invalid("/anchor", "references.anchor_malformed")
	}
	return Reference{Kind: kind, ID: id, Anchor: anchor}, nil
}

// StrengthOf is how a reference marked as drawn in place (`!`) or inline is shown: a viewable
// kind in place is a live view, anything else in place a card, and inline always a mention.
func StrengthOf(kind Kind, inPlace bool) Strength {
	switch {
	case !inPlace:
		return Mention
	case kind.Viewable():
		return Embed
	default:
		return Card
	}
}

// Address is the reference as text writes it: `hubtask:<kind>/<id>` with `#<anchor>` when there
// is one. A Markdown link to the address is a mention, an image of it (`![](…)`) a card or a live
// view; the database trigger of migration 0214 reads the same grammar (Extract).
func (r Reference) Address() string {
	address := "hubtask:" + strings.ToLower(string(r.Kind)) + "/" + r.ID.String()
	if r.Anchor != "" {
		address += "#" + r.Anchor
	}
	return address
}

// Found is one reference in a text, with how it is drawn there.
type Found struct {
	Reference Reference
	Strength  Strength
}

var addressPattern = regexp.MustCompile(
	`(!?)\[[^\]]*\]\(hubtask:([a-z]+)/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(?:#([A-Za-z0-9_-]{1,64}))?\)`)

// MaxPerText bounds the references one text holds, so that a pasted list cannot turn one write
// into thousands of rows.
const MaxPerText = 200

// Extract finds the references a Markdown text holds, once each per object and anchor, the
// strongest drawing winning. A malformed address is text, not an error: a note may quote one.
func Extract(markdown string) []Found {
	if !strings.Contains(markdown, "hubtask:") {
		return nil
	}
	var found []Found
	index := map[Reference]int{}
	for _, match := range addressPattern.FindAllStringSubmatch(markdown, -1) {
		kind := Kind(strings.ToUpper(match[2]))
		id, err := shared.ParseID(strings.ToLower(match[3]))
		if err != nil {
			continue
		}
		ref, err := New(kind, id, match[4])
		if err != nil {
			continue
		}
		strength := StrengthOf(kind, match[1] == "!")
		if at, seen := index[ref]; seen {
			if rank(strength) > rank(found[at].Strength) {
				found[at].Strength = strength
			}
			continue
		}
		if len(found) == MaxPerText {
			break
		}
		index[ref] = len(found)
		found = append(found, Found{Reference: ref, Strength: strength})
	}
	return found
}

func rank(s Strength) int {
	switch s {
	case Embed:
		return 2
	case Card:
		return 1
	}
	return 0
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}
