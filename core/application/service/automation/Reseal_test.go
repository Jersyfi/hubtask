// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"encoding/base64"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
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
	return crypto.Sealed{KeyID: "k2", Ciphertext: append([]byte("moved:"), sealed.Ciphertext...)}, nil
}

func (r *rotatingRing) ActiveKeyID() string { return "k2" }
func (r *rotatingRing) KeyIDs() []string    { return []string{"k2", "k1"} }

type ruleSealings struct {
	rules     []domain.Rule
	rewritten map[string][]domain.Action
}

func (s *ruleSealings) SealedNotUnder(context.Context, string) ([]domain.Rule, error) {
	return s.rules, nil
}

func (s *ruleSealings) RewrapActions(_ context.Context, id shared.ID, actions []domain.Action, _ int) (bool, error) {
	if s.rewritten == nil {
		s.rewritten = map[string][]domain.Action{}
	}
	s.rewritten[id.String()] = actions
	return true, nil
}

func sealedDocument(keyID, ciphertext, purpose string) map[string]any {
	return domain.SealedSecret{
		Ciphertext: base64.StdEncoding.EncodeToString([]byte(ciphertext)),
		KeyID:      keyID, Purpose: purpose,
	}.Document()
}

func resealHTTPAction(sealed map[string]any) map[string]any {
	return map[string]any{
		"method": "POST", "url": "https://hooks.example/x", "secret_header_name": "X-Token",
		"secret_header_sealed": sealed,
	}
}

func TestHeaderSecretsMoveAtAnyDepthUnderTheRulesOwnPurpose(t *testing.T) {
	ruleID := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000e1")
	purpose := string(ruleSecretPurpose(ruleID))
	rule := domain.Rule{ID: ruleID, Version: 3, Actions: []domain.Action{
		{Kind: domain.ActionHTTPRequest, Params: resealHTTPAction(sealedDocument("k1", "top", purpose))},
		{Kind: domain.ActionBranch, Params: map[string]any{
			"then": []any{map[string]any{
				"kind": domain.ActionHTTPRequest, "params": resealHTTPAction(sealedDocument("k1", "nested", purpose)),
			}},
			"else": []any{map[string]any{
				"kind": domain.ActionHTTPRequest, "params": resealHTTPAction(sealedDocument("gone", "lost", purpose)),
			}},
		}},
		{Kind: domain.ActionHTTPRequest, Params: resealHTTPAction(sealedDocument("k2", "current", purpose))},
	}}
	store := &ruleSealings{rules: []domain.Rule{rule}}
	ring := &rotatingRing{}

	outcome, err := RuleResealer{Rules: store, Encryptor: ring}.Reseal(t.Context(), shared.ID(""))
	if err != nil {
		t.Fatalf("re-sealing: %v", err)
	}
	if outcome.Rewrapped != 2 || outcome.Skipped != 1 {
		t.Errorf("outcome %+v", outcome)
	}
	for _, seen := range ring.purposes {
		if string(seen) != purpose {
			t.Errorf("a secret was rewrapped under %q, want the rule's own purpose", seen)
		}
	}

	written := store.rewritten[ruleID.String()]
	if written == nil {
		t.Fatal("the actions were not written back")
	}
	top, _ := domain.ReadHTTPRequest(written[0].Params, "/actions/0")
	if top.Sealed == nil || top.Sealed.KeyID != "k2" || top.Sealed.Purpose != purpose {
		t.Errorf("the top-level secret after the round: %+v", top.Sealed)
	}
	then := written[1].Params["then"].([]any)[0].(map[string]any)["params"].(map[string]any)
	nested, _ := domain.ReadHTTPRequest(then, "/actions/1/then/0")
	if nested.Sealed == nil || nested.Sealed.KeyID != "k2" {
		t.Errorf("the nested secret after the round: %+v", nested.Sealed)
	}
	lost := written[1].Params["else"].([]any)[0].(map[string]any)["params"].(map[string]any)
	stuck, _ := domain.ReadHTTPRequest(lost, "/actions/1/else/0")
	if stuck.Sealed == nil || stuck.Sealed.KeyID != "gone" {
		t.Errorf("a secret under a foreign key was touched: %+v", stuck.Sealed)
	}
}

func TestARuleWhoseSecretsAreAllForeignIsNotRewritten(t *testing.T) {
	ruleID := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000e2")
	rule := domain.Rule{ID: ruleID, Version: 1, Actions: []domain.Action{
		{Kind: domain.ActionHTTPRequest, Params: resealHTTPAction(sealedDocument("gone", "x", string(ruleSecretPurpose(ruleID))))},
	}}
	store := &ruleSealings{rules: []domain.Rule{rule}}

	outcome, err := RuleResealer{Rules: store, Encryptor: &rotatingRing{}}.Reseal(t.Context(), shared.ID(""))
	if err != nil || outcome.Rewrapped != 0 || outcome.Skipped != 1 {
		t.Fatalf("outcome %+v, %v", outcome, err)
	}
	if len(store.rewritten) != 0 {
		t.Error("a rule with nothing to move was written back")
	}
}
