// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	port "github.com/Jersyfi/hubtask/core/port/chatbot"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
)

// Mattermost is a Mattermost slash command and outgoing webhook posting to a bot's intake
// (UC-INT-19): the command captures, the webhook - on the bot's channel, without trigger words -
// unfurls. Each sends the token Mattermost generated for it; both are answered in the response.
// The one call out is Tell's, through an incoming webhook the administrator gave the bot.
type Mattermost struct {
	Client httpport.Port
}

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
		return sender(port.Inbound{Asks: port.AsksCapture, Room: form["channel_id"], Text: form["text"], Bare: true, Reply: map[string]string{"kind": "command"}}, form), nil
	}
	if !sameSecret(form["token"], creds.WebhookToken) {
		return port.Inbound{}, refused("a wrong webhook token")
	}
	return sender(port.Inbound{Asks: port.AsksUnfurl, Room: form["channel_id"], Text: form["text"], Reply: map[string]string{"kind": "webhook"}}, form), nil
}

var mattermostName = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

// sender names who said it, and their direct channel by name - what an incoming webhook posts to.
func sender(in port.Inbound, form map[string]string) port.Inbound {
	in.Sender = form["user_id"]
	if in.Sender != "" && mattermostName.MatchString(form["user_name"]) {
		in.Address = "@" + form["user_name"]
	}
	return in
}

// Tell posts the card to the person's direct channel through the bot's incoming webhook, at the
// server's `/hooks/<key>`.
func (m Mattermost) Tell(ctx context.Context, creds port.Credentials, server, address string, card port.Card) error {
	if !strings.HasPrefix(address, "@") || !mattermostName.MatchString(address[1:]) {
		return cannotTell("not a Mattermost name")
	}
	if creds.PostHook.IsEmpty() || !strings.HasPrefix(server, "https://") {
		return cannotTell("no incoming webhook")
	}
	if m.Client == nil {
		return unreachable(errors.New("chat bot: no outbound client is wired"))
	}
	raw, _ := json.Marshal(map[string]string{"channel": address, "text": mattermostText([]port.Card{card})})
	response, err := m.Client.Do(ctx, httpport.Request{
		Method: http.MethodPost, URL: strings.TrimSuffix(server, "/") + "/hooks/" + url.PathEscape(creds.PostHook.Reveal()),
		Body: raw, TargetClass: TargetClass, Header: map[string][]string{"Content-Type": {"application/json"}},
	})
	switch {
	case err != nil:
		return unreachable(err)
	case response.Status == http.StatusUnauthorized || response.Status == http.StatusForbidden || response.Status == http.StatusNotFound:
		return revoked("Mattermost refused the incoming webhook")
	case response.Status/100 != 2:
		return unreachable(errors.New("chat bot: Mattermost answered " + http.StatusText(response.Status)))
	}
	return nil
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
	switch {
	case in.Privately:
		kind = "ephemeral"
	case in.Asks == port.AsksCapture:
		kind = "in_channel"
	}
	raw, _ := json.Marshal(map[string]string{"response_type": kind, "text": mattermostText(cards)})
	return port.Response{Body: raw}, nil
}
