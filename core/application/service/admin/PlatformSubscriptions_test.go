// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// subscriptionStore is the installation's subscriptions as the table keeps them.
type subscriptionStore struct {
	rows    map[shared.ID]adminrepo.StoredPlatformSubscription
	order   []shared.ID
	outcome map[shared.ID]string
}

func newSubscriptionStore() *subscriptionStore {
	return &subscriptionStore{rows: map[shared.ID]adminrepo.StoredPlatformSubscription{}, outcome: map[shared.ID]string{}}
}

func (s *subscriptionStore) Insert(_ context.Context, sub integration.PlatformSubscription, secret adminrepo.PlatformSecret) error {
	s.rows[sub.ID] = adminrepo.StoredPlatformSubscription{Subscription: sub, Secret: secret}
	s.order = append(s.order, sub.ID)
	return nil
}

func (s *subscriptionStore) List(context.Context) ([]integration.PlatformSubscription, error) {
	listed := []integration.PlatformSubscription{}
	for _, id := range s.order {
		if stored, ok := s.rows[id]; ok {
			listed = append(listed, stored.Subscription)
		}
	}
	return listed, nil
}

func (s *subscriptionStore) Find(_ context.Context, id shared.ID) (adminrepo.StoredPlatformSubscription, error) {
	stored, ok := s.rows[id]
	if !ok {
		return adminrepo.StoredPlatformSubscription{}, shared.ErrNotFound.WithDetail("admin.subscription_not_found")
	}
	return stored, nil
}

func (s *subscriptionStore) Update(_ context.Context, sub integration.PlatformSubscription, expected int, _ time.Time) (bool, error) {
	stored, ok := s.rows[sub.ID]
	if !ok || stored.Subscription.Version != expected {
		return false, nil
	}
	sub.Version = expected + 1
	stored.Subscription = sub
	s.rows[sub.ID] = stored
	return true, nil
}

func (s *subscriptionStore) Rotate(_ context.Context, id shared.ID, secret adminrepo.PlatformSecret, _ time.Time) (bool, error) {
	stored, ok := s.rows[id]
	if !ok {
		return false, nil
	}
	stored.Secret = secret
	stored.Subscription.Version++
	s.rows[id] = stored
	return true, nil
}

func (s *subscriptionStore) Delete(_ context.Context, id shared.ID) (bool, error) {
	_, ok := s.rows[id]
	delete(s.rows, id)
	return ok, nil
}

func (s *subscriptionStore) Active(context.Context) ([]shared.ID, error) {
	var active []shared.ID
	for _, id := range s.order {
		if stored, ok := s.rows[id]; ok && stored.Subscription.State == integration.PlatformActive {
			active = append(active, id)
		}
	}
	return active, nil
}

func (s *subscriptionStore) MarkFromFile(_ context.Context, id shared.ID) error {
	stored := s.rows[id]
	stored.Subscription.FromFile = true
	s.rows[id] = stored
	return nil
}

func (s *subscriptionStore) Outcome(_ context.Context, id shared.ID, _ time.Time, lastError string) error {
	s.outcome[id] = lastError
	return nil
}

// roundTrip opens what it sealed, as the envelope does.
type roundTrip struct{ *plainEnvelope }

func (e roundTrip) Open(_ context.Context, sealed cryptoport.Sealed, _ cryptoport.Purpose) (secret.Secret, error) {
	return secret.New(strings.TrimPrefix(string(sealed.Ciphertext), "sealed:")), nil
}

type platformFixture struct {
	writer   PlatformWriter
	store    *subscriptionStore
	journal  *journalStore
	envelope *plainEnvelope
	registry *usecase.Registry
}

func newPlatformFixture(t *testing.T, tenancy env.TenancyMode, descriptors ...func(PlatformWriter) usecase.Descriptor) platformFixture {
	t.Helper()
	instance, _, journal := newInstanceWriter(newRegister())
	f := platformFixture{store: newSubscriptionStore(), journal: journal, envelope: &plainEnvelope{}}
	f.writer = PlatformWriter{
		Instance: instance, Subscriptions: f.store, Encryptor: roundTrip{f.envelope},
		Entropy: clock.FixedEntropy{}, StepUp: &stepUpFake{expect: "hbt_sup_proof"}, Tenancy: tenancy,
	}
	list := []usecase.Descriptor{
		ListPlatformSubscriptions{f.writer}.Descriptor(), CreatePlatformSubscription{f.writer}.Descriptor(),
		ReadPlatformSubscription{f.writer}.Descriptor(),
	}
	for _, d := range descriptors {
		list = append(list, d(f.writer))
	}
	registry, err := usecase.NewRegistry(nil, list...)
	if err != nil {
		t.Fatal(err)
	}
	f.registry = registry
	return f
}

func subscribe(t *testing.T, f platformFixture) usecase.Output {
	t.Helper()
	out, err := f.registry.Invoke(t.Context(), CreatePlatformSubscriptionName, operator(), usecase.Input{
		"target_url": "https://shop.example/hubtask", "step_up_token": "hbt_sup_proof",
	})
	if err != nil {
		t.Fatalf("subscribing: %v", err)
	}
	return out
}

// Subscribing answers the secret once and seals it for this subscription; neither the list nor a
// read answers it again, and the journal names the subscription by its id alone (UC-INS-14 check 1).
func TestThePlatformIsSubscribedWithItsSecretShownOnce(t *testing.T) {
	f := newPlatformFixture(t, env.TenancyMulti)
	created := subscribe(t, f)
	id := shared.ID(created.String("id"))

	if created.String("secret") == "" || created.String("state") != "ACTIVE" || created.String("target_url") != "https://shop.example/hubtask" {
		t.Errorf("answered %v", created)
	}
	stored := f.store.rows[id]
	if strings.Contains(string(stored.Secret.Ciphertext), "https") || !strings.Contains(string(stored.Secret.Ciphertext), created.String("secret")) {
		t.Errorf("stored %q", stored.Secret.Ciphertext)
	}
	if len(f.envelope.purposes) != 1 || string(f.envelope.purposes[0]) != integration.PlatformSecretPurposeFor(id) {
		t.Errorf("sealed for %v", f.envelope.purposes)
	}

	listed, err := f.registry.Invoke(t.Context(), ListPlatformSubscriptionsName, operator(), usecase.Input{})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := listed["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0]["secret"] != nil {
		t.Errorf("listed %v", rows)
	}
	read, err := f.registry.Invoke(t.Context(), ReadPlatformSubscriptionName, operator(), usecase.Input{"subscription_id": id.String()})
	if err != nil || read["secret"] != nil || read.String("id") != id.String() {
		t.Errorf("read %v, %v", read, err)
	}

	last := f.journal.entries[len(f.journal.entries)-1]
	if last.Action != string(platformSubscriptionCreatedAction) || last.Details["subscription_id"] != id.String() {
		t.Errorf("journal %+v", last)
	}
	for _, value := range last.Details {
		if text, ok := value.(string); ok && strings.Contains(text, "shop.example") {
			t.Errorf("the journal carries the address: %v", last.Details)
		}
	}
}

func TestASubscriptionIsRefusedWhatTheRulesRefuse(t *testing.T) {
	cases := []struct {
		name    string
		tenancy env.TenancyMode
		in      usecase.Input
		want    string
		kind    error
	}{
		{"single mode", env.TenancySingle, usecase.Input{"target_url": "https://shop.example", "step_up_token": "hbt_sup_proof"},
			"admin.multi_mode_required", shared.ErrForbidden},
		{"no step-up", env.TenancyMulti, usecase.Input{"target_url": "https://shop.example"},
			"auth.step_up_required", shared.ErrForbidden},
		{"a malformed target", env.TenancyMulti, usecase.Input{"target_url": "ftp://shop.example", "step_up_token": "hbt_sup_proof"},
			"webhooks.target_scheme_unsupported", shared.ErrValidation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newPlatformFixture(t, c.tenancy)
			_, err := f.registry.Invoke(t.Context(), CreatePlatformSubscriptionName, operator(), c.in)
			if refusalCode(err) != c.want || !errors.Is(err, c.kind) {
				t.Errorf("answered %v, want %s", err, c.want)
			}
			if len(f.store.rows) != 0 {
				t.Error("a refused subscription was written")
			}
		})
	}

	enforced := newPlatformFixture(t, env.TenancyMulti)
	enforced.writer.Enforced = true
	registry, err := usecase.NewRegistry(nil, CreatePlatformSubscription{enforced.writer}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Invoke(t.Context(), CreatePlatformSubscriptionName, operator(), usecase.Input{
		"target_url": "https://shop.example", "step_up_token": "hbt_sup_proof",
	})
	if refusalCode(err) != "admin.instance_enforced_from_file" {
		t.Errorf("a subscription while the file decides answered %v", err)
	}
}

func TestAnUnknownSubscriptionIsNotFound(t *testing.T) {
	f := newPlatformFixture(t, env.TenancyMulti)
	_, err := f.registry.Invoke(t.Context(), ReadPlatformSubscriptionName, operator(),
		usecase.Input{"subscription_id": "0192f000-0000-7000-8000-0000000000ff"})
	if refusalCode(err) != "admin.subscription_not_found" || !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("answered %v", err)
	}
}

func changeDescriptors() []func(PlatformWriter) usecase.Descriptor {
	return []func(PlatformWriter) usecase.Descriptor{
		func(w PlatformWriter) usecase.Descriptor { return UpdatePlatformSubscription{w}.Descriptor() },
		func(w PlatformWriter) usecase.Descriptor { return DeletePlatformSubscription{w}.Descriptor() },
		func(w PlatformWriter) usecase.Descriptor { return RotatePlatformSubscriptionSecret{w}.Descriptor() },
	}
}

// Pausing and resuming need no proof; a new target does, and a stale version is refused.
func TestASubscriptionIsPausedRetargetedAndGuardedOnItsVersion(t *testing.T) {
	f := newPlatformFixture(t, env.TenancyMulti, changeDescriptors()...)
	id := subscribe(t, f).String("id")

	paused, err := f.registry.Invoke(t.Context(), UpdatePlatformSubscriptionName, operator(), usecase.Input{
		"subscription_id": id, "state": "PAUSED", "expected_version": 1,
	})
	if err != nil || paused.String("state") != "PAUSED" || paused.Int("version") != 2 {
		t.Fatalf("pausing: %v, %v", paused, err)
	}
	_, err = f.registry.Invoke(t.Context(), UpdatePlatformSubscriptionName, operator(), usecase.Input{
		"subscription_id": id, "state": "ACTIVE", "expected_version": 1,
	})
	if refusalCode(err) != "admin.subscription_version_moved" || !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version answered %v", err)
	}
	proof := f.writer.StepUp.(*stepUpFake)
	spent := proof.consumed
	_, err = f.registry.Invoke(t.Context(), UpdatePlatformSubscriptionName, operator(), usecase.Input{
		"subscription_id": id, "target_url": "https://elsewhere.example/hook", "expected_version": 1,
		"step_up_token": "hbt_sup_proof",
	})
	if refusalCode(err) != "admin.subscription_version_moved" || proof.consumed != spent {
		t.Errorf("a stale retarget answered %v and spent %d proofs", err, proof.consumed-spent)
	}
	_, err = f.registry.Invoke(t.Context(), UpdatePlatformSubscriptionName, operator(), usecase.Input{
		"subscription_id": id, "target_url": "https://elsewhere.example/hook",
	})
	if refusalCode(err) != "auth.step_up_required" {
		t.Errorf("a new target without a proof answered %v", err)
	}
	moved, err := f.registry.Invoke(t.Context(), UpdatePlatformSubscriptionName, operator(), usecase.Input{
		"subscription_id": id, "target_url": "https://elsewhere.example/hook", "step_up_token": "hbt_sup_proof",
	})
	if err != nil || moved.String("target_url") != "https://elsewhere.example/hook" {
		t.Errorf("retargeting: %v, %v", moved, err)
	}
	_, err = f.registry.Invoke(t.Context(), UpdatePlatformSubscriptionName, operator(), usecase.Input{
		"subscription_id": id, "state": "DISABLED",
	})
	if refusalCode(err) != "admin.subscription_state_invalid" {
		t.Errorf("switching a subscription off answered %v", err)
	}
}

func TestASubscriptionsSecretIsRotatedOnceShownAndItIsRemoved(t *testing.T) {
	f := newPlatformFixture(t, env.TenancyMulti, changeDescriptors()...)
	created := subscribe(t, f)
	id := shared.ID(created.String("id"))

	if _, err := f.registry.Invoke(t.Context(), RotatePlatformSubscriptionSecretName, operator(),
		usecase.Input{"subscription_id": id.String()}); refusalCode(err) != "auth.step_up_required" {
		t.Errorf("a rotation without a proof answered %v", err)
	}
	rotated, err := f.registry.Invoke(t.Context(), RotatePlatformSubscriptionSecretName, operator(),
		usecase.Input{"subscription_id": id.String(), "step_up_token": "hbt_sup_proof"})
	if err != nil || rotated.String("secret") == "" {
		t.Fatalf("rotating: %v, %v", rotated, err)
	}
	if !strings.Contains(string(f.store.rows[id].Secret.Ciphertext), rotated.String("secret")) {
		t.Error("the stored secret is not the rotated one")
	}

	if _, err := f.registry.Invoke(t.Context(), DeletePlatformSubscriptionName, operator(),
		usecase.Input{"subscription_id": id.String()}); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if len(f.store.rows) != 0 {
		t.Error("the subscription is still there")
	}
	if _, err := f.registry.Invoke(t.Context(), DeletePlatformSubscriptionName, operator(),
		usecase.Input{"subscription_id": id.String()}); refusalCode(err) != "admin.subscription_not_found" {
		t.Errorf("removing it again answered %v", err)
	}
	actions := map[string]bool{}
	for _, entry := range f.journal.entries {
		actions[entry.Action] = true
	}
	if !actions[string(platformSubscriptionRotatedAction)] || !actions[string(platformSubscriptionRemovedAction)] {
		t.Errorf("journal %v", actions)
	}
}

// deliveryStore is a subscription's deliveries as the table keeps them.
type deliveryStore struct {
	adminrepo.PlatformDeliveries
	rows []integration.PlatformDelivery
}

func (d *deliveryStore) Page(_ context.Context, sub shared.ID, status string, before shared.ID, size int) ([]integration.PlatformDelivery, error) {
	var page []integration.PlatformDelivery
	for i := len(d.rows) - 1; i >= 0; i-- {
		row := d.rows[i]
		if row.SubscriptionID != sub || (status != "" && string(row.Status) != status) || (!before.IsZero() && row.ID >= before) {
			continue
		}
		if len(page) < size {
			page = append(page, row)
		}
	}
	return page, nil
}

func (d *deliveryStore) Requeue(_ context.Context, sub shared.ID, _ time.Time, batch int) ([]shared.ID, error) {
	var ids []shared.ID
	for i, row := range d.rows {
		if row.SubscriptionID == sub && row.Status == integration.DeliveryDeadLetter && len(ids) < batch {
			d.rows[i].Status = integration.DeliveryPending
			ids = append(ids, row.ID)
		}
	}
	return ids, nil
}

type idCursors struct{}

func (idCursors) EncodeID(id shared.ID) string { return "c:" + id.String() }
func (idCursors) DecodeID(cursor string) (shared.ID, error) {
	return shared.ID(strings.TrimPrefix(cursor, "c:")), nil
}

func deliveriesOf(sub shared.ID, statuses ...integration.DeliveryStatus) *deliveryStore {
	store := &deliveryStore{}
	for i, status := range statuses {
		store.rows = append(store.rows, integration.PlatformDelivery{
			ID: shared.ID("0192f000-0000-7000-8000-0000000001" + string(rune('a'+i)) + "0"), SubscriptionID: sub,
			EventID: "e", EventType: "t", Attempt: 8, Status: status,
		})
	}
	return store
}

// A subscription's deliveries read page by page, newest first, and its dead letters queued again -
// each once, as installation jobs - while it is active; paused, the redelivery is refused.
func TestASubscriptionsDeliveriesAreReadAndItsDeadLettersRedelivered(t *testing.T) {
	jobs := &jobsFake{}
	sub := shared.ID("0192f000-0000-7000-8000-0000000000b1")
	deliveries := deliveriesOf(sub, integration.DeliveryDeadLetter, integration.DeliverySucceeded, integration.DeliveryDeadLetter)
	f := newPlatformFixture(t, env.TenancyMulti, append(changeDescriptors(),
		func(w PlatformWriter) usecase.Descriptor {
			return ListPlatformDeliveries{Writer: w, Deliveries: deliveries, Cursors: idCursors{}}.Descriptor()
		},
		func(w PlatformWriter) usecase.Descriptor {
			return RedeliverPlatformSubscription{Writer: w, Deliveries: deliveries, Jobs: jobs}.Descriptor()
		})...)
	created, _ := integration.NewPlatformSubscription(sub, "https://shop.example", false, "", time.Now())
	if err := f.store.Insert(t.Context(), created, adminrepo.PlatformSecret{}); err != nil {
		t.Fatal(err)
	}

	first, err := f.registry.Invoke(t.Context(), ListPlatformDeliveriesName, operator(), usecase.Input{"subscription_id": sub.String(), "limit": 2})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := first["data"].([]usecase.Output)
	page, _ := first["page"].(map[string]any)
	if len(rows) != 2 || page["has_more"] != true || page["next_cursor"] == nil {
		t.Fatalf("first page %v %v", rows, page)
	}
	second, err := f.registry.Invoke(t.Context(), ListPlatformDeliveriesName, operator(), usecase.Input{
		"subscription_id": sub.String(), "limit": 2, "cursor": page["next_cursor"],
	})
	if rest, _ := second["data"].([]usecase.Output); err != nil || len(rest) != 1 {
		t.Errorf("second page %v, %v", second, err)
	}

	out, err := f.registry.Invoke(t.Context(), RedeliverPlatformSubscriptionName, operator(), usecase.Input{"subscription_id": sub.String()})
	if err != nil || out.Int("queued") != 2 || len(jobs.requests) != 2 {
		t.Fatalf("redelivered %v (%v), %d jobs", out, err, len(jobs.requests))
	}
	for _, request := range jobs.requests {
		if request.Kind != queue.KindPlatformDeliver || !request.TenantID.IsZero() {
			t.Errorf("asked %+v", request)
		}
	}
	again, _ := f.registry.Invoke(t.Context(), RedeliverPlatformSubscriptionName, operator(), usecase.Input{"subscription_id": sub.String()})
	if again.Int("queued") != 0 {
		t.Errorf("a second redelivery queued %v", again)
	}

	paused, _ := created.InState(integration.PlatformPaused)
	f.store.rows[sub] = adminrepo.StoredPlatformSubscription{Subscription: paused}
	_, err = f.registry.Invoke(t.Context(), RedeliverPlatformSubscriptionName, operator(), usecase.Input{"subscription_id": sub.String()})
	if refusalCode(err) != "admin.subscription_paused" || !errors.Is(err, shared.ErrConflict) {
		t.Errorf("redelivering a paused subscription answered %v", err)
	}
}

// Seeding writes the file's subscriptions once, where none stands, and leaves them the operator's;
// enforcing makes the file the whole set at every start, marks what it keeps as the file's, and
// rotates a secret only where the file's differs - a restart writes nothing.
func TestTheInstanceFileDecidesTheSubscriptions(t *testing.T) {
	file := []FileSubscription{
		{TargetURL: "https://shop.example/hubtask", Secret: secret.New("from-the-file")},
		{TargetURL: "https://billing.example/hook", Secret: secret.New("another")},
	}

	seeding := newPlatformFixture(t, env.TenancyMulti)
	seeded, err := seeding.writer.ApplyFile(t.Context(), file, false)
	if err != nil || !seeded.Seeded || seeded.Created != 2 {
		t.Fatalf("seeding an empty installation: %+v, %v", seeded, err)
	}
	for _, stored := range seeding.store.rows {
		if stored.Subscription.FromFile {
			t.Error("a seeded subscription is not the operator's")
		}
	}
	again, err := seeding.writer.ApplyFile(t.Context(), file[:1], false)
	if err != nil || again.Seeded || len(seeding.store.rows) != 2 {
		t.Errorf("seeding again wrote %+v, %v", again, err)
	}

	f := newPlatformFixture(t, env.TenancyMulti)
	byHand := subscribe(t, f)
	handID := shared.ID(byHand.String("id"))
	enforced, err := f.writer.ApplyFile(t.Context(), file, true)
	if err != nil || enforced.Created != 1 || enforced.Rotated != 1 {
		t.Fatalf("enforcing: %+v, %v", enforced, err)
	}
	if kept := f.store.rows[handID]; !kept.Subscription.FromFile || !strings.Contains(string(kept.Secret.Ciphertext), "from-the-file") {
		t.Errorf("the kept subscription: %+v", kept)
	}
	restarted, err := f.writer.ApplyFile(t.Context(), file, true)
	if err != nil || restarted.Rotated != 0 || restarted.Kept != 2 || restarted.Created != 0 {
		t.Errorf("a restart wrote %+v, %v", restarted, err)
	}
	narrowed, err := f.writer.ApplyFile(t.Context(), file[1:], true)
	if err != nil || narrowed.Removed != 1 || len(f.store.rows) != 1 {
		t.Fatalf("narrowing: %+v, %v (%d left)", narrowed, err, len(f.store.rows))
	}
	removed := false
	for _, entry := range f.journal.entries {
		if entry.Action == string(platformSubscriptionRemovedAction) && entry.ActorLabel == fileActor {
			removed = true
		}
	}
	if !removed {
		t.Error("the file's removal is not journalled")
	}

	single := newPlatformFixture(t, env.TenancySingle)
	if out, err := single.writer.ApplyFile(t.Context(), file, true); err != nil || !out.Skipped || len(single.store.rows) != 0 {
		t.Errorf("single mode applied %+v, %v", out, err)
	}
}

// UC-INS-06 check 6: the platform's token manages its subscription. A registered service account
// creates, retargets and rotates without the step-up it cannot give, as its deletion request does; a
// person with either scope still proves themselves.
func TestThePlatformsTokenManagesItsSubscriptionWithoutAStepUp(t *testing.T) {
	unregistered := shared.ID("018f2a1b-0000-7000-8000-0000000000c8")
	instance, _, _ := newInstanceWriter(newRegister(operatorID, platformID, unregistered))
	store := newSubscriptionStore()
	writer := PlatformWriter{
		Instance: instance, Subscriptions: store, Encryptor: roundTrip{&plainEnvelope{}},
		Entropy: clock.FixedEntropy{}, StepUp: &stepUpFake{expect: "hbt_sup_proof"}, Tenancy: env.TenancyMulti,
		Register: &platformRegister{rows: map[shared.ID]bool{platformID: true}},
	}
	registry, err := usecase.NewRegistry(nil,
		CreatePlatformSubscription{writer}.Descriptor(), RotatePlatformSubscriptionSecret{writer}.Descriptor(),
		ListPlatformSubscriptions{writer}.Descriptor(), UpdatePlatformSubscription{writer}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}

	created, err := registry.Invoke(t.Context(), CreatePlatformSubscriptionName, shopActor(),
		usecase.Input{"target_url": "https://shop.example/hubtask"})
	if err != nil {
		t.Fatalf("the platform's token subscribing without a step-up: %v", err)
	}
	if _, err := registry.Invoke(t.Context(), RotatePlatformSubscriptionSecretName, shopActor(),
		usecase.Input{"subscription_id": created.String("id")}); err != nil {
		t.Errorf("the platform's token rotating its secret: %v", err)
	}
	if _, err := registry.Invoke(t.Context(), ListPlatformSubscriptionsName, shopActor(), usecase.Input{}); err != nil {
		t.Errorf("the platform's token listing: %v", err)
	}
	if _, err := registry.Invoke(t.Context(), UpdatePlatformSubscriptionName, shopActor(),
		usecase.Input{"subscription_id": created.String("id"), "target_url": "https://shop.example/moved"}); err != nil {
		t.Errorf("the platform's token retargeting its subscription: %v", err)
	}

	// A service account the register does not name as one proves itself like a person.
	stranger := shopActor()
	stranger.AccountID = unregistered
	_, err = registry.Invoke(t.Context(), CreatePlatformSubscriptionName, stranger,
		usecase.Input{"target_url": "https://shop.example/stranger"})
	if refusalCode(err) != "auth.step_up_required" {
		t.Errorf("an unregistered service account subscribing without a step-up answered %v", err)
	}

	_, err = registry.Invoke(t.Context(), CreatePlatformSubscriptionName, operator(),
		usecase.Input{"target_url": "https://shop.example/other"})
	if refusalCode(err) != "auth.step_up_required" {
		t.Errorf("a person subscribing without a step-up answered %v, want auth.step_up_required", err)
	}
}
