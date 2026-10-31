// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"context"
	"encoding/json"
	"strings"

	port "github.com/Jersyfi/hubtask/core/port/chatbot"
)

// Mattermost is a Mattermost slash command and outgoing webhook posting to a bot's intake
// (UC-INT-19): the command captures, the webhook - on the bot's channel, without trigger words -
// unfurls. Each sends the token Mattermost generated for it; both are answered in the response,
// so nothing goes out.
type Mattermost struct{}

var _ port.Platform = Mattermost{}

// Check has nothing to ask: Mattermost proves the tokens on the first post.
func (Mattermost) Check(context.Context, port.Credentials) (string, error) { return "", nil }

// Read holds a slash command to its token and an outgoing webhook to its own.
func (Mattermost) Read(_ context.Context, creds port.Credentials, _ string, d port.Delivery) (port.Inbound, error) {
	form, err := fields(d.Body)
	if err != nil {
		return port.Inbound{}, refused("no form")
	}
	if form["command"] != "" {
		if !sameSecret(form["token"], creds.CommandToken) {
			return port.Inbound{}, refused("a wrong command token")
		}
		return port.Inbound{Asks: port.AsksCapture, Room: form["channel_id"], Text: form["text"], Reply: map[string]string{"kind": "command"}}, nil
	}
	if !sameSecret(form["token"], creds.WebhookToken) {
		return port.Inbound{}, refused("a wrong webhook token")
	}
	return port.Inbound{Asks: port.AsksUnfurl, Room: form["channel_id"], Text: form["text"], Reply: map[string]string{"kind": "webhook"}}, nil
}

func mattermostText(cards []port.Card) string {
	parts := make([]string, 0, len(cards))
	for _, c := range cards {
		text := "**" + markdown(c.Headline) + "**"
		switch {
		case c.Link != "" && c.Title != "":
			text += "\n[" + markdown(c.Title) + "](" + c.Link + ")"
		case c.Link != "":
			text += "\n" + c.Link
		case c.Title != "":
			text += "\n" + markdown(c.Title)
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n")
}

// Answer posts the cards in the response: a command's in the channel, a webhook's as a reply in
// the message's thread. Nothing to say is an empty body, which Mattermost posts nothing for.
func (Mattermost) Answer(_ context.Context, _ port.Credentials, in port.Inbound, cards []port.Card) (port.Response, error) {
	if len(cards) == 0 {
		return port.Response{}, nil
	}
	kind := "comment"
	if in.Asks == port.AsksCapture {
		kind = "in_channel"
	}
	raw, _ := json.Marshal(map[string]string{"response_type": kind, "text": mattermostText(cards)})
	return port.Response{Body: raw}, nil
}
