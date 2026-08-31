// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"encoding/base64"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	HTTPRequestName = "HttpRequest"

	// HTTPRequestedAction is an outbound call somebody asked for - through the API, or through a
	// rule's HTTP_REQUEST action running as its account. Required, because pointing this
	// installation's egress at an address is exactly the kind of act a review looks for (T-07).
	HTTPRequestedAction audit.Action = "automation.http_requested"

	httpTarget = "http_request"

	// HTTPAttempts is the retry budget of one outbound call: the webhook ladder's eight, for the
	// webhook ladder's reason - a target that is briefly down deserves the backoff, and one that
	// is gone deserves the dead letter.
	HTTPAttempts = 8
)

// ruleSecretPurpose binds a rule's sealed header secret to the rule it belongs to, so a ciphertext
// lifted out of one rule and dropped into another no longer opens (E-02).
func ruleSecretPurpose(ruleID shared.ID) crypto.Purpose {
	return crypto.Purpose("automation.rule.http:" + ruleID.String())
}

// requestSecretPurpose binds a direct call's sealed secret to the one request it was sealed for.
func requestSecretPurpose(requestID shared.ID) crypto.Purpose {
	return crypto.Purpose("automation.http:" + requestID.String())
}

// sealOutboundSecrets walks a rule's actions - branch arms included - and seals every HTTP_REQUEST
// header secret that arrived in plaintext (E-02, T-21). After this pass the rule stores ciphertext
// or nothing: the plaintext exists in memory between the request and this line, and nowhere after.
//
// A value sent as the mask asks for the stored secret to be kept: the previous rule's sealed value
// at the same path is copied forward. The path is the best identity an edit leaves us - an action
// moved to another position reads as a new action, and its secret has to be sent again, which is
// the honest answer for a value this system can no longer show.
func sealOutboundSecrets(
	ctx context.Context, encryptor crypto.Encryptor, rule *domain.Rule, previous *domain.Rule,
) error {
	kept := map[string]*domain.SealedSecret{}
	if previous != nil {
		gatherSealed(previous.Actions, "", kept)
	}
	return sealActions(ctx, encryptor, rule.ID, rule.Actions, "", kept)
}

func gatherSealed(actions []domain.Action, parent string, into map[string]*domain.SealedSecret) {
	_ = walkOutbound(actions, parent, func(path string, params map[string]any) error {
		request, err := domain.ReadHTTPRequest(params, path)
		if err == nil && request.Sealed != nil {
			into[path] = request.Sealed
		}
		return nil
	})
}

func sealActions(
	ctx context.Context, encryptor crypto.Encryptor, ruleID shared.ID,
	actions []domain.Action, parent string, kept map[string]*domain.SealedSecret,
) error {
	return walkOutbound(actions, parent, func(path string, params map[string]any) error {
		request, err := domain.ReadHTTPRequest(params, path)
		if err != nil {
			// Unreachable: the aggregate read the same parameters when the rule was built.
			return err
		}

		switch {
		case request.SecretValue != "":
			if encryptor == nil {
				return shared.ErrInternal.WithDetail("automation.http_sealing_unavailable")
			}
			sealed, err := encryptor.Seal(
				ctx, secret.New(request.SecretValue), ruleSecretPurpose(ruleID))
			if err != nil {
				return err
			}
			params["secret_header_sealed"] = domain.SealedSecret{
				Ciphertext: base64.StdEncoding.EncodeToString(sealed.Ciphertext),
				KeyID:      sealed.KeyID,
				Purpose:    string(ruleSecretPurpose(ruleID)),
			}.Document()
			delete(params, "secret_header_value")
		case request.SecretMasked:
			stored, exists := kept[path]
			if !exists {
				// The mask asks for a secret this rule does not hold at this position. Refused
				// rather than stored, because a rule whose secret is three asterisks would send
				// them as the credential.
				return shared.ErrValidation.
					WithDetail("automation.http_secret_unknown").
					WithFields(shared.FieldError{
						Path: path + "/params/secret_header_value",
						Code: "automation.http_secret_unknown",
					})
			}
			params["secret_header_sealed"] = stored.Document()
			delete(params, "secret_header_value")
		}
		return nil
	})
}

// walkOutbound visits every HTTP_REQUEST's parameters in an action tree, at its path.
//
// The arms of a branch are raw documents rather than domain.Action values - that is how the rule
// stores them - so the walk reads them the way the aggregate does.
func walkOutbound(
	actions []domain.Action, parent string, visit func(path string, params map[string]any) error,
) error {
	for i, action := range actions {
		if err := visitOutbound(
			action.Kind, action.Params, domain.ActionPath(parent, i), visit); err != nil {
			return err
		}
	}
	return nil
}

func visitOutbound(
	kind string, params map[string]any, path string,
	visit func(path string, params map[string]any) error,
) error {
	switch kind {
	case domain.ActionHTTPRequest:
		return visit(path, params)
	case domain.ActionBranch:
		for _, arm := range []string{"then", "else"} {
			rows, _ := params[arm].([]any)
			for j, row := range rows {
				document, ok := row.(map[string]any)
				if !ok {
					continue
				}
				nestedKind, _ := document["kind"].(string)
				nested, _ := document["params"].(map[string]any)
				if nested == nil {
					continue
				}
				if err := visitOutbound(
					nestedKind, nested, path+"/"+arm+"/"+itoa(j), visit); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// maskedActionParams is an action's parameters as every channel answers them: a deep copy with the
// sealed secret replaced by the mask, so what was stored encrypted is never readable again and
// what a client sends back unchanged means "keep it" (T-21).
func maskedActionParams(kind string, params map[string]any) map[string]any {
	masked := make(map[string]any, len(params))
	for name, value := range params {
		masked[name] = value
	}

	switch kind {
	case domain.ActionHTTPRequest:
		if _, sealed := masked["secret_header_sealed"]; sealed {
			delete(masked, "secret_header_sealed")
			masked["secret_header_value"] = domain.SecretMask
		}
	case domain.ActionBranch:
		for _, arm := range []string{"then", "else"} {
			rows, ok := masked[arm].([]any)
			if !ok {
				continue
			}
			out := make([]any, 0, len(rows))
			for _, row := range rows {
				document, ok := row.(map[string]any)
				if !ok {
					out = append(out, row)
					continue
				}
				kind, _ := document["kind"].(string)
				params, _ := document["params"].(map[string]any)
				copied := make(map[string]any, len(document))
				for name, value := range document {
					copied[name] = value
				}
				if params != nil {
					copied["params"] = maskedActionParams(kind, params)
				}
				out = append(out, copied)
			}
			masked[arm] = out
		}
	}
	return masked
}

// HttpRequest is the outbound call as a use case (G-09, automation.md §1.3) - which is what makes
// HTTP_REQUEST an action like every other: registered, so REST, MCP and a rule reach the same
// code, and audited, because pointing this installation's egress somewhere is an act.
//
// It enqueues rather than calls. The actual HTTP happens on a detached job through the guarded
// client - private ranges refused unless released, the response bounded in size and time and then
// discarded, because a rule cannot read an answer (ADR-0009; the refusal is documented in
// automation.md rather than silently true).
//
// The name carries the descriptor name the channels derive from: HttpRequest is what makes the
// REST operation httpRequest and the action HTTP_REQUEST, where HTTPRequest would derive
// hTTPRequest.
//
//nolint:revive // the descriptor name the channels derive from, as the comment above says
type HttpRequest struct {
	Jobs       Queue
	Authorizer Authorizer
	// Encryptor seals a plaintext header secret before it touches the queue (E-02). The job
	// carries ciphertext; the sender opens it for the length of one call.
	Encryptor crypto.Encryptor
	// Conditions compiles the body template at the write, so a template that cannot be read is
	// answered to its author rather than to a dead letter.
	Conditions expression.Compiler
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// HTTPRequested is what the caller is told: the request's identity and the job performing it.
type HTTPRequested struct {
	RequestID shared.ID
	JobID     shared.ID
}

// Execute queues the call.
func (h HttpRequest) Execute(
	ctx context.Context, actor appshared.ActorContext, params map[string]any, eventID shared.ID,
) (HTTPRequested, error) {
	request, err := domain.ReadHTTPRequest(params, "")
	if err != nil {
		return HTTPRequested{}, err
	}

	requestID := h.IDs.NewID()
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionAutomation,
		Path:       domain.Scope{Type: domain.ScopeTenant}.Path(),
		Action:     HTTPRequestedAction,
		TokenScope: automationScope,
		TargetType: httpTarget,
		TargetID:   requestID,
	}); err != nil {
		return HTTPRequested{}, err
	}

	if request.BodyTemplate != "" {
		if h.Conditions == nil {
			return HTTPRequested{}, shared.ErrInternal.
				WithDetail("automation.expression_engine_unavailable")
		}
		if _, err := h.Conditions.Compile(
			request.BodyTemplate, condition.RuleEnvironment(), expression.Text); err != nil {
			return HTTPRequested{}, err
		}
	}
	if request.SecretMasked {
		// The mask means "keep the stored one", and a direct call stores nothing to keep.
		return HTTPRequested{}, shared.ErrValidation.
			WithDetail("automation.http_secret_unknown").
			WithFields(shared.FieldError{
				Path: "/params/secret_header_value", Code: "automation.http_secret_unknown",
			})
	}

	sealed := request.Sealed
	if request.SecretValue != "" {
		if h.Encryptor == nil {
			return HTTPRequested{}, shared.ErrInternal.
				WithDetail("automation.http_sealing_unavailable")
		}
		fresh, err := h.Encryptor.Seal(
			ctx, secret.New(request.SecretValue), requestSecretPurpose(requestID))
		if err != nil {
			return HTTPRequested{}, err
		}
		sealed = &domain.SealedSecret{
			Ciphertext: base64.StdEncoding.EncodeToString(fresh.Ciphertext),
			KeyID:      fresh.KeyID,
			Purpose:    string(requestSecretPurpose(requestID)),
		}
	}

	payload := map[string]any{
		"request_id": requestID.String(),
		"method":     request.Method,
		"url":        request.URL,
	}
	if len(request.Headers) > 0 {
		headers := make(map[string]any, len(request.Headers))
		for name, value := range request.Headers {
			headers[name] = value
		}
		payload["headers"] = headers
	}
	if request.SecretHeaderName != "" {
		payload["secret_header_name"] = request.SecretHeaderName
	}
	if sealed != nil {
		payload["secret_header_sealed"] = sealed.Document()
	}
	if request.SignatureHeader != "" {
		payload["signature_header"] = request.SignatureHeader
	}
	if request.BodyTemplate != "" {
		payload["body_template"] = request.BodyTemplate
	}
	if !eventID.IsZero() {
		payload["event_id"] = eventID.String()
	}

	var jobID shared.ID
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var enqueueErr error
		jobID, enqueueErr = h.Jobs.Enqueue(ctx, queue.Request{
			Kind:        queue.KindAutomationHTTP,
			TenantID:    actor.TenantID,
			Payload:     payload,
			MaxAttempts: HTTPAttempts,
		})
		return enqueueErr
	})
	if err != nil {
		return HTTPRequested{}, err
	}

	h.record(ctx, actor, request, requestID)
	return HTTPRequested{RequestID: requestID, JobID: jobID}, nil
}

// record writes the trail entry. The method and the address - egress is what a review asks about -
// and never a header, a body, or anything sealed (rule 10, T-21).
func (h HttpRequest) record(
	ctx context.Context, actor appshared.ActorContext, request domain.HTTPRequest, id shared.ID,
) {
	if h.Audit == nil {
		return
	}
	_ = h.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: h.Clock.Now(),
		Action:     HTTPRequestedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: httpTarget,
		TargetID:   id,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "method", Classification: audit.Open, To: request.Method},
			audit.Change{Field: "url", Classification: audit.Open, To: request.URL},
		),
	})
}

// Descriptor is the catalogue entry - and the automation action HTTP_REQUEST with it.
func (h HttpRequest) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: HTTPRequestName,
		Summary: "Calls an external HTTP address, through the guarded client: private and " +
			"link-local ranges are refused unless the installation released them, the response " +
			"is bounded in size and time, and it is discarded - a rule cannot read an answer. " +
			"The header secret is sealed at rest and masked everywhere after creation. As a rule " +
			"action, the run supplies the event the body template is rendered from.",
		SideEffects: "Queues the call and writes an audit entry. The HTTP happens on a job with " +
			"the webhook ladder's retries, and its response is read by nothing.",
		TokenScope: automationScope,
		Input: []usecase.Field{
			{
				Name: "method", Kind: usecase.KindString, Required: true,
				Enum:        domain.HTTPMethods,
				Description: "The verb.",
			},
			{Name: "url", Kind: usecase.KindString, Required: true, Description: "The address, http or https."},
			{
				Name: "headers", Kind: usecase.KindObject,
				Description: "Plain headers - a content type, an API version. Never a credential: " +
					"that is what the secret header is for.",
			},
			{
				Name: "secret_header_name", Kind: usecase.KindString,
				Description: "The header the secret travels in, e.g. Authorization.",
			},
			{
				Name: "secret_header_value", Kind: usecase.KindString,
				Description: "The secret. Sealed at rest, masked as *** everywhere after " +
					"creation; sending *** on an edit keeps the stored one.",
			},
			{
				Name: "secret_header_sealed", Kind: usecase.KindObject,
				Description: "The stored form the rule writer's sealing produces. Not something " +
					"a caller sends.",
			},
			{
				Name: "signature_header", Kind: usecase.KindString,
				Description: "When set, carries an HMAC-SHA256 over the body computed with the " +
					"secret - the same t=<ts>,v1=<hex> shape a webhook signature has.",
			},
			{
				Name: "body_template", Kind: usecase.KindString,
				Description: "A CEL expression producing the body, rendered against the run's " +
					"event when the call is made. A static body is a string literal.",
			},
			{
				Name: "event_id", Kind: usecase.KindID,
				Description: "The event the body template reads. A rule leaves this out and the " +
					"run supplies the event it is about.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: HTTPRequestedAction, TargetType: httpTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An outbound call is about the workspace's egress rather than about one entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h HttpRequest) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	var eventID shared.ID
	if in.Present("event_id") {
		var err error
		if eventID, err = in.ID("event_id"); err != nil {
			return nil, err
		}
	}

	params := map[string]any{}
	for _, name := range []string{
		"method", "url", "headers", "secret_header_name", "secret_header_value",
		"secret_header_sealed", "signature_header", "body_template",
	} {
		if value, present := in[name]; present {
			params[name] = value
		}
	}

	requested, err := h.Execute(ctx, actor, params, eventID)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"request_id": requested.RequestID.String(),
		"job_id":     requested.JobID.String(),
		"status":     "QUEUED",
	}, nil
}
