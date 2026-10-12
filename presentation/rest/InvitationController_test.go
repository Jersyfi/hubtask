// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const invitedAccountID = "0192f000-0000-7000-8000-0000000000e9"

func invitedOut(link string) usecase.Output {
	out := usecase.Output{
		"id": invitedAccountID, "kind": "USER", "display_name": "bert", "status": "INVITED",
		"email": "bert@example.org",
	}
	if link != "" {
		out["invitation_link"] = link
	}
	return out
}

// Without mail the invitation's answer carries the link, marked as the one appearance of a
// credential; with mail it carries none, and the answer is an ordinary one a replay may keep.
func TestTheInvitationAnswersItsLinkOnlyWhereItIsHandedOver(t *testing.T) {
	cases := []struct {
		name string
		link string
	}{
		{"mail configured", ""},
		{"no mail", "https://home.example/redeem#token=hbt_inv_x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			registry := &catalogue{out: invitedOut(c.link)}

			recorder := identityRequestWithBody(t, registry, http.MethodPost, "/accounts:invite",
				`{"email":"bert@example.org"}`)

			if recorder.Code != http.StatusCreated {
				t.Fatalf("status %d, want 201: %s", recorder.Code, recorder.Body)
			}
			if registry.name != inviteAccountUseCase {
				t.Errorf("the handler invoked %q", registry.name)
			}
			var body openapi.InvitedAccount
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding: %v", err)
			}
			if body.Id.String() != invitedAccountID || body.Status != "INVITED" {
				t.Errorf("answered %+v, want the invited account", body)
			}
			if c.link == "" {
				if body.InvitationLink != nil {
					t.Errorf("a link %q where the mail carries it", *body.InvitationLink)
				}
				return
			}
			if body.InvitationLink == nil || *body.InvitationLink != c.link {
				t.Errorf("link %v, want %q", body.InvitationLink, c.link)
			}
			if recorder.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control %q, want no-store", recorder.Header().Get("Cache-Control"))
			}
		})
	}
}

// The link is not kept for a replay: a repeat with the same key answers the status and nothing
// else, so the idempotency record never holds the credential in clear.
func TestTheInvitationLinkIsNotKeptForAReplay(t *testing.T) {
	g := &guard{}
	controller := NewRestController()
	controller.UseCases = &catalogue{out: invitedOut("https://home.example/redeem#token=hbt_inv_x")}
	request := idempotentRequest(t, http.MethodPost, `{"email":"bert@example.org"}`, true)
	request.Header.Set("Content-Type", "application/json")

	response := serveIdempotent(t, g, request, inviteRoute(controller))

	if response.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if !g.completed || g.gotStatus != http.StatusCreated {
		t.Errorf("the key was completed %v with %d, want the status kept", g.completed, g.gotStatus)
	}
	if len(g.gotBody) != 0 {
		t.Errorf("the record keeps %q", g.gotBody)
	}
}

// With mail the answer is kept as any other, so a repeat answers what the first one did.
func TestAMailedInvitationIsKeptForAReplay(t *testing.T) {
	g := &guard{}
	controller := NewRestController()
	controller.UseCases = &catalogue{out: invitedOut("")}
	request := idempotentRequest(t, http.MethodPost, `{"email":"bert@example.org"}`, true)
	request.Header.Set("Content-Type", "application/json")

	serveIdempotent(t, g, request, inviteRoute(controller))

	if len(g.gotBody) == 0 {
		t.Error("the mailed invitation's answer was not kept for a replay")
	}
}

// inviteRoute hands a request to the controller's own route for the invitation, whatever path the
// idempotency harness gave it.
func inviteRoute(controller *RestController) http.HandlerFunc {
	routes := controller.Routes()
	return func(w http.ResponseWriter, r *http.Request) {
		invite := r.Clone(r.Context())
		invite.URL.Path = APIBasePath + "/accounts:invite"
		routes.ServeHTTP(w, invite)
	}
}
