// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// rotatingRing is an encryptor mid-rotation: k2 current, k1 still held, anything else unknown.
type rotatingRing struct{ purposes []crypto.Purpose }

func (r *rotatingRing) Seal(context.Context, secret.Secret, crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{}, nil
}

func (r *rotatingRing) Open(context.Context, crypto.Sealed, crypto.Purpose) (secret.Secret, error) {
	return secret.Secret{}, nil
}

func (r *rotatingRing) Rewrap(_ context.Context, sealed crypto.Sealed, purpose crypto.Purpose) (crypto.Sealed, error) {
	r.purposes = append(r.purposes, purpose)
	if sealed.KeyID != "k1" {
		return crypto.Sealed{}, shared.ErrUnavailable.WithDetail(crypto.CodeUnknownKey)
	}
	return crypto.Sealed{KeyID: "k2", Ciphertext: sealed.Ciphertext}, nil
}

func (r *rotatingRing) ActiveKeyID() string { return "k2" }
func (r *rotatingRing) KeyIDs() []string    { return []string{"k2", "k1"} }

type webhookSealings struct {
	rows      []repository.StoredSubscription
	rewrapped []string
}

func (w *webhookSealings) SealedNotUnder(context.Context, string) ([]repository.StoredSubscription, error) {
	return w.rows, nil
}

func (w *webhookSealings) Rewrap(_ context.Context, id shared.ID, secret, previous repository.SealedSecret, version int) (bool, error) {
	w.rewrapped = append(w.rewrapped, id.String()+":"+secret.KeyID+"/"+previous.KeyID)
	return true, nil
}

func TestBothSigningSecretsMoveWhenTheGraceIsStillRunning(t *testing.T) {
	id := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000f1")
	store := &webhookSealings{rows: []repository.StoredSubscription{{
		Subscription: domain.WebhookSubscription{ID: id, Version: 4},
		Secret:       repository.SealedSecret{KeyID: "k1", Ciphertext: []byte("now")},
		Previous:     repository.SealedSecret{KeyID: "k1", Ciphertext: []byte("before")},
	}}}
	ring := &rotatingRing{}

	outcome, err := WebhookResealer{Subscriptions: store, Encryptor: ring}.Reseal(t.Context(), shared.ID(""))
	if err != nil || outcome.Rewrapped != 2 || outcome.Skipped != 0 {
		t.Fatalf("outcome %+v, %v", outcome, err)
	}
	if len(store.rewrapped) != 1 || store.rewrapped[0] != id.String()+":k2/k2" {
		t.Errorf("rewrapped %v", store.rewrapped)
	}
	for _, seen := range ring.purposes {
		if seen != SecretPurpose(id) {
			t.Errorf("a secret was rewrapped under %q, want the subscription's own", seen)
		}
	}
}

func TestASubscriptionUnderAForeignKeyIsCountedAndLeftAlone(t *testing.T) {
	id := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000f2")
	store := &webhookSealings{rows: []repository.StoredSubscription{{
		Subscription: domain.WebhookSubscription{ID: id, Version: 1},
		Secret:       repository.SealedSecret{KeyID: "gone", Ciphertext: []byte("x")},
	}}}

	outcome, err := WebhookResealer{Subscriptions: store, Encryptor: &rotatingRing{}}.Reseal(t.Context(), shared.ID(""))
	if err != nil || outcome.Rewrapped != 0 || outcome.Skipped != 1 {
		t.Fatalf("outcome %+v, %v", outcome, err)
	}
	if len(store.rewrapped) != 0 {
		t.Error("a subscription with nothing to move was written")
	}
}

type offeredSealings struct {
	rows      []repository.SealedOfferedKey
	rewrapped []string
}

func (o *offeredSealings) ListSealed(context.Context) ([]repository.SealedOfferedKey, error) {
	return o.rows, nil
}

func (o *offeredSealings) RewrapKey(_ context.Context, id shared.ID, sealed crypto.Sealed, expected string) (bool, error) {
	o.rewrapped = append(o.rewrapped, id.String()+":"+expected+"->"+sealed.KeyID)
	return true, nil
}

// Each offered key moves under its own row's purpose; one already under the current key is left
// alone, and one under a key the ring lacks is counted rather than failed on.
func TestEachOfferedModelsKeyMovesUnderItsOwnPurpose(t *testing.T) {
	first := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000e1")
	second := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000e2")
	current := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000e3")
	lost := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000e4")
	store := &offeredSealings{rows: []repository.SealedOfferedKey{
		{ProviderID: first, Sealed: crypto.Sealed{KeyID: "k1"}},
		{ProviderID: second, Sealed: crypto.Sealed{KeyID: "k1"}},
		{ProviderID: current, Sealed: crypto.Sealed{KeyID: "k2"}},
		{ProviderID: lost, Sealed: crypto.Sealed{KeyID: "gone"}},
	}}
	ring := &rotatingRing{}

	outcome, err := OfferedAiProviderResealer{Providers: store, Encryptor: ring}.Reseal(t.Context(), shared.ID(""))
	if err != nil || outcome.Rewrapped != 2 || outcome.Skipped != 1 {
		t.Fatalf("outcome %+v, %v", outcome, err)
	}
	want := []string{first.String() + ":k1->k2", second.String() + ":k1->k2"}
	if len(store.rewrapped) != 2 || store.rewrapped[0] != want[0] || store.rewrapped[1] != want[1] {
		t.Errorf("rewrapped %v, want %v", store.rewrapped, want)
	}
	if len(ring.purposes) != 3 || ring.purposes[0] != OfferedAiKeyPurpose(first) ||
		ring.purposes[1] != OfferedAiKeyPurpose(second) {
		t.Errorf("purposes %v", ring.purposes)
	}
}
