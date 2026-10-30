// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The bounds of a comment.
const (
	MaxCommentBytes = 64 << 10
	// MaxMentions bounds the people one comment tells, so that a pasted list cannot turn one
	// comment into a mail to the whole workspace.
	MaxMentions = 20
)

// Comment is one contribution on a board (UC-CNV-09). A thread is pinned: at a place on the board
// (X, Y) and, when it is about an object, at that object (Element) - the pin follows the object
// while it stands and stays at its place once it is gone. A reply names its thread's first
// comment, one level only, as a page's comments; resolving is the thread's.
type Comment struct {
	ID         shared.ID
	TenantID   shared.ID
	BoardID    shared.ID
	ParentID   shared.ID
	AuthorID   shared.ID
	Body       string
	Element    string
	X, Y       float64
	ResolvedAt *time.Time
	ResolvedBy shared.ID
	CreatedAt  time.Time
	EditedAt   *time.Time
	// DeletedAt marks a comment taken back: its row stays so the thread reads on, its words go.
	DeletedAt *time.Time
	Version   int
}

// IsDeleted reports whether the comment was taken back.
func (c Comment) IsDeleted() bool { return c.DeletedAt != nil }

// NewCommentInput is what a comment is made from.
type NewCommentInput struct {
	ID, TenantID, BoardID, AuthorID shared.ID
	// Parent is the comment replied to, nil for a new thread.
	Parent  *Comment
	Body    string
	Element string
	X, Y    float64
	Now     time.Time
	Text    text.Normalizer
}

// NewComment checks a comment. A reply carries no pin of its own: it stands in its thread.
func NewComment(in NewCommentInput) (Comment, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.BoardID.IsZero() || in.AuthorID.IsZero() {
		return Comment{}, shared.ErrInternal.WithDetail("whiteboards.identity_incomplete")
	}
	body, err := CheckCommentBody(in.Body, in.Text)
	if err != nil {
		return Comment{}, err
	}
	comment := Comment{
		ID: in.ID, TenantID: in.TenantID, BoardID: in.BoardID, AuthorID: in.AuthorID,
		Body: body, CreatedAt: in.Now, Version: 1,
	}
	if in.Parent != nil {
		switch {
		case in.Parent.BoardID != in.BoardID, in.Parent.IsDeleted():
			return Comment{}, shared.ErrNotFound.WithDetail("whiteboards.comment_not_found")
		case !in.Parent.ParentID.IsZero():
			return Comment{}, invalid("/parent_id", "whiteboards.comment_reply_nested")
		case in.Element != "" || in.X != 0 || in.Y != 0:
			return Comment{}, invalid("/element_id", "whiteboards.comment_reply_pinned")
		}
		comment.ParentID = in.Parent.ID
		return comment, nil
	}
	element := strings.TrimSpace(in.Element)
	if element != "" && !elementID.MatchString(element) {
		return Comment{}, invalid("/element_id", "whiteboards.comment_pin_malformed")
	}
	for _, value := range []float64{in.X, in.Y} {
		if math.IsNaN(value) || math.Abs(value) > MaxCoordinate {
			return Comment{}, invalid("/x", "whiteboards.comment_pin_malformed")
		}
	}
	comment.Element, comment.X, comment.Y = element, in.X, in.Y
	return comment, nil
}

// CheckCommentBody bounds a comment's words; prose is stored as sent.
func CheckCommentBody(raw string, form text.Normalizer) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", invalid("/body", "whiteboards.comment_empty")
	}
	if len(raw) > MaxCommentBytes {
		return "", invalid("/body", "whiteboards.comment_too_long").WithParams(map[string]string{"maximum": "65536"})
	}
	return shared.NFC(raw, form)
}

// Resolved answers the thread resolved or opened again. Only a thread's first comment resolves.
func (c Comment) Resolved(resolved bool, by shared.ID, now time.Time) (Comment, error) {
	if !c.ParentID.IsZero() {
		return Comment{}, invalid("/resolved", "whiteboards.comment_reply_resolved")
	}
	next := c
	if resolved {
		at := now
		next.ResolvedAt, next.ResolvedBy = &at, by
	} else {
		next.ResolvedAt, next.ResolvedBy = nil, ""
	}
	return next, nil
}

var mentionPattern = regexp.MustCompile(
	`\[[^\]]*\]\(hubtask:person/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})\)`)

// Mentions are the people a comment names - `[@Name](hubtask:person/<account>)` - once each, in
// the order written, at most MaxMentions. A malformed address is text, not an error.
func Mentions(body string) []shared.ID {
	if !strings.Contains(body, "hubtask:person/") {
		return nil
	}
	var out []shared.ID
	for _, match := range mentionPattern.FindAllStringSubmatch(body, -1) {
		id, err := shared.ParseID(strings.ToLower(match[1]))
		if err != nil || slices.Contains(out, id) {
			continue
		}
		if len(out) == MaxMentions {
			break
		}
		out = append(out, id)
	}
	return out
}

// NewlyMentioned are the people a rewritten comment names that its earlier words did not: only
// they are told again.
func NewlyMentioned(before, after string) []shared.ID {
	earlier := Mentions(before)
	var out []shared.ID
	for _, id := range Mentions(after) {
		if !slices.Contains(earlier, id) {
			out = append(out, id)
		}
	}
	return out
}
