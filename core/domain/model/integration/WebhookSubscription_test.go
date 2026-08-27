// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	hookID     = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
	hookTenant = shared.ID("01936f2a-7c1e-7000-8000-0000000000d2")
	hookAuthor = shared.ID("01936f2a-7c1e-7000-8000-0000000000d3")
	hookNow    = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
)

func validSubscription() NewWebhookSubscriptionInput {
	return NewWebhookSubscriptionInput{
		ID: hookID, TenantID: hookTenant, CreatedBy: hookAuthor,
		TargetURL: "https://example.org/hooks/hubtask",
		EventTypes: []string{
			string(event.ItemCreated), string(event.ContainerCreated), string(event.ItemCreated),
		},
		Now: hookNow,
	}
}

func TestANewSubscriptionIsActiveAndNormalised(t *testing.T) {
	subscription, err := NewWebhookSubscription(validSubscription())
	if err != nil {
		t.Fatalf("a valid subscription was refused: %v", err)
	}

	if subscription.State != SubscriptionActive {
		t.Errorf("state = %s, want active", subscription.State)
	}
	// Sorted and deduplicated, so two subscriptions asking for the same events are stored
	// identically and a listing reads the same way twice.
	if len(subscription.EventTypes) != 2 {
		t.Errorf("event types = %v, want the two distinct ones", subscription.EventTypes)
	}
	if subscription.FailureCount != 0 || subscription.Version != 1 {
		t.Errorf("subscription = %+v", subscription)
	}
	// The aggregate carries no secret at all: what is stored is sealed by the adapter that holds
	// the key, and this is the test that says the domain never sees one.
	if strings.Contains(subscription.TargetURL, "@") {
		t.Error("the target carries credentials")
	}
}

func TestATargetIsCheckedForShapeAndNotForReachability(t *testing.T) {
	cases := map[string]struct {
		target     string
		wantDetail string
	}{
		"empty":                {"   ", "webhooks.target_required"},
		"not a URL":            {"this is not a url", "webhooks.target_malformed"},
		"a scheme we not dial": {"ftp://example.org/hook", "webhooks.target_scheme_unsupported"},
		"credentials in it":    {"https://user:pw@example.org/hook", "webhooks.target_carries_credentials"},
		"longer than the column": {
			"https://example.org/" + strings.Repeat("p", maxTargetURL), "webhooks.target_too_long",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := validSubscription()
			in.TargetURL = c.target

			_, err := NewWebhookSubscription(in)
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != c.wantDetail {
				t.Fatalf("error = %v, want %s", err, c.wantDetail)
			}
			if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != "/target_url" {
				t.Errorf("fields = %v, want one at /target_url", domainErr.Fields)
			}
		})
	}

	// A private address passes here on purpose: whether this installation may dial it is the
	// egress guard's question, and only the guard knows what the operator released (T-07).
	in := validSubscription()
	in.TargetURL = "http://10.0.0.5/hook"
	if _, err := NewWebhookSubscription(in); err != nil {
		t.Errorf("a private address was refused by the domain rather than by the guard: %v", err)
	}
}

// A subscription waiting for something that will never arrive looks exactly like a working one.
func TestAnEventTypeThisBuildDoesNotEmitIsRefusedByName(t *testing.T) {
	in := validSubscription()
	in.EventTypes = []string{string(event.ItemCreated), "de.hubtask.work.item.teleported.v1"}

	_, err := NewWebhookSubscription(in)
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "webhooks.event_type_unknown" {
		t.Fatalf("error = %v, want the unknown-type refusal", err)
	}
	if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != "/event_types/1" {
		t.Fatalf("fields = %v, want the second entry named", domainErr.Fields)
	}
	if got := domainErr.Fields[0].Params["event_type"]; got != "de.hubtask.work.item.teleported.v1" {
		t.Errorf("the refusal does not name the type: %q", got)
	}

	in.EventTypes = nil
	if _, err := NewWebhookSubscription(in); err == nil {
		t.Error("a subscription to nothing was accepted")
	}
}

// E-08's ACCOUNT lesson: a field accepted and silently not honoured is worse than one that is not
// offered, because the caller has been told their instruction was understood.
func TestTheFilterIsRefusedRatherThanStoredAndIgnored(t *testing.T) {
	in := validSubscription()
	in.Filter = `event.type == "de.hubtask.work.item.created.v1"`

	_, err := NewWebhookSubscription(in)
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "webhooks.filter_not_supported" {
		t.Fatalf("error = %v, want the filter refusal", err)
	}
	// Blank is not a filter, so it passes - otherwise every subscription would need one.
	in.Filter = "   "
	if _, err := NewWebhookSubscription(in); err != nil {
		t.Errorf("a blank filter was treated as a filter: %v", err)
	}
}

func TestOnlyAnActiveSubscriptionWantsAnything(t *testing.T) {
	subscription, err := NewWebhookSubscription(validSubscription())
	if err != nil {
		t.Fatalf("preparing: %v", err)
	}

	if !subscription.Wants(event.ItemCreated) {
		t.Error("an active subscription does not want a type it asked for")
	}
	if subscription.Wants(event.ItemCompleted) {
		t.Error("it wants a type it did not ask for")
	}
	for _, stopped := range []WebhookSubscription{subscription.Paused(), disabled(t, subscription)} {
		if stopped.Wants(event.ItemCreated) {
			t.Errorf("a %s subscription still wants events", stopped.State)
		}
	}
}

// The run is of deliveries rather than attempts, which is what keeps the threshold small: one
// dead-lettered delivery is already eight attempts over two days.
func TestARunOfFailuresDisablesTheSubscriptionOnce(t *testing.T) {
	subscription, err := NewWebhookSubscription(validSubscription())
	if err != nil {
		t.Fatalf("preparing: %v", err)
	}

	for attempt := 1; attempt < MaxConsecutiveFailures; attempt++ {
		var justDisabled bool
		subscription, justDisabled = subscription.Failed(hookNow, "webhooks.target_unreachable")
		if justDisabled {
			t.Fatalf("disabled after %d failures, want %d", attempt, MaxConsecutiveFailures)
		}
		if subscription.FailureCount != attempt {
			t.Errorf("count = %d after %d failures", subscription.FailureCount, attempt)
		}
	}

	subscription, justDisabled := subscription.Failed(hookNow, "webhooks.target_unreachable")
	if !justDisabled || !subscription.IsDisabled() {
		t.Fatalf("the run did not disable it: %+v", subscription)
	}
	if subscription.DisabledAt != hookNow {
		t.Errorf("disabled at %v, want %v", subscription.DisabledAt, hookNow)
	}

	// And a failure against something already stopped does not report a second disabling - the
	// owner is told once.
	if _, again := subscription.Failed(hookNow, "webhooks.target_unreachable"); again {
		t.Error("a disabled subscription reported that it had just been disabled")
	}
}

// A paused subscription must not accumulate failures, or it would be disabled the moment somebody
// resumed it - punished for having been paused.
func TestAPausedSubscriptionDoesNotAccumulateFailures(t *testing.T) {
	subscription, err := NewWebhookSubscription(validSubscription())
	if err != nil {
		t.Fatalf("preparing: %v", err)
	}
	paused := subscription.Paused()

	after, justDisabled := paused.Failed(hookNow, "webhooks.target_unreachable")
	if justDisabled || after.FailureCount != 0 {
		t.Errorf("a paused subscription counted a failure: %+v", after)
	}
}

// The count is about now rather than about history: a subscription re-enabled with two failures
// against it would disable itself on the next hiccup.
func TestSuccessAndReEnablingBothClearTheRun(t *testing.T) {
	subscription, err := NewWebhookSubscription(validSubscription())
	if err != nil {
		t.Fatalf("preparing: %v", err)
	}

	failed, _ := subscription.Failed(hookNow, "webhooks.target_unreachable")
	if recovered := failed.Delivered(); recovered.FailureCount != 0 || recovered.LastError != "" {
		t.Errorf("a success left %+v", recovered)
	}

	enabled := disabled(t, subscription).Enabled()
	if enabled.State != SubscriptionActive || enabled.FailureCount != 0 || !enabled.DisabledAt.IsZero() {
		t.Errorf("re-enabling left %+v", enabled)
	}
}

// A subscriber cannot deploy atomically, so a rotation taking effect instantly would drop every
// event arriving between the call and the deployment.
func TestRotationKeepsThePreviousSecretVerifyingForItsGrace(t *testing.T) {
	subscription, err := NewWebhookSubscription(validSubscription())
	if err != nil {
		t.Fatalf("preparing: %v", err)
	}

	rotated, err := subscription.Rotated(hookNow, DefaultRotationGrace)
	if err != nil {
		t.Fatalf("rotating: %v", err)
	}
	if !rotated.PreviousSecretVerifies(hookNow.Add(time.Hour)) {
		t.Error("the previous secret stopped verifying inside its grace")
	}
	if rotated.PreviousSecretVerifies(hookNow.Add(DefaultRotationGrace)) {
		t.Error("the previous secret still verifies at the end of its grace")
	}
	if rotated.Version != subscription.Version+1 {
		t.Errorf("version = %d, want one more than %d", rotated.Version, subscription.Version)
	}

	// Zero retires it at once, which is what a leak calls for.
	immediate, err := subscription.Rotated(hookNow, 0)
	if err != nil {
		t.Fatalf("rotating with no grace: %v", err)
	}
	if immediate.PreviousSecretVerifies(hookNow) {
		t.Error("a rotation with no grace left the old secret verifying")
	}

	// Beyond a week the old secret is not a grace period but a second live credential.
	if _, err := subscription.Rotated(hookNow, MaxRotationGrace+time.Hour); err == nil {
		t.Error("a grace longer than a week was accepted")
	}
}

func disabled(t *testing.T, subscription WebhookSubscription) WebhookSubscription {
	t.Helper()
	for range MaxConsecutiveFailures {
		subscription, _ = subscription.Failed(hookNow, "webhooks.target_unreachable")
	}
	if !subscription.IsDisabled() {
		t.Fatal("the fixture did not disable the subscription")
	}
	return subscription
}
