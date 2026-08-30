// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

var (
	outboundTenant = shared.ID("01936f2a-7c1e-7000-8000-000000000c01")
	outboundEvent  = shared.ID("01936f2a-7c1e-7000-8000-000000000c02")
	sentAt         = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
)

// caller records the request and answers what the test told it to.
type caller struct {
	requests []httpport.Request
	status   int
	err      error
}

func (c *caller) Do(_ context.Context, req httpport.Request) (httpport.Response, error) {
	c.requests = append(c.requests, req)
	if c.err != nil {
		return httpport.Response{}, c.err
	}
	// A body comes back and is deliberately never looked at by the handler under test.
	return httpport.Response{Status: c.status, Body: []byte(`{"answer":"ignored"}`)}, nil
}

type opener struct{ opened int }

func (o *opener) Seal(_ context.Context, plaintext secret.Secret, _ crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{KeyID: "k1", Ciphertext: []byte(plaintext.Reveal())}, nil
}
func (o *opener) ActiveKeyID() string { return "k1" }
func (o *opener) Open(_ context.Context, sealed crypto.Sealed, _ crypto.Purpose) (secret.Secret, error) {
	o.opened++
	return secret.New(string(sealed.Ciphertext)), nil
}

type sourced struct{ missing bool }

func (s sourced) FindEvent(_ context.Context, id shared.ID) (event.Envelope, error) {
	if s.missing {
		return event.Envelope{}, shared.ErrNotFound.WithDetail("events.event_not_found")
	}
	return event.Envelope{
		ID: id, Type: event.ItemCreated, TenantID: outboundTenant,
		Subject: "item/01936f2a-7c1e-7000-8000-000000000c03", OccurredAt: sentAt,
	}, nil
}

// renderer is the expression port: it renders every template to a fixed body, which is enough to
// prove the rendered text is what travels.
type renderer struct{}

func (renderer) Compile(string, expression.Environment, expression.Result) (expression.Program, error) {
	return rendered{}, nil
}

type rendered struct{}

func (rendered) Evaluate(context.Context, expression.Activation) (expression.Value, error) {
	return expression.Value{Text: `{"rendered":true}`}, nil
}

type unitOfWork struct{}

func (unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}
func (u unitOfWork) WithinReadOnly(ctx context.Context, s persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, s, fn)
}

func outboundJobRow(payload map[string]any) queue.Job {
	base := map[string]any{
		"request_id": "01936f2a-7c1e-7000-8000-000000000c04",
		"method":     "POST",
		"url":        "https://example.org/hook",
	}
	for key, value := range payload {
		base[key] = value
	}
	return queue.Job{
		ID: shared.ID("01936f2a-7c1e-7000-8000-000000000c05"), TenantID: outboundTenant,
		Kind: queue.KindAutomationHTTP, Payload: base, Attempts: 1, MaxAttempts: 8,
	}
}

func sender(client *caller) OutboundCall {
	return OutboundCall{
		Events: sourced{}, Encryptor: &opener{}, Compiler: renderer{},
		Signer: security.NewWebhookSigner(), Client: client,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(sentAt),
	}
}

// The acceptance criterion, positive half: the sealed secret is opened for the length of one call,
// travels in exactly the header the rule named, and the response is discarded.
func TestTheCallCarriesTheOpenedSecretAndDiscardsTheAnswer(t *testing.T) {
	client := &caller{status: 200}
	call := sender(client)

	result, err := call.Run(context.Background(), outboundJobRow(map[string]any{
		"headers":            map[string]any{"Content-Type": "application/json"},
		"secret_header_name": "Authorization",
		"secret_header_sealed": map[string]any{
			"ciphertext": base64.StdEncoding.EncodeToString([]byte("Bearer s3cret")),
			"key_id":     "k1", "purpose": "automation.rule.http:x",
		},
		"body_template": "anything",
		"event_id":      outboundEvent.String(),
	}))
	if err != nil {
		t.Fatalf("the call failed: %v", err)
	}
	if result.Repeat {
		t.Error("a finished call asked to repeat")
	}

	if len(client.requests) != 1 {
		t.Fatalf("%d requests made", len(client.requests))
	}
	request := client.requests[0]
	if request.Method != "POST" || request.URL != "https://example.org/hook" {
		t.Errorf("the request is %+v", request)
	}
	if got := request.Header["Authorization"]; len(got) != 1 || got[0] != "Bearer s3cret" {
		t.Errorf("the secret header is %v", got)
	}
	if request.TargetClass != "automation" {
		t.Errorf("the metric class is %q", request.TargetClass)
	}
	if string(request.Body) != `{"rendered":true}` {
		t.Errorf("the body is %q, want the rendered template", request.Body)
	}
}

// The signature is the webhook signature's own shape, computed with the same secret.
func TestTheSignatureUsesTheWebhookShape(t *testing.T) {
	client := &caller{status: 200}
	call := sender(client)

	if _, err := call.Run(context.Background(), outboundJobRow(map[string]any{
		"signature_header": "X-Signature",
		"secret_header_sealed": map[string]any{
			"ciphertext": base64.StdEncoding.EncodeToString([]byte("signing-key")),
			"key_id":     "k1", "purpose": "automation.http:x",
		},
	})); err != nil {
		t.Fatalf("the call failed: %v", err)
	}

	header := client.requests[0].Header["X-Signature"]
	if len(header) != 1 {
		t.Fatalf("the signature header is %v", header)
	}
	if !security.NewWebhookSigner().Verify(header[0], nil, secret.New("signing-key")) {
		t.Errorf("the signature %q does not verify with the secret", header[0])
	}
}

// A refusal from the guard is its own code: "this installation would not dial that" is a
// configuration answer, not the target's problem.
func TestAGuardRefusalIsItsOwnCode(t *testing.T) {
	client := &caller{err: shared.ErrForbidden.WithDetail("httpclient.private_address_blocked")}
	call := sender(client)

	_, err := call.Run(context.Background(), outboundJobRow(nil))
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if code := shared.AsError(err).DetailCode; code != "automation.http_target_blocked" {
		t.Errorf("detail code %s", code)
	}
}

// A target that answers badly is an error, so the queue's ladder retries it - and the answer's
// own words appear nowhere.
func TestABadAnswerRetriesThroughTheQueue(t *testing.T) {
	for status, code := range map[int]string{
		503: "automation.http_target_unavailable",
		404: "automation.http_target_refused",
	} {
		client := &caller{status: status}
		call := sender(client)

		_, err := call.Run(context.Background(), outboundJobRow(nil))
		if err == nil {
			t.Fatalf("a %d answered no error", status)
		}
		coded := shared.AsError(err)
		if coded.DetailCode != code {
			t.Errorf("a %d failed with %s, want %s", status, coded.DetailCode, code)
		}
	}
}

// An event swept while the job waited renders against an empty event rather than failing: what
// the template would have said is no longer knowable, and the call still happens.
func TestASweptEventStillSends(t *testing.T) {
	client := &caller{status: 200}
	call := sender(client)
	call.Events = sourced{missing: true}

	if _, err := call.Run(context.Background(), outboundJobRow(map[string]any{
		"body_template": "anything",
		"event_id":      outboundEvent.String(),
	})); err != nil {
		t.Fatalf("the call failed: %v", err)
	}
	if len(client.requests) != 1 {
		t.Error("the call never happened")
	}
}
