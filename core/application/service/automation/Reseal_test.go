// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"encoding/base64"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
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

type callSealings struct {
	calls     []repository.PendingCall
	rewritten map[string]domain.SealedSecret
}

func (s *callSealings) SealedNotUnder(context.Context, string) ([]repository.PendingCall, error) {
	return s.calls, nil
}

func (s *callSealings) RewrapCall(_ context.Context, id shared.ID, sealed domain.SealedSecret, expected string) (bool, error) {
	if s.rewritten == nil {
		s.rewritten = map[string]domain.SealedSecret{}
	}
	if expected != "k1" {
		return false, nil
	}
	s.rewritten[id.String()] = sealed
	return true, nil
}

// refusingRing refuses a ciphertext that was not sealed for the purpose it names, the way the
// envelope does, so the round can be shown to continue past a caller-shaped job.
type refusingRing struct{ rotatingRing }

func (r *refusingRing) Rewrap(ctx context.Context, sealed crypto.Sealed, purpose crypto.Purpose) (crypto.Sealed, error) {
	switch string(sealed.Ciphertext) {
	case "forged":
		r.purposes = append(r.purposes, purpose)
		return crypto.Sealed{}, crypto.NotAuthentic()
	case "junk":
		r.purposes = append(r.purposes, purpose)
		return crypto.Sealed{}, shared.ErrValidation.WithDetail(crypto.CodeCiphertextMalformed)
	}
	return r.rotatingRing.Rewrap(ctx, sealed, purpose)
}

func pendingCall(job, request, keyID, ciphertext, purpose string) repository.PendingCall {
	return repository.PendingCall{
		JobID: shared.MustParseID(job), RequestID: request,
		Sealed: domain.SealedSecret{
			Ciphertext: base64.StdEncoding.EncodeToString([]byte(ciphertext)), KeyID: keyID, Purpose: purpose,
		},
	}
}

// UC-INS-16 check 2: a call still on its ladder moves under the current key, under the purpose it
// was sealed for - the request's own, or the rule's it was carried over from - and the round goes on
// past every job it cannot move rather than failing the workspace on one of them.
func TestPendingCallSecretsMoveUnderTheirOwnPurposeAndTheRestIsSkipped(t *testing.T) {
	request := "018f2a1b-0000-7000-8000-0000000000f1"
	other := "018f2a1b-0000-7000-8000-0000000000f2"
	rule := shared.MustParseID("018f2a1b-0000-7000-8000-0000000000f3")
	requestPurpose := string(requestSecretPurpose(shared.MustParseID(request)))
	rulePurpose := string(ruleSecretPurpose(rule))
	store := &callSealings{calls: []repository.PendingCall{
		pendingCall("018f2a1b-0000-7000-8000-0000000000a1", request, "k1", "direct", requestPurpose),
		pendingCall("018f2a1b-0000-7000-8000-0000000000a2", request, "k1", "from-a-rule", rulePurpose),
		// Another request's purpose: no job of this system is written so.
		pendingCall("018f2a1b-0000-7000-8000-0000000000a3", other, "k1", "lifted", requestPurpose),
		pendingCall("018f2a1b-0000-7000-8000-0000000000a4", request, "k1", "x", "webhook.secret:whatever"),
		pendingCall("018f2a1b-0000-7000-8000-0000000000a5", request, "gone", "lost", requestPurpose),
		pendingCall("018f2a1b-0000-7000-8000-0000000000a6", request, "k1", "forged", requestPurpose),
		pendingCall("018f2a1b-0000-7000-8000-0000000000a7", request, "k1", "junk", requestPurpose),
		{JobID: shared.MustParseID("018f2a1b-0000-7000-8000-0000000000a8"), RequestID: request,
			Sealed: domain.SealedSecret{Ciphertext: "%%not base64", KeyID: "k1", Purpose: requestPurpose}},
	}}
	ring := &refusingRing{}

	outcome, err := CallResealer{Calls: store, Encryptor: ring}.Reseal(t.Context(), shared.ID(""))
	if err != nil {
		t.Fatalf("re-sealing: %v", err)
	}
	if outcome.Rewrapped != 2 || outcome.Skipped != 6 {
		t.Errorf("outcome %+v, want 2 moved and 6 skipped", outcome)
	}
	direct := store.rewritten["018f2a1b-0000-7000-8000-0000000000a1"]
	if direct.KeyID != "k2" || direct.Purpose != requestPurpose {
		t.Errorf("the direct call's secret after the round: %+v", direct)
	}
	carried := store.rewritten["018f2a1b-0000-7000-8000-0000000000a2"]
	if carried.KeyID != "k2" || carried.Purpose != rulePurpose {
		t.Errorf("the rule's carried secret after the round: %+v", carried)
	}
	if len(store.rewritten) != 2 {
		t.Errorf("jobs written back: %v", store.rewritten)
	}
	for _, seen := range ring.purposes {
		if string(seen) != requestPurpose && string(seen) != rulePurpose {
			t.Errorf("a rewrap was attempted under %q", seen)
		}
	}
}
