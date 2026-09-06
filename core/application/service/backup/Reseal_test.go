// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
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

type credentialSealings struct {
	rows      []repository.SealedCredential
	rewrapped []string
}

func (c *credentialSealings) SealedNotUnder(context.Context, string) ([]repository.SealedCredential, error) {
	return c.rows, nil
}

func (c *credentialSealings) Rewrap(_ context.Context, id shared.ID, sealed crypto.Sealed, expected string) (bool, error) {
	c.rewrapped = append(c.rewrapped, id.String()+":"+expected+"->"+sealed.KeyID)
	return true, nil
}

func TestCredentialsMoveUnderTheirTargetsPurposeAndAForeignKeyIsSkipped(t *testing.T) {
	held := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000a1")
	lost := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000a2")
	store := &credentialSealings{rows: []repository.SealedCredential{
		{TargetID: held, Credential: crypto.Sealed{KeyID: "k1", Ciphertext: []byte("c")}},
		{TargetID: lost, Credential: crypto.Sealed{KeyID: "gone", Ciphertext: []byte("d")}},
	}}
	ring := &rotatingRing{}

	outcome, err := TargetResealer{Targets: store, Encryptor: ring}.Reseal(t.Context(), shared.ID(""))
	if err != nil || outcome.Rewrapped != 1 || outcome.Skipped != 1 {
		t.Fatalf("outcome %+v, %v", outcome, err)
	}
	if len(store.rewrapped) != 1 || store.rewrapped[0] != held.String()+":k1->k2" {
		t.Errorf("rewrapped %v", store.rewrapped)
	}
	if len(ring.purposes) != 2 || ring.purposes[0] != credentialPurpose(held) || ring.purposes[1] != credentialPurpose(lost) {
		t.Errorf("purposes %v", ring.purposes)
	}
}
