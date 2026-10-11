// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"testing"
)

// An answer marked as shown once is not kept for a replay: a start password, a token or any other
// credential shown once would otherwise sit in clear in the idempotency record. The key is still
// completed with the status, so a repeat runs nothing twice - it answers the status with no body.
// The mark never reaches the client.
func TestAnAnswerShownOnceKeepsNoBody(t *testing.T) {
	g := &guard{}
	secret := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set(ShownOnceHeader, "start_password")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"start_password":"shown-once"}`))
	}

	response := serveIdempotent(t, g, idempotentRequest(t, http.MethodPost, `{"name":"a"}`, true), secret)

	if response.Code != http.StatusCreated || response.Body.String() != `{"start_password":"shown-once"}` {
		t.Fatalf("the client got %d %q", response.Code, response.Body.String())
	}
	if response.Header().Get(ShownOnceHeader) != "" {
		t.Error("the internal mark reached the client")
	}
	if !g.completed || g.gotStatus != http.StatusCreated {
		t.Errorf("the key was completed %v with %d, want the status kept", g.completed, g.gotStatus)
	}
	if len(g.gotBody) != 0 {
		t.Errorf("the record keeps %q", g.gotBody)
	}
}

// Every JSON answer is `no-store` for caches; that alone keeps the replay's body, which is what makes
// a repeated write answer what the first one answered.
func TestAnOrdinaryNoStoreAnswerIsStillKept(t *testing.T) {
	g := &guard{}
	ordinary := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"018f0000-0000-7000-8000-000000000001"}`))
	}

	serveIdempotent(t, g, idempotentRequest(t, http.MethodPost, `{}`, true), ordinary)

	if string(g.gotBody) != `{"id":"018f0000-0000-7000-8000-000000000001"}` {
		t.Errorf("the record keeps %q, want the answer", g.gotBody)
	}
}
