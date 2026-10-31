// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"io"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	chatbotservice "github.com/Jersyfi/hubtask/core/application/service/chatbot"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	roomport "github.com/Jersyfi/hubtask/core/port/chatbot"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// ChatDeliverer takes a chat platform's post to a bot in.
type ChatDeliverer interface {
	Receive(ctx context.Context, d chatbotservice.Delivery) (roomport.Response, error)
}

// DeliverChat answers POST /chat-intake/{tenantId}/{botId}. The body is read whole - the request
// middleware bounds it - and passed on with every header, since Slack signs over the bytes as
// they came and Teams carries its token in Authorization: whether they prove the bot's
// credentials is decided inwards.
func (c *IntakeDoor) DeliverChat(w http.ResponseWriter, r *http.Request, tenantID, botID openapi_types.UUID) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.ChatIntake == nil {
		pending{}.DeliverChat(w, r, tenantID, botID)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		WriteProblem(w, shared.ErrMalformedRequest.WithDetail("request.body_unreadable").WithCause(err), requestID)
		return
	}
	answer, err := c.ChatIntake.Receive(r.Context(), chatbotservice.Delivery{
		TenantID: shared.ID(tenantID.String()), BotID: shared.ID(botID.String()), Header: r.Header, Body: raw,
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	body := answer.Body
	if len(body) == 0 {
		body = []byte("{}")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
