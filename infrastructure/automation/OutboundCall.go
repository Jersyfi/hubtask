// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// outboundClass is the metric label for these calls. What the call is, never who it goes to: a
// label per target host would grow a series per customer rule (rule 10).
const outboundClass = "automation"

// Events is the slice of the outbox the sender reads: the event a body template renders from.
type Events interface {
	FindEvent(ctx context.Context, id shared.ID) (event.Envelope, error)
}

// Entries and Containers are the reads a body template naming `item` or `collection` costs -
// the same lookups a condition's activation makes, injected for the same laziness.
type Entries = condition.Entries

// Containers is the container half of the same contract.
type Containers = condition.Containers

// OutboundCall performs one HTTP_REQUEST action's call (G-09): the riskiest surface in the
// milestone, treated the way a backup target is.
//
//   - Every call goes through the guarded client (rule 6, T-07): private and link-local ranges
//     are refused unless the installation released them, redirects are limited, the call has a
//     deadline and the response a size cap.
//   - The header secret is opened from its sealed form for the length of one call, and appears in
//     no log, no metric and no error (T-21, rule 10).
//   - The response is discarded unread past the status line. A rule cannot read an answer
//     (ADR-0009) - external data in conditions was excluded - so nothing is stored for anybody
//     to read later.
//
// It is a queue handler and Detached: the call to somebody else's server happens between two
// short transactions rather than inside one (observability-reliability.md §8). The retry ladder
// is the queue's own - eight attempts with the runner's backoff, then the dead letter.
type OutboundCall struct {
	Events    Events
	Encryptor crypto.Encryptor
	// Compiler renders the body template. The same engine that compiled it at the write, so the
	// two cannot disagree about what a template means.
	Compiler   expression.Compiler
	Signer     security.WebhookSigner
	Client     httpport.Port
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Entries    Entries
	Containers Containers
}

var (
	_ queue.Handler  = OutboundCall{}
	_ queue.Detached = OutboundCall{}
)

// OwnsItsTransactions declares the exception this handler needs (queue.Detached). What it gives
// up is acceptable here: the call has no bookkeeping row of its own, so a repeat costs a repeated
// request against a target that was told to expect at-least-once - the same contract a webhook
// subscriber holds.
func (OutboundCall) OwnsItsTransactions() {}

// Run makes one attempt.
func (c OutboundCall) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	call, err := callOf(job)
	if err != nil {
		return queue.Result{}, err
	}

	body, err := c.render(ctx, job, call)
	if err != nil {
		return queue.Result{}, err
	}

	header := make(map[string][]string, len(call.headers)+2)
	for name, value := range call.headers {
		header[name] = []string{value}
	}
	if call.sealed != nil {
		opened, err := c.Encryptor.Open(ctx, crypto.Sealed{
			KeyID: call.sealed.KeyID, Ciphertext: call.ciphertext,
		}, crypto.Purpose(call.sealed.Purpose))
		if err != nil {
			return queue.Result{}, err
		}
		if call.secretHeaderName != "" {
			header[call.secretHeaderName] = []string{opened.Reveal()}
		}
		if call.signatureHeader != "" {
			// The webhook signature's own shape - t=<ts>,v1=<hmac-sha256> - so a receiver that
			// verifies one can verify the other with the same code.
			header[call.signatureHeader] = []string{c.Signer.Sign(opened, c.Clock.Now(), body)}
		}
	}

	response, callErr := c.Client.Do(ctx, httpport.Request{
		Method:      call.method,
		URL:         call.url,
		Header:      header,
		Body:        body,
		TargetClass: outboundClass,
	})
	if callErr != nil {
		// Never reached the target: refused by the guard, or the dial did not work. The code is
		// ours; the cause is the client's to log (rule 10). Returned as an error, so the queue's
		// backoff and dead letter apply.
		if errors.Is(callErr, shared.ErrForbidden) {
			return queue.Result{}, shared.ErrForbidden.
				WithDetail("automation.http_target_blocked").
				WithCause(callErr)
		}
		return queue.Result{}, shared.ErrUnavailable.
			WithDetail("automation.http_target_unreachable").
			WithCause(callErr)
	}

	// The response is discarded here, deliberately and completely: the status decides whether the
	// job is done, and the body - already bounded by the client's size cap - is read by nothing
	// and stored nowhere. A rule cannot read an answer (ADR-0009).
	if response.Status >= 200 && response.Status < 300 {
		return queue.Result{}, nil
	}
	code := "automation.http_target_refused"
	if response.Status >= 500 || response.Status == http.StatusTooManyRequests {
		code = "automation.http_target_unavailable"
	}
	return queue.Result{}, shared.ErrUnavailable.
		WithDetail(code).
		WithParams(map[string]string{"status": itoa(response.Status)})
}

// render evaluates the body template against the run's event, inside one short read transaction -
// the reads a template naming `item` costs happen here, and never while the call is in flight.
func (c OutboundCall) render(
	ctx context.Context, job queue.Job, call outboundJob,
) ([]byte, error) {
	if call.bodyTemplate == "" {
		return nil, nil
	}
	if c.Compiler == nil {
		return nil, shared.ErrInternal.WithDetail("automation.expression_engine_unavailable")
	}

	program, err := c.Compiler.Compile(
		call.bodyTemplate, condition.RuleEnvironment(), expression.Text)
	if err != nil {
		return nil, err
	}

	var rendered string
	err = c.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: job.TenantID},
		func(ctx context.Context) error {
			envelope := event.Envelope{}
			if !call.eventID.IsZero() && c.Events != nil {
				found, err := c.Events.FindEvent(ctx, call.eventID)
				if err != nil && !errors.Is(err, shared.ErrNotFound) {
					return err
				}
				// Swept while the job waited reads as an empty event, exactly as the engine's
				// conditions read one: what the template would have said is no longer knowable.
				envelope = found
			}

			out, err := program.Evaluate(ctx, condition.Values{
				Envelope: envelope, Now: c.Clock.Now(),
				Entries: c.Entries, Containers: c.Containers,
			})
			if err != nil {
				return shared.ErrValidation.
					WithDetail("automation.http_body_unrenderable").
					WithCause(err)
			}
			rendered = out.Text
			return nil
		})
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}

// outboundJob is one call as the job row carries it.
type outboundJob struct {
	method           string
	url              string
	headers          map[string]string
	secretHeaderName string
	signatureHeader  string
	bodyTemplate     string
	sealed           *domain.SealedSecret
	ciphertext       []byte
	eventID          shared.ID
}

// callOf reads the payload. A payload this handler cannot read is a programming error rather than
// a call to retry.
func callOf(job queue.Job) (outboundJob, error) {
	call := outboundJob{}
	call.method, _ = job.Payload["method"].(string)
	call.url, _ = job.Payload["url"].(string)
	if call.method == "" || call.url == "" {
		return outboundJob{}, shared.ErrInternal.
			WithDetail("automation.http_job_malformed").
			WithCause(fmt.Errorf("method %q", call.method))
	}

	if raw, present := job.Payload["headers"].(map[string]any); present {
		call.headers = make(map[string]string, len(raw))
		for name, value := range raw {
			if text, ok := value.(string); ok {
				call.headers[name] = text
			}
		}
	}
	call.secretHeaderName, _ = job.Payload["secret_header_name"].(string)
	call.signatureHeader, _ = job.Payload["signature_header"].(string)
	call.bodyTemplate, _ = job.Payload["body_template"].(string)

	if raw, present := job.Payload["secret_header_sealed"].(map[string]any); present {
		sealed := domain.SealedSecret{}
		sealed.Ciphertext, _ = raw["ciphertext"].(string)
		sealed.KeyID, _ = raw["key_id"].(string)
		sealed.Purpose, _ = raw["purpose"].(string)
		decoded, err := base64.StdEncoding.DecodeString(sealed.Ciphertext)
		if err != nil || sealed.KeyID == "" || sealed.Purpose == "" {
			return outboundJob{}, shared.ErrInternal.
				WithDetail("automation.http_job_malformed").
				WithCause(fmt.Errorf("sealed secret unreadable"))
		}
		call.sealed, call.ciphertext = &sealed, decoded
	}

	if text, present := job.Payload["event_id"].(string); present && text != "" {
		id, err := shared.ParseID(text)
		if err != nil {
			return outboundJob{}, shared.ErrInternal.
				WithDetail("automation.http_job_malformed").
				WithCause(err)
		}
		call.eventID = id
	}
	return call, nil
}

// itoa is fmt.Sprintf's narrow case, matching the deliverer's helper.
func itoa(n int) string { return fmt.Sprintf("%d", n) }
