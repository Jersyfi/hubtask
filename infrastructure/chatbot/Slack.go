// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/chatbot"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
)

// SlackAPI is Slack's Web API.
const SlackAPI = "https://slack.com/api"

// slackSkew is how far a delivery's timestamp may be off, as Slack itself advises: a replayed
// delivery older than that is refused even with its signature.
const slackSkew = 5 * time.Minute

// Slack is a Slack app posting to a bot's intake (UC-INT-19): its Events API's `link_shared`
// unfurled through `chat.unfurl`, its slash command answered in the intake's response. Every
// delivery is signed with the app's signing secret over its timestamp and body.
type Slack struct {
	Client httpport.Port
	// API is Slack's Web API; SlackAPI when empty.
	API string
}

var _ port.Platform = Slack{}

func (s Slack) api() string {
	if s.API != "" {
		return strings.TrimSuffix(s.API, "/")
	}
	return SlackAPI
}

func (s Slack) call(ctx context.Context, method string, creds port.Credentials, body any, into any) error {
	if s.Client == nil {
		return unreachable(errors.New("chat bot: no outbound client is wired"))
	}
	raw, _ := json.Marshal(body)
	response, err := s.Client.Do(ctx, httpport.Request{
		Method: http.MethodPost, URL: s.api() + "/" + method, Body: raw, TargetClass: TargetClass,
		Header: map[string][]string{
			"Authorization": {"Bearer " + creds.Token.Reveal()}, "Content-Type": {"application/json; charset=utf-8"},
		},
	})
	if err != nil {
		return unreachable(err)
	}
	if response.Status/100 != 2 {
		return unreachable(errors.New("chat bot: Slack answered " + http.StatusText(response.Status)))
	}
	var answer struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if json.Unmarshal(response.Body, &answer) != nil {
		return unreachable(errors.New("chat bot: Slack answered no JSON"))
	}
	if !answer.OK {
		switch answer.Error {
		case "invalid_auth", "not_authed", "token_revoked", "token_expired", "account_inactive", "missing_scope", "not_allowed_token_type":
			return revoked("Slack answered " + answer.Error)
		}
		return unreachable(errors.New("chat bot: Slack answered " + answer.Error))
	}
	if into != nil {
		_ = json.Unmarshal(response.Body, into)
	}
	return nil
}

// Check asks `auth.test` whom the bot token is: the bot user's identifier.
func (s Slack) Check(ctx context.Context, creds port.Credentials) (string, error) {
	var who struct {
		UserID string `json:"user_id"`
	}
	if err := s.call(ctx, "auth.test", creds, map[string]any{}, &who); err != nil {
		return "", err
	}
	if who.UserID == "" {
		return "", unreachable(errors.New("chat bot: Slack named no bot user"))
	}
	return who.UserID, nil
}

// verify holds a delivery to Slack's signature: `v0=` and the HMAC-SHA256 of `v0:<ts>:<body>`.
func verify(creds port.Credentials, d port.Delivery) error {
	ts := header(d.Header, "X-Slack-Request-Timestamp")
	at, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return refused("no timestamp")
	}
	if skew := d.Now.Sub(time.Unix(at, 0)); skew > slackSkew || skew < -slackSkew {
		return refused("a stale timestamp")
	}
	given := header(d.Header, "X-Slack-Signature")
	if creds.SigningSecret.IsEmpty() || !strings.HasPrefix(given, "v0=") {
		return refused("no signature")
	}
	mac := hmac.New(sha256.New, []byte(creds.SigningSecret.Reveal()))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(d.Body)
	want := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(given), []byte(want)) {
		return refused("a wrong signature")
	}
	return nil
}

type slackEvent struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Event     struct {
		Type      string `json:"type"`
		Channel   string `json:"channel"`
		User      string `json:"user"`
		MessageTS string `json:"message_ts"`
		UnfurlID  string `json:"unfurl_id"`
		Source    string `json:"source"`
		Links     []struct {
			URL string `json:"url"`
		} `json:"links"`
	} `json:"event"`
}

// Read verifies the signature, then reads an Events API callback or a slash command.
func (s Slack) Read(_ context.Context, creds port.Credentials, botUser string, d port.Delivery) (port.Inbound, error) {
	if err := verify(creds, d); err != nil {
		return port.Inbound{}, err
	}
	if strings.HasPrefix(strings.TrimSpace(string(d.Body)), "{") {
		var doc slackEvent
		if json.Unmarshal(d.Body, &doc) != nil {
			return port.Inbound{}, refused("no JSON")
		}
		switch {
		case doc.Type == "url_verification":
			return port.Inbound{Asks: port.AsksChallenge, Challenge: doc.Challenge}, nil
		case doc.Type == "event_callback" && doc.Event.Type == "link_shared" && doc.Event.User != botUser:
			in := port.Inbound{Asks: port.AsksUnfurl, Room: doc.Event.Channel, Reply: map[string]string{
				"channel": doc.Event.Channel, "ts": doc.Event.MessageTS, "unfurl_id": doc.Event.UnfurlID, "source": doc.Event.Source,
			}}
			for _, l := range doc.Event.Links {
				in.Links = append(in.Links, l.URL)
			}
			return in, nil
		}
		return port.Inbound{}, nil
	}
	form, err := fields(d.Body)
	if err != nil {
		return port.Inbound{}, refused("no form")
	}
	if form["command"] == "" || form["user_id"] == botUser {
		return port.Inbound{}, nil
	}
	return port.Inbound{Asks: port.AsksCapture, Room: form["channel_id"], Text: form["text"], Reply: map[string]string{"kind": "command"}}, nil
}

// slackText escapes what Slack's mrkdwn reads as markup: `&`, `<` and `>`, so a title can neither
// link nor call a channel (`<!channel>`).
func slackText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\n", " ").Replace(s)
}

func slackBlocks(card port.Card) []map[string]any {
	text := "*" + slackText(card.Headline) + "*"
	switch {
	case card.Link != "" && card.Title != "":
		text += "\n<" + card.Link + "|" + slackText(card.Title) + ">"
	case card.Link != "":
		text += "\n<" + card.Link + ">"
	case card.Title != "":
		text += "\n" + slackText(card.Title)
	}
	return []map[string]any{{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": text}}}
}

// Answer answers a challenge with it, a command in the response, and unfurls through
// `chat.unfurl` - the event itself is acknowledged with an empty body.
func (s Slack) Answer(ctx context.Context, creds port.Credentials, in port.Inbound, cards []port.Card) (port.Response, error) {
	switch {
	case in.Asks == port.AsksChallenge:
		raw, _ := json.Marshal(map[string]string{"challenge": in.Challenge})
		return port.Response{Body: raw}, nil
	case in.Asks == port.AsksCapture && len(cards) > 0:
		blocks := []map[string]any{}
		fallback := []string{}
		for _, c := range cards {
			blocks = append(blocks, slackBlocks(c)...)
			fallback = append(fallback, slackText(c.Headline))
		}
		raw, _ := json.Marshal(map[string]any{"response_type": "in_channel", "text": strings.Join(fallback, " "), "blocks": blocks})
		return port.Response{Body: raw}, nil
	case in.Asks == port.AsksUnfurl && len(cards) > 0:
		unfurls := map[string]any{}
		for _, c := range cards {
			if c.For != "" {
				unfurls[c.For] = map[string]any{"blocks": slackBlocks(c)}
			}
		}
		if len(unfurls) == 0 {
			return port.Response{}, nil
		}
		body := map[string]any{"unfurls": unfurls}
		if in.Reply["unfurl_id"] != "" && in.Reply["source"] != "" {
			body["unfurl_id"], body["source"] = in.Reply["unfurl_id"], in.Reply["source"]
		} else {
			body["channel"], body["ts"] = in.Reply["channel"], in.Reply["ts"]
		}
		return port.Response{}, s.call(ctx, "chat.unfurl", creds, body, nil)
	}
	return port.Response{}, nil
}
