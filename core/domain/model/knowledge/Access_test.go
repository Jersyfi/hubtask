// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestARestrictionAdmitsOnlyThePeopleEveryLevelNames(t *testing.T) {
	third := shared.ID("00000000-0000-0000-0000-0000000000c3")
	cases := []struct {
		name  string
		sets  [][]shared.ID
		admit map[shared.ID]bool
	}{
		{"unrestricted", nil, map[shared.ID]bool{author: true, other: true}},
		{"one level", [][]shared.ID{{author, other}}, map[shared.ID]bool{author: true, other: true, third: false}},
		{"narrowed below", [][]shared.ID{{author}, {author, other}}, map[shared.ID]bool{author: true, other: false}},
		{"never widened", [][]shared.ID{{author, other, third}, {other}}, map[shared.ID]bool{author: false, other: true, third: false}},
		{"nobody left", [][]shared.ID{{}}, map[shared.ID]bool{author: false, other: false}},
		{"disjoint", [][]shared.ID{{author}, {other}}, map[shared.ID]bool{author: false, other: false}},
	}
	for _, c := range cases {
		readers := CombineRestrictions(c.sets)
		for account, want := range c.admit {
			if got := readers.Admits(account); got != want {
				t.Errorf("%s: Admits(%s) = %v, want %v", c.name, account, got, want)
			}
		}
	}
}

func TestARestrictionKeepsTheOneRestricting(t *testing.T) {
	if _, err := CheckRestriction([]shared.ID{other}, author); detail(err) != "pages.restriction_excludes_you" {
		t.Errorf("a restriction without its author answered %v", err)
	}
	got, err := CheckRestriction([]shared.ID{author, other, author, ""}, author)
	if err != nil || len(got) != 2 {
		t.Errorf("repeats and blanks: %v %v", got, err)
	}
	many := []shared.ID{author}
	for i := 0; i < MaxRestrictedTo; i++ {
		many = append(many, shared.ID(time.Unix(int64(i), 0).UTC().Format(time.RFC3339Nano)))
	}
	if _, err := CheckRestriction(many, author); detail(err) != "pages.restriction_too_large" {
		t.Errorf("an oversized restriction answered %v", err)
	}
}

func TestAShareNeverPassesARestriction(t *testing.T) {
	restricted := CombineRestrictions([][]shared.ID{{author}})
	if _, err := NewShare(tenant, pageA, other, author, restricted, now); detail(err) != "pages.share_restricted" {
		t.Errorf("a share past a restriction answered %v", err)
	}
	if _, err := NewShare(tenant, pageA, other, author, Readers{}, now); err != nil {
		t.Errorf("a share of an open page: %v", err)
	}
	if _, err := NewShare(tenant, pageA, "", author, Readers{}, now); detail(err) != "pages.share_account_missing" {
		t.Errorf("a share with nobody answered %v", err)
	}
}

func TestADailyNoteIsForTheReadersToday(t *testing.T) {
	late := time.Date(2026, 10, 29, 23, 30, 0, 0, time.UTC)
	vienna, err := time.LoadLocation("Europe/Vienna")
	if err != nil {
		t.Skip("no zone database")
	}
	cases := []struct {
		raw  string
		zone *time.Location
		want string
		code string
	}{
		{"", time.UTC, "2026-10-29", ""},
		{"", vienna, "2026-10-30", ""},
		{"2026-10-01", vienna, "2026-10-01", ""},
		{"29.10.2026", vienna, "", "pages.day_malformed"},
		{"1999-01-01", vienna, "", "pages.day_out_of_range"},
	}
	for _, c := range cases {
		day, err := DailyNoteDay(c.raw, late, c.zone)
		if c.code != "" {
			if detail(err) != c.code {
				t.Errorf("%q answered %v, want %s", c.raw, err, c.code)
			}
			continue
		}
		if err != nil || DailyNoteTitle(day) != c.want {
			t.Errorf("%q in %v: %v %v, want %s", c.raw, c.zone, DailyNoteTitle(day), err, c.want)
		}
	}
}

// UC-DOC-08 check 5: a public link is made for an open page and never for one behind a
// restriction; the workspace's switch is off by default and a plan may lock it.
func TestAPublicLinkIsNeverMadePastARestriction(t *testing.T) {
	tenant, page, by := shared.ID("t"), shared.ID("p"), shared.ID("a")
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	link, err := NewPublicLink(tenant, page, by, Readers{}, now)
	if err != nil || link.PageID != page || link.CreatedBy != by || !link.CreatedAt.Equal(now) {
		t.Fatalf("an open page's link: %+v, %v", link, err)
	}
	if _, err := NewPublicLink(tenant, page, by, Readers{Restricted: true, Accounts: []shared.ID{by}}, now); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a restricted page got a link: %v", err)
	}
	definition := PublicLinksDefinition()
	if err := definition.Validate(); err != nil {
		t.Fatal(err)
	}
	if PublicLinksOf(setting.Resolve(definition)) {
		t.Fatal("public links are on by default")
	}
	if !definition.Lockable {
		t.Fatal("a plan cannot lock public links")
	}
}
