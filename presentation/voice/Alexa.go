// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package voice speaks Alexa's protocol (ADR-0093): a signed JSON request in, a sentence out. What
// the sentence does is the voice service's; this only checks who is asking and translates.
package voice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	voiceservice "github.com/Jersyfi/hubtask/core/application/service/voice"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
	"github.com/Jersyfi/hubtask/core/port/i18n"
)

// requestTolerance is Amazon's replay bound: a request older or newer than this is refused.
const requestTolerance = 150 * time.Second

// Verifier proves a request came from Alexa.
type Verifier interface {
	Verify(ctx context.Context, header http.Header, body []byte) error
}

// Authenticator turns the linked account's access token into the person acting.
type Authenticator func(ctx context.Context, token, locale string) (appshared.ActorContext, error)

// Assistant is the voice service.
type Assistant interface {
	Enabled(ctx context.Context, actor appshared.ActorContext) (domain.Settings, error)
	Add(ctx context.Context, actor appshared.ActorContext, things, target string, language domain.Language) (voiceservice.Reply, error)
	Read(ctx context.Context, actor appshared.ActorContext, target string, language domain.Language) (voiceservice.Reply, error)
	Complete(ctx context.Context, actor appshared.ActorContext, thing, target string, language domain.Language) (voiceservice.Reply, error)
	Today(ctx context.Context, actor appshared.ActorContext, language domain.Language) (voiceservice.Reply, error)
}

// Alexa answers Alexa's requests.
type Alexa struct {
	Verifier     Verifier
	Authenticate Authenticator
	Assistant    Assistant
	Renderer     i18n.Renderer
	// SkillIDs are the installation's published skills; a workspace's own skill is in its settings.
	SkillIDs []string
	Now      func() time.Time
}

type request struct {
	Version string `json:"version"`
	Session struct {
		Application struct {
			ApplicationID string `json:"applicationId"`
		} `json:"application"`
	} `json:"session"`
	Context struct {
		System struct {
			Application struct {
				ApplicationID string `json:"applicationId"`
			} `json:"application"`
			User struct {
				AccessToken string `json:"accessToken"`
			} `json:"user"`
		} `json:"System"`
	} `json:"context"`
	Request struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Locale    string `json:"locale"`
		Intent    struct {
			Name  string `json:"name"`
			Slots map[string]struct {
				Value string `json:"value"`
			} `json:"slots"`
		} `json:"intent"`
	} `json:"request"`
}

// Response is Alexa's response document.
type Response struct {
	Version  string       `json:"version"`
	Response responseBody `json:"response"`
}

type responseBody struct {
	OutputSpeech     *speech `json:"outputSpeech,omitempty"`
	Card             *card   `json:"card,omitempty"`
	ShouldEndSession bool    `json:"shouldEndSession"`
}

type speech struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type card struct {
	Type string `json:"type"`
}

// Handle answers one request. An error is a request that is not Alexa's or not for this
// installation; everything Alexa sent properly gets a sentence, refusals included.
func (a Alexa) Handle(ctx context.Context, header http.Header, body []byte) (Response, error) {
	if err := a.Verifier.Verify(ctx, header, body); err != nil {
		return Response{}, err
	}
	var in request
	if err := json.Unmarshal(body, &in); err != nil {
		return Response{}, shared.ErrMalformedRequest.WithDetail("voice.request_unreadable")
	}
	at, err := time.Parse(time.RFC3339, in.Request.Timestamp)
	if err != nil || a.now().Sub(at).Abs() > requestTolerance {
		return Response{}, shared.ErrUnauthenticated.WithDetail("voice.not_alexa")
	}
	locale := in.Request.Locale
	language := domain.LanguageOf(locale)
	if in.Request.Type == "SessionEndedRequest" {
		return Response{Version: "1.0", Response: responseBody{ShouldEndSession: true}}, nil
	}

	token := in.Context.System.User.AccessToken
	if token == "" {
		return a.linkAccount(locale), nil
	}
	actor, err := a.Authenticate(ctx, token, locale)
	if err != nil {
		return a.linkAccount(locale), nil
	}
	settings, err := a.Assistant.Enabled(ctx, actor)
	if err != nil {
		return Response{}, err
	}
	skill := in.Context.System.Application.ApplicationID
	if skill == "" {
		skill = in.Session.Application.ApplicationID
	}
	if !slices.Contains(a.SkillIDs, skill) && (settings.SkillID == "" || settings.SkillID != skill) {
		return Response{}, shared.ErrForbidden.WithDetail("voice.skill_unknown")
	}
	if !settings.Enabled {
		return a.say(locale, voiceservice.Reply{Code: "voice.disabled"}), nil
	}

	slot := func(name string) string { return strings.TrimSpace(in.Request.Intent.Slots[name].Value) }
	var answer voiceservice.Reply
	switch {
	case in.Request.Type == "LaunchRequest":
		answer = voiceservice.Reply{Code: "voice.welcome", Open: true}
	case in.Request.Type != "IntentRequest":
		answer = voiceservice.Reply{Code: "voice.not_understood"}
	default:
		switch in.Request.Intent.Name {
		case voiceservice.IntentAdd:
			answer, err = a.Assistant.Add(ctx, actor, slot(voiceservice.SlotThing), slot(voiceservice.SlotList), language)
		case voiceservice.IntentAddSentence:
			answer, err = a.Assistant.Add(ctx, actor, slot(voiceservice.SlotSentence), "", language)
		case voiceservice.IntentRead:
			answer, err = a.Assistant.Read(ctx, actor, slot(voiceservice.SlotList), language)
		case voiceservice.IntentComplete:
			answer, err = a.Assistant.Complete(ctx, actor, slot(voiceservice.SlotThing), slot(voiceservice.SlotList), language)
		case voiceservice.IntentToday:
			answer, err = a.Assistant.Today(ctx, actor, language)
		case "AMAZON.HelpIntent":
			answer = voiceservice.Reply{Code: "voice.help", Open: true}
		case "AMAZON.StopIntent", "AMAZON.CancelIntent", "AMAZON.NavigateHomeIntent":
			answer = voiceservice.Reply{Code: "voice.bye"}
		default:
			answer = voiceservice.Reply{Code: "voice.not_understood", Open: true}
		}
	}
	if err != nil {
		if errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrNotFound) {
			return a.say(locale, voiceservice.Reply{Code: "voice.not_allowed_here"}), nil
		}
		return a.say(locale, voiceservice.Reply{Code: "voice.failed"}), nil
	}
	return a.say(locale, answer), nil
}

func (a Alexa) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a Alexa) say(locale string, answer voiceservice.Reply) Response {
	return Response{Version: "1.0", Response: responseBody{
		OutputSpeech:     &speech{Type: "PlainText", Text: a.Renderer.Render(locale, answer.Code, answer.Params)},
		ShouldEndSession: !answer.Open,
	}}
}

// linkAccount asks the person to link their Hubtask account in the Alexa app, with the card that
// takes them there.
func (a Alexa) linkAccount(locale string) Response {
	out := a.say(locale, voiceservice.Reply{Code: "voice.link_account"})
	out.Response.Card = &card{Type: "LinkAccount"}
	return out
}
