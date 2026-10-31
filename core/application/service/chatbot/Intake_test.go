// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	roomport "github.com/Jersyfi/hubtask/core/port/chatbot"
)

// platform is a Slack stand-in: a delivery is proven when its body names the signing secret, and
// it asks what the test put in front of it.
type platform struct {
	asks     roomport.Inbound
	answered [][]roomport.Card
	fail     error
	checked  roomport.Credentials
}

func (p *platform) Check(_ context.Context, c roomport.Credentials) (string, error) {
	p.checked = c
	return "U0BOT", nil
}

func (p *platform) Read(_ context.Context, c roomport.Credentials, botUser string, d roomport.Delivery) (roomport.Inbound, error) {
	if string(d.Body) != c.SigningSecret.Reveal() || botUser != "U0BOT" || !d.Now.Equal(now) {
		return roomport.Inbound{}, errors.New("unsigned")
	}
	return p.asks, nil
}

func (p *platform) Answer(_ context.Context, _ roomport.Credentials, in roomport.Inbound, cards []roomport.Card) (roomport.Response, error) {
	if in.Asks == roomport.AsksChallenge {
		return roomport.Response{Body: []byte(in.Challenge)}, nil
	}
	if in.Asks == roomport.AsksNothing {
		return roomport.Response{}, nil
	}
	p.answered = append(p.answered, cards)
	if p.fail != nil {
		return roomport.Response{}, p.fail
	}
	return roomport.Response{Body: []byte("answered")}, nil
}

func slackFixture(t *testing.T) (fixture, *platform, shared.ID) {
	t.Helper()
	f := newFixture()
	p := &platform{}
	f.bots.Platforms = map[integration.DeliveryFormat]roomport.Platform{integration.FormatSlack: p}
	out, err := CreateChatBot{f.bots}.Descriptor().Handler.Invoke(context.Background(), actorOf(anna), usecase.Input{
		"hub_id": hub.String(), "collection_id": collection.String(), "platform": "SLACK", "room": "C0123ABCD",
		"access_token": "xoxb-1", "signing_secret": "sig", "unfurl": true, "capture": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return f, p, shared.ID(out["id"].(string))
}

func deliver(f fixture, id shared.ID, body string) (roomport.Response, error) {
	return f.bots.Receive(context.Background(), Delivery{TenantID: tenant, BotID: id, Body: []byte(body)})
}

// UC-INT-19 check 1: a Slack bot is seated with its app's credentials proven by the platform,
// sealed together under the bot's purpose and never answered; nothing is queued, since Slack
// posts to the intake.
func TestASlackBotIsSeatedWithItsAppsCredentials(t *testing.T) {
	f, p, id := slackFixture(t)
	bot := f.store.bots[id]
	if bot.Platform != integration.FormatSlack || bot.Homeserver != integration.SlackOrigin || bot.BotUser != "U0BOT" || len(f.jobs.requests) != 0 {
		t.Fatalf("bot = %+v, jobs %d", bot, len(f.jobs.requests))
	}
	if p.checked.Token.Reveal() != "xoxb-1" || p.checked.SigningSecret.Reveal() != "sig" {
		t.Fatalf("checked = %+v", p.checked)
	}
	var sealed map[string]string
	if err := json.Unmarshal(f.store.sealed[id].Ciphertext, &sealed); err != nil || sealed["token"] != "xoxb-1" || sealed["signing_secret"] != "sig" {
		t.Fatalf("sealed = %s", f.store.sealed[id].Ciphertext)
	}
	out := Output(bot)
	if out["platform"] != "SLACK" || out["access_token"] != nil || out["signing_secret"] != nil {
		t.Fatalf("out = %v", out)
	}
	for _, c := range []struct {
		in   usecase.Input
		code string
	}{
		{usecase.Input{"platform": "SLACK", "room": "C0123ABCD", "access_token": "xoxp-user", "signing_secret": "s"}, "chatbot.token_required"},
		{usecase.Input{"platform": "SLACK", "room": "C0123ABCD", "access_token": "xoxb-1"}, "chatbot.signing_secret_required"},
		{usecase.Input{"platform": "TEAMS", "room": "19:a@thread.tacv2", "app_id": "nope", "app_tenant": tenant.String(), "app_password": "p"}, "chatbot.app_required"},
		{usecase.Input{"platform": "MATTERMOST", "homeserver": "https://chat.example.org", "room": "abcdefghijklmnopqrstuvwxyz", "command_token": "c"}, "chatbot.webhook_token_required"},
		{usecase.Input{"platform": "IRC", "room": "C0123ABCD"}, "chatbot.platform_unknown"},
	} {
		c.in["hub_id"], c.in["collection_id"], c.in["unfurl"], c.in["capture"] = hub.String(), collection.String(), true, true
		_, err := CreateChatBot{f.bots}.Descriptor().Handler.Invoke(context.Background(), actorOf(anna), c.in)
		var e *shared.Error
		if !errors.As(err, &e) || e.DetailCode != c.code {
			t.Errorf("%v: %v", c.in, err)
		}
	}
}

// UC-INT-19 check 2: a link shared in the bot's channel is answered with the entry's title, status
// and link as the bot's actor - never its notes, nothing for another hub's entry.
func TestASharedLinkIsUnfurledAsTheBotsActor(t *testing.T) {
	f, p, id := slackFixture(t)
	p.asks = roomport.Inbound{Asks: roomport.AsksUnfurl, Room: "C0123ABCD", Links: []string{
		base + "/items/" + entry.String(), base + "/items/" + elsewhere.String(), "https://other.example/items/" + entry.String(),
	}}
	answer, err := deliver(f, id, "sig")
	if err != nil || string(answer.Body) != "answered" || len(p.answered) != 1 {
		t.Fatalf("answer = %s %v %v", answer.Body, err, p.answered)
	}
	cards := p.answered[0]
	if len(cards) != 1 || cards[0].Title != "Renew the certificate" || cards[0].For != base+"/items/"+entry.String() ||
		cards[0].Link != base+"/items/"+entry.String() || !strings.Contains(cards[0].Headline, "IN_PROGRESS") {
		t.Fatalf("cards = %+v", cards)
	}
	if strings.Contains(cards[0].Headline+cards[0].Title, "secret plan") {
		t.Fatal("an entry's notes reached the room")
	}
	if len(f.invoker.calls) != 1 || f.invoker.calls[0] != getEntry+":"+anna.String() {
		t.Fatalf("calls = %v", f.invoker.calls)
	}
}

// UC-INT-19 check 3: a command's text becomes an entry in the bot's collection, answered with its
// link; a bot that does not capture answers nothing.
func TestASlashCommandCapturesAnEntry(t *testing.T) {
	f, p, id := slackFixture(t)
	p.asks = roomport.Inbound{Asks: roomport.AsksCapture, Room: "C0123ABCD", Text: "  Renew the certificate\nmore"}
	if _, err := deliver(f, id, "sig"); err != nil {
		t.Fatal(err)
	}
	cards := p.answered[0]
	if len(cards) != 1 || cards[0].Title != "Renew the certificate" || !strings.HasSuffix(cards[0].Link, "/items/0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a59") {
		t.Fatalf("cards = %+v", cards)
	}
	bot := f.store.bots[id]
	bot.Capture = false
	f.store.bots[id] = bot
	if _, err := deliver(f, id, "sig"); err != nil || len(p.answered[1]) != 0 || len(f.invoker.calls) != 1 {
		t.Fatalf("a bot that does not capture made an entry: %v %v", p.answered, f.invoker.calls)
	}
}

// UC-INT-19 check 4: every reason not to serve is one 404 - a wrong signature, a bot switched off,
// a Matrix bot, an unknown one; a delivery from another channel is acknowledged and not answered;
// Slack's check of the address is answered.
func TestAnIntakeRefusesAlikeAndAnswersItsOwnChannelAlone(t *testing.T) {
	f, p, id := slackFixture(t)
	p.asks = roomport.Inbound{Asks: roomport.AsksUnfurl, Room: "C0OTHER00", Links: []string{base + "/items/" + entry.String()}}
	if answer, err := deliver(f, id, "sig"); err != nil || len(answer.Body) != 0 || len(p.answered) != 0 || len(f.invoker.calls) != 0 {
		t.Fatalf("another channel was answered: %s %v %v", answer.Body, err, p.answered)
	}
	p.asks = roomport.Inbound{Asks: roomport.AsksChallenge, Challenge: "c-1"}
	if answer, err := deliver(f, id, "sig"); err != nil || string(answer.Body) != "c-1" {
		t.Fatalf("challenge = %s %v", answer.Body, err)
	}
	matrix := f.store.bots[id]
	matrix.ID, matrix.Platform = "00000000-0000-7000-8000-0000000000f2", integration.FormatMatrix
	f.store.bots[matrix.ID], f.store.sealed[matrix.ID] = matrix, f.store.sealed[id]
	off := f.store.bots[id]
	off.ID, off.Enabled = "00000000-0000-7000-8000-0000000000f3", false
	f.store.bots[off.ID], f.store.sealed[off.ID] = off, f.store.sealed[id]
	for _, c := range []struct {
		id   shared.ID
		body string
	}{{id, "forged"}, {matrix.ID, "sig"}, {off.ID, "sig"}, {"00000000-0000-7000-8000-0000000000f9", "sig"}} {
		_, err := deliver(f, c.id, c.body)
		var e *shared.Error
		if !errors.As(err, &e) || !errors.Is(err, shared.ErrNotFound) || e.DetailCode != "chatbot.unknown" {
			t.Errorf("%v: %v", c, err)
		}
	}
}

// UC-INT-19 check 5: an answer the platform no longer takes is recorded on the bot, the delivery
// still acknowledged; the next one that goes through clears it.
func TestARefusedAnswerIsRecordedAndHeals(t *testing.T) {
	f, p, id := slackFixture(t)
	p.asks = roomport.Inbound{Asks: roomport.AsksUnfurl, Room: "C0123ABCD", Links: []string{base + "/items/" + entry.String()}}
	p.fail = shared.ErrForbidden.WithDetail("chatbot.token_revoked")
	if _, err := deliver(f, id, "sig"); err != nil || f.store.bots[id].Failure != "chatbot.token_revoked" {
		t.Fatalf("failure = %q %v", f.store.bots[id].Failure, err)
	}
	p.fail = nil
	if _, err := deliver(f, id, "sig"); err != nil || f.store.bots[id].Failure != "" {
		t.Fatalf("failure stayed %q %v", f.store.bots[id].Failure, err)
	}
	if next, err := f.bots.Run(context.Background(), tenant, id); err != nil || !next.IsZero() {
		t.Fatalf("a Slack bot was read like a Matrix room: %v %v", next, err)
	}
}
