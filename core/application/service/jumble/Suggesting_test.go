// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// Asking queues and answers; it does not wait, and it changes nothing. Everything else follows.
func TestAskingQueuesAQuestionAndChangesNothing(t *testing.T) {
	ask, jobs, h := suggester(true)
	entry := seedEntry(t, h, "Call the customer back")

	if err := ask.Execute(context.Background(), actor(), entry.ID); err != nil {
		t.Fatalf("asking: %v", err)
	}

	if len(jobs.queued) != 1 {
		t.Fatalf("%d jobs queued, want one", len(jobs.queued))
	}
	job := jobs.queued[0]
	if job.Kind != queue.KindAiSuggest || job.TenantID != actor().TenantID {
		t.Errorf("the job is %+v", job)
	}
	if job.Payload["target_id"] != entry.ID.String() ||
		job.Payload["target_type"] != "JUMBLE_ENTRY" ||
		job.Payload["asked_by"] != actor().AccountID.String() {
		t.Errorf("the payload is %v", job.Payload)
	}
	// The entry's text is not in it. A payload holding the subject and the body would be a second
	// copy of the least trusted text in the system, in the one table without row level security.
	for key, value := range job.Payload {
		if text, isText := value.(string); isText && text == entry.RawBody {
			t.Errorf("the job payload carries the entry's text in %s", key)
		}
	}
	if stored := h.store.rows[entry.ID]; stored.Status != domain.StatusNew {
		t.Errorf("asking settled the entry: %q", stored.Status)
	}
}

// Two proposals rather than one refusal: a duplicate suggestion is a record somebody dismisses,
// where a duplicate conversion would be an item somebody has to delete.
func TestAskingTwiceIsAllowed(t *testing.T) {
	ask, jobs, h := suggester(true)
	entry := seedEntry(t, h, "Call the customer back")

	for range 2 {
		if err := ask.Execute(context.Background(), actor(), entry.ID); err != nil {
			t.Fatalf("asking: %v", err)
		}
	}
	if len(jobs.queued) != 2 {
		t.Fatalf("%d jobs queued, want two", len(jobs.queued))
	}
	for _, job := range jobs.queued {
		if job.DedupeKey != "" {
			t.Error("the question is deduplicated, so a second ask is silently dropped")
		}
	}
}

// A workspace with no provider, or one that has not consented, is told so - and nothing is queued,
// so nothing is sent and no worker discovers it three retries later.
func TestAWorkspaceWithoutAiIsRefusedBeforeAnythingIsQueued(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		wired bool
	}{
		{"a provider that cannot be asked", true},
		{"an installation with no AI surface at all", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ask, jobs, h := suggester(false)
			if !testCase.wired {
				ask.AI = nil
			}
			entry := seedEntry(t, h, "Call the customer back")

			err := ask.Execute(context.Background(), actor(), entry.ID)
			if !errors.Is(err, shared.ErrUnavailable) {
				t.Fatalf("the answer was %v, want an unavailable dependency", err)
			}
			if got := shared.AsError(err).DetailCode; got != "ai.unavailable" {
				t.Errorf("detail code %q, want ai.unavailable", got)
			}
			if len(jobs.queued) != 0 {
				t.Error("a refused workspace had a question queued for it")
			}
		})
	}
}

// An entry somebody has already decided about is not asked about: the proposal would be one nobody
// can take, which is an inbox of noise rather than a feature.
func TestASettledEntryIsNotAskedAbout(t *testing.T) {
	ask, jobs, h := suggester(true)
	entry := seedEntry(t, h, "Call the customer back")
	settled := h.store.rows[entry.ID]
	settled.Status = domain.StatusDismissed
	h.store.rows[entry.ID] = settled

	err := ask.Execute(context.Background(), actor(), entry.ID)
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("the answer was %v, want a conflict", err)
	}
	if len(jobs.queued) != 0 {
		t.Error("a settled entry had a question queued for it")
	}
}

// Nothing is queued and nothing is learned unless the actor may act (rule 2). The availability
// question comes after the permission check, so it cannot become a way to find out what an
// installation has configured.
func TestAnUnauthorisedActorLearnsNothing(t *testing.T) {
	ask, jobs, h := suggester(true)
	availability := ask.AI.(*availabilityDouble)
	ask.Writer.Authorizer = refuseAll{}
	entry := seedEntry(t, h, "Call the customer back")

	if err := ask.Execute(context.Background(), actor(), entry.ID); err == nil {
		t.Fatal("an unauthorised actor asked for a suggestion")
	}
	if len(jobs.queued) != 0 {
		t.Error("a refused caller queued a question")
	}
	if availability.asked {
		t.Error("a refused caller learned whether this installation has AI configured")
	}
}

// The fixtures.

func suggester(available bool) (SuggestFromJumbleEntry, *jobQueue, *harness) {
	h := newHarness()
	jobs := &jobQueue{}
	return SuggestFromJumbleEntry{
		Writer: h.writer,
		AI:     &availabilityDouble{available: available},
		Queue:  jobs,
	}, jobs, h
}

type jobQueue struct{ queued []queue.Request }

func (q *jobQueue) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	q.queued = append(q.queued, request)
	return shared.MustParseID("0192f000-0000-7000-8000-00000000ab09"), nil
}

type availabilityDouble struct {
	available bool
	asked     bool
}

func (a *availabilityDouble) CanSuggest(context.Context, appshared.ActorContext) (bool, error) {
	a.asked = true
	return a.available, nil
}

type refuseAll struct{}

func (refuseAll) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return shared.ErrForbidden
}
