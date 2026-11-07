// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/voice"
)

// refusingAlexa refuses every request the way the verifier does when Amazon's certificate cannot
// be fetched - the egress allowlist kept s3.amazonaws.com out.
type refusingAlexa struct{}

func (refusingAlexa) Handle(context.Context, http.Header, []byte) (voice.Response, error) {
	return voice.Response{}, shared.ErrUnauthenticated.WithDetail("voice.not_alexa").
		WithCause(errors.New("certificate unreadable: host s3.amazonaws.com is not allowed"))
}

func TestARefusedAlexaRequestSaysWhyInTheLogAndNothingToTheCaller(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	controller := NewRestController()
	controller.Intake = &IntakeDoor{Alexa: refusingAlexa{}}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/voice/alexa",
		strings.NewReader(`{"version":"1.0"}`))
	recorder := httptest.NewRecorder()
	controller.AnswerAlexa(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("answered %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "s3.amazonaws.com") {
		t.Error("the caller was told which check failed")
	}
	if !strings.Contains(logged.String(), "certificate unreadable: host s3.amazonaws.com is not allowed") {
		t.Errorf("the log does not say why: %s", logged.String())
	}
}

func TestAnErrorWithoutACauseIsLoggedAsItself(t *testing.T) {
	if got := errorWithCause(errors.New("plain")); got != "plain" {
		t.Errorf("got %q", got)
	}
}
