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
	postNow = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	groupA  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c001")
	groupB  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c002")
)

func at(hours int) *time.Time { t := postNow.Add(time.Duration(hours) * time.Hour); return &t }

// B-1, B-3, B-5, B-6: what a writer asks of a post is checked - a known template, groups exactly
// for a narrowed audience, and a feature and a confirmation ending after the post goes out.
func TestAPostsSettingsAreCheckedAsAskedFor(t *testing.T) {
	cases := []struct {
		name string
		in   PostSettings
		code string
	}{
		{"published now for the hub", PostSettings{Template: PostAnnouncement}, ""},
		{"scheduled, featured and confirmed after it", PostSettings{PublishAt: at(24), FeaturedUntil: at(48), ConfirmBy: at(72)}, ""},
		{"narrowed to groups", PostSettings{Audience: AudienceGroups, GroupIDs: []shared.ID{groupA, groupA, groupB}}, ""},
		{"an unknown template", PostSettings{Template: "MEMO"}, "posts.template_unknown"},
		{"an unknown audience", PostSettings{Audience: "WORLD"}, "posts.audience_unknown"},
		{"groups without narrowing", PostSettings{GroupIDs: []shared.ID{groupA}}, "posts.groups_without_audience"},
		{"narrowed to nobody", PostSettings{Audience: AudienceGroups}, "posts.groups_count"},
		{"featured until before it goes out", PostSettings{PublishAt: at(24), FeaturedUntil: at(12)}, "posts.featured_until_past"},
		{"a confirmation due in the past", PostSettings{ConfirmBy: at(-1)}, "posts.confirm_by_past"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewPostSettings(c.in, postNow)
			if c.code == "" {
				if err != nil || got.Audience == "" {
					t.Fatalf("NewPostSettings = %+v, %v", got, err)
				}
				return
			}
			var e *shared.Error
			if !errors.As(err, &e) || e.DetailCode != c.code {
				t.Fatalf("NewPostSettings answered %v, want %s", err, c.code)
			}
		})
	}
	got, _ := NewPostSettings(PostSettings{Audience: AudienceGroups, GroupIDs: []shared.ID{groupB, groupA, groupB}}, postNow)
	if len(got.GroupIDs) != 2 {
		t.Fatalf("the groups were not kept once each: %v", got.GroupIDs)
	}
}

// B-3: a post without a moment is a draft, one ahead is scheduled, and the scheduler publishes it
// once its moment has come - at the moment asked for, not when it ran.
func TestAPostIsScheduledAndPublishedAtItsMoment(t *testing.T) {
	draft := Post{}
	if draft.State(postNow) != PostDraft || draft.IsDue(postNow) {
		t.Fatal("a draft is due")
	}
	scheduled := Post{PublishAt: at(2)}
	if scheduled.State(postNow) != PostScheduled || scheduled.IsDue(postNow) {
		t.Fatal("a scheduled post is due before its moment")
	}
	later := postNow.Add(3 * time.Hour)
	if !scheduled.IsDue(later) {
		t.Fatal("a scheduled post is not due after its moment")
	}
	published := scheduled.Published(later)
	if published.PublishedAt == nil || !published.PublishedAt.Equal(*at(2)) || published.IsDue(later) || published.State(postNow) != PostPublished {
		t.Fatalf("published: %+v", published.PublishedAt)
	}
	now := Post{}.Published(postNow)
	if now.PublishAt == nil || !now.PublishedAt.Equal(postNow) {
		t.Fatal("publishing now names no moment")
	}
	featured := Post{PublishedAt: at(-1), FeaturedUntil: at(1)}
	if !featured.IsFeatured(postNow) || featured.IsFeatured(postNow.Add(2*time.Hour)) {
		t.Fatal("a post is featured outside its window")
	}
}

// B-3, B-6: who sees a post - never somebody who may not read the hub; a draft or a scheduled post
// only the hub's writers; a narrowed post its groups and the writers.
func TestTheAudienceOnlyNarrows(t *testing.T) {
	published := Post{PublishedAt: at(-1)}
	narrowed := Post{PublishedAt: at(-1), Audience: AudienceGroups, GroupIDs: []shared.ID{groupA}}
	scheduled := Post{PublishAt: at(2)}
	reader := Reader{ReadsHub: true}
	inA := Reader{ReadsHub: true, Groups: []shared.ID{groupA}}
	writer := Reader{ReadsHub: true, WritesHub: true}
	outsider := Reader{Groups: []shared.ID{groupA}}
	cases := []struct {
		name   string
		post   Post
		reader Reader
		sees   bool
	}{
		{"a reader, a published post", published, reader, true},
		{"somebody outside the hub", published, outsider, false},
		{"outside the hub, though in the group", narrowed, outsider, false},
		{"a reader outside the groups", narrowed, reader, false},
		{"a reader in a group", narrowed, inA, true},
		{"a writer, a narrowed post", narrowed, writer, true},
		{"a reader, a scheduled post", scheduled, reader, false},
		{"a writer, a scheduled post", scheduled, writer, true},
		{"a reader, a draft", Post{}, inA, false},
	}
	for _, c := range cases {
		if got := c.post.Sees(c.reader, postNow); got != c.sees {
			t.Errorf("%s: Sees = %v", c.name, got)
		}
	}
}

// B-4, B-5: a reply takes only the few fixed reactions; a writer may remind while somebody has
// not confirmed, and the confirmation is overdue past its deadline.
func TestReactionsAndConfirmations(t *testing.T) {
	if _, err := NewReaction("THANKS"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewReaction("ANGRY"); err == nil {
		t.Fatal("an unknown reaction was taken")
	}
	open := Confirmations{Confirmed: 21, Audience: 45, ConfirmBy: postNow}
	if !open.MayRemind() || open.IsOverdue(postNow) || !open.IsOverdue(postNow.Add(time.Minute)) {
		t.Fatalf("open: %+v", open)
	}
	all := Confirmations{Confirmed: 45, Audience: 45, ConfirmBy: postNow}
	if all.MayRemind() || all.IsOverdue(postNow.Add(time.Hour)) {
		t.Fatal("everybody confirmed, yet a reminder is offered")
	}
}

func TestAPostsListShowsItsOpeningAndReadingTime(t *testing.T) {
	long := strings.Repeat("word ", 80)
	cases := []struct {
		name, lead, want string
	}{
		{"marks go", "## Was neu ist\n\nAb dem **1. Dezember** gilt [das](https://x.example).", "Was neu ist Ab dem 1. Dezember gilt das."},
		{"an escaped mark is its character", "Ab dem **1\\. Dezember** \\(neu\\) \\- ab 9\\:00.", "Ab dem 1. Dezember (neu) - ab 9:00."},
		{"empty stays empty", "", ""},
		{"cut at a word", long, strings.TrimSpace(strings.Repeat("word ", 47)) + "…"},
	}
	for _, c := range cases {
		if got := PostExcerpt(c.lead); got != c.want {
			t.Errorf("%s: PostExcerpt = %q, want %q", c.name, got, c.want)
		}
	}
	for words, want := range map[int]int{0: 1, 99: 1, 300: 2, 1000: 5} {
		if got := ReadingMinutes(words); got != want {
			t.Errorf("ReadingMinutes(%d) = %d, want %d", words, got, want)
		}
	}
}
