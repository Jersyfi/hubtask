// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package chatbot speaks the chat platforms for a hub's bot, every call through the guarded client
// (rule 6): the Matrix client-server API - who the account is, joining its room, a sync filtered
// to that room's messages, a notice posted once per transaction (UC-INT-18) - and the deliveries
// Slack, Teams and Mattermost post to the bot's intake, verified and answered in each platform's
// format (UC-INT-19).
package chatbot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/chatbot"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// TargetClass labels the calls for the outbound duration histogram.
const TargetClass = "chat_bot"

const api = "/_matrix/client/v3"

// Matrix is the client-server API.
type Matrix struct {
	Client httpport.Port
}

var _ port.Room = Matrix{}

func unreachable(cause error) error {
	return shared.ErrUnavailable.WithDetail("chatbot.homeserver_unreachable").WithCause(cause)
}

func (m Matrix) call(ctx context.Context, method, address string, token secret.Secret, body any, into any) error {
	if m.Client == nil {
		return unreachable(errors.New("chat bot: no outbound client is wired"))
	}
	header := map[string][]string{"Authorization": {"Bearer " + token.Reveal()}, "Accept": {"application/json"}}
	var raw []byte
	if body != nil {
		header["Content-Type"] = []string{"application/json"}
		raw, _ = json.Marshal(body)
	}
	response, err := m.Client.Do(ctx, httpport.Request{Method: method, URL: address, Header: header, Body: raw, TargetClass: TargetClass})
	if err != nil {
		return unreachable(err)
	}
	switch {
	case response.Status == http.StatusUnauthorized || response.Status == http.StatusForbidden:
		return shared.ErrForbidden.WithDetail("chatbot.token_revoked").
			WithCause(errors.New("chat bot: the homeserver answered " + http.StatusText(response.Status)))
	case response.Status/100 != 2:
		return unreachable(errors.New("chat bot: the homeserver answered " + http.StatusText(response.Status)))
	}
	if into != nil && json.Unmarshal(response.Body, into) != nil {
		return unreachable(errors.New("chat bot: the homeserver answered no JSON"))
	}
	return nil
}

func (m Matrix) WhoAmI(ctx context.Context, homeserver string, token secret.Secret) (string, error) {
	var answer struct {
		UserID string `json:"user_id"`
	}
	if err := m.call(ctx, http.MethodGet, homeserver+api+"/account/whoami", token, nil, &answer); err != nil {
		return "", err
	}
	if answer.UserID == "" {
		return "", unreachable(errors.New("chat bot: the homeserver named no account"))
	}
	return answer.UserID, nil
}

func (m Matrix) Join(ctx context.Context, homeserver string, token secret.Secret, room string) error {
	return m.call(ctx, http.MethodPost, homeserver+api+"/join/"+url.PathEscape(room), token, map[string]any{}, nil)
}

type syncAnswer struct {
	NextBatch string `json:"next_batch"`
	Rooms     struct {
		Join map[string]struct {
			Timeline struct {
				Events []struct {
					Type    string `json:"type"`
					EventID string `json:"event_id"`
					Sender  string `json:"sender"`
					Content struct {
						MsgType string `json:"msgtype"`
						Body    string `json:"body"`
					} `json:"content"`
				} `json:"events"`
			} `json:"timeline"`
		} `json:"join"`
	} `json:"rooms"`
}

func (m Matrix) Read(ctx context.Context, homeserver string, token secret.Secret, room, since string) (string, []port.Message, error) {
	limit := integration.MaxMessagesPerRun
	if since == "" {
		limit = 1
	}
	filter, _ := json.Marshal(map[string]any{
		"presence":     map[string]any{"types": []string{}},
		"account_data": map[string]any{"types": []string{}},
		"room": map[string]any{
			"rooms": []string{room}, "state": map[string]any{"types": []string{}}, "ephemeral": map[string]any{"types": []string{}},
			"account_data": map[string]any{"types": []string{}},
			"timeline":     map[string]any{"types": []string{"m.room.message"}, "limit": limit},
		},
	})
	query := url.Values{"filter": {string(filter)}, "timeout": {"0"}}
	if since != "" {
		query.Set("since", since)
	}
	var answer syncAnswer
	if err := m.call(ctx, http.MethodGet, homeserver+api+"/sync?"+query.Encode(), token, nil, &answer); err != nil {
		return "", nil, err
	}
	if since == "" {
		return answer.NextBatch, nil, nil
	}
	messages := []port.Message{}
	for _, e := range answer.Rooms.Join[room].Timeline.Events {
		if e.Type != "m.room.message" || e.Content.MsgType != "m.text" || e.Content.Body == "" {
			continue
		}
		messages = append(messages, port.Message{EventID: e.EventID, Sender: e.Sender, Body: e.Content.Body})
	}
	return answer.NextBatch, messages, nil
}

func (m Matrix) Post(ctx context.Context, homeserver string, token secret.Secret, room, txn string, notice port.Notice) error {
	body := map[string]any{"msgtype": "m.notice", "body": notice.Body}
	if notice.HTML != "" {
		body["format"], body["formatted_body"] = "org.matrix.custom.html", notice.HTML
	}
	address := homeserver + api + "/rooms/" + url.PathEscape(room) + "/send/m.room.message/" + url.PathEscape(txn)
	return m.call(ctx, http.MethodPut, address, token, body, nil)
}
