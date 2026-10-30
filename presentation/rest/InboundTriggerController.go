// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/service/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// InboundRunStarter is the slice of the inbound path this controller needs. An interface rather
// than the service, so the route can be tested without a database and the presentation layer keeps
// pointing inwards.
type InboundRunStarter interface {
	Execute(
		ctx context.Context, delivery automation.InboundDelivery,
	) (automation.TriggerRuleResult, error)
}

// IntakeDoor serves the doors the contract delegates to it that a sender outside this system
// holds the credential of - an inbound webhook, the jumble's doors, the forge hooks, the CMDB's
// intakes, the receiving domain and the voice assistant. None is a catalogue entry: each answers
// a credential nobody in this system holds, in the sender's own format, and there is nothing for
// MCP or an automation rule to call. A nil field leaves its route answering the pending 404.
type IntakeDoor struct {
	// InboundRuns serves the public inbound-webhook route, which is not a catalogue entry either:
	// it answers a credential nobody in this system holds, and there is nothing for MCP or an
	// automation rule to call. Nil leaves the route answering the pending 404.
	InboundRuns InboundRunStarter

	// JumbleIntake serves the jumble's public webhook door, for the same reasons.
	JumbleIntake JumbleIntakeDeliverer

	// MailIntake serves the mail door beside it: the same credential, and a body that is
	// the message itself rather than a shape somebody had to agree on.
	MailIntake MailDeliverer
	// CodeIntake serves the forge hooks: GitHub, GitLab, Forgejo and Gitea post their pushes and
	// pull requests. Nil leaves the route answering the pending 404.
	CodeIntake CodeDeliverer
	// CmdbAlerts serves the CMDB's alert intakes, for the same reasons: a monitoring program holds
	// the credential, not a person. Nil leaves the route answering the pending 404.
	CmdbAlerts CmdbAlertDeliverer
	// CmdbInventory serves the CMDB's inventory intakes, a GLPI Agent posting what it found. Nil
	// leaves the route answering the pending 404.
	CmdbInventory CmdbInventoryDeliverer
	// AddressedMail serves the receiving domain's inbound route (ADR-0092). Nil leaves it answering
	// the pending 404.
	AddressedMail AddressedMailDeliverer
	// Alexa serves the voice assistant's endpoint (ADR-0093). Nil leaves it answering the pending
	// 404.
	Alexa AlexaAnswerer
}

// StartInboundRun answers POST /automation/inbound/{token} - the second route in this API that
// carries no bearer credential (automation.md §1.1).
//
// What happens here is parsing, and nothing else. The shape of the token is a string question and
// belongs to an adapter; whether a rule has this address, whether it is enabled and whether its
// trigger is still an inbound webhook are all decided inwards of here (ADR-0005). Every one of
// those answers looks the same from out here, which is the point: a route that distinguished them
// would answer questions for whoever is trying tokens (T-21).
func (c *IntakeDoor) StartInboundRun(
	w http.ResponseWriter, r *http.Request, presented string,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.InboundRuns == nil {
		// The pending 404 rather than an internal error, which is what the stream's route does and
		// what a *public* route should do in particular: an installation that does not serve this
		// tells the internet nothing about why.
		pending{}.StartInboundRun(w, r, presented)
		return
	}

	token, err := integration.ParseInboundToken(presented)
	if err != nil {
		// A malformed token answers what an unknown one answers. Not the parser's own refusal:
		// "that is not the right shape" tells somebody guessing that the shape is what to fix.
		WriteProblem(w, inboundNotFound(), requestID)
		return
	}

	payload, err := inboundPayload(r)
	if errors.Is(err, errInboundPayloadTooLarge) {
		// 413 with the route's own bound, which is a smaller number than the request middleware's:
		// what bounds a transfer and what bounds an evaluation are two different questions.
		WriteTooLarge(w, integration.MaxInboundPayloadBytes, requestID)
		return
	}
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	result, err := c.InboundRuns.Execute(r.Context(), automation.InboundDelivery{
		Token: token, Payload: payload,
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	writeJSON(w, r, http.StatusAccepted, openapi.RuleRunAccepted{
		RunId: uuidValue(result.RunID.String()), RuleId: uuidValue(result.RuleID.String()),
	})
}

// inboundPayload reads the delivered body defensively.
//
// Three refusals, and each is a different mistake. A body larger than what an expression may read
// is refused rather than truncated - a rule that evaluated half a document would be a rule whose
// conditions answered about something the sender never wrote. A body that is not JSON is refused
// because `payload` is a document, not a string. A body that is JSON but not an object is refused
// for the same reason: `payload.order_id` has to mean something, and a top-level array has no
// names at all.
//
// An empty body is an empty document, which is the ordinary shape of a "something happened" ping.
//
// The read is bounded here as well as by the request middleware, and the two bounds are different
// numbers on purpose: the middleware stops anything large being *transferred*, and this stops
// anything large being *evaluated* (integration.MaxInboundPayloadBytes).
func inboundPayload(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}

	// One byte more than the bound, so that a body exactly at the limit is accepted and the first
	// byte past it is what tells us it was exceeded.
	body, err := io.ReadAll(io.LimitReader(r.Body, integration.MaxInboundPayloadBytes+1))
	if err != nil {
		// Includes the request middleware's own limit firing on a chunked body. Either way what
		// arrived is not a document this route can read.
		return nil, shared.ErrValidation.WithDetail("automation.inbound_payload_unreadable")
	}
	if len(body) > integration.MaxInboundPayloadBytes {
		return nil, errInboundPayloadTooLarge
	}
	if len(body) == 0 {
		return map[string]any{}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, shared.ErrValidation.WithDetail("automation.inbound_payload_not_an_object")
	}
	if payload == nil {
		// `null` parses into a nil map. An empty document rather than a missing one, so that a
		// condition naming `payload` sees something.
		return map[string]any{}, nil
	}
	return payload, nil
}

// errInboundPayloadTooLarge is this route's own bound being exceeded, translated into the contract's
// 413 by the handler. A sentinel rather than a coded refusal, because the status mapping for "too
// large" already exists and lives in Problem.go with the number it reports.
var errInboundPayloadTooLarge = errors.New("rest: the inbound payload is larger than a condition may read")

// inboundNotFound is the one answer this route ever gives when it will not serve, and it is the
// same code the application layer answers for a rotated token or a switched-off rule.
func inboundNotFound() error {
	return shared.ErrNotFound.WithDetail("automation.inbound_not_found")
}
