// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	tenant = shared.ID("00000000-0000-0000-0000-0000000000a1")
	hub    = shared.ID("00000000-0000-0000-0000-0000000000b1")
	author = shared.ID("00000000-0000-0000-0000-0000000000c1")
	other  = shared.ID("00000000-0000-0000-0000-0000000000c2")
	pageA  = shared.ID("00000000-0000-0000-0000-0000000000d1")
	pageB  = shared.ID("00000000-0000-0000-0000-0000000000d2")
	pageC  = shared.ID("00000000-0000-0000-0000-0000000000d3")
	now    = time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
)

func newPage(t *testing.T, title, body string) Page {
	t.Helper()
	page, err := NewPage(NewPageInput{
		ID: pageA, TenantID: tenant, HubID: hub, Title: title, Body: body, OrderKey: "a0",
		CreatedBy: author, Now: now,
	})
	if err != nil {
		t.Fatalf("NewPage: %v", err)
	}
	return page
}

func detail(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func TestAPageIsCheckedWhenMade(t *testing.T) {
	cases := []struct {
		name, title, body, icon, language, want string
	}{
		{name: "a page", title: "  Handbook ", body: "# Welcome"},
		{name: "no title", title: "  ", want: "pages.title_empty"},
		{name: "a long title", title: strings.Repeat("x", 201), want: "pages.title_too_long"},
		{name: "two lines", title: "a\nb", want: "pages.title_malformed"},
		{name: "a large body", title: "x", body: strings.Repeat("x", MaxBodyBytes+1), want: "pages.body_too_large"},
		{name: "a zero byte", title: "x", body: "a\x00b", want: "pages.body_malformed"},
		{name: "a long icon", title: "x", icon: strings.Repeat("x", 17), want: "pages.icon_malformed"},
		{name: "a language", title: "x", language: "de-AT"},
		{name: "a bad language", title: "x", language: "not a tag", want: "pages.language_malformed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			page, err := NewPage(NewPageInput{
				ID: pageA, TenantID: tenant, HubID: hub, Title: c.title, Body: c.body, Icon: c.icon,
				ContentLanguage: c.language, OrderKey: "a0", CreatedBy: author, Now: now,
			})
			if got := detail(err); got != c.want {
				t.Fatalf("got %q (%v), want %q", got, err, c.want)
			}
			if c.want == "" && page.Title != strings.TrimSpace(c.title) {
				t.Fatalf("title %q", page.Title)
			}
		})
	}
}

func TestAChangeNamesWhatItChanged(t *testing.T) {
	page := newPage(t, "Handbook", "old")
	title, body, archived := "Handbook", "new", true
	next, fields, err := page.Changed(Change{Title: &title, Body: &body, Archived: &archived}, other, now.Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(fields, ",") != "body,archived" {
		t.Fatalf("fields %v", fields)
	}
	if !next.IsArchived() || next.UpdatedBy != other || next.Body != "new" {
		t.Fatalf("page %+v", next)
	}
	same, fields, _ := next.Changed(Change{Body: &body}, author, now, nil)
	if len(fields) != 0 || same.UpdatedBy != other {
		t.Fatalf("a change that changes nothing moved the page: %v", fields)
	}
}

func TestAParentIsJudged(t *testing.T) {
	parent := Page{ID: pageB, HubID: hub}
	cases := []struct {
		name      string
		page      shared.ID
		parent    Page
		ancestors []shared.ID
		want      string
	}{
		{name: "a sibling", page: pageA, parent: parent},
		{name: "itself", page: pageB, parent: parent, want: "pages.parent_cycle"},
		{name: "under itself", page: pageA, parent: parent, ancestors: []shared.ID{pageC, pageA}, want: "pages.parent_cycle"},
		{name: "another space", page: pageA, parent: Page{ID: pageB, HubID: tenant}, want: "pages.parent_other_space"},
		{name: "too deep", page: pageA, parent: parent, ancestors: make([]shared.ID, MaxDepth-1), want: "pages.too_deep"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detail(CheckParent(c.page, c.parent, c.ancestors, hub)); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestAPageIsDueForReviewWhenItsVerificationRunsOut(t *testing.T) {
	until := now.Add(24 * time.Hour)
	page := Page{VerifiedUntil: &until}
	if page.DueForReview(now) || !page.DueForReview(until) {
		t.Fatal("due for review judged wrong")
	}
	if detail(CheckVerification(&now, now.Add(time.Second))) != "pages.verified_until_past" {
		t.Fatal("a verification in the past was taken")
	}
}

func TestTheReferencesOfAPageAreItsBodys(t *testing.T) {
	page := newPage(t, "Plan", "See [the task](hubtask:entry/00000000-0000-0000-0000-0000000000e1) and "+
		"![board](hubtask:view/00000000-0000-0000-0000-0000000000e2)")
	found := page.References()
	if len(found) != 2 || found[0].Strength != "MENTION" || found[1].Strength != "EMBED" {
		t.Fatalf("references %+v", found)
	}
}

func TestWritesOfOneSessionFoldIntoOneVersion(t *testing.T) {
	page := newPage(t, "Plan", "one")
	first, fold := Snapshot(nil, page, author, now)
	if fold || first.Seq != 1 {
		t.Fatalf("first %+v fold %v", first, fold)
	}
	cases := []struct {
		name    string
		last    Version
		author  shared.ID
		at      time.Time
		fold    bool
		wantSeq int
	}{
		{name: "the same session", last: first, author: author, at: now.Add(5 * time.Minute), fold: true, wantSeq: 1},
		{name: "after a pause", last: first, author: author, at: now.Add(SessionGap), wantSeq: 2},
		{name: "somebody else", last: first, author: other, at: now.Add(time.Minute), wantSeq: 2},
		{name: "a named version", last: Version{Seq: 3, AuthorID: author, Name: "Sent", CreatedAt: now}, author: author, at: now, wantSeq: 4},
		{name: "a restore", last: Version{Seq: 3, AuthorID: author, Restored: 1, CreatedAt: now}, author: author, at: now, wantSeq: 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			last := c.last
			got, fold := Snapshot(&last, page, c.author, c.at)
			if fold != c.fold || got.Seq != c.wantSeq {
				t.Fatalf("got seq %d fold %v", got.Seq, fold)
			}
		})
	}
}

func TestACommentIsCheckedWhenMade(t *testing.T) {
	thread := Comment{ID: pageC, PageID: pageA}
	reply := Comment{ID: pageB, PageID: pageA, ParentID: pageC}
	cases := []struct {
		name         string
		parent       *Comment
		body, anchor string
		quote, want  string
	}{
		{name: "on the page", body: "Looks good"},
		{name: "inline", body: "Why?", anchor: "block-12", quote: "the budget"},
		{name: "empty", body: "  ", want: "pages.comment_empty"},
		{name: "a quote without a place", body: "x", quote: "words", want: "pages.comment_quote_unanchored"},
		{name: "a malformed anchor", body: "x", anchor: "a b", want: "pages.comment_anchor_malformed"},
		{name: "a reply", parent: &thread, body: "Agreed"},
		{name: "a reply to a reply", parent: &reply, body: "x", want: "pages.comment_reply_nested"},
		{name: "a reply anchored", parent: &thread, body: "x", anchor: "b1", want: "pages.comment_reply_anchored"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewComment(NewCommentInput{
				ID: other, TenantID: tenant, PageID: pageA, AuthorID: author, Parent: c.parent,
				Body: c.body, Anchor: c.anchor, Quote: c.quote, Now: now,
			})
			if got := detail(err); got != c.want {
				t.Fatalf("got %q (%v), want %q", got, err, c.want)
			}
		})
	}
	if _, err := reply.Resolved(true, author, now); detail(err) != "pages.comment_reply_resolved" {
		t.Fatal("a reply resolved its thread")
	}
	resolved, _ := thread.Resolved(true, author, now)
	if resolved.ResolvedAt == nil || resolved.ResolvedBy != author {
		t.Fatal("the thread is not resolved")
	}
}

func TestATreeIsExportedAsFoldersOfMarkdown(t *testing.T) {
	root := Page{ID: pageA, Title: "Handbook", Body: "# Welcome"}
	child := Page{ID: pageB, ParentID: pageA, Title: "On/boarding?", Body: "Day one"}
	twin := Page{ID: pageC, ParentID: pageA, Title: "onboarding"}
	files := Tree([]Page{root, child, twin}, pageA.String())
	if len(files) != 3 {
		t.Fatalf("files %+v", files)
	}
	if files[0].Path != "Handbook.md" || files[1].Path != "Handbook/Onboarding.md" || files[2].Path != "Handbook/onboarding-2.md" {
		t.Fatalf("paths %s %s %s", files[0].Path, files[1].Path, files[2].Path)
	}
	if !strings.Contains(files[1].Content, "parent: "+pageA.String()) || !strings.HasSuffix(files[1].Content, "Day one\n") {
		t.Fatalf("content %q", files[1].Content)
	}
}
