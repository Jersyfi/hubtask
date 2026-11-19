// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webui_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/presentation/webui"
)

// UC-MED-06/7 (ADR-0104, security.md §9): a page of a workspace that entered a document
// server may frame that origin and post a form to it - and to no other -; a page of another
// workspace, an asset, and the same page once the server is removed keep the closed policy.
func TestTheEditorsFrameOpensOnlyForTheWorkspacesServer(t *testing.T) {
	entered := map[string]string{"nordlicht.hub.example": "https://office.nordlicht.example"}
	h := handler(t).WithDocumentServers(func(_ context.Context, host string) (string, error) {
		if host == "broken.hub.example" {
			return "", errors.New("the database is away")
		}
		return entered[host], nil
	})
	at := func(host, path string) string {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		request.Host = host
		h.ServeHTTP(recorder, request)
		return recorder.Header().Get("Content-Security-Policy")
	}
	closed := at("hub.example", "/")
	if strings.Contains(closed, "frame-src") || !strings.Contains(closed, "form-action 'none'") {
		t.Fatalf("the base policy: %s", closed)
	}

	open := at("nordlicht.hub.example", "/items/1")
	if !strings.Contains(open, "frame-src https://office.nordlicht.example") ||
		!strings.Contains(open, "form-action https://office.nordlicht.example;") {
		t.Errorf("the workspace's page: %s", open)
	}
	if strings.Count(open, "office.nordlicht.example") != 2 {
		t.Errorf("the origin stands outside the two directives: %s", open)
	}
	for _, c := range []struct{ host, path string }{
		{"boreal.hub.example", "/"},
		{"nordlicht.hub.example", "/assets/index-CBRxRnMw.js"},
		{"broken.hub.example", "/"},
	} {
		if got := at(c.host, c.path); got != closed {
			t.Errorf("%s%s opened: %s", c.host, c.path, got)
		}
	}
	delete(entered, "nordlicht.hub.example")
	if got := at("nordlicht.hub.example", "/"); got != closed {
		t.Errorf("a removed server still opens: %s", got)
	}

	// Beside the operator's own frame sources, the server joins the one directive.
	both := webui.WithDocumentServer(webui.FramingSources(closed, []string{"https://embed.example"}), "https://office.example")
	if !strings.Contains(both, "frame-src https://office.example https://embed.example") || strings.Count(both, "frame-src") != 1 {
		t.Errorf("with frame sources: %s", both)
	}
}
