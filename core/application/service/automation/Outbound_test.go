// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// sealer is the E-02 encryptor as these tests see it: reversible and legible, so an assertion can
// say what was sealed without a real key in the test.
type sealer struct{ purposes []crypto.Purpose }

func (e *sealer) Seal(_ context.Context, plaintext secret.Secret, purpose crypto.Purpose) (crypto.Sealed, error) {
	e.purposes = append(e.purposes, purpose)
	return crypto.Sealed{KeyID: "k1", Ciphertext: []byte("sealed:" + plaintext.Reveal())}, nil
}

func (e *sealer) ActiveKeyID() string { return "k1" }

func (e *sealer) KeyIDs() []string { return []string{"k1"} }

func (e *sealer) Rewrap(_ context.Context, sealed crypto.Sealed, _ crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{KeyID: "k1", Ciphertext: sealed.Ciphertext}, nil
}

func (e *sealer) Open(_ context.Context, sealed crypto.Sealed, _ crypto.Purpose) (secret.Secret, error) {
	return secret.New(strings.TrimPrefix(string(sealed.Ciphertext), "sealed:")), nil
}

func httpAction(secretValue string) domain.Action {
	params := map[string]any{
		"method":             "POST",
		"url":                "https://example.org/hook",
		"secret_header_name": "Authorization",
		"body_template":      "now",
	}
	if secretValue != "" {
		params["secret_header_value"] = secretValue
	}
	return domain.Action{Kind: domain.ActionHTTPRequest, Params: params}
}

func httpRuleCommand(secretValue string) CreateRuleCommand {
	cmd := validCommand()
	cmd.Actions = []domain.Action{httpAction(secretValue)}
	return cmd
}

// The acceptance criterion: the header secret never appears anywhere after creation. The stored
// rule carries ciphertext, and every channel's projection answers the mask.
func TestAnHTTPSecretIsSealedAtTheWriteAndMaskedEverAfter(t *testing.T) {
	h := newHarness()
	h.writer.Conditions = compiler{}
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	actor := writerActor()
	actor.Scopes = append(actor.Scopes, "automation:manage")
	rule, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), actor, httpRuleCommand("Bearer s3cret"))
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	stored := h.store.rows[rule.ID]
	request, err := domain.ReadHTTPRequest(stored.Actions[0].Params, "0")
	if err != nil {
		t.Fatalf("reading the stored action back: %v", err)
	}
	if request.SecretValue != "" || request.SecretMasked {
		t.Errorf("the stored rule still carries a plaintext: %+v", request)
	}
	if request.Sealed == nil {
		t.Fatal("the stored rule carries no sealed secret")
	}
	if !strings.Contains(request.Sealed.Purpose, rule.ID.String()) {
		t.Errorf("the sealing purpose %q does not name the rule", request.Sealed.Purpose)
	}

	// The projection every channel answers from masks it.
	out := ruleOutput(stored)
	rows, _ := out["actions"].([]any)
	row, _ := rows[0].(map[string]any)
	params, _ := row["params"].(map[string]any)
	if params["secret_header_value"] != domain.SecretMask {
		t.Errorf("the projection answers %v, want the mask", params["secret_header_value"])
	}
	if _, leaked := params["secret_header_sealed"]; leaked {
		t.Error("the projection leaks the sealed form")
	}
	// And the stored action itself is untouched by the masking - the mask is a copy.
	if _, kept := stored.Actions[0].Params["secret_header_sealed"]; !kept {
		t.Error("masking the projection mutated the stored rule")
	}
}

// Sending the mask back on an edit keeps the stored secret - which is the only way a client that
// can never read the secret again can edit the rest of the rule.
func TestTheMaskKeepsTheStoredSecretOnAnEdit(t *testing.T) {
	h := newHarness()
	h.writer.Conditions = compiler{}
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	actor := writerActor()
	actor.Scopes = append(actor.Scopes, "automation:manage")
	rule, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), actor, httpRuleCommand("Bearer s3cret"))
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	edited := []domain.Action{httpAction(domain.SecretMask)}
	updated, err := (UpdateRule{Writer: h.writer}).Execute(context.Background(), actor,
		UpdateRuleCommand{ID: rule.ID, Actions: &edited})
	if err != nil {
		t.Fatalf("editing the rule: %v", err)
	}

	request, err := domain.ReadHTTPRequest(updated.Actions[0].Params, "0")
	if err != nil {
		t.Fatalf("reading the edited action: %v", err)
	}
	if request.Sealed == nil || !strings.Contains(request.Sealed.Ciphertext, "") {
		t.Fatal("the stored secret was not kept")
	}
	opened, err := h.sealer.Open(context.Background(), crypto.Sealed{
		Ciphertext: mustDecode(t, request.Sealed.Ciphertext),
	}, "")
	if err != nil || opened.Reveal() != "Bearer s3cret" {
		t.Errorf("the kept secret opens to %q", opened.Reveal())
	}
}

// The mask at a position that stores no secret is refused: a rule whose secret is three asterisks
// would send them as the credential.
func TestTheMaskWithNothingStoredIsRefused(t *testing.T) {
	h := newHarness()
	h.writer.Conditions = compiler{}
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	actor := writerActor()
	actor.Scopes = append(actor.Scopes, "automation:manage")
	_, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), actor, httpRuleCommand(domain.SecretMask))
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation refusal", err)
	}
	if code := shared.AsError(err).DetailCode; code != "automation.http_secret_unknown" {
		t.Errorf("detail code %s", code)
	}
}

// A body template that cannot be read is answered to its author at the write, exactly as a
// condition is - not to a dead letter at three in the morning.
func TestAnUnreadableBodyTemplateIsRefusedAtTheWrite(t *testing.T) {
	h := newHarness()
	h.writer.Conditions = compiler{}
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	cmd := httpRuleCommand("")
	cmd.Actions[0].Params["body_template"] = "event.type =="
	delete(cmd.Actions[0].Params, "secret_header_name")

	actor := writerActor()
	actor.Scopes = append(actor.Scopes, "automation:manage")
	_, err := (CreateRule{Writer: h.writer}).Execute(context.Background(), actor, cmd)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation refusal", err)
	}
}

// The direct call: the use case seals the secret before it touches the queue, and the job payload
// never carries a plaintext.
func TestADirectCallSealsTheSecretBeforeTheQueue(t *testing.T) {
	sealed := &sealer{}
	queued := &queued{}
	sink := &auditSink{}
	uc := HttpRequest{
		Jobs: queued, Authorizer: &authorizer{}, Encryptor: sealed,
		Conditions: compiler{}, Audit: sink,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: newRuleID},
	}

	actor := writerActor()
	actor.Scopes = append(actor.Scopes, "automation:manage")
	requested, err := uc.Execute(context.Background(), actor, map[string]any{
		"method": "POST", "url": "https://example.org/hook",
		"secret_header_name": "Authorization", "secret_header_value": "Bearer s3cret",
	}, "")
	if err != nil {
		t.Fatalf("requesting: %v", err)
	}
	if requested.RequestID.IsZero() || requested.JobID.IsZero() {
		t.Fatalf("the answer is %+v", requested)
	}

	if len(queued.requests) != 1 {
		t.Fatalf("%d jobs queued", len(queued.requests))
	}
	request := queued.requests[0]
	if request.Kind != queue.KindAutomationHTTP || request.MaxAttempts != HTTPAttempts {
		t.Errorf("the job is %+v", request)
	}
	if _, leaked := request.Payload["secret_header_value"]; leaked {
		t.Error("the job payload carries a plaintext secret")
	}
	sealedDoc, _ := request.Payload["secret_header_sealed"].(map[string]any)
	if sealedDoc == nil || sealedDoc["ciphertext"] == "" {
		t.Errorf("the job payload carries no sealed secret: %v", request.Payload)
	}
	if len(sink.entries) != 1 || sink.entries[0].Action != HTTPRequestedAction {
		t.Errorf("the act was not audited: %+v", sink.entries)
	}
	for _, change := range sink.entries[0].Changes {
		if text, ok := change.(string); ok && strings.Contains(text, "s3cret") {
			t.Error("the audit entry carries the secret")
		}
	}
}

// A direct call sending the mask has nothing stored to keep.
func TestADirectCallWithTheMaskIsRefused(t *testing.T) {
	uc := HttpRequest{
		Jobs: &queued{}, Authorizer: &authorizer{}, Encryptor: &sealer{},
		Conditions: compiler{}, UnitOfWork: unitOfWork{},
		Clock: clock.Fixed(now), IDs: ids{next: newRuleID},
	}
	actor := writerActor()
	actor.Scopes = append(actor.Scopes, "automation:manage")

	_, err := uc.Execute(context.Background(), actor, map[string]any{
		"method": "GET", "url": "https://example.org",
		"secret_header_name": "Authorization", "secret_header_value": domain.SecretMask,
	}, "")
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation refusal", err)
	}
}

func mustDecode(t *testing.T, encoded string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	return decoded
}
