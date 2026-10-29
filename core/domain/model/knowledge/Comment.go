// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The bounds of a comment.
const (
	MaxCommentBytes = 64 << 10
	MaxQuoteLength  = 500
)

var anchorPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Comment is one contribution beside a page: on the page as a whole, or inline on a passage - the
// block it stands at (Anchor) and the words it was written about (Quote), so that it stays
// readable once the passage changes. A reply names its thread's first comment; one level only, as
// an entry's comments (domain-model.md §3.5). Resolving is the thread's, on its first comment.
type Comment struct {
	ID         shared.ID
	TenantID   shared.ID
	PageID     shared.ID
	ParentID   shared.ID
	AuthorID   shared.ID
	Body       string
	Anchor     string
	Quote      string
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
	ID, TenantID, PageID, AuthorID shared.ID
	// Parent is the comment replied to, nil for a new thread.
	Parent *Comment
	Body   string
	Anchor string
	Quote  string
	Now    time.Time
	Text   text.Normalizer
}

// NewComment checks a comment.
func NewComment(in NewCommentInput) (Comment, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.PageID.IsZero() || in.AuthorID.IsZero() {
		return Comment{}, shared.ErrInternal.WithDetail("pages.identity_incomplete")
	}
	body, err := CheckCommentBody(in.Body, in.Text)
	if err != nil {
		return Comment{}, err
	}
	comment := Comment{
		ID: in.ID, TenantID: in.TenantID, PageID: in.PageID, AuthorID: in.AuthorID,
		Body: body, CreatedAt: in.Now, Version: 1,
	}
	if in.Parent != nil {
		switch {
		case in.Parent.PageID != in.PageID:
			return Comment{}, shared.ErrNotFound.WithDetail("pages.comment_not_found")
		case !in.Parent.ParentID.IsZero():
			return Comment{}, invalid("/parent_id", "pages.comment_reply_nested")
		case in.Parent.IsDeleted():
			return Comment{}, shared.ErrNotFound.WithDetail("pages.comment_not_found")
		}
		comment.ParentID = in.Parent.ID
		if in.Anchor != "" || in.Quote != "" {
			return Comment{}, invalid("/anchor", "pages.comment_reply_anchored")
		}
		return comment, nil
	}
	anchor := strings.TrimSpace(in.Anchor)
	if anchor != "" && !anchorPattern.MatchString(anchor) {
		return Comment{}, invalid("/anchor", "pages.comment_anchor_malformed")
	}
	quote, err := shared.NFC(strings.TrimSpace(in.Quote), in.Text)
	if err != nil {
		return Comment{}, err
	}
	if utf8.RuneCountInString(quote) > MaxQuoteLength {
		return Comment{}, invalid("/quote", "pages.comment_quote_too_long").WithParams(map[string]string{"maximum": "500"})
	}
	if quote != "" && anchor == "" {
		return Comment{}, invalid("/anchor", "pages.comment_quote_unanchored")
	}
	comment.Anchor, comment.Quote = anchor, quote
	return comment, nil
}

// CheckCommentBody bounds a comment's words; prose is stored as sent.
func CheckCommentBody(raw string, form text.Normalizer) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", invalid("/body", "pages.comment_empty")
	}
	if len(raw) > MaxCommentBytes {
		return "", invalid("/body", "pages.comment_too_long").WithParams(map[string]string{"maximum": "65536"})
	}
	return shared.NFC(raw, form)
}

// Resolved answers the thread resolved or opened again. Only a thread's first comment resolves.
func (c Comment) Resolved(resolved bool, by shared.ID, now time.Time) (Comment, error) {
	if !c.ParentID.IsZero() {
		return Comment{}, invalid("/resolved", "pages.comment_reply_resolved")
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
