// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var (
	slippedBaseline = shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")
	slippedPlan     = shared.ID("01936f2a-7c1e-7000-8000-0000000000b2")
)

type baselineStore struct{}

func (baselineStore) Find(_ context.Context, id shared.ID) (planning.Baseline, error) {
	if id != slippedBaseline {
		return planning.Baseline{}, shared.ErrNotFound
	}
	return planning.Baseline{ID: id, TenantID: tenant, PlanID: slippedPlan}, nil
}

// UC-PLN-04 check 6: a plan in exception is told to whoever decided its tolerance, once per
// baseline - a second read of the same exception writes nothing - and never to the reader who is
// looking at it.
func TestAPlanInExceptionIsToldOnceToWhoeverDecidedItsTolerance(t *testing.T) {
	notifications, jobs := newNotifications(), &jobQueue{}
	recorder := RecordPlanException{
		Notifications: notifications,
		Accounts:      newAccounts(person(anna, "Anna", "anna@example.org", "en"), person(bert, "Bert", "bert@example.org", "de")),
		Preferences:   newPreferences(), Jobs: jobs, IDs: &idSequence{},
	}
	for range 2 {
		if err := recorder.PlanExceeded(t.Context(), tenant, slippedBaseline, anna, bert, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := recorder.PlanExceeded(t.Context(), tenant, slippedBaseline, bert, bert, now); err != nil {
		t.Fatal(err)
	}
	written := notifications.written()
	if len(written) != 1 || written[0].RecipientID != anna || written[0].BaselineID != slippedBaseline ||
		written[0].Category != domain.CategoryReminder {
		t.Fatalf("records %+v", written)
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindPlanExceptionEmail {
		t.Fatalf("queued %+v", jobs.requests)
	}
}

// At the delivery: the mail names the plan's place and links to its Plan screen.
func TestThePlanExceptionMailLinksToThePlan(t *testing.T) {
	fixture := delivery(t, domain.CategoryReminder, true)
	record, err := domain.New(domain.NewInput{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000b3"), TenantID: tenant, RecipientID: bert,
		Category: domain.CategoryReminder, Channel: domain.ChannelEmail, BaselineID: slippedBaseline, At: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.notifications.Insert(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	fixture.delivery.Baselines = baselineStore{}
	fixture.delivery.Places = placeStore{place: work.Container{ID: slippedPlan, TenantID: tenant, Type: work.ContainerHub, Name: "Garden"}}
	fixture.delivery.Clock = clock.Fixed(now)
	if err := fixture.delivery.Execute(t.Context(), tenant, record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(fixture.mailbox.sent) != 1 {
		t.Fatalf("%d messages sent, want one", len(fixture.mailbox.sent))
	}
	body := fixture.mailbox.sent[0].Body
	if !strings.Contains(body, bodyPlanException+" ") || !strings.Contains(body, "/hubs/"+slippedPlan.String()+"/plan") ||
		!strings.Contains(body, "title=Garden") {
		t.Errorf("the body %q, want the exception naming Garden and linking to its plan", body)
	}
}
