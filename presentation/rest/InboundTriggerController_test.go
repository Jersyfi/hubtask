// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The public inbound route (G-08). Three things are decided out here and are therefore tested out
// here: what a malformed token gets, what the payload bound refuses, and what shape a body has to
// have before it can become `payload`.

type inboundStarter struct {
	result   automation.TriggerRuleResult
	err      error
	received automation.InboundDelivery
	invoked  bool
}

func (s *inboundStarter) Execute(
	_ context.Context, delivery automation.InboundDelivery,
) (automation.TriggerRuleResult, error) {
	s.received, s.invoked = delivery, true
	return s.result, s.err
}

func deliver(
	t *testing.T, starter InboundRunStarter, token, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.InboundRuns = starter

	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var request *http.Request
	if reader == nil {
		request = httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			APIBasePath+"/automation/inbound/"+token, nil)
	} else {
		request = httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			APIBasePath+"/automation/inbound/"+token, reader)
		request.Header.Set("Content-Type", "application/json")
	}

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func inboundToken(t *testing.T) string {
	t.Helper()
	entropy := make([]byte, integration.InboundTokenSecretBytes)
	for i := range entropy {
		entropy[i] = byte(i)
	}
	token, err := integration.NewInboundToken(
		shared.MustParseID("0192f000-0000-7000-8000-00000000000a"), entropy)
	if err != nil {
		t.Fatal(err)
	}
	return token.Secret()
}

func acceptedRun() automation.TriggerRuleResult {
	return automation.TriggerRuleResult{
		RunID:  shared.MustParseID("0192f000-0000-7000-8000-000000000401"),
		RuleID: shared.MustParseID("0192f000-0000-7000-8000-000000000402"),
	}
}

func TestAnInboundDeliveryIsAcceptedWithTheRunItWillProduce(t *testing.T) {
	starter := &inboundStarter{result: acceptedRun()}

	recorder := deliver(t, starter, inboundToken(t), `{"order_id":"A-17"}`)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if starter.received.Payload["order_id"] != "A-17" {
		t.Errorf("the payload did not reach the application layer: %v", starter.received.Payload)
	}
	if starter.received.Token.TenantID().String() != "0192f000-0000-7000-8000-00000000000a" {
		t.Errorf("the token's tenant was lost: %v", starter.received.Token.TenantID())
	}

	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	if answer["run_id"] != acceptedRun().RunID.String() {
		t.Errorf("answered run %v, want the one that will carry the work", answer["run_id"])
	}
}

// The acceptance criterion's size half: a 2 MB payload is refused at the boundary. The route's own
// bound is far below the request limit, because what bounds a transfer and what bounds an
// evaluation are two different numbers - this document becomes a CEL activation.
func TestAPayloadLargerThanAConditionMayReadIsRefused(t *testing.T) {
	starter := &inboundStarter{result: acceptedRun()}
	filling := strings.Repeat("x", 2*1024*1024)

	recorder := deliver(t, starter, inboundToken(t), `{"note":"`+filling+`"}`)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413: %s", recorder.Code, recorder.Body)
	}
	if starter.invoked {
		t.Error("a 2 MB body reached the application layer")
	}

	t.Run("and the bound is the route's own", func(t *testing.T) {
		starter := &inboundStarter{result: acceptedRun()}
		// One byte past the bound, counting the JSON wrapper around it.
		body := `{"note":"` + strings.Repeat("y", integration.MaxInboundPayloadBytes) + `"}`

		recorder := deliver(t, starter, inboundToken(t), body)

		if recorder.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status %d, want 413 for a body past the route's bound", recorder.Code)
		}
	})

	t.Run("and a body inside it is served", func(t *testing.T) {
		starter := &inboundStarter{result: acceptedRun()}
		body := `{"note":"` + strings.Repeat("z", 1024) + `"}`

		recorder := deliver(t, starter, inboundToken(t), body)

		if recorder.Code != http.StatusAccepted {
			t.Errorf("status %d, want an ordinary body accepted: %s", recorder.Code, recorder.Body)
		}
	})
}

// `payload.order_id` has to mean something, so the body has to be an object. A top-level array has
// no names at all, and text is not a document.
func TestTheBodyHasToBeAnObjectOrNothingAtAll(t *testing.T) {
	for name, body := range map[string]string{
		"an array": `[1,2,3]`,
		"a string": `"hello"`,
		"a number": `17`,
		"not json": `{`,
	} {
		t.Run(name, func(t *testing.T) {
			starter := &inboundStarter{result: acceptedRun()}

			recorder := deliver(t, starter, inboundToken(t), body)

			if recorder.Code != http.StatusUnprocessableEntity && recorder.Code != http.StatusBadRequest {
				t.Errorf("status %d, want a refusal: %s", recorder.Code, recorder.Body)
			}
			if starter.invoked {
				t.Error("a body that is not an object reached the application layer")
			}
		})
	}

	t.Run("an empty body", func(t *testing.T) {
		starter := &inboundStarter{result: acceptedRun()}

		recorder := deliver(t, starter, inboundToken(t), "")

		if recorder.Code != http.StatusAccepted {
			t.Fatalf("status %d, want a ping accepted: %s", recorder.Code, recorder.Body)
		}
		if starter.received.Payload == nil {
			t.Error("an empty body reached the application layer as a missing document")
		}
	})
}

// A malformed token answers what an unknown one answers, and the application layer is never asked:
// a route that distinguished them would answer questions for whoever is trying tokens (T-21).
func TestAMalformedInboundTokenAnswersWhatAnUnknownOneDoes(t *testing.T) {
	for name, token := range map[string]string{
		"another prefix": strings.Replace(inboundToken(t), integration.InboundTokenPrefix, "hbt_cal_", 1),
		"nonsense":       "nonsense",
		"a bare prefix":  integration.InboundTokenPrefix,
	} {
		t.Run(name, func(t *testing.T) {
			starter := &inboundStarter{result: acceptedRun()}

			recorder := deliver(t, starter, token, `{}`)

			if recorder.Code != http.StatusNotFound {
				t.Errorf("status %d, want 404: %s", recorder.Code, recorder.Body)
			}
			if starter.invoked {
				t.Error("a malformed token reached the application layer")
			}
		})
	}
}

// An installation built without the starter answers the pending 404 rather than panicking - the
// same shape the calendar feed's route has.
func TestTheInboundRouteWithoutAStarterAnswers404(t *testing.T) {
	controller := NewRestController()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/automation/inbound/"+inboundToken(t), strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()

	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status %d, want the pending 404", recorder.Code)
	}
}
