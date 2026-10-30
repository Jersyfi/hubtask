// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
	"github.com/Jersyfi/hubtask/presentation/voice"
)

// Voice assistants (ADR-0093). The controller maps documents; who may change the settings and
// what a sentence does are decided inwards of here.

const (
	showAlexaModelUseCase = "ShowAlexaModel"
)

// AlexaAnswerer serves Alexa's endpoint. Not a catalogue entry: it proves its caller with Amazon's
// signature and the linked account's token, not with this API's credentials.
type AlexaAnswerer interface {
	Handle(ctx context.Context, header http.Header, body []byte) (voice.Response, error)
}

// ShowAlexaModel answers GET /voice/alexa/model.
func (c *RestController) ShowAlexaModel(w http.ResponseWriter, r *http.Request, params openapi.ShowAlexaModelParams) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		input := usecase.Input{}
		if params.Locale != nil {
			input["locale"] = string(*params.Locale)
		}
		return c.UseCases.Invoke(r.Context(), showAlexaModelUseCase, actor, input)
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, out["model"])
	})
}

// AnswerAlexa answers POST /voice/alexa.
//
// A request that is not Alexa's, or not for this installation, answers the pending not-found an
// unwired route answers: a forger learns nothing about which check failed.
func (c *RestController) AnswerAlexa(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.Alexa == nil {
		c.pending.AnswerAlexa(w, r)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		c.pending.AnswerAlexa(w, r)
		return
	}
	answer, err := c.Alexa.Handle(r.Context(), r.Header, body)
	if err != nil {
		// The caller learns nothing about which check failed; the operator does. The cause says
		// which - a certificate the egress allowlist kept out, a signature that did not match, a
		// skill this installation does not answer - and never the request's token or body.
		slog.WarnContext(r.Context(), "an Alexa request was refused",
			slog.String("request_id", requestID), slog.String("error", errorWithCause(err)))
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, answer)
}

func voicePlace(place map[string]any) openapi.VoicePlace {
	collection, _ := place["collection_id"].(string)
	name, _ := place["name"].(string)
	value := openapi.VoicePlace{CollectionId: uuidValue(collection), Name: name}
	if label, _ := place["label_id"].(string); label != "" {
		id := uuidValue(label)
		value.LabelId = &id
	}
	return value
}

func voiceSettingsResponse(out usecase.Output) openapi.VoiceSettings {
	settings := openapi.VoiceSettings{Aliases: []openapi.VoiceAlias{}}
	settings.Enabled, _ = out["enabled"].(bool)
	if installation, ok := out["installation_skill"].(bool); ok {
		settings.InstallationSkill = &installation
	}
	if id := out.String("default_collection_id"); id != "" {
		value := uuidValue(id)
		settings.DefaultCollectionId = &value
	}
	if skill := out.String("skill_id"); skill != "" {
		settings.SkillId = &skill
	}
	if name := out.String("invocation_name"); name != "" {
		settings.InvocationName = &name
	}
	if rows, ok := out["aliases"].([]any); ok {
		for _, raw := range rows {
			row, _ := raw.(map[string]any)
			phrase, _ := row["phrase"].(string)
			collection, _ := row["collection_id"].(string)
			alias := openapi.VoiceAlias{Phrase: phrase, CollectionId: uuidValue(collection)}
			if label, _ := row["label_id"].(string); label != "" {
				id := uuidValue(label)
				alias.LabelId = &id
			}
			settings.Aliases = append(settings.Aliases, alias)
		}
	}
	return settings
}

// interpretationResponse maps what a sentence would do.
func interpretationResponse(out usecase.Output) openapi.VoiceInterpretation {
	answer := openapi.VoiceInterpretation{Ambiguous: []openapi.VoicePlace{}}
	answer.Found, _ = out["found"].(bool)
	answer.Default, _ = out["default"].(bool)
	if things, ok := out["things"].([]any); ok {
		for _, raw := range things {
			thing, _ := raw.(map[string]any)
			title, _ := thing["title"].(string)
			amount, _ := thing["amount"].(string)
			entry := struct {
				Amount *string `json:"amount,omitempty"`
				Title  string  `json:"title"`
			}{Title: title}
			if amount != "" {
				entry.Amount = &amount
			}
			answer.Things = append(answer.Things, entry)
		}
	}
	if answer.Things == nil {
		answer.Things = answer.Things[:0]
	}
	if place, ok := out["place"].(map[string]any); ok {
		value := voicePlace(place)
		answer.Place = &value
	}
	if places, ok := out["ambiguous"].([]any); ok {
		for _, raw := range places {
			if place, ok := raw.(map[string]any); ok {
				answer.Ambiguous = append(answer.Ambiguous, voicePlace(place))
			}
		}
	}
	return answer
}

// errorWithCause is an error for the log with the cause it wraps: the refusal's code alone does
// not say which of Alexa's checks failed.
func errorWithCause(err error) string {
	text := err.Error()
	if cause := errors.Unwrap(err); cause != nil && cause.Error() != text {
		text += ": " + cause.Error()
	}
	return text
}
