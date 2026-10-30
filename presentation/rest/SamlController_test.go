// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

type keeperDouble struct{ state, answer string }

func (k *keeperDouble) Execute(_ context.Context, state, answer string) (secret.Secret, error) {
	k.state, k.answer = state, answer
	if state != "hbt_osf_live" {
		return secret.Secret{}, shared.ErrNotFound.WithDetail("auth.oidc_flow_not_found")
	}
	return secret.New("hbt_sam_code"), nil
}

// UC-ID-30 check 3: the provider's post is kept and the browser sent on to the callback with the
// handoff and the state; an answer for no live flow is refused and sent nowhere.
func TestTheAssertionConsumerServiceSendsTheBrowserOn(t *testing.T) {
	keeper := &keeperDouble{}
	controller := NewRestController()
	controller.SAML = SAMLDoor{Answers: keeper, CallbackURL: "https://hubtask.example/auth/callback", Metadata: []byte("<md/>")}

	post := func(state string) *httptest.ResponseRecorder {
		form := url.Values{"SAMLResponse": {"PHg+"}, "RelayState": {state}}
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/auth/saml:acs",
			strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		controller.Routes().ServeHTTP(recorder, request)
		return recorder
	}

	answered := post("hbt_osf_live")
	if answered.Code != http.StatusSeeOther {
		t.Fatalf("status %d: %s", answered.Code, answered.Body)
	}
	location, _ := url.Parse(answered.Header().Get("Location"))
	if location.Host != "hubtask.example" || location.Path != "/auth/callback" ||
		location.Query().Get("code") != "hbt_sam_code" || location.Query().Get("state") != "hbt_osf_live" ||
		keeper.answer != "PHg+" {
		t.Fatalf("sent to %s, kept %q", location, keeper.answer)
	}

	refused := post("hbt_osf_spent")
	if refused.Code != http.StatusNotFound || refused.Header().Get("Location") != "" ||
		!strings.Contains(refused.Body.String(), "auth.oidc_flow_not_found") {
		t.Fatalf("status %d: %s", refused.Code, refused.Body)
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/auth/saml:metadata", nil)
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "application/samlmetadata+xml" {
		t.Fatalf("metadata %d %s", recorder.Code, recorder.Header().Get("Content-Type"))
	}
}
