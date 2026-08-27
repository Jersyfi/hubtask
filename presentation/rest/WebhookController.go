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
