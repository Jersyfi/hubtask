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
	env "github.com/Jersyfi/hubtask/core/port/environment"
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

func (s *subscriptionStore) Outcome(_ context.Context, id shared.ID, _ time.Time, lastError string) error {
	s.outcome[id] = lastError
	return nil
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
		Instance: instance, Subscriptions: f.store, Encryptor: f.envelope,
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
