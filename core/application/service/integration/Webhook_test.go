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
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var (
	tenant   = shared.ID("01936f2a-7c1e-7000-8000-000000000f01")
	author   = shared.ID("01936f2a-7c1e-7000-8000-000000000f02")
	hookID   = shared.ID("01936f2a-7c1e-7000-8000-000000000f03")
	now      = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	someType = string(event.ItemCreated)
)

// subscriptionStore is the repository in memory, keyed the way the table is.
type subscriptionStore struct {
	rows      map[shared.ID]repository.StoredSubscription
	insertErr error
	updates   int
}

func newStore(existing ...repository.StoredSubscription) *subscriptionStore {
	store := &subscriptionStore{rows: map[shared.ID]repository.StoredSubscription{}}
	for _, stored := range existing {
		store.rows[stored.Subscription.ID] = stored
	}
	return store
}

func (s *subscriptionStore) Insert(_ context.Context, stored repository.StoredSubscription) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.rows[stored.Subscription.ID] = stored
	return nil
}

func (s *subscriptionStore) Find(_ context.Context, id shared.ID) (repository.StoredSubscription, error) {
	stored, found := s.rows[id]
	if !found {
		return repository.StoredSubscription{}, shared.ErrNotFound.
			WithDetail("webhooks.subscription_not_found")
	}
	return stored, nil
}

func (s *subscriptionStore) List(context.Context) ([]domain.WebhookSubscription, error) {
	var all []domain.WebhookSubscription
	for _, stored := range s.rows {
		all = append(all, stored.Subscription)
	}
	return all, nil
}

func (s *subscriptionStore) WantingEvent(_ context.Context, wanted event.Type) ([]repository.StoredSubscription, error) {
	var found []repository.StoredSubscription
	for _, stored := range s.rows {
		if stored.Subscription.Wants(wanted) {
			found = append(found, stored)
		}
	}
	return found, nil
}

func (s *subscriptionStore) Update(
	_ context.Context, subscription domain.WebhookSubscription, expectedVersion int,
) (bool, error) {
	stored, found := s.rows[subscription.ID]
	if !found || stored.Subscription.Version != expectedVersion {
		return false, nil
	}
	s.updates++
	subscription.Version = expectedVersion + 1
	stored.Subscription = subscription
	s.rows[subscription.ID] = stored
	return true, nil
}

func (s *subscriptionStore) Rotate(
	_ context.Context, id shared.ID, sealed repository.SealedSecret,
	previousUntil time.Time, expectedVersion int,
) (bool, error) {
	stored, found := s.rows[id]
	if !found || stored.Subscription.Version != expectedVersion {
		return false, nil
	}
	stored.Previous = stored.Secret
	stored.Secret = sealed
	stored.Subscription.PreviousSecretUntil = previousUntil
	stored.Subscription.Version = expectedVersion + 1
	s.rows[id] = stored
	return true, nil
}

func (s *subscriptionStore) Delete(_ context.Context, id shared.ID) (bool, error) {
	if _, found := s.rows[id]; !found {
		return false, nil
	}
	delete(s.rows, id)
	return true, nil
}

type deliveryStore struct{ rows []domain.WebhookDelivery }

func (d *deliveryStore) Insert(_ context.Context, delivery domain.WebhookDelivery) error {
	d.rows = append(d.rows, delivery)
	return nil
}

func (d *deliveryStore) Find(_ context.Context, id shared.ID) (domain.WebhookDelivery, error) {
	for _, delivery := range d.rows {
		if delivery.ID == id {
			return delivery, nil
		}
	}
	return domain.WebhookDelivery{}, shared.ErrNotFound.WithDetail("webhooks.delivery_not_found")
}

func (d *deliveryStore) List(_ context.Context, query repository.DeliveryQuery) ([]domain.WebhookDelivery, error) {
	var found []domain.WebhookDelivery
	for _, delivery := range d.rows {
		if delivery.SubscriptionID != query.SubscriptionID {
			continue
		}
		if query.Status != "" && delivery.Status != query.Status {
			continue
		}
		found = append(found, delivery)
	}
	return found, nil
}

func (d *deliveryStore) RecordOutcome(_ context.Context, outcome repository.DeliveryOutcome) error {
	for index, delivery := range d.rows {
		if delivery.ID == outcome.ID {
			delivery.Status = outcome.Status
			delivery.ResponseStatus = outcome.ResponseStatus
			delivery.ErrorCode = outcome.ErrorCode
			delivery.NextAttemptAt = outcome.NextAttemptAt
			d.rows[index] = delivery
		}
	}
	return nil
}

type authorizer struct {
	refuse   error
	requests []access.Request
}

func (a *authorizer) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	a.requests = append(a.requests, request)
	return a.refuse
}

type auditSink struct{ entries []audit.Entry }

func (s *auditSink) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

type unitOfWork struct{}

func (u *unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, scope, fn)
}

// encryptor seals by prefixing, which is enough for a test: what matters is that a plaintext went
// in and something that is not the plaintext came out under a purpose that names the row.
type encryptor struct{ purposes []crypto.Purpose }

func (e *encryptor) Seal(_ context.Context, plaintext secret.Secret, purpose crypto.Purpose) (crypto.Sealed, error) {
	e.purposes = append(e.purposes, purpose)
	return crypto.Sealed{KeyID: "k1", Ciphertext: []byte("sealed:" + plaintext.Reveal())}, nil
}

func (e *encryptor) ActiveKeyID() string { return "k1" }

func (e *encryptor) KeyIDs() []string { return []string{"k1"} }

func (e *encryptor) Rewrap(_ context.Context, sealed crypto.Sealed, _ crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{KeyID: "k1", Ciphertext: sealed.Ciphertext}, nil
}

func (e *encryptor) Open(_ context.Context, sealed crypto.Sealed, _ crypto.Purpose) (secret.Secret, error) {
	return secret.New(strings.TrimPrefix(string(sealed.Ciphertext), "sealed:")), nil
}

// countingEntropy draws different bytes each time, which clock.FixedEntropy deliberately does not.
// A rotation whose second draw equalled its first would be a test that could not tell a rotation
// from a no-op.
type countingEntropy struct{ draws byte }

func (e *countingEntropy) Bytes(n int) ([]byte, error) {
	e.draws++
	drawn := make([]byte, n)
	for i := range drawn {
		drawn[i] = e.draws + byte(i)
	}
	return drawn, nil
}

type ids struct{ next shared.ID }

func (i ids) NewID() shared.ID { return i.next }

func actor() appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: author, AccountName: "Anna",
		Kind: shared.ActorUser, Scopes: []string{automationScope},
	}
}

type harness struct {
	writer    Writer
	store     *subscriptionStore
	delivered *deliveryStore
	auth      *authorizer
	sink      *auditSink
	sealer    *encryptor
}

func newHarness(existing ...repository.StoredSubscription) *harness {
	h := &harness{
		store: newStore(existing...), delivered: &deliveryStore{},
		auth: &authorizer{}, sink: &auditSink{}, sealer: &encryptor{},
	}
	h.writer = Writer{
		Subscriptions: h.store, Deliveries: h.delivered, Authorizer: h.auth,
		Encryptor: h.sealer, Audit: h.sink, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now), IDs: ids{next: hookID}, Entropy: &countingEntropy{},
	}
	return h
}

func validCreate() CreateWebhookSubscriptionCommand {
	return CreateWebhookSubscriptionCommand{
		TargetURL:  "https://example.org/hooks/hubtask",
		EventTypes: []string{someType},
	}
}

// The secret is answered once and stored sealed, under a purpose that names the row - so a
// ciphertext lifted out of one subscription and dropped into another no longer opens.
func TestACreationAnswersTheSecretOnceAndStoresItSealed(t *testing.T) {
	h := newHarness()

	minted, err := CreateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(), validCreate())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	if minted.Secret.Reveal() == "" {
		t.Fatal("the creation answered no secret")
	}
	stored := h.store.rows[hookID]
	if string(stored.Secret.Ciphertext) == minted.Secret.Reveal() {
		t.Fatal("the plaintext was stored")
	}
	if stored.Secret.KeyID == "" {
		t.Error("the sealed value was stored without the key it opens under")
	}
	if len(h.sealer.purposes) != 1 || !strings.Contains(string(h.sealer.purposes[0]), hookID.String()) {
		t.Errorf("purposes = %v, want one naming the subscription", h.sealer.purposes)
	}
	// The projection carries no secret: it exists in the answer to the creation and nowhere else.
	if _, present := subscriptionOutput(minted.Subscription)["secret"]; present {
		t.Error("the projection carries the signing secret")
	}
}

// Subscribing an external system to the stream and writing a rule that reacts to it are the same
// power over the same stream.
func TestEveryWebhookOperationAsksForAutomation(t *testing.T) {
	h := newHarness()

	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), validCreate()); err != nil {
		t.Fatalf("creating: %v", err)
	}
	if _, err := (ListWebhookSubscriptions{Writer: h.writer}).Execute(t.Context(), actor()); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if err := (DeleteWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), hookID); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	if len(h.auth.requests) != 3 {
		t.Fatalf("the authoriser was asked %d times, want once per operation", len(h.auth.requests))
	}
	for _, request := range h.auth.requests {
		if request.Permission != service.PermissionAutomation {
			t.Errorf("permission = %s, want %s", request.Permission, service.PermissionAutomation)
		}
		if request.TokenScope != automationScope {
			t.Errorf("token scope = %s, want %s", request.TokenScope, automationScope)
		}
	}

	h.auth.refuse = shared.ErrForbidden.WithDetail("access.not_permitted")
	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), validCreate()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("error = %v, want the authoriser's refusal", err)
	}
}

// Where a workspace's events are being sent is exactly what an auditor is looking for. The key
// that signs them is exactly what they are not.
func TestTheTrailRecordsTheTargetAndNeverTheSecret(t *testing.T) {
	h := newHarness()

	minted, err := CreateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(), validCreate())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if len(h.sink.entries) != 1 {
		t.Fatalf("wrote %d entries, want one", len(h.sink.entries))
	}

	entry := h.sink.entries[0]
	if entry.Action != WebhookCreatedAction || entry.TargetType != webhookTarget {
		t.Errorf("entry = %s on %s", entry.Action, entry.TargetType)
	}
	// Notice rather than info: this workspace's events now go somewhere outside it.
	if entry.Severity != audit.SeverityNotice {
		t.Errorf("severity = %s, want notice", entry.Severity)
	}
	rendered := fmt.Sprintf("%v", entry.Changes)
	if !strings.Contains(rendered, "example.org") {
		t.Errorf("the trail does not say where events are being sent: %s", rendered)
	}
	if strings.Contains(rendered, minted.Secret.Reveal()) {
		t.Fatalf("the signing secret reached the audit trail: %s", rendered)
	}
}

func TestAnUpdateLeavesOmittedFieldsAlone(t *testing.T) {
	h := newHarness()
	created, err := CreateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(), validCreate())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	target := "https://elsewhere.example/hooks"
	updated, err := UpdateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(),
		UpdateWebhookSubscriptionCommand{ID: hookID, TargetURL: &target})
	if err != nil {
		t.Fatalf("updating: %v", err)
	}

	if updated.TargetURL != target {
		t.Errorf("target = %q, want the new one", updated.TargetURL)
	}
	if len(updated.EventTypes) != len(created.Subscription.EventTypes) {
		t.Errorf("the omitted event types changed: %v", updated.EventTypes)
	}
	if updated.Version != created.Subscription.Version+1 {
		t.Errorf("version = %d, want one more than %d", updated.Version, created.Subscription.Version)
	}
}

// DISABLED is the system's conclusion from a run of failures, not a value to type. A subscription
// somebody wants stopped is paused, and the two say different things to whoever reads the list.
func TestDisabledCannotBeSetByHandAndReEnablingIsAudited(t *testing.T) {
	h := newHarness()
	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), validCreate()); err != nil {
		t.Fatalf("creating: %v", err)
	}

	byHand := string(domain.SubscriptionDisabled)
	_, err := UpdateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(),
		UpdateWebhookSubscriptionCommand{ID: hookID, State: &byHand})
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "webhooks.state_not_settable" {
		t.Fatalf("error = %v, want the not-settable refusal", err)
	}

	// A subscription unreachability disabled, re-enabled by hand: an ordinary write, and the trail
	// says who decided the target is reachable again.
	stored := h.store.rows[hookID]
	for range domain.MaxConsecutiveFailures {
		stored.Subscription, _ = stored.Subscription.Failed(now, "webhooks.target_unreachable")
	}
	h.store.rows[hookID] = stored

	active := string(domain.SubscriptionActive)
	before := len(h.sink.entries)
	enabled, err := UpdateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(),
		UpdateWebhookSubscriptionCommand{ID: hookID, State: &active})
	if err != nil {
		t.Fatalf("re-enabling: %v", err)
	}
	if enabled.State != domain.SubscriptionActive || enabled.FailureCount != 0 {
		t.Errorf("re-enabling left %+v", enabled)
	}
	if len(h.sink.entries) != before+1 {
		t.Error("re-enabling wrote no audit entry")
	}
}

// The version in the request is what the caller last read. A mismatch is a conflict rather than an
// overwrite of somebody else's change.
func TestAStaleVersionIsAConflictRatherThanAnOverwrite(t *testing.T) {
	h := newHarness()
	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), validCreate()); err != nil {
		t.Fatalf("creating: %v", err)
	}

	target := "https://elsewhere.example/hooks"
	_, err := UpdateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(),
		UpdateWebhookSubscriptionCommand{ID: hookID, TargetURL: &target, ExpectedVersion: 99})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error = %v, want a conflict", err)
	}
	if h.store.rows[hookID].Subscription.TargetURL == target {
		t.Error("the stale write landed anyway")
	}
}

// The catalogue path, which is what REST, MCP and a rule all reach the use cases through.
func TestTheFiveReachTheirWorkThroughTheirDescriptors(t *testing.T) {
	h := newHarness()

	create := CreateWebhookSubscription{Writer: h.writer}.Descriptor()
	input := usecase.Input{
		"target_url":  "https://example.org/hooks/hubtask",
		"event_types": []any{someType},
	}
	if err := create.ValidateInput(input); err != nil {
		t.Fatalf("the creation's own input is refused by its declaration: %v", err)
	}
	out, err := create.Handler.Invoke(t.Context(), actor(), input)
	if err != nil {
		t.Fatalf("creating through the descriptor failed: %v", err)
	}
	if out.String("secret") == "" {
		t.Error("the mint answered no secret")
	}

	get := GetWebhookSubscription{Writer: h.writer}.Descriptor()
	if _, err := get.Handler.Invoke(t.Context(), actor(), usecase.Input{"webhook_id": hookID.String()}); err != nil {
		t.Fatalf("reading through the descriptor failed: %v", err)
	}

	list := ListWebhookSubscriptions{Writer: h.writer}.Descriptor()
	listed, err := list.Handler.Invoke(t.Context(), actor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing through the descriptor failed: %v", err)
	}
	rows, _ := listed["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("listed %d subscriptions, want one", len(rows))
	}
	if _, present := rows[0]["secret"]; present {
		t.Error("a listed subscription carries its secret")
	}

	update := UpdateWebhookSubscription{Writer: h.writer}.Descriptor()
	if _, err := update.Handler.Invoke(t.Context(), actor(), usecase.Input{
		"webhook_id": hookID.String(), "state": "PAUSED",
	}); err != nil {
		t.Fatalf("updating through the descriptor failed: %v", err)
	}

	remove := DeleteWebhookSubscription{Writer: h.writer}.Descriptor()
	if _, err := remove.Handler.Invoke(t.Context(), actor(), usecase.Input{"webhook_id": hookID.String()}); err != nil {
		t.Fatalf("deleting through the descriptor failed: %v", err)
	}

	if !create.Audit.Required || !update.Audit.Required || !remove.Audit.Required {
		t.Error("a write does not declare its audit obligation")
	}
	if !list.ReadOnly || !get.ReadOnly || !remove.Destructive {
		t.Error("the MCP annotations do not match what the use cases do")
	}
}

// targetQuotaFake refuses when told to - the resolution is the quota engine's; this package
// owes that the wall holds the subscription door (H-08).
type targetQuotaFake struct{ refused error }

func (q targetQuotaFake) WebhookTargets(context.Context, string) error { return q.refused }

func TestTheTargetsCeilingHoldsTheSubscriptionDoor(t *testing.T) {
	h := newHarness()

	_, err := (CreateWebhookSubscription{
		Writer: h.writer,
		Quota:  targetQuotaFake{refused: shared.ErrValidation.WithDetail("capacity.webhook_targets")},
	}).Execute(t.Context(), actor(), validCreate())

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "capacity.webhook_targets" {
		t.Errorf("answer %v, want capacity.webhook_targets", err)
	}
	if len(h.store.rows) != 0 {
		t.Error("a subscription landed past the wall")
	}
}
