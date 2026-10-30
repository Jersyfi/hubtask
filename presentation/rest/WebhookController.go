// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The webhook subscriptions. The controller holds no rules: who may subscribe an external
// system to this workspace's events is decided inwards of here, like every other authorisation
// (ADR-0005). What this layer does is map a request to an input and an answer to a document - and,
// twice, carry a signing secret from the use case to the response without letting it touch
// anything else on the way.

// webhookSecretResponse is a subscription with its signing secret: the creation's and a
// rotation's answer, and no projection's - which is what makes "shown once" a property of the code.
func webhookSecretResponse(out usecase.Output) openapi.WebhookSubscriptionSecret {
	answer := openapi.WebhookSubscriptionSecret{
		Id:           uuidValue(out.String("id")),
		TargetUrl:    out.String("target_url"),
		EventTypes:   scopeList(out["event_types"]),
		State:        openapi.WebhookSubscriptionSecretState(out.String("state")),
		FailureCount: out.Int("failure_count"),
		CreatedAt:    timeValue(out["created_at"]),
		Version:      out.Int("version"),
		Secret:       out.String("secret"),
		Scope:        webhookScopeResponse(out["scope"]),
	}
	if filter := out.String("filter"); filter != "" {
		answer.Filter = &filter
	}
	answer.Format, answer.Room, answer.Locale = chatResponse(out)
	return answer
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
		Scope:        webhookScopeResponse(out["scope"]),
	}
	if filter := out.String("filter"); filter != "" {
		subscription.Filter = &filter
	}
	if lastError := out.String("last_error"); lastError != "" {
		subscription.LastError = &lastError
	}
	subscription.Format, subscription.Room, subscription.Locale = chatResponse(out)
	return subscription
}

// chatResponse is a subscription's format and, for a chat room, its room and language.
func chatResponse(out usecase.Output) (*openapi.WebhookFormat, *string, *string) {
	format := openapi.WebhookFormat(out.String("format"))
	if format == "" {
		format = openapi.WebhookFormatCLOUDEVENTS
	}
	var room, locale *string
	if value := out.String("room"); value != "" {
		room = &value
	}
	if value := out.String("locale"); value != "" {
		locale = &value
	}
	return &format, room, locale
}

// webhookScopeResponse is the projection's scope as the contract answers it.
func webhookScopeResponse(value any) *openapi.WebhookScope {
	document, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	level, _ := document["type"].(string)
	scope := openapi.WebhookScope{Type: openapi.WebhookScopeType(level)}
	if raw, _ := document["id"].(string); raw != "" {
		id := uuidValue(raw)
		scope.Id = &id
	}
	return &scope
}

// The deliveries and the rotation. Two of the three carry a secret or a log rather than a
// subscription, so their responses are shaped here beside the ones above.

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

// The polling trigger. Its own operation rather than a variant of the subscription routes:
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
