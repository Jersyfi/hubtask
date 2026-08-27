// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxDeliveryAttempts is where retrying stops and the dead letter begins (automation.md §3.1).
//
// Eight, with the backoff reaching a day, is a little over two days of trying. That is the shape
// the number is for: a target that is down for an afternoon loses nothing, and one that is down
// for a weekend is a target somebody has to look at rather than one this system keeps calling.
const MaxDeliveryAttempts = 8

// MaxConsecutiveFailures is the run of failed deliveries after which a subscription disables
// itself, with a notification to its owner.
//
// Counted in deliveries rather than in attempts, and that is the distinction that makes the number
// small: one dead-lettered delivery is already eight attempts over two days, so three of them is a
// target that has been unreachable for the better part of a week.
const MaxConsecutiveFailures = 3

// MaxRotationGrace is the longest overlap a rotation may ask for. A week is generous for "deploy
// the new secret"; beyond that the old secret is not a grace period but a second live credential.
const MaxRotationGrace = 7 * 24 * time.Hour

// DefaultRotationGrace is what a rotation that names no period gets.
const DefaultRotationGrace = 24 * time.Hour

// maxTargetURL matches the column and the contract.
const maxTargetURL = 2000

// SubscriptionState is what a subscription is doing.
type SubscriptionState string

const (
	// SubscriptionActive delivers.
	SubscriptionActive SubscriptionState = "ACTIVE"
	// SubscriptionPaused is somebody's decision to stop deliveries.
	SubscriptionPaused SubscriptionState = "PAUSED"
	// SubscriptionDisabled is this system's conclusion that the target is not reachable. The two
	// are deliberately different states: both deliver nothing, and only one of them is a problem
	// for somebody to look into.
	SubscriptionDisabled SubscriptionState = "DISABLED"
)

// WebhookSubscription is one external system's standing request to be told what happens here.
//
// It carries no secret and no ciphertext. What is stored is sealed under the installation's key by
// the adapter that holds it (E-02's precedent, the same division the calendar feed's token uses),
// and what this aggregate knows is when the previous secret stops verifying - which is a rule
// about time rather than about a value.
type WebhookSubscription struct {
	ID       shared.ID
	TenantID shared.ID
	// TargetURL is where the CloudEvent is POSTed. Whether the address is one this installation
	// may reach at all is the egress guard's question and not this one (T-07): a private range is
	// refused by the adapter that would dial it, because only the adapter knows what the operator
	// released.
	TargetURL string
	// EventTypes are the types this subscription wants. Never empty: a subscription to nothing is
	// a row that costs a delivery check per event and can never produce one.
	EventTypes []event.Type
	// Filter is a CEL expression narrowing the subscription further, and is empty until the
	// expression engine lands. It is refused rather than stored and ignored - a filter that is
	// accepted and does nothing is a subscriber receiving events they asked not to receive.
	Filter string
	State  SubscriptionState
	// FailureCount is the run of consecutive failed deliveries. The first success resets it, which
	// is what makes it a measure of "is this target reachable now" rather than of history.
	FailureCount int
	// LastError is the message code of the most recent failure. A code, never the target's own
	// response body: that body is somebody else's system's output (rule 10).
	LastError string
	// DisabledAt is when unreachability disabled it, and the zero time otherwise. Separate from
	// the state because a re-enabled subscription goes back to ACTIVE, and the moment the trouble
	// started is what somebody asks about afterwards.
	DisabledAt time.Time
	// PreviousSecretUntil is when the previous signing secret stops verifying. Zero when there is
	// no previous secret, which is the ordinary state.
	PreviousSecretUntil time.Time
	CreatedBy           shared.ID
	CreatedAt           time.Time
	Version             int
}

// NewWebhookSubscriptionInput is what creating one needs. The secret is absent for the reason the
// aggregate carries none: it is drawn and sealed outside the domain.
type NewWebhookSubscriptionInput struct {
	ID         shared.ID
	TenantID   shared.ID
	TargetURL  string
	EventTypes []string
	Filter     string
	CreatedBy  shared.ID
	Now        time.Time
}

// NewWebhookSubscription builds one and refuses what automation.md §3.1 does not allow.
func NewWebhookSubscription(in NewWebhookSubscriptionInput) (WebhookSubscription, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.CreatedBy.IsZero() {
		return WebhookSubscription{}, shared.ErrInternal.WithDetail("webhooks.subscription_incomplete")
	}

	target, err := TargetURL(in.TargetURL)
	if err != nil {
		return WebhookSubscription{}, err
	}
	types, err := SubscribedTypes(in.EventTypes)
	if err != nil {
		return WebhookSubscription{}, err
	}
	if err := RefuseFilter(in.Filter); err != nil {
		return WebhookSubscription{}, err
	}

	return WebhookSubscription{
		ID: in.ID, TenantID: in.TenantID,
		TargetURL: target, EventTypes: types,
		State:     SubscriptionActive,
		CreatedBy: in.CreatedBy, CreatedAt: in.Now.UTC(), Version: 1,
	}, nil
}

// TargetURL checks the shape of an address. Whether it may be dialled is the guard's question.
//
// The scheme is the one rule worth having here: `https` in general, and `http` allowed because an
// installation on a private network legitimately talks to a service without a certificate - the
// guard is what decides whether that network is reachable at all, so refusing the scheme here as
// well would refuse the case twice and forbid the one that is fine.
func TargetURL(raw string) (string, error) {
	target := strings.TrimSpace(raw)
	switch {
	case target == "":
		return "", fieldError("/target_url", "webhooks.target_required")
	case len(target) > maxTargetURL:
		return "", fieldError("/target_url", "webhooks.target_too_long")
	}

	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" {
		return "", fieldError("/target_url", "webhooks.target_malformed")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fieldError("/target_url", "webhooks.target_scheme_unsupported")
	}
	if parsed.User != nil {
		// Credentials in the URL would be a second secret, stored in clear in a column, travelling
		// in every log line that records the target. The signature is how a subscriber knows the
		// call is ours.
		return "", fieldError("/target_url", "webhooks.target_carries_credentials")
	}
	return target, nil
}

// SubscribedTypes checks the requested event types against the ones this build emits.
//
// A type the catalogue does not carry is refused rather than stored: a subscription waiting for
// something that will never arrive looks exactly like a working one, and the difference only shows
// up as an integration that mysteriously never fires.
func SubscribedTypes(requested []string) ([]event.Type, error) {
	types := make([]event.Type, 0, len(requested))
	seen := make(map[event.Type]bool, len(requested))

	var fields []shared.FieldError
	for index, raw := range requested {
		wanted := event.Type(strings.TrimSpace(raw))
		if wanted == "" || seen[wanted] {
			continue
		}
		if !wanted.Valid() {
			fields = append(fields, shared.FieldError{
				Path: "/event_types/" + itoa(index),
				Code: "webhooks.event_type_unknown",
				// The type is the caller's own input and a protocol identifier rather than
				// content, so naming it back is what makes the refusal actionable.
				Params: map[string]string{"event_type": string(wanted)},
			})
			continue
		}
		seen[wanted] = true
		types = append(types, wanted)
	}

	if len(fields) > 0 {
		return nil, shared.ErrValidation.
			WithDetail("webhooks.event_type_unknown").
			WithParams(map[string]string{"event_type": fields[0].Params["event_type"]}).
			WithFields(fields...)
	}
	if len(types) == 0 {
		return nil, fieldError("/event_types", "webhooks.event_types_required")
	}

	// Sorted, so that two subscriptions asking for the same events are stored identically and a
	// listing reads the same way twice.
	slices.Sort(types)
	return types, nil
}

// RefuseFilter is the whole of the CEL field until the expression engine lands.
//
// Refused rather than stored and ignored, which is E-08's `ACCOUNT` lesson: a field accepted and
// silently not honoured is worse than one that is not offered, because the caller has been told
// their instruction was understood. When G-06 lands, this becomes a parse and the test that
// asserts the refusal becomes one that asserts the acceptance.
func RefuseFilter(filter string) error {
	if strings.TrimSpace(filter) == "" {
		return nil
	}
	return shared.ErrValidation.
		WithDetail("webhooks.filter_not_supported").
		WithFields(shared.FieldError{Path: "/filter", Code: "webhooks.filter_not_supported"})
}

// Wants reports whether this subscription is to be told about an event of this type.
//
// State first: a paused or disabled subscription wants nothing, and asking the type question of it
// would be asking whether a subscription that delivers nothing would have liked this one.
func (s WebhookSubscription) Wants(eventType event.Type) bool {
	if s.State != SubscriptionActive {
		return false
	}
	return slices.Contains(s.EventTypes, eventType)
}

// Verifies reports whether the previous secret still counts at this moment. The subscriber's side
// of a rotation: signatures are computed with the current secret from the moment of rotation, and
// this is how long the one they have not deployed yet keeps working.
func (s WebhookSubscription) PreviousSecretVerifies(now time.Time) bool {
	return !s.PreviousSecretUntil.IsZero() && now.Before(s.PreviousSecretUntil)
}

// Rotated stamps the grace period the previous secret keeps verifying for.
//
// A grace of zero retires the old secret at once, which is what a leak calls for - and is why the
// period is a parameter rather than a constant: "somebody may have my secret" and "I am tidying up
// my credentials" want opposite answers.
func (s WebhookSubscription) Rotated(now time.Time, grace time.Duration) (WebhookSubscription, error) {
	if grace < 0 || grace > MaxRotationGrace {
		return WebhookSubscription{}, shared.ErrValidation.
			WithDetail("webhooks.rotation_grace_too_long").
			WithParams(map[string]string{"max_seconds": itoa(int(MaxRotationGrace.Seconds()))}).
			WithFields(shared.FieldError{Path: "/grace_seconds", Code: "webhooks.rotation_grace_too_long"})
	}

	if grace == 0 {
		s.PreviousSecretUntil = time.Time{}
	} else {
		s.PreviousSecretUntil = now.Add(grace).UTC()
	}
	s.Version++
	return s, nil
}

// Delivered records a success: the run of failures ends, and whatever was wrong is no longer the
// last thing that happened.
func (s WebhookSubscription) Delivered() WebhookSubscription {
	s.FailureCount = 0
	s.LastError = ""
	return s
}

// Failed records a failed delivery and reports whether that failure disabled the subscription.
//
// The count is of deliveries rather than attempts, which is what keeps the threshold small: one
// dead-lettered delivery is already eight attempts over two days.
//
// A subscription that is not active does not accumulate failures. Nothing should be delivering to
// one, and a paused subscription whose count crept up would be disabled the moment somebody
// resumed it - punished for having been paused.
func (s WebhookSubscription) Failed(now time.Time, code string) (WebhookSubscription, bool) {
	if s.State != SubscriptionActive {
		return s, false
	}

	s.FailureCount++
	s.LastError = code
	if s.FailureCount < MaxConsecutiveFailures {
		return s, false
	}

	s.State = SubscriptionDisabled
	s.DisabledAt = now.UTC()
	return s, true
}

// Enabled is somebody deciding the target is reachable again. The failure run starts over: the
// count is about now rather than about history, and a subscription re-enabled with two failures
// against it would disable itself on the next hiccup.
func (s WebhookSubscription) Enabled() WebhookSubscription {
	s.State = SubscriptionActive
	s.FailureCount = 0
	s.DisabledAt = time.Time{}
	return s
}

// Paused is somebody stopping deliveries deliberately. It clears the disabled stamp, because a
// paused subscription is not a broken one and a listing that showed both would be saying two
// things at once.
func (s WebhookSubscription) Paused() WebhookSubscription {
	s.State = SubscriptionPaused
	s.DisabledAt = time.Time{}
	return s
}

// IsDisabled reports the state unreachability produces.
func (s WebhookSubscription) IsDisabled() bool { return s.State == SubscriptionDisabled }

// fieldError is the shape a refusal a client can act on takes: the code and the field it is about,
// because "invalid request" is not something a caller can fix (api-guidelines.md §6).
func fieldError(path, code string) error {
	return shared.ErrValidation.
		WithDetail(code).
		WithFields(shared.FieldError{Path: path, Code: code})
}

// itoa avoids strconv for one call in a package that otherwise needs none.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// SecretPurposeFor is what a sealed signing secret is bound to: the field and the row, so that a
// ciphertext lifted out of one subscription and dropped into another no longer opens
// (core/port/crypto).
//
// A string here rather than a crypto.Purpose, because the domain imports no port - and it lives
// here rather than in either caller because the application seals with it and the delivery adapter
// opens with it, and a second copy of the string is a second chance to change one of them.
func SecretPurposeFor(id shared.ID) string {
	return "webhook_subscription.secret:" + id.String()
}
