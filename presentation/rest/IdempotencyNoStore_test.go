// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"testing"
)

// An answer that says it must not be kept is not kept for a replay either: a start password, a
// token or any other credential shown once would otherwise sit in clear in the idempotency record.
// The key is still completed with the status, so a repeat runs nothing twice - it answers the status
// with no body.
func TestAnAnswerMarkedNoStoreKeepsNoBody(t *testing.T) {
	g := &guard{}
	secret := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"start_password":"shown-once"}`))
	}

	response := serveIdempotent(t, g, idempotentRequest(t, http.MethodPost, `{"name":"a"}`, true), secret)

	if response.Code != http.StatusCreated || response.Body.String() != `{"start_password":"shown-once"}` {
		t.Fatalf("the client got %d %q", response.Code, response.Body.String())
	}
	if !g.completed || g.gotStatus != http.StatusCreated {
		t.Errorf("the key was completed %v with %d, want the status kept", g.completed, g.gotStatus)
	}
	if len(g.gotBody) != 0 {
		t.Errorf("the record keeps %q", g.gotBody)
	}
}

// Any directive list that holds no-store counts, whatever else it says.
func TestNoStoreIsReadAmongOtherDirectives(t *testing.T) {
	g := &guard{}
	secret := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "private, No-Store")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token":"shown-once"}`))
	}

	serveIdempotent(t, g, idempotentRequest(t, http.MethodPost, `{}`, true), secret)

	if len(g.gotBody) != 0 {
		t.Errorf("the record keeps %q", g.gotBody)
	}
}
