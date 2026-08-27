// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var (
	deliveryID = shared.ID("01936f2a-7c1e-7000-8000-000000000f11")
	replayID   = shared.ID("01936f2a-7c1e-7000-8000-000000000f12")
	eventID    = shared.ID("01936f2a-7c1e-7000-8000-000000000f13")
)

// jobs records what was asked of the queue. A replay does not deliver: it records the attempt and
// asks for it, so that one code path sends every webhook this system has ever sent.
type jobs struct {
	requests []queue.Request
	err      error
}

func (j *jobs) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	if j.err != nil {
		return "", j.err
	}
	j.requests = append(j.requests, request)
	return shared.ID("01936f2a-7c1e-7000-8000-000000000f14"), nil
}

// withSubscription is a harness whose store already holds one subscription, which is what every
// delivery test needs before it can say anything.
func withSubscription(t *testing.T) *harness {
	t.Helper()
	h := newHarness()
	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), validCreate()); err != nil {
		t.Fatalf("preparing the subscription: %v", err)
	}
	return h
}

func deadLettered(t *testing.T, h *harness) domain.WebhookDelivery {
	t.Helper()
	delivery, err := domain.NewWebhookDelivery(
		deliveryID, tenant, hookID, eventID, domain.MaxDeliveryAttempts, now)
	if err != nil {
		t.Fatalf("building the delivery: %v", err)
	}
	delivery = delivery.Failed(502, "webhooks.target_refused", time.Time{})
	if err := h.delivered.Insert(t.Context(), delivery); err != nil {
		t.Fatalf("seeding the delivery: %v", err)
	}
	return delivery
}

func TestTheDeliveryLogIsPagedAndFilterable(t *testing.T) {
	h := withSubscription(t)
	dead := deadLettered(t, h)

	succeeded, err := domain.NewWebhookDelivery(replayID, tenant, hookID, eventID, 1, now)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if err := h.delivered.Insert(t.Context(), succeeded.Succeeded(200)); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	all, _, err := ListWebhookDeliveries{Writer: h.writer}.Execute(t.Context(), actor(),
		DeliveryListing{SubscriptionID: hookID})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("listed %d deliveries, want both", len(all))
	}

	// DEAD_LETTER is the one an operator usually wants.
	letters, _, err := ListWebhookDeliveries{Writer: h.writer}.Execute(t.Context(), actor(),
		DeliveryListing{SubscriptionID: hookID, Status: string(domain.DeliveryDeadLetter)})
	if err != nil {
		t.Fatalf("listing the dead letters: %v", err)
	}
	if len(letters) != 1 || letters[0].ID != dead.ID {
		t.Errorf("the dead letter filter answered %+v", letters)
	}

	// One more than the page is what answers "is there another page" without a second query.
	page, hasMore, err := ListWebhookDeliveries{Writer: h.writer}.Execute(t.Context(), actor(),
		DeliveryListing{SubscriptionID: hookID, PageSize: 1})
	if err != nil {
		t.Fatalf("listing one: %v", err)
	}
	if len(page) != 1 || !hasMore {
		t.Errorf("a page of one answered %d rows, hasMore = %v", len(page), hasMore)
	}
}

// A caller who mistyped DEAD_LETTER and got the whole log back would read it as an empty dead
// letter, which is the one wrong answer this refusal exists to prevent.
func TestAnUnknownStatusFilterIsRefusedRatherThanIgnored(t *testing.T) {
	h := withSubscription(t)

	_, _, err := ListWebhookDeliveries{Writer: h.writer}.Execute(t.Context(), actor(),
		DeliveryListing{SubscriptionID: hookID, Status: "DEADLETTER"})
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "webhooks.delivery_status_unknown" {
		t.Fatalf("error = %v, want the unknown-status refusal", err)
	}
}

// The same event sent again: the identifier a subscriber deduplicates on is the one it always had,
// and the attempt carries on so the log stays a true account.
func TestAReplayQueuesANewAttemptOfTheSameEvent(t *testing.T) {
	h := withSubscription(t)
	dead := deadLettered(t, h)
	queued := &jobs{}

	replayed, err := ReplayWebhookDelivery{Writer: h.writer, Jobs: queued}.
		Execute(t.Context(), actor(), hookID, dead.ID)
	if err != nil {
		t.Fatalf("replaying: %v", err)
	}

	if replayed.EventID != dead.EventID {
		t.Errorf("event = %s, want the one it always had", replayed.EventID)
	}
	if replayed.Attempt != dead.Attempt+1 {
		t.Errorf("attempt = %d, want %d", replayed.Attempt, dead.Attempt+1)
	}
	if len(queued.requests) != 1 {
		t.Fatalf("queued %d jobs, want one", len(queued.requests))
	}
	request := queued.requests[0]
	if request.Kind != queue.KindWebhookDeliver || request.TenantID != tenant {
		t.Errorf("the job is %+v", request)
	}
	if request.Payload["event_id"] != dead.EventID.String() {
		t.Errorf("the job names event %v", request.Payload["event_id"])
	}

	// The act is audited: somebody looked at a dead letter and decided the target is ready.
	last := h.sink.entries[len(h.sink.entries)-1]
	if last.Action != WebhookReplayedAction || last.TargetID != replayed.ID {
		t.Errorf("entry = %s on %s", last.Action, last.TargetID)
	}
}

// A delivery of another subscription, named through this one's path: not found rather than
// forbidden, for the reason every other read of somebody else's thing is (T-04).
func TestADeliveryOfAnotherSubscriptionIsNotFound(t *testing.T) {
	h := withSubscription(t)
	other := shared.ID("01936f2a-7c1e-7000-8000-000000000f15")
	delivery, err := domain.NewWebhookDelivery(deliveryID, tenant, other, eventID, 1, now)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if err := h.delivered.Insert(t.Context(), delivery); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	_, err = ReplayWebhookDelivery{Writer: h.writer, Jobs: &jobs{}}.
		Execute(t.Context(), actor(), hookID, deliveryID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("error = %v, want not found", err)
	}
}

// A subscriber cannot deploy atomically, so the old secret keeps verifying for its grace - and the
// clock decides that, not a sleep.
func TestARotationAnswersANewSecretAndKeepsTheOldOneVerifying(t *testing.T) {
	h := withSubscription(t)
	before := h.store.rows[hookID]

	rotated, err := RotateWebhookSecret{Writer: h.writer}.
		Execute(t.Context(), actor(), hookID, domain.DefaultRotationGrace)
	if err != nil {
		t.Fatalf("rotating: %v", err)
	}

	after := h.store.rows[hookID]
	if string(after.Secret.Ciphertext) == string(before.Secret.Ciphertext) {
		t.Error("the secret did not change")
	}
	if string(after.Previous.Ciphertext) != string(before.Secret.Ciphertext) {
		t.Error("the old secret was not kept as the previous one")
	}
	if rotated.Secret.Reveal() == "" {
		t.Error("the rotation answered no secret")
	}

	// Verified against a clock rather than a sleep: inside the grace it counts, at the end it does
	// not.
	subscription := after.Subscription
	if !subscription.PreviousSecretVerifies(now.Add(time.Hour)) {
		t.Error("the previous secret stopped verifying inside its grace")
	}
	if subscription.PreviousSecretVerifies(now.Add(domain.DefaultRotationGrace)) {
		t.Error("the previous secret still verifies at the end of its grace")
	}

	// The trail says when the old secret stops working, and never what either secret is.
	last := h.sink.entries[len(h.sink.entries)-1]
	rendered := fmt.Sprintf("%v", last.Changes)
	if last.Action != WebhookRotatedAction {
		t.Errorf("entry = %s", last.Action)
	}
	if strings.Contains(rendered, rotated.Secret.Reveal()) {
		t.Fatalf("the new secret reached the audit trail: %s", rendered)
	}
	if !strings.Contains(rendered, "previous_secret_until") {
		t.Errorf("the trail does not say when the old secret stops: %s", rendered)
	}
}

// Zero retires the old secret at once, which is what a leak calls for - and is why the period is a
// parameter rather than a constant.
func TestARotationWithNoGraceRetiresTheOldSecretAtOnce(t *testing.T) {
	h := withSubscription(t)

	if _, err := (RotateWebhookSecret{Writer: h.writer}).Execute(t.Context(), actor(), hookID, 0); err != nil {
		t.Fatalf("rotating: %v", err)
	}
	if h.store.rows[hookID].Subscription.PreviousSecretVerifies(now) {
		t.Error("a rotation with no grace left the old secret verifying")
	}

	// And beyond a week the old secret is not a grace period but a second live credential.
	_, err := RotateWebhookSecret{Writer: h.writer}.
		Execute(t.Context(), actor(), hookID, domain.MaxRotationGrace+time.Hour)
	if err == nil {
		t.Error("a grace longer than a week was accepted")
	}
}

// The catalogue path, and the distinction the rotation's input turns on: an absent period is the
// default, and an explicit zero is "retire it now".
func TestTheThreeReachTheirWorkThroughTheirDescriptors(t *testing.T) {
	h := withSubscription(t)
	dead := deadLettered(t, h)
	queued := &jobs{}

	list := ListWebhookDeliveries{Writer: h.writer}.Descriptor()
	listed, err := list.Handler.Invoke(t.Context(), actor(), usecase.Input{
		"webhook_id": hookID.String(), "status": string(domain.DeliveryDeadLetter),
	})
	if err != nil {
		t.Fatalf("listing through the descriptor failed: %v", err)
	}
	if rows, _ := listed["data"].([]usecase.Output); len(rows) != 1 {
		t.Errorf("listed %d rows, want the dead letter", len(rows))
	}

	replay := ReplayWebhookDelivery{Writer: h.writer, Jobs: queued}.Descriptor()
	if _, err := replay.Handler.Invoke(t.Context(), actor(), usecase.Input{
		"webhook_id": hookID.String(), "delivery_id": dead.ID.String(),
	}); err != nil {
		t.Fatalf("replaying through the descriptor failed: %v", err)
	}

	rotate := RotateWebhookSecret{Writer: h.writer}.Descriptor()
	out, err := rotate.Handler.Invoke(t.Context(), actor(), usecase.Input{"webhook_id": hookID.String()})
	if err != nil {
		t.Fatalf("rotating through the descriptor failed: %v", err)
	}
	if out.String("secret") == "" {
		t.Error("the rotation answered no secret")
	}
	// Absent means the default grace.
	if !h.store.rows[hookID].Subscription.PreviousSecretVerifies(now.Add(time.Hour)) {
		t.Error("an omitted period did not produce the default grace")
	}

	// An explicit zero is a different instruction, and Present is what tells them apart.
	if _, err := rotate.Handler.Invoke(t.Context(), actor(), usecase.Input{
		"webhook_id": hookID.String(), "grace_seconds": 0,
	}); err != nil {
		t.Fatalf("rotating with no grace failed: %v", err)
	}
	if h.store.rows[hookID].Subscription.PreviousSecretVerifies(now) {
		t.Error("an explicit zero was read as the default")
	}

	if !replay.Audit.Required || !rotate.Audit.Required {
		t.Error("a write does not declare its audit obligation")
	}
	if !list.ReadOnly {
		t.Error("the listing is not annotated read-only")
	}
}

var _ = repository.DeliveryQuery{}
