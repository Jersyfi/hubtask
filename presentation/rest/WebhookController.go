// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The webhook subscriptions (G-03). The controller holds no rules: who may subscribe an external
// system to this workspace's events is decided inwards of here, like every other authorisation
// (ADR-0005). What this layer does is map a request to an input and an answer to a document - and,
// twice, carry a signing secret from the use case to the response without letting it touch
// anything else on the way.

const (
	listWebhookSubscriptionsUseCase  = "ListWebhookSubscriptions"
	createWebhookSubscriptionUseCase = "CreateWebhookSubscription"
	getWebhookSubscriptionUseCase    = "GetWebhookSubscription"
	updateWebhookSubscriptionUseCase = "UpdateWebhookSubscription"
	deleteWebhookSubscriptionUseCase = "DeleteWebhookSubscription"
)

// ListWebhookSubscriptions answers GET /integrations/webhooks.
//
// Written out rather than through the identity helper, for ListServiceAccounts' reason: the
// helper's closure takes no context, and an operation with no parameters gives the linter nothing
// to trace the request's context through.
func (c *RestController) ListWebhookSubscriptions(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(
		r.Context(), listWebhookSubscriptionsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["data"].([]usecase.Output)
	subscriptions := make([]openapi.WebhookSubscription, 0, len(rows))
	for _, row := range rows {
		subscriptions = append(subscriptions, webhookResponse(row))
	}
	writeJSON(w, r, http.StatusOK, subscriptions)
}

// CreateWebhookSubscription answers POST /integrations/webhooks.
func (c *RestController) CreateWebhookSubscription(
	w http.ResponseWriter, r *http.Request, _ openapi.CreateWebhookSubscriptionParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.WebhookSubscriptionCreate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}

		types := make([]any, 0, len(body.EventTypes))
		for _, wanted := range body.EventTypes {
			types = append(types, wanted)
		}
		input := usecase.Input{
			"target_url":  body.TargetUrl,
			"event_types": types,
		}
		if body.Filter != nil {
			// Sent only when the client sent one, so that the refusal is about a filter the caller
			// actually asked for rather than about an absent field.
			input["filter"] = *body.Filter
		}
		return c.UseCases.Invoke(r.Context(), createWebhookSubscriptionUseCase, actor, input)
	}, func(out usecase.Output) {
		created := openapi.WebhookSubscriptionSecret{
			Id:           uuidValue(out.String("id")),
			TargetUrl:    out.String("target_url"),
			EventTypes:   scopeList(out["event_types"]),
			State:        openapi.WebhookSubscriptionSecretState(out.String("state")),
			FailureCount: out.Int("failure_count"),
			CreatedAt:    timeValue(out["created_at"]),
			Version:      out.Int("version"),
			// The one response that carries a signing secret, beside a rotation. It is here and in
			// no projection, which is what makes "shown once" a property of the code.
			Secret: out.String("secret"),
		}
		w.Header().Set("Location", APIBasePath+"/integrations/webhooks/"+created.Id.String())
		writeJSON(w, r, http.StatusCreated, created)
	})
}

// GetWebhookSubscription answers GET /integrations/webhooks/{webhookId}.
func (c *RestController) GetWebhookSubscription(
	w http.ResponseWriter, r *http.Request, webhookID openapi.WebhookId,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), getWebhookSubscriptionUseCase, actor, usecase.Input{
			"webhook_id": webhookID.String(),
		})
	}, func(out usecase.Output) {
		w.Header().Set("ETag", etag(out.Int("version")))
		writeJSON(w, r, http.StatusOK, webhookResponse(out))
	})
}

// UpdateWebhookSubscription answers PATCH /integrations/webhooks/{webhookId}.
func (c *RestController) UpdateWebhookSubscription(
	w http.ResponseWriter, r *http.Request, webhookID openapi.WebhookId,
	params openapi.UpdateWebhookSubscriptionParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.WebhookSubscriptionUpdate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}

		input := usecase.Input{
			"webhook_id": webhookID.String(),
			"target_url": optionalStringField(body.TargetUrl),
			"filter":     optionalStringField(body.Filter),
		}
		if body.State != nil {
			input["state"] = string(*body.State)
		}
		if body.EventTypes != nil {
			// Absent leaves the set alone; present replaces it. Setting the key only when the
			// client sent one is what carries that distinction into the catalogue.
			types := make([]any, 0, len(*body.EventTypes))
			for _, wanted := range *body.EventTypes {
				types = append(types, wanted)
			}
			input["event_types"] = types
		}
		if version, ok := versionFromIfMatch(params.IfMatch); ok {
			input["expected_version"] = version
		}
		return c.UseCases.Invoke(r.Context(), updateWebhookSubscriptionUseCase, actor, input)
	}, func(out usecase.Output) {
		w.Header().Set("ETag", etag(out.Int("version")))
		writeJSON(w, r, http.StatusOK, webhookResponse(out))
	})
}

// DeleteWebhookSubscription answers DELETE /integrations/webhooks/{webhookId}.
func (c *RestController) DeleteWebhookSubscription(
	w http.ResponseWriter, r *http.Request, webhookID openapi.WebhookId,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), deleteWebhookSubscriptionUseCase, actor, usecase.Input{
			"webhook_id": webhookID.String(),
		})
	}, func(usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

func webhookResponse(out usecase.Output) openapi.WebhookSubscription {
	subscription := openapi.WebhookSubscription{
		Id:           uuidValue(out.String("id")),
		TargetUrl:    out.String("target_url"),
		EventTypes:   scopeList(out["event_types"]),
		State:        openapi.WebhookSubscriptionState(out.String("state")),
		FailureCount: out.Int("failure_count"),
		CreatedAt:    timeValue(out["created_at"]),
		Version:      out.Int("version"),
	}
	if filter := out.String("filter"); filter != "" {
		subscription.Filter = &filter
	}
	if lastError := out.String("last_error"); lastError != "" {
		subscription.LastError = &lastError
	}
	return subscription
}

// The deliveries and the rotation. Two of the three carry a secret or a log rather than a
// subscription, so their responses are shaped here beside the ones above.
const (
	listWebhookDeliveriesUseCase = "ListWebhookDeliveries"
	replayWebhookDeliveryUseCase = "ReplayWebhookDelivery"
	sendWebhookUseCase           = "SendWebhook"
	rotateWebhookSecretUseCase   = "RotateWebhookSecret" //nolint:gosec // G101: a use case name, not a credential.
)

// ListWebhookDeliveries answers GET /integrations/webhooks/{webhookId}/deliveries.
func (c *RestController) ListWebhookDeliveries(
	w http.ResponseWriter, r *http.Request, webhookID openapi.WebhookId,
	params openapi.ListWebhookDeliveriesParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		input := usecase.Input{"webhook_id": webhookID.String()}
		if params.Status != nil {
			input["status"] = string(*params.Status)
		}
		if params.Cursor != nil {
			input["cursor"] = *params.Cursor
		}
		if params.Size != nil {
			input["size"] = *params.Size
		}
		return c.UseCases.Invoke(r.Context(), listWebhookDeliveriesUseCase, actor, input)
	}, func(out usecase.Output) {
		rows, _ := out["data"].([]usecase.Output)
		deliveries := make([]openapi.WebhookDelivery, 0, len(rows))
		for _, row := range rows {
			deliveries = append(deliveries, deliveryResponse(row))
		}
		writeJSON(w, r, http.StatusOK, openapi.WebhookDeliveryPage{
			Data: deliveries, Page: pageResponse(out),
		})
	})
}

// ReplayWebhookDelivery answers POST /integrations/webhooks/{id}/deliveries/{deliveryId}:replay.
func (c *RestController) ReplayWebhookDelivery(
	w http.ResponseWriter, r *http.Request, webhookID openapi.WebhookId, deliveryID openapi.DeliveryId,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), replayWebhookDeliveryUseCase, actor, usecase.Input{
			"webhook_id":  webhookID.String(),
			"delivery_id": deliveryID.String(),
		})
	}, func(out usecase.Output) {
		// Accepted rather than created: the attempt is recorded and queued, and whether the target
		// answers is not something this response can know.
		writeJSON(w, r, http.StatusAccepted, deliveryResponse(out))
	})
}

// SendWebhook answers POST /integrations/webhooks/{webhookId}:send.
func (c *RestController) SendWebhook(
	w http.ResponseWriter, r *http.Request, webhookID openapi.WebhookId,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.WebhookSend
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		return c.UseCases.Invoke(r.Context(), sendWebhookUseCase, actor, usecase.Input{
			"subscription_id": webhookID.String(),
			"event_id":        body.EventId.String(),
		})
	}, func(out usecase.Output) {
		// Accepted rather than created: the delivery is recorded and queued, and whether the
		// target answers is not something this response can know.
		writeJSON(w, r, http.StatusAccepted, deliveryResponse(out))
	})
}

// RotateWebhookSecret answers POST /integrations/webhooks/{webhookId}:rotate-secret.
func (c *RestController) RotateWebhookSecret(
	w http.ResponseWriter, r *http.Request, webhookID openapi.WebhookId,
	_ openapi.RotateWebhookSecretParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		input := usecase.Input{"webhook_id": webhookID.String()}

		// The body is optional, and an absent one is not an empty one: omitting the period asks
		// for the default, and sending zero asks for the old secret to be retired at once. A
		// rotation with no body at all is the common case - "give me a new secret, the usual
		// grace" - so an empty body is read as that rather than refused.
		if r.ContentLength > 0 {
			var body openapi.WebhookSecretRotation
			if err := decodeJSON(r, &body); err != nil {
				return nil, err
			}
			if body.GraceSeconds != nil {
				input["grace_seconds"] = *body.GraceSeconds
			}
		}
		return c.UseCases.Invoke(r.Context(), rotateWebhookSecretUseCase, actor, input)
	}, func(out usecase.Output) {
		rotated := openapi.WebhookSubscriptionSecret{
			Id:           uuidValue(out.String("id")),
			TargetUrl:    out.String("target_url"),
			EventTypes:   scopeList(out["event_types"]),
			State:        openapi.WebhookSubscriptionSecretState(out.String("state")),
			FailureCount: out.Int("failure_count"),
			CreatedAt:    timeValue(out["created_at"]),
			Version:      out.Int("version"),
			Secret:       out.String("secret"),
		}
		writeJSON(w, r, http.StatusOK, rotated)
	})
}

func deliveryResponse(out usecase.Output) openapi.WebhookDelivery {
	delivery := openapi.WebhookDelivery{
		Id:             uuidValue(out.String("id")),
		SubscriptionId: uuidValue(out.String("subscription_id")),
		EventId:        uuidValue(out.String("event_id")),
		Attempt:        out.Int("attempt"),
		Status:         openapi.WebhookDeliveryStatus(out.String("status")),
		CreatedAt:      timeValue(out["created_at"]),
		NextAttemptAt:  optionalTimeField(out["next_attempt_at"]),
	}
	if status, ok := out["response_status"].(int); ok {
		delivery.ResponseStatus = &status
	}
	if code := out.String("error_code"); code != "" {
		delivery.ErrorCode = &code
	}
	return delivery
}

// The polling trigger (G-04). Its own operation rather than a variant of the subscription routes:
// the two are one stream and two transports, and this is the transport that carries no state.
const pollTriggerEventsUseCase = "PollTriggerEvents"

// PollTriggerEvents answers GET /integrations/triggers/{eventType}.
//
// The entries pass through unread. What the use case rendered is the CloudEvents document a webhook
// delivery would have carried, and a controller that re-assembled it field by field would be a
// second statement of the wire format - one that drops an extension attribute the day one is added,
// silently and only for the pull half.
func (c *RestController) PollTriggerEvents(
	w http.ResponseWriter, r *http.Request, eventType openapi.EventType,
	params openapi.PollTriggerEventsParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		input := usecase.Input{"event_type": string(eventType)}
		if params.Since != nil {
			input["cursor"] = *params.Since
		}
		if params.Limit != nil {
			input["limit"] = *params.Limit
		}
		return c.UseCases.Invoke(r.Context(), pollTriggerEventsUseCase, actor, input)
	}, func(out usecase.Output) {
		rows, _ := out["data"].([]any)
		events := make([]openapi.TriggerEvent, 0, len(rows))
		for _, row := range rows {
			rendered, ok := row.(map[string]any)
			if !ok {
				continue
			}
			events = append(events, rendered)
		}
		writeJSON(w, r, http.StatusOK, openapi.TriggerEventPage{
			Data: events, Page: pageResponse(out),
		})
	})
}
