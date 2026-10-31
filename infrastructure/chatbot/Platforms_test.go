// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/chatbot"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var arrived = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func detail(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func signedSlack(body []byte, signing string, at time.Time) port.Delivery {
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(signing))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(body)
	return port.Delivery{Body: body, Now: arrived, Header: map[string][]string{
		"X-Slack-Request-Timestamp": {ts}, "X-Slack-Signature": {"v0=" + hex.EncodeToString(mac.Sum(nil))},
	}}
}

var slackCreds = port.Credentials{Token: secret.New("xoxb-1"), SigningSecret: secret.New("8f742231b10e8888abcd99yyyzzz85a5")}

// UC-INT-19 check 4: Slack's delivery holds only with its signature over its own timestamp and
// body; a forged one, a replayed old one or one without is refused.
func TestSlackDeliveriesHoldToTheirSignature(t *testing.T) {
	s := Slack{}
	ctx := context.Background()
	body := fixture(t, "slack_url_verification.json")
	in, err := s.Read(ctx, slackCreds, "U0BOT", signedSlack(body, slackCreds.SigningSecret.Reveal(), arrived))
	if err != nil || in.Asks != port.AsksChallenge || in.Challenge != "3eZbrw1aBm2rZgRNFdxV2595E9CY3gmdALWMmHkvFXO7tYXAYM8P" {
		t.Fatalf("challenge = %+v %v", in, err)
	}
	answer, _ := s.Answer(ctx, slackCreds, in, nil)
	if string(answer.Body) != `{"challenge":"3eZbrw1aBm2rZgRNFdxV2595E9CY3gmdALWMmHkvFXO7tYXAYM8P"}` {
		t.Fatalf("answer = %s", answer.Body)
	}
	for name, d := range map[string]port.Delivery{
		"forged":   signedSlack(body, "another secret", arrived),
		"replayed": signedSlack(body, slackCreds.SigningSecret.Reveal(), arrived.Add(-6*time.Minute)),
		"unsigned": {Body: body, Now: arrived},
	} {
		if _, err := s.Read(ctx, slackCreds, "U0BOT", d); detail(err) != "chatbot.delivery_refused" {
			t.Errorf("%s: %v", name, err)
		}
	}
	tampered := signedSlack(body, slackCreds.SigningSecret.Reveal(), arrived)
	tampered.Body = []byte(strings.Replace(string(body), "3eZ", "4eZ", 1))
	if _, err := s.Read(ctx, slackCreds, "U0BOT", tampered); detail(err) != "chatbot.delivery_refused" {
		t.Errorf("a changed body was taken: %v", err)
	}
}

type recorder struct {
	answers  []httpport.Response
	requests []httpport.Request
}

func (r *recorder) Do(_ context.Context, req httpport.Request) (httpport.Response, error) {
	r.requests = append(r.requests, req)
	answer := r.answers[0]
	r.answers = r.answers[1:]
	return answer, nil
}

var card = port.Card{Headline: "Entry, status IN_PROGRESS", Title: "Renew <!channel> & more", Link: "https://hub.example.org/items/0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a51"}

// UC-INT-19 checks 1 and 2: the bot token is asked whom it is; a shared link is unfurled through
// `chat.unfurl` with the bot token, the title escaped so it calls no channel.
func TestSlackUnfurlsThroughItsAPI(t *testing.T) {
	fake := &recorder{answers: []httpport.Response{
		{Status: 200, Body: []byte(`{"ok":true,"url":"https://example.slack.com/","team":"Example","user":"hubtask","team_id":"T0123ABCD","user_id":"U0BOT","bot_id":"B0BOT"}`)},
		{Status: 200, Body: []byte(`{"ok":true}`)},
		{Status: 200, Body: []byte(`{"ok":false,"error":"token_revoked"}`)},
	}}
	s := Slack{Client: fake}
	ctx := context.Background()
	if who, err := s.Check(ctx, slackCreds); err != nil || who != "U0BOT" || fake.requests[0].URL != SlackAPI+"/auth.test" {
		t.Fatalf("check = %q %v", who, err)
	}
	in, err := s.Read(ctx, slackCreds, "U0BOT", signedSlack(fixture(t, "slack_link_shared.json"), slackCreds.SigningSecret.Reveal(), arrived))
	if err != nil || in.Asks != port.AsksUnfurl || in.Room != "C0123ABCD" || len(in.Links) != 2 {
		t.Fatalf("in = %+v %v", in, err)
	}
	c := card
	c.For = in.Links[0]
	if _, err := s.Answer(ctx, slackCreds, in, []port.Card{c}); err != nil {
		t.Fatal(err)
	}
	sent := fake.requests[1]
	if sent.URL != SlackAPI+"/chat.unfurl" || sent.Header["Authorization"][0] != "Bearer xoxb-1" {
		t.Fatalf("sent = %+v", sent)
	}
	var body struct {
		UnfurlID string                    `json:"unfurl_id"`
		Source   string                    `json:"source"`
		Unfurls  map[string]map[string]any `json:"unfurls"`
	}
	if err := json.Unmarshal(sent.Body, &body); err != nil || body.UnfurlID != "C0123ABCD.1730289600.000100.7a9f" || body.Source != "conversations_history" {
		t.Fatalf("body = %s", sent.Body)
	}
	text := string(sent.Body)
	if _, ok := body.Unfurls[in.Links[0]]; !ok || strings.Contains(text, "<!channel>") || !strings.Contains(text, "Renew \\u0026lt;!channel\\u0026gt; \\u0026amp; more") {
		t.Fatalf("unfurls = %s", text)
	}
	if _, err := s.Answer(ctx, slackCreds, in, []port.Card{c}); detail(err) != "chatbot.token_revoked" {
		t.Fatalf("a revoked token = %v", err)
	}
}

// UC-INT-19 check 3: the slash command's text is a capture in its channel, answered in the
// response in the channel.
func TestSlackSlashCommandIsAnsweredInTheResponse(t *testing.T) {
	s := Slack{}
	ctx := context.Background()
	in, err := s.Read(ctx, slackCreds, "U0BOT", signedSlack(fixture(t, "slack_command.txt"), slackCreds.SigningSecret.Reveal(), arrived))
	if err != nil || in.Asks != port.AsksCapture || in.Room != "C0123ABCD" || in.Text != "Renew the certificate" {
		t.Fatalf("in = %+v %v", in, err)
	}
	answer, err := s.Answer(ctx, slackCreds, in, []port.Card{card})
	var doc map[string]any
	if err != nil || json.Unmarshal(answer.Body, &doc) != nil || doc["response_type"] != "in_channel" {
		t.Fatalf("answer = %s %v", answer.Body, err)
	}
}

var mattermostCreds = port.Credentials{CommandToken: secret.New("cmdtoken9pqm3sj6b5z7rgb6xw"), WebhookToken: secret.New("hooktoken7a3ckxkjz8yrfbx8sa")}

// UC-INT-19 checks 2, 3 and 4: Mattermost's slash command holds to its token and captures, its
// outgoing webhook holds to its own and unfurls; one token never opens the other door.
func TestMattermostHoldsEachDoorToItsToken(t *testing.T) {
	m := Mattermost{}
	ctx := context.Background()
	in, err := m.Read(ctx, mattermostCreds, "", port.Delivery{Body: fixture(t, "mattermost_command.txt")})
	if err != nil || in.Asks != port.AsksCapture || in.Room != "abcdefghijklmnopqrstuvwxyz" || in.Text != "Renew the certificate" {
		t.Fatalf("command = %+v %v", in, err)
	}
	in, err = m.Read(ctx, mattermostCreds, "", port.Delivery{Body: fixture(t, "mattermost_webhook.json")})
	if err != nil || in.Asks != port.AsksUnfurl || !strings.Contains(in.Text, "/items/0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a51") {
		t.Fatalf("webhook = %+v %v", in, err)
	}
	answer, _ := m.Answer(ctx, mattermostCreds, in, []port.Card{{Headline: "Entry", Title: "@channel [x](y)", Link: card.Link}})
	var doc map[string]string
	if json.Unmarshal(answer.Body, &doc) != nil || doc["response_type"] != "comment" || strings.Contains(doc["text"], "@channel") || !strings.Contains(doc["text"], `\[x\]\(y\)`) {
		t.Fatalf("answer = %s", answer.Body)
	}
	swapped := port.Credentials{CommandToken: mattermostCreds.WebhookToken, WebhookToken: mattermostCreds.CommandToken}
	for _, name := range []string{"mattermost_command.txt", "mattermost_webhook.json"} {
		if _, err := m.Read(ctx, swapped, "", port.Delivery{Body: fixture(t, name)}); detail(err) != "chatbot.delivery_refused" {
			t.Errorf("%s with the other door's token: %v", name, err)
		}
	}
	if _, err := m.Read(ctx, port.Credentials{}, "", port.Delivery{Body: []byte("command=%2Ftask&token=")}); detail(err) != "chatbot.delivery_refused" {
		t.Errorf("an empty token opened a door: %v", err)
	}
	if answer, _ := m.Answer(ctx, mattermostCreds, in, nil); len(answer.Body) != 0 {
		t.Errorf("nothing to say answered %s", answer.Body)
	}
}

const (
	teamsApp    = "00000000-0000-7000-8000-0000000000bb"
	teamsTenant = "00000000-0000-7000-8000-0000000000aa"
)

var teamsCreds = port.Credentials{AppID: teamsApp, AppTenant: teamsTenant, AppPassword: secret.New("pw")}

// botFrameworkToken signs a token as the Bot Framework would.
func botFrameworkToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	signing := enc(map[string]string{"alg": "RS256", "typ": "JWT", "kid": "k1"}) + "." + enc(claims)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func teamsClaims(audience, service string) map[string]any {
	return map[string]any{
		"iss": BotFrameworkIssuer, "aud": audience, "serviceurl": service,
		"nbf": arrived.Add(-time.Minute).Unix(), "exp": arrived.Add(time.Hour).Unix(),
	}
}

// UC-INT-19 checks 2, 3 and 4: a Teams activity holds only with the Bot Framework's token for the
// bot's application naming the activity's service address; a link is unfurled in the invoke's
// response, a command answered through the Bot Connector with the bot's own token.
func TestTeamsHoldsToTheBotFrameworksTokenAndReplies(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	fake := &recorder{answers: []httpport.Response{
		{Status: 200, Body: []byte(`{"token_type":"Bearer","expires_in":3599,"ext_expires_in":3599,"access_token":"bot-token"}`)},
		{Status: 200, Body: []byte(`{"token_type":"Bearer","expires_in":3599,"ext_expires_in":3599,"access_token":"bot-token"}`)},
		{Status: 201, Body: []byte(`{"id":"1730289600001"}`)},
		{Status: 400, Body: []byte(`{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided."}`)},
	}}
	teams := Teams{Client: fake, Keys: &gooidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}}
	ctx := context.Background()
	if who, err := teams.Check(ctx, teamsCreds); err != nil || who != teamsApp {
		t.Fatalf("check = %q %v", who, err)
	}
	form, _ := url.ParseQuery(string(fake.requests[0].Body))
	if fake.requests[0].URL != MicrosoftLogin+"/"+teamsTenant+"/oauth2/v2.0/token" || form.Get("client_id") != teamsApp || form.Get("scope") != botFrameworkScope {
		t.Fatalf("token asked %s %v", fake.requests[0].URL, form)
	}
	service := "https://smba.trafficmanager.net/emea/"
	delivery := func(name string, token string) port.Delivery {
		return port.Delivery{Body: fixture(t, name), Now: arrived, Header: map[string][]string{"Authorization": {"Bearer " + token}}}
	}
	good := botFrameworkToken(t, key, teamsClaims(teamsApp, service))

	in, err := teams.Read(ctx, teamsCreds, teamsApp, delivery("teams_query_link.json", good))
	if err != nil || in.Asks != port.AsksUnfurl || in.Room != "19:ops@thread.tacv2;messageid=1730289600000" || len(in.Links) != 1 {
		t.Fatalf("queryLink = %+v %v", in, err)
	}
	answer, _ := teams.Answer(ctx, teamsCreds, in, []port.Card{card})
	if !strings.Contains(string(answer.Body), `"composeExtension"`) || !strings.Contains(string(answer.Body), `"preview"`) || len(fake.requests) != 1 {
		t.Fatalf("queryLink answer = %s", answer.Body)
	}

	in, err = teams.Read(ctx, teamsCreds, teamsApp, delivery("teams_message.json", good))
	if err != nil || in.Asks != port.AsksCapture || in.Text != "Renew the certificate" {
		t.Fatalf("message = %+v %v", in, err)
	}
	if _, err := teams.Answer(ctx, teamsCreds, in, []port.Card{card}); err != nil {
		t.Fatal(err)
	}
	reply := fake.requests[2]
	if reply.URL != "https://smba.trafficmanager.net/emea/v3/conversations/19:ops@thread.tacv2%3Bmessageid=1730289600000/activities/1730289600000" ||
		reply.Header["Authorization"][0] != "Bearer bot-token" {
		t.Fatalf("reply = %s %v", reply.URL, reply.Header)
	}
	if _, err := teams.Answer(ctx, teamsCreds, in, []port.Card{card}); detail(err) != "chatbot.token_revoked" {
		t.Fatalf("a refused secret = %v", err)
	}

	for name, token := range map[string]string{
		"another audience": botFrameworkToken(t, key, teamsClaims("00000000-0000-7000-8000-0000000000cc", service)),
		"another signer":   botFrameworkToken(t, other, teamsClaims(teamsApp, service)),
		"another service":  botFrameworkToken(t, key, teamsClaims(teamsApp, "https://smba.trafficmanager.net/amer/")),
		"another issuer":   botFrameworkToken(t, key, map[string]any{"iss": "https://evil.example", "aud": teamsApp, "serviceurl": service, "exp": arrived.Add(time.Hour).Unix()}),
		"an expired token": botFrameworkToken(t, key, map[string]any{"iss": BotFrameworkIssuer, "aud": teamsApp, "serviceurl": service, "exp": arrived.Add(-time.Minute).Unix()}),
		"no token":         "",
	} {
		if _, err := teams.Read(ctx, teamsCreds, teamsApp, delivery("teams_message.json", token)); detail(err) != "chatbot.delivery_refused" {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A token naming a service address outside the Bot Framework is refused though it holds.
	outside := strings.Replace(string(fixture(t, "teams_message.json")), service, "https://evil.example/", 1)
	d := port.Delivery{Body: []byte(outside), Now: arrived, Header: map[string][]string{"Authorization": {"Bearer " + botFrameworkToken(t, key, teamsClaims(teamsApp, "https://evil.example/"))}}}
	if _, err := teams.Read(ctx, teamsCreds, teamsApp, d); detail(err) != "chatbot.delivery_refused" {
		t.Errorf("a service address outside the Bot Framework: %v", err)
	}
}
