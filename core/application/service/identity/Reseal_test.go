// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// rotatingRing is an encryptor mid-rotation: k2 current, k1 still held, anything else unknown.
type rotatingRing struct{ purposes []cryptoport.Purpose }

func (r *rotatingRing) Seal(context.Context, secret.Secret, cryptoport.Purpose) (cryptoport.Sealed, error) {
	return cryptoport.Sealed{}, nil
}

func (r *rotatingRing) Open(context.Context, cryptoport.Sealed, cryptoport.Purpose) (secret.Secret, error) {
	return secret.Secret{}, nil
}

func (r *rotatingRing) Rewrap(_ context.Context, sealed cryptoport.Sealed, purpose cryptoport.Purpose) (cryptoport.Sealed, error) {
	r.purposes = append(r.purposes, purpose)
	if sealed.KeyID != "k1" {
		return cryptoport.Sealed{}, shared.ErrUnavailable.WithDetail(cryptoport.CodeUnknownKey)
	}
	return cryptoport.Sealed{KeyID: "k2", Ciphertext: sealed.Ciphertext}, nil
}

func (r *rotatingRing) ActiveKeyID() string { return "k2" }
func (r *rotatingRing) KeyIDs() []string    { return []string{"k2", "k1"} }

type mfaSealings struct {
	rows         []repository.MfaEnrollment
	replacements []repository.MfaEnrollment
	rewrapped    []string
}

func (m *mfaSealings) ReplacementsSealedNotUnder(_ context.Context, keyID string) ([]repository.MfaEnrollment, error) {
	var out []repository.MfaEnrollment
	for _, row := range m.replacements {
		if row.Secret.KeyID != keyID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (m *mfaSealings) RewrapReplacement(_ context.Context, accountID shared.ID, sealed cryptoport.Sealed, expected string) (bool, error) {
	m.rewrapped = append(m.rewrapped, "replacement "+accountID.String()+":"+expected+"->"+sealed.KeyID)
	return true, nil
}

func (m *mfaSealings) SealedNotUnder(_ context.Context, keyID string) ([]repository.MfaEnrollment, error) {
	var out []repository.MfaEnrollment
	for _, row := range m.rows {
		if row.Secret.KeyID != keyID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (m *mfaSealings) Rewrap(_ context.Context, accountID shared.ID, sealed cryptoport.Sealed, expected string) (bool, error) {
	m.rewrapped = append(m.rewrapped, accountID.String()+":"+expected+"->"+sealed.KeyID)
	return true, nil
}

func TestTheSecondFactorsMoveUnderTheirOwnPurposeAndAForeignKeyIsSkipped(t *testing.T) {
	first := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000c1")
	second := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000c2")
	current := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000c3")
	store := &mfaSealings{rows: []repository.MfaEnrollment{
		{AccountID: first, Secret: cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("a")}},
		{AccountID: second, Secret: cryptoport.Sealed{KeyID: "gone", Ciphertext: []byte("b")}},
		{AccountID: current, Secret: cryptoport.Sealed{KeyID: "k2", Ciphertext: []byte("c")}},
	}}
	ring := &rotatingRing{}

	outcome, err := MfaResealer{Enrollments: store, Encryptor: ring}.Reseal(t.Context(), shared.ID(""))
	if err != nil {
		t.Fatalf("re-sealing: %v", err)
	}
	if outcome.Rewrapped != 1 || outcome.Skipped != 1 {
		t.Errorf("outcome %+v", outcome)
	}
	if len(store.rewrapped) != 1 || store.rewrapped[0] != first.String()+":k1->k2" {
		t.Errorf("rewrapped %v", store.rewrapped)
	}
	// The purpose is the row's own, the one verification binds to - never a value handed in.
	if len(ring.purposes) != 2 || ring.purposes[0] != mfaSecretPurpose(first) {
		t.Errorf("purposes %v", ring.purposes)
	}
}

// A replacement waiting to be confirmed (SC-17) is a sealed value too, under the same purpose as the
// factor it will become: the census counts it, so the re-seal moves it.
func TestAWaitingReplacementMovesUnderTheFactorsPurpose(t *testing.T) {
	waiting := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000c4")
	store := &mfaSealings{replacements: []repository.MfaEnrollment{
		{AccountID: waiting, Secret: cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("r")}},
	}}
	ring := &rotatingRing{}

	outcome, err := MfaResealer{Enrollments: store, Encryptor: ring}.Reseal(t.Context(), shared.ID(""))
	if err != nil {
		t.Fatalf("re-sealing: %v", err)
	}
	if outcome.Rewrapped != 1 {
		t.Errorf("outcome %+v", outcome)
	}
	if len(store.rewrapped) != 1 || store.rewrapped[0] != "replacement "+waiting.String()+":k1->k2" {
		t.Errorf("rewrapped %v", store.rewrapped)
	}
	if len(ring.purposes) != 1 || ring.purposes[0] != mfaSecretPurpose(waiting) {
		t.Errorf("purposes %v", ring.purposes)
	}
}

type resealProviderStore struct {
	sealed    cryptoport.Sealed
	err       error
	rewrapped []string
}

// resealProviderRow is the one row these tests work through. Plural since SI-10, and the fake
// answers a list of one: what the tests are about is the purpose and the guard, not the count.
const resealProviderRow = shared.ID("018f2a1b-0000-7000-8000-0000000000e9")

func (p *resealProviderStore) ListSealed(context.Context) ([]repository.SealedProviderSecret, error) {
	if p.err != nil {
		return nil, p.err
	}
	if p.sealed.IsZero() {
		return nil, nil
	}
	return []repository.SealedProviderSecret{
		{ProviderID: resealProviderRow, Sealed: p.sealed},
	}, nil
}

func (p *resealProviderStore) RewrapSecret(
	_ context.Context, providerID shared.ID, sealed cryptoport.Sealed, expected string,
) (bool, error) {
	p.rewrapped = append(p.rewrapped, providerID.String()+":"+expected+"->"+sealed.KeyID)
	return true, nil
}

func TestTheClientSecretMovesUnderTheWorkspacesPurpose(t *testing.T) {
	tenant := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000d1")
	ring := &rotatingRing{}
	store := &resealProviderStore{sealed: cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("s")}}

	outcome, err := IdentityProviderResealer{Providers: store, Encryptor: ring}.Reseal(t.Context(), tenant)
	if err != nil || outcome.Rewrapped != 1 {
		t.Fatalf("re-sealing: %+v, %v", outcome, err)
	}
	if len(ring.purposes) != 1 || ring.purposes[0] != ClientSecretPurpose(tenant) {
		t.Errorf("purposes %v", ring.purposes)
	}
	if len(store.rewrapped) != 1 || store.rewrapped[0] != resealProviderRow.String()+":k1->k2" {
		t.Errorf("rewrapped %v", store.rewrapped)
	}
}

func TestAWorkspaceWithoutAProviderOrAlreadyCurrentHasNothingToMove(t *testing.T) {
	ring := &rotatingRing{}
	none := &resealProviderStore{err: shared.ErrNotFound.WithDetail("identity_provider.not_configured")}
	if outcome, err := (IdentityProviderResealer{Providers: none, Encryptor: ring}).Reseal(t.Context(), shared.ID("")); err != nil || outcome != (sealing.Outcome{}) {
		t.Errorf("no provider: %+v, %v", outcome, err)
	}
	current := &resealProviderStore{sealed: cryptoport.Sealed{KeyID: "k2", Ciphertext: []byte("s")}}
	if outcome, err := (IdentityProviderResealer{Providers: current, Encryptor: ring}).Reseal(t.Context(), shared.ID("")); err != nil || outcome != (sealing.Outcome{}) || len(current.rewrapped) != 0 {
		t.Errorf("already current: %+v, %v", outcome, err)
	}
	gone := &resealProviderStore{sealed: cryptoport.Sealed{KeyID: "gone", Ciphertext: []byte("s")}}
	if outcome, err := (IdentityProviderResealer{Providers: gone, Encryptor: ring}).Reseal(t.Context(), shared.ID("")); err != nil || outcome.Skipped != 1 {
		t.Errorf("a foreign key: %+v, %v", outcome, err)
	}
	broken := &resealProviderStore{err: errors.New("boom")}
	if _, err := (IdentityProviderResealer{Providers: broken, Encryptor: ring}).Reseal(t.Context(), shared.ID("")); err == nil {
		t.Error("a broken store did not fail the round")
	}
}
