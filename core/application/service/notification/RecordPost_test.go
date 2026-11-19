// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var announcedPost = shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")

// UC-DOC-17 check 5 and UC-DOC-18 check 2 at the recording: each person told gets one POST record
// naming the post's page, its delivery queued under its own kind; a publication names nobody, a
// reminder the writer, who is never told of their own reminder; a person who switched posts off is
// recorded as suppressed and nothing is queued for them.
func TestAPostIsRecordedForItsAudience(t *testing.T) {
	for _, tc := range []struct {
		name  string
		remit func(RecordPost, []shared.ID) error
		actor shared.ID
	}{
		{"published", func(r RecordPost, to []shared.ID) error {
			return r.PostPublished(t.Context(), tenant, knowledge.Post{PageID: announcedPost}, to)
		}, ""},
		{"reminded", func(r RecordPost, to []shared.ID) error {
			return r.PostReminder(t.Context(), tenant, anna, knowledge.Post{PageID: announcedPost}, to)
		}, anna},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notifications, jobs, preferences := newNotifications(), &jobQueue{}, newPreferences()
			carl := shared.ID("01936f2a-7c1e-7000-8000-0000000000c9")
			preferences.switchOff(carl, domain.CategoryPost)
			recorder := RecordPost{
				Notifications: notifications,
				Accounts: newAccounts(
					person(anna, "Anna", "anna@example.org", "en"),
					person(bert, "Bert", "bert@example.org", "de"),
					person(carl, "Carl", "carl@example.org", "en"),
				),
				Preferences: preferences, Jobs: jobs, Clock: clock.Fixed(now), IDs: &idSequence{},
			}
			if err := tc.remit(recorder, []shared.ID{anna, bert, carl}); err != nil {
				t.Fatalf("recording: %v", err)
			}
			written := notifications.written()
			sent, suppressed := 0, 0
			for _, record := range written {
				if record.Category != domain.CategoryPost || record.PageID != announcedPost || record.ActorID != tc.actor {
					t.Fatalf("record %+v", record)
				}
				if record.RecipientID == tc.actor && !tc.actor.IsZero() {
					t.Fatalf("the writer was told of their own reminder")
				}
				switch record.State {
				case domain.StatePending:
					sent++
				case domain.StateSuppressed:
					suppressed++
					if record.RecipientID != carl {
						t.Fatalf("suppressed %+v", record)
					}
				}
			}
			want := 2
			if !tc.actor.IsZero() {
				want = 1
			}
			if sent != want || suppressed != 1 {
				t.Fatalf("%d pending, %d suppressed; want %d and 1", sent, suppressed, want)
			}
			if len(jobs.requests) != want || jobs.requests[0].Kind != queue.KindPostEmail {
				t.Fatalf("queued %+v", jobs.requests)
			}
		})
	}
}

// At the delivery: a publication's mail names the post and links to it as a post; a reminder's
// names the writer who reminds.
func TestThePostMailNamesThePost(t *testing.T) {
	for _, tc := range []struct {
		name  string
		actor shared.ID
		body  string
	}{
		{"published", "", bodyPostPublished},
		{"reminded", anna, bodyPostReminder},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := delivery(t, domain.CategoryPost, true)
			record, err := domain.New(domain.NewInput{
				ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000c8"), TenantID: tenant, RecipientID: bert,
				Category: domain.CategoryPost, Channel: domain.ChannelEmail, PageID: announcedPost, ActorID: tc.actor, At: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.notifications.Insert(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			fixture.delivery.Pages = pageStore{page: knowledge.Page{ID: announcedPost, TenantID: tenant, Title: "New ticket system"}}
			if err := fixture.delivery.Execute(t.Context(), tenant, record.ID, false); err != nil {
				t.Fatalf("delivering: %v", err)
			}
			if len(fixture.mailbox.sent) != 1 {
				t.Fatalf("%d messages sent, want one", len(fixture.mailbox.sent))
			}
			body := fixture.mailbox.sent[0].Body
			if !strings.HasPrefix(body, tc.body+" ") || !strings.Contains(body, "/posts/"+announcedPost.String()) ||
				!strings.Contains(body, "title=New ticket system") {
				t.Errorf("the body %q, want %s naming the post and linking to it", body, tc.body)
			}
		})
	}
}
