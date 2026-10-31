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
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"

	port "github.com/Jersyfi/hubtask/core/port/chatbot"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
)

// The Bot Framework's fixed addresses: the issuer of the token a channel sends a bot, where its
// signing keys are published, and where a bot's own token is drawn.
const (
	BotFrameworkIssuer = "https://api.botframework.com"
	BotFrameworkKeys   = "https://login.botframework.com/v1/.well-known/keys"
	MicrosoftLogin     = "https://login.microsoftonline.com"
	botFrameworkScope  = "https://api.botframework.com/.default"
)

// teamsHosts are the hosts a Teams activity's service address may name; any other is refused
// even with a valid token, so a reply never goes elsewhere.
var teamsHosts = []string{".trafficmanager.net", ".botframework.com", ".teams.microsoft.com"}

// Teams is a Microsoft Teams bot posting to a bot's intake through the Bot Framework (UC-INT-19):
// a link pasted into a message unfurled by the messaging extension's `composeExtension/queryLink`
// in the response, and `task <title>` said to the bot answered as a reply through the Bot
// Connector. Every activity carries the Bot Framework's token for the bot's application.
type Teams struct {
	Client httpport.Port
	// Keys are the Bot Framework's signing keys.
	Keys gooidc.KeySet
	// Login is Microsoft's identity platform; MicrosoftLogin when empty.
	Login string
}

var _ port.Platform = Teams{}

func (t Teams) login() string {
	if t.Login != "" {
		return strings.TrimSuffix(t.Login, "/")
	}
	return MicrosoftLogin
}

// token draws the bot's own token from its Entra tenant, by its client secret.
func (t Teams) token(ctx context.Context, creds port.Credentials) (string, error) {
	if t.Client == nil {
		return "", unreachable(errors.New("chat bot: no outbound client is wired"))
	}
	form := url.Values{
		"grant_type": {"client_credentials"}, "client_id": {creds.AppID},
		"client_secret": {creds.AppPassword.Reveal()}, "scope": {botFrameworkScope},
	}
	response, err := t.Client.Do(ctx, httpport.Request{
		Method: http.MethodPost, URL: t.login() + "/" + url.PathEscape(creds.AppTenant) + "/oauth2/v2.0/token",
		Header: map[string][]string{"Content-Type": {"application/x-www-form-urlencoded"}, "Accept": {"application/json"}},
		Body:   []byte(form.Encode()), TargetClass: TargetClass,
	})
	if err != nil {
		return "", unreachable(err)
	}
	if response.Status == http.StatusBadRequest || response.Status == http.StatusUnauthorized {
		return "", revoked("Entra refused the bot's secret")
	}
	if response.Status/100 != 2 {
		return "", unreachable(errors.New("chat bot: Entra answered " + http.StatusText(response.Status)))
	}
	var answer struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(response.Body, &answer) != nil || answer.AccessToken == "" {
		return "", unreachable(errors.New("chat bot: Entra answered no token"))
	}
	return answer.AccessToken, nil
}

// Check draws a token: the application, its tenant and its secret belong together. The bot's own
// identifier is its application's.
func (t Teams) Check(ctx context.Context, creds port.Credentials) (string, error) {
	if _, err := t.token(ctx, creds); err != nil {
		return "", err
	}
	return creds.AppID, nil
}

type teamsActivity struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	ID         string `json:"id"`
	ServiceURL string `json:"serviceUrl"`
	Text       string `json:"text"`
	From       struct {
		ID string `json:"id"`
	} `json:"from"`
	Conversation struct {
		ID string `json:"id"`
	} `json:"conversation"`
	Value struct {
		URL string `json:"url"`
	} `json:"value"`
}

var (
	mention = regexp.MustCompile(`(?is)<at>.*?</at>`)
	markup  = regexp.MustCompile(`<[^>]*>`)
)

// said strips the bot's mention and markup from a message: Teams sends HTML.
func said(text string) string {
	text = mention.ReplaceAllString(text, " ")
	text = markup.ReplaceAllString(text, " ")
	return strings.TrimSpace(strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(text))
}

func allowedService(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range teamsHosts {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func sameService(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(a, "/"), strings.TrimSuffix(b, "/"))
}

// Read verifies the Bot Framework's token - its issuer, its audience the bot's application, its
// signature by a published key, its moment - and that it names the activity's service address.
func (t Teams) Read(ctx context.Context, creds port.Credentials, botUser string, d port.Delivery) (port.Inbound, error) {
	raw, ok := strings.CutPrefix(header(d.Header, "Authorization"), "Bearer ")
	if !ok || t.Keys == nil || creds.AppID == "" {
		return port.Inbound{}, refused("no token")
	}
	verifier := gooidc.NewVerifier(BotFrameworkIssuer, t.Keys, &gooidc.Config{ClientID: creds.AppID, Now: func() time.Time { return d.Now }})
	token, err := verifier.Verify(ctx, raw)
	if err != nil {
		return port.Inbound{}, refused("a token that does not hold")
	}
	var claims struct {
		ServiceURL string `json:"serviceurl"`
	}
	_ = token.Claims(&claims)
	var a teamsActivity
	if json.Unmarshal(d.Body, &a) != nil {
		return port.Inbound{}, refused("no JSON")
	}
	if !allowedService(a.ServiceURL) || !sameService(a.ServiceURL, claims.ServiceURL) {
		return port.Inbound{}, refused("a service address the token does not name")
	}
	reply := map[string]string{"service": a.ServiceURL, "conversation": a.Conversation.ID, "activity": a.ID}
	switch {
	case a.Type == "invoke" && a.Name == "composeExtension/queryLink" && a.Value.URL != "":
		reply["kind"] = "invoke"
		return port.Inbound{Asks: port.AsksUnfurl, Room: a.Conversation.ID, Links: []string{a.Value.URL}, Reply: reply}, nil
	case a.Type == "message" && a.From.ID != "28:"+botUser:
		reply["kind"] = "message"
		text := said(a.Text)
		for _, command := range []string{"!task", "task"} {
			if len(text) > len(command) && strings.EqualFold(text[:len(command)], command) && (text[len(command)] == ' ' || text[len(command)] == '\t') {
				return port.Inbound{Asks: port.AsksCapture, Room: a.Conversation.ID, Text: text[len(command)+1:], Reply: reply}, nil
			}
		}
		return port.Inbound{Asks: port.AsksUnfurl, Room: a.Conversation.ID, Text: text, Reply: reply}, nil
	}
	return port.Inbound{}, nil
}

// teamsCard is a thumbnail card: Teams draws it the same in a message and as a link's preview.
func teamsCard(c port.Card) map[string]any {
	content := map[string]any{"title": c.Title, "subtitle": c.Headline}
	if c.Title == "" {
		content["title"] = c.Headline
		delete(content, "subtitle")
	}
	if c.Link != "" {
		content["tap"] = map[string]any{"type": "openUrl", "value": c.Link}
		content["buttons"] = []map[string]any{{"type": "openUrl", "title": c.Headline, "value": c.Link}}
	}
	return map[string]any{"contentType": "application/vnd.microsoft.card.thumbnail", "content": content}
}

// Answer unfurls in the invoke's response, and replies to a message through the Bot Connector at
// the activity's service address, with the bot's own token.
func (t Teams) Answer(ctx context.Context, creds port.Credentials, in port.Inbound, cards []port.Card) (port.Response, error) {
	if in.Reply["kind"] == "invoke" {
		attachments := []map[string]any{}
		for _, c := range cards {
			card := teamsCard(c)
			card["preview"] = teamsCard(c)
			attachments = append(attachments, card)
		}
		raw, _ := json.Marshal(map[string]any{"composeExtension": map[string]any{
			"type": "result", "attachmentLayout": "list", "attachments": attachments,
		}})
		return port.Response{Body: raw}, nil
	}
	if in.Reply["kind"] != "message" || len(cards) == 0 {
		return port.Response{}, nil
	}
	if !allowedService(in.Reply["service"]) {
		return port.Response{}, refused("a service address outside the Bot Framework")
	}
	bearer, err := t.token(ctx, creds)
	if err != nil {
		return port.Response{}, err
	}
	attachments := []map[string]any{}
	for _, c := range cards {
		attachments = append(attachments, teamsCard(c))
	}
	raw, _ := json.Marshal(map[string]any{
		"type": "message", "replyToId": in.Reply["activity"], "attachmentLayout": "list", "attachments": attachments,
	})
	address := strings.TrimSuffix(in.Reply["service"], "/") + "/v3/conversations/" + url.PathEscape(in.Reply["conversation"]) +
		"/activities/" + url.PathEscape(in.Reply["activity"])
	response, err := t.Client.Do(ctx, httpport.Request{
		Method: http.MethodPost, URL: address, Body: raw, TargetClass: TargetClass,
		Header: map[string][]string{"Authorization": {"Bearer " + bearer}, "Content-Type": {"application/json"}},
	})
	switch {
	case err != nil:
		return port.Response{}, unreachable(err)
	case response.Status == http.StatusUnauthorized || response.Status == http.StatusForbidden:
		return port.Response{}, revoked("the Bot Connector refused the bot's token")
	case response.Status/100 != 2:
		return port.Response{}, unreachable(errors.New("chat bot: the Bot Connector answered " + http.StatusText(response.Status)))
	}
	return port.Response{}, nil
}
