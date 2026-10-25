// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	voiceservice "github.com/Jersyfi/hubtask/core/application/service/voice"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
)

const skill = "amzn1.ask.skill.00000000-0000-0000-0000-000000000001"

var now = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

type verifier struct{ err error }

func (v verifier) Verify(context.Context, http.Header, []byte) error { return v.err }

type renderer struct{}

func (renderer) Render(locale, code string, params map[string]string) string {
	parts := []string{locale, code}
	for _, key := range []string{"things", "list"} {
		if params[key] != "" {
			parts = append(parts, params[key])
		}
	}
	return strings.Join(parts, "|")
}

type assistant struct {
	settings domain.Settings
	added    []string
}

func (a *assistant) Enabled(context.Context, appshared.ActorContext) (domain.Settings, error) {
	return a.settings, nil
}
func (a *assistant) Add(_ context.Context, _ appshared.ActorContext, things, target string, _ domain.Language) (voiceservice.Reply, error) {
	a.added = append(a.added, things+"@"+target)
	return voiceservice.Reply{Code: "voice.added", Params: map[string]string{"things": things, "list": target}}, nil
}
func (a *assistant) Read(context.Context, appshared.ActorContext, string, domain.Language) (voiceservice.Reply, error) {
	return voiceservice.Reply{Code: "voice.list_reads"}, nil
}
func (a *assistant) Complete(context.Context, appshared.ActorContext, string, string, domain.Language) (voiceservice.Reply, error) {
	return voiceservice.Reply{Code: "voice.completed"}, nil
}
func (a *assistant) Today(context.Context, appshared.ActorContext, domain.Language) (voiceservice.Reply, error) {
	return voiceservice.Reply{}, shared.ErrForbidden
}

func alexa(settings domain.Settings) (Alexa, *assistant) {
	helper := &assistant{settings: settings}
	return Alexa{
		Verifier: verifier{},
		Authenticate: func(_ context.Context, token, _ string) (appshared.ActorContext, error) {
			if token != "good" {
				return appshared.ActorContext{}, shared.ErrUnauthenticated
			}
			return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: "t1", AccountID: "a1"}, nil
		},
		Assistant: helper, Renderer: renderer{}, Now: func() time.Time { return now },
	}, helper
}

func body(t *testing.T, token, skillID, kind, intent string, slots map[string]string, at time.Time) []byte {
	t.Helper()
	slotDoc := map[string]any{}
	for name, value := range slots {
		slotDoc[name] = map[string]any{"name": name, "value": value}
	}
	doc := map[string]any{
		"version": "1.0",
		"context": map[string]any{"System": map[string]any{
			"application": map[string]any{"applicationId": skillID},
			"user":        map[string]any{"accessToken": token},
		}},
		"request": map[string]any{
			"type": kind, "timestamp": at.Format(time.RFC3339), "locale": "de-DE",
			"intent": map[string]any{"name": intent, "slots": slotDoc},
		},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestASpokenThingIsAddedAndSaidBack(t *testing.T) {
	handler, helper := alexa(domain.Settings{Enabled: true, SkillID: skill})
	answer, err := handler.Handle(context.Background(), http.Header{},
		body(t, "good", skill, "IntentRequest", voiceservice.IntentAdd, map[string]string{"Thing": "Butter", "List": "Einkaufsliste"}, now))
	if err != nil {
		t.Fatal(err)
	}
	if len(helper.added) != 1 || helper.added[0] != "Butter@Einkaufsliste" {
		t.Errorf("added %v", helper.added)
	}
	if answer.Response.OutputSpeech.Text != "de-DE|voice.added|Butter|Einkaufsliste" || !answer.Response.ShouldEndSession {
		t.Errorf("answered %+v", answer.Response)
	}
	sentence, _ := handler.Handle(context.Background(), http.Header{},
		body(t, "good", skill, "IntentRequest", voiceservice.IntentAddSentence, map[string]string{"Sentence": "Butter auf Einkauf"}, now))
	if helper.added[1] != "Butter auf Einkauf@" || sentence.Response.OutputSpeech == nil {
		t.Errorf("the sentence intent added %v", helper.added)
	}
}

func TestWhatIsNotAlexaOrNotOursIsRefused(t *testing.T) {
	handler, _ := alexa(domain.Settings{Enabled: true, SkillID: skill})
	if _, err := (Alexa{Verifier: verifier{err: shared.ErrUnauthenticated}}).Handle(context.Background(), http.Header{}, []byte(`{}`)); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("an unsigned request answered %v", err)
	}
	if _, err := handler.Handle(context.Background(), http.Header{},
		body(t, "good", skill, "LaunchRequest", "", nil, now.Add(-3*time.Minute))); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("an old request answered %v", err)
	}
	if _, err := handler.Handle(context.Background(), http.Header{},
		body(t, "good", "amzn1.ask.skill.other", "LaunchRequest", "", nil, now)); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("another skill answered %v", err)
	}
}

func TestAnUnlinkedOrSwitchedOffWorkspaceSaysSo(t *testing.T) {
	handler, _ := alexa(domain.Settings{Enabled: true, SkillID: skill})
	unlinked, err := handler.Handle(context.Background(), http.Header{}, body(t, "", skill, "LaunchRequest", "", nil, now))
	if err != nil || unlinked.Response.Card == nil || unlinked.Response.Card.Type != "LinkAccount" {
		t.Errorf("unlinked answered %+v, %v", unlinked.Response, err)
	}
	stale, _ := handler.Handle(context.Background(), http.Header{}, body(t, "revoked", skill, "LaunchRequest", "", nil, now))
	if stale.Response.Card == nil {
		t.Errorf("a revoked token answered %+v", stale.Response)
	}
	off, _ := alexa(domain.Settings{Enabled: false, SkillID: skill})
	answer, _ := off.Handle(context.Background(), http.Header{}, body(t, "good", skill, "LaunchRequest", "", nil, now))
	if answer.Response.OutputSpeech.Text != "de-DE|voice.disabled" {
		t.Errorf("switched off answered %+v", answer.Response.OutputSpeech)
	}
}

func TestTheInstallationsSkillAndTheConversation(t *testing.T) {
	handler, _ := alexa(domain.Settings{Enabled: true})
	handler.SkillIDs = []string{skill}
	welcome, err := handler.Handle(context.Background(), http.Header{}, body(t, "good", skill, "LaunchRequest", "", nil, now))
	if err != nil || welcome.Response.ShouldEndSession {
		t.Errorf("the welcome closed the session: %+v, %v", welcome.Response, err)
	}
	refused, _ := handler.Handle(context.Background(), http.Header{}, body(t, "good", skill, "IntentRequest", voiceservice.IntentToday, nil, now))
	if refused.Response.OutputSpeech.Text != "de-DE|voice.not_allowed_here" {
		t.Errorf("a refusal answered %+v", refused.Response.OutputSpeech)
	}
	ended, _ := handler.Handle(context.Background(), http.Header{}, body(t, "good", skill, "SessionEndedRequest", "", nil, now))
	if ended.Response.OutputSpeech != nil || !ended.Response.ShouldEndSession {
		t.Errorf("the end answered %+v", ended.Response)
	}
}
