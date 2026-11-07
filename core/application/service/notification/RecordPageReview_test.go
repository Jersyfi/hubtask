// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var reviewedPage = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")

func duePage(owner, verifier shared.ID) knowledge.Page {
	until := now.Add(-time.Minute)
	return knowledge.Page{
		ID: reviewedPage, TenantID: tenant, Title: "VPN setup",
		OwnerID: owner, VerifiedBy: verifier, VerifiedUntil: &until,
	}
}

// UC-DOC-12 check 4 at the recording: the page's owner gets one REMINDER record naming the page,
// its delivery queued under its own kind; without an owner the verifier is told, without either
// nobody.
func TestAPageDueForReviewIsRecordedForItsOwner(t *testing.T) {
	for _, tc := range []struct {
		name      string
		page      knowledge.Page
		recipient shared.ID
	}{
		{"the owner", duePage(bert, anna), bert},
		{"the verifier", duePage("", anna), anna},
		{"nobody", duePage("", ""), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notifications, jobs := newNotifications(), &jobQueue{}
			recorder := RecordPageReview{
				Notifications: notifications,
				Accounts: newAccounts(
					person(anna, "Anna", "anna@example.org", "en"),
					person(bert, "Bert", "bert@example.org", "de"),
				),
				Preferences: newPreferences(), Jobs: jobs, Clock: clock.Fixed(now), IDs: &idSequence{},
			}
			if err := recorder.PageReviewDue(t.Context(), tc.page); err != nil {
				t.Fatalf("recording: %v", err)
			}
			written := notifications.written()
			if tc.recipient.IsZero() {
				if len(written) != 0 || len(jobs.requests) != 0 {
					t.Fatalf("records %+v for a page nobody answers for", written)
				}
				return
			}
			if len(written) != 1 || written[0].PageID != reviewedPage || written[0].RecipientID != tc.recipient ||
				written[0].Category != domain.CategoryReminder || written[0].State != domain.StatePending {
				t.Fatalf("records %+v, want one pending reminder about the page for %s", written, tc.recipient)
			}
			if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindPageReviewEmail ||
				jobs.requests[0].MaxAttempts != queue.PageReviewAttempts {
				t.Fatalf("queued %+v, want the page review's own kind", jobs.requests)
			}
		})
	}
}

// pageStore answers the one page a test named.
type pageStore struct{ page knowledge.Page }

func (s pageStore) Find(_ context.Context, id shared.ID) (knowledge.Page, error) {
	if id != s.page.ID {
		return knowledge.Page{}, shared.ErrNotFound
	}
	return s.page, nil
}

// At the delivery: the mail names the page and links to it; a page that is gone leaves the
// withheld sentence.
func TestThePageReviewMailNamesThePage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pages pageStore
		body  string
	}{
		{"the page", pageStore{page: duePage(bert, anna)}, bodyPageReview},
		{"the page gone", pageStore{}, bodyPageReview + withheldSuffix},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := delivery(t, domain.CategoryReminder, true)
			record, err := domain.New(domain.NewInput{
				ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e8"), TenantID: tenant, RecipientID: bert,
				Category: domain.CategoryReminder, Channel: domain.ChannelEmail, PageID: reviewedPage, At: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.notifications.Insert(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			fixture.delivery.Pages = tc.pages
			if err := fixture.delivery.Execute(t.Context(), tenant, record.ID, false); err != nil {
				t.Fatalf("delivering: %v", err)
			}
			if len(fixture.mailbox.sent) != 1 {
				t.Fatalf("%d messages sent, want one", len(fixture.mailbox.sent))
			}
			body := fixture.mailbox.sent[0].Body
			if !strings.HasPrefix(body, tc.body+" ") || !strings.Contains(body, "/pages/"+reviewedPage.String()) {
				t.Errorf("the body %q, want %s linking to the page", body, tc.body)
			}
			if tc.body == bodyPageReview && !strings.Contains(body, "title=VPN setup") {
				t.Errorf("the body %q does not name the page", body)
			}
		})
	}
}
