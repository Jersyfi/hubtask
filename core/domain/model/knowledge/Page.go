// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package knowledge is the suite's knowledge module (M3): a hub's pages, the tree they form, their
// versions and the comments beside them.
//
// A hub is a space (products.md, M3): its pages form a tree under it and inherit its memberships,
// so a private hub is a private space and ADR-0073 holds for its pages unchanged. A page is not an
// entry - it has no level, no completion and no status - but it is an object every reference can
// point at (P-19): `hubtask:page/<id>`.
//
// The body is the page's readable projection in Markdown, the grammar an entry's notes already use
// for mentions, cards and live views (reference.Extract). While several people edit at once the
// live document (realtime, kind `page`) is the editors' CRDT; the editor saves the projection
// beside it, and the projection is what search, versions, references and export read - the server
// never reads a CRDT update (offline-sync.md §13).
package knowledge

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The bounds of a page.
const (
	MaxTitleLength = 200
	// MaxBodyBytes is the projection of a large handbook page (research: 1 MB of text, 2,000
	// blocks), and the bound of one write.
	MaxBodyBytes  = 1 << 20
	MaxIconLength = 16
	// MaxDepth bounds the tree, so that a breadcrumb and a move's cycle check stay a short walk.
	MaxDepth = 12
	// MaxPagesPerHub bounds one space's tree, which a client reads whole.
	MaxPagesPerHub = 5000
)

// DocumentKind is the live document kind a page registers (realtime.Kind).
const DocumentKind = "page"

// Page is one page of a hub's space.
type Page struct {
	ID       shared.ID
	TenantID shared.ID
	HubID    shared.ID
	// ParentID is the page this one sits under, empty for a page at the top of the space.
	ParentID shared.ID
	Title    string
	Body     string
	Icon     string
	OrderKey string
	// Template marks a page the space offers as the start of new ones (UC-DOC-05).
	Template bool
	// ContentLanguage is the BCP-47 tag search stems the page by; empty is the workspace's.
	ContentLanguage string
	// OwnerID and VerifiedUntil are the page's verification: who answers for it and until when it
	// is known to be right. Past VerifiedUntil the page is shown as due for review.
	OwnerID       shared.ID
	VerifiedUntil *time.Time
	VerifiedAt    *time.Time
	VerifiedBy    shared.ID
	// Restricted marks a page that names who of its hub's readers may read it (Readers); the
	// people are kept beside the page.
	Restricted bool
	// DailyFor is the day a daily note of a personal space is for (UC-DOC-07); nil for any other
	// page.
	DailyFor   *time.Time
	ArchivedAt *time.Time
	DeletedAt  *time.Time
	CreatedBy  shared.ID
	CreatedAt  time.Time
	UpdatedBy  shared.ID
	UpdatedAt  time.Time
	Version    int
}

// IsArchived reports whether the page is archived: hidden from the tree's default and from search,
// readable and restorable.
func (p Page) IsArchived() bool { return p.ArchivedAt != nil }

// IsTrashed reports whether the page is in the trash.
func (p Page) IsTrashed() bool { return p.DeletedAt != nil }

// DueForReview reports whether the page's verification has run out.
func (p Page) DueForReview(now time.Time) bool {
	return p.VerifiedUntil != nil && !now.Before(*p.VerifiedUntil)
}

// References are the objects the body points at, the strongest drawing winning.
func (p Page) References() []reference.Found { return reference.Extract(p.Body) }

// NewPageInput is what a new page is made from.
type NewPageInput struct {
	ID, TenantID, HubID, ParentID shared.ID
	Title, Body, Icon             string
	OrderKey                      string
	Template                      bool
	ContentLanguage               string
	CreatedBy                     shared.ID
	Now                           time.Time
	Text                          text.Normalizer
}

// NewPage checks a new page.
func NewPage(in NewPageInput) (Page, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.HubID.IsZero() || in.CreatedBy.IsZero() {
		return Page{}, shared.ErrInternal.WithDetail("pages.identity_incomplete")
	}
	title, err := CheckTitle(in.Title, in.Text)
	if err != nil {
		return Page{}, err
	}
	body, err := CheckBody(in.Body, in.Text)
	if err != nil {
		return Page{}, err
	}
	icon, err := checkIcon(in.Icon)
	if err != nil {
		return Page{}, err
	}
	language, err := checkLanguage(in.ContentLanguage)
	if err != nil {
		return Page{}, err
	}
	key := in.OrderKey
	if key == "" {
		return Page{}, shared.ErrInternal.WithDetail("pages.identity_incomplete")
	}
	if in.ParentID == in.ID {
		return Page{}, invalid("/parent_id", "pages.parent_cycle")
	}
	return Page{
		ID: in.ID, TenantID: in.TenantID, HubID: in.HubID, ParentID: in.ParentID,
		Title: title, Body: body, Icon: icon, OrderKey: key, Template: in.Template,
		ContentLanguage: language, CreatedBy: in.CreatedBy, CreatedAt: in.Now,
		UpdatedBy: in.CreatedBy, UpdatedAt: in.Now, Version: 1,
	}, nil
}

// Change is what a write changes; a nil member is left alone.
type Change struct {
	Title           *string
	Body            *string
	Icon            *string
	Template        *bool
	ContentLanguage *string
	Archived        *bool
}

// The fields a change names, as the trail records them.
const (
	FieldTitle    = "title"
	FieldBody     = "body"
	FieldIcon     = "icon"
	FieldTemplate = "template"
	FieldLanguage = "content_language"
	FieldArchived = "archived"
)

// Changed answers the page after the change and the fields it changed. A change that changes
// nothing answers no fields, and the caller writes nothing.
func (p Page) Changed(change Change, by shared.ID, now time.Time, form text.Normalizer) (Page, []string, error) {
	next, fields := p, []string{}
	if change.Title != nil {
		title, err := CheckTitle(*change.Title, form)
		if err != nil {
			return Page{}, nil, err
		}
		if title != p.Title {
			next.Title, fields = title, append(fields, FieldTitle)
		}
	}
	if change.Body != nil {
		body, err := CheckBody(*change.Body, form)
		if err != nil {
			return Page{}, nil, err
		}
		if body != p.Body {
			next.Body, fields = body, append(fields, FieldBody)
		}
	}
	if change.Icon != nil {
		icon, err := checkIcon(*change.Icon)
		if err != nil {
			return Page{}, nil, err
		}
		if icon != p.Icon {
			next.Icon, fields = icon, append(fields, FieldIcon)
		}
	}
	if change.Template != nil && *change.Template != p.Template {
		next.Template, fields = *change.Template, append(fields, FieldTemplate)
	}
	if change.ContentLanguage != nil {
		language, err := checkLanguage(*change.ContentLanguage)
		if err != nil {
			return Page{}, nil, err
		}
		if language != p.ContentLanguage {
			next.ContentLanguage, fields = language, append(fields, FieldLanguage)
		}
	}
	if change.Archived != nil && *change.Archived != p.IsArchived() {
		next.ArchivedAt = nil
		if *change.Archived {
			at := now
			next.ArchivedAt = &at
		}
		fields = append(fields, FieldArchived)
	}
	if len(fields) > 0 {
		next.UpdatedBy, next.UpdatedAt = by, now
	}
	return next, fields, nil
}

// CheckTitle brings a title to its stored form: normal form C, trimmed, one line, bounded.
func CheckTitle(raw string, form text.Normalizer) (string, error) {
	title, err := shared.NFC(strings.TrimSpace(raw), form)
	if err != nil {
		return "", err
	}
	switch {
	case title == "":
		return "", invalid("/title", "pages.title_empty")
	case utf8.RuneCountInString(title) > MaxTitleLength:
		return "", invalid("/title", "pages.title_too_long").WithParams(map[string]string{"maximum": "200"})
	case strings.ContainsAny(title, "\r\n"):
		return "", invalid("/title", "pages.title_malformed")
	}
	return title, nil
}

// CheckBody brings a body to its stored form: normal form C, bounded, and holding no more
// references than one text may (reference.MaxPerText). It is prose and stored as sent otherwise.
func CheckBody(raw string, form text.Normalizer) (string, error) {
	if len(raw) > MaxBodyBytes {
		return "", invalid("/body", "pages.body_too_large").WithParams(map[string]string{"maximum": "1048576"})
	}
	body, err := shared.NFC(raw, form)
	if err != nil {
		return "", err
	}
	if !utf8.ValidString(body) || strings.ContainsRune(body, 0) {
		return "", invalid("/body", "pages.body_malformed")
	}
	return body, nil
}

func checkIcon(raw string) (string, error) {
	icon := strings.TrimSpace(raw)
	if utf8.RuneCountInString(icon) > MaxIconLength || strings.ContainsAny(icon, "\r\n") {
		return "", invalid("/icon", "pages.icon_malformed")
	}
	return icon, nil
}

func checkLanguage(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	tag, ok := shared.LanguageTag(raw)
	if !ok {
		return "", invalid("/content_language", "pages.language_malformed")
	}
	return tag, nil
}

// CheckParent judges a new parent for a page: in the same space, not trashed, not the page itself
// or one under it, and not deeper than the tree allows. ancestors are the parent's own, nearest
// first, ending at the top of the space.
func CheckParent(page shared.ID, parent Page, ancestors []shared.ID, hubID shared.ID) error {
	if parent.HubID != hubID {
		return invalid("/parent_id", "pages.parent_other_space")
	}
	if parent.IsTrashed() {
		return shared.ErrNotFound.WithDetail("pages.not_found")
	}
	if parent.ID == page {
		return invalid("/parent_id", "pages.parent_cycle")
	}
	for _, ancestor := range ancestors {
		if ancestor == page {
			return invalid("/parent_id", "pages.parent_cycle")
		}
	}
	if len(ancestors)+2 > MaxDepth {
		return invalid("/parent_id", "pages.too_deep").WithParams(map[string]string{"maximum": "12"})
	}
	return nil
}

// CheckVerification judges a verification: a date not in the past.
func CheckVerification(until *time.Time, now time.Time) error {
	if until != nil && until.Before(now) {
		return invalid("/verified_until", "pages.verified_until_past")
	}
	return nil
}

func invalid(path, code string) *shared.Error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}
