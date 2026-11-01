// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"strings"
	"testing"

	port "github.com/Jersyfi/hubtask/core/port/chatbot"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var waiting = port.Card{Headline: "Waiting for your yes: !approve 099A8057", Title: "Laptop for Jonas", Link: "https://acme.hubtask.example/approvals"}

// UC-SVC-09 check 6: a slash command names who said it and where a card to them alone goes - the
// app's conversation with them - and an approval waiting is posted there through chat.postMessage.
func TestSlackTellsALinkedPersonInTheirOwnConversation(t *testing.T) {
	ctx := context.Background()
	s := Slack{Client: &recorder{answers: []httpport.Response{{Status: 200, Body: []byte(`{"ok":true}`)}}}}
	in, err := s.Read(ctx, slackCreds, "U0BOT", signedSlack(fixture(t, "slack_command.txt"), slackCreds.SigningSecret.Reveal(), arrived))
	if err != nil || in.Sender != "U0123ADA" || in.Address != "U0123ADA" || !in.Bare {
		t.Fatalf("command = %+v %v", in, err)
	}
	if err := s.Tell(ctx, slackCreds, "", in.Address, waiting); err != nil {
		t.Fatal(err)
	}
	sent := s.Client.(*recorder).requests[0]
	var body map[string]any
	_ = json.Unmarshal(sent.Body, &body)
	if sent.URL != SlackAPI+"/chat.postMessage" || body["channel"] != "U0123ADA" || !strings.Contains(string(sent.Body), "Laptop for Jonas") {
		t.Fatalf("posted %s %s", sent.URL, sent.Body)
	}
	if err := s.Tell(ctx, slackCreds, "", "C0123ABCD", waiting); detail(err) != "chatbot.cannot_tell" {
		t.Fatalf("a channel is no person: %v", err)
	}
}

// UC-SVC-09 check 6: Mattermost's command names its sender by name, and the card goes to their
// direct channel through the bot's incoming webhook; without one nothing is posted.
func TestMattermostTellsALinkedPersonThroughItsIncomingWebhook(t *testing.T) {
	ctx := context.Background()
	fake := &recorder{answers: []httpport.Response{{Status: 200, Body: []byte(`ok`)}, {Status: 404}}}
	m := Mattermost{Client: fake}
	creds := port.Credentials{CommandToken: secret.New("cmdtoken9pqm3sj6b5z7rgb6xw"), PostHook: secret.New("hook7k3m")}
	in, err := m.Read(ctx, creds, "", port.Delivery{Body: fixture(t, "mattermost_command.txt"), Now: arrived})
	if err != nil || in.Sender != "u5fhf1ohujgo8brxkx1hb5chwa" || in.Address != "@ada" || !in.Bare {
		t.Fatalf("command = %+v %v", in, err)
	}
	if err := m.Tell(ctx, creds, "https://chat.example.org", in.Address, waiting); err != nil {
		t.Fatal(err)
	}
	sent := fake.requests[0]
	if sent.URL != "https://chat.example.org/hooks/hook7k3m" || !strings.Contains(string(sent.Body), `"channel":"@ada"`) {
		t.Fatalf("posted %s %s", sent.URL, sent.Body)
	}
	if err := m.Tell(ctx, creds, "https://chat.example.org", in.Address, waiting); detail(err) != "chatbot.token_revoked" {
		t.Fatalf("a webhook gone: %v", err)
	}
	if err := m.Tell(ctx, port.Credentials{}, "https://chat.example.org", in.Address, waiting); detail(err) != "chatbot.cannot_tell" || len(fake.requests) != 2 {
		t.Fatalf("no webhook: %v", err)
	}
}

// UC-SVC-09 check 6: only the bot's personal chat is an address a card to one person may go to;
// the card is posted there through the Bot Connector with the bot's own token.
func TestTeamsTellsALinkedPersonInThePersonalChat(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fake := &recorder{answers: []httpport.Response{
		{Status: 200, Body: []byte(`{"access_token":"bot-token"}`)},
		{Status: 201, Body: []byte(`{"id":"1"}`)},
	}}
	teams := Teams{Client: fake, Keys: StaticKeys{"k1": &key.PublicKey}}
	ctx := context.Background()
	service := "https://smba.trafficmanager.net/emea/"
	token := botFrameworkToken(t, key, teamsClaims(teamsApp, service))
	channel := fixture(t, "teams_message.json")
	in, err := teams.Read(ctx, teamsCreds, teamsApp, port.Delivery{Body: channel, Now: arrived, Header: map[string][]string{"Authorization": {"Bearer " + token}}})
	if err != nil || in.Sender != "29:1ada" || in.Address != "" {
		t.Fatalf("a channel's message = %+v %v", in, err)
	}
	personal := strings.Replace(strings.Replace(string(channel), `"conversationType":"channel"`, `"conversationType":"personal"`, 1),
		"19:ops@thread.tacv2;messageid=1730289600000", "a:1personal", 1)
	in, err = teams.Read(ctx, teamsCreds, teamsApp, port.Delivery{Body: []byte(personal), Now: arrived, Header: map[string][]string{"Authorization": {"Bearer " + token}}})
	if err != nil || in.Address != service+" a:1personal" {
		t.Fatalf("a personal message = %+v %v", in, err)
	}
	if err := teams.Tell(ctx, teamsCreds, "", in.Address, waiting); err != nil {
		t.Fatal(err)
	}
	if sent := fake.requests[1]; sent.URL != service+"v3/conversations/a:1personal/activities" || sent.Header["Authorization"][0] != "Bearer bot-token" {
		t.Fatalf("posted %s %v", sent.URL, sent.Header)
	}
	if err := teams.Tell(ctx, teamsCreds, "", "https://evil.example/ a:1personal", waiting); detail(err) != "chatbot.cannot_tell" {
		t.Fatalf("a service outside the Bot Framework: %v", err)
	}
}
