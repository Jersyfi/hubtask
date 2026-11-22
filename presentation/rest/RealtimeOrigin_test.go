// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/presentation/realtime"
)

// The socket admits exactly the origins the REST layer does: the apps' webviews and the configured
// allowlist; the server's own origin needs no entry; anything else is refused at the handshake.
func TestTheRealtimeSocketAdmitsTheOriginsRESTAdmits(t *testing.T) {
	controller := RealtimeController{Handler: realtime.Handler{
		OriginPatterns: SocketOrigins(env.CORSConfig{AllowedOrigins: []string{"https://portal.example.org"}}),
	}}
	server := httptest.NewServer(http.HandlerFunc(controller.ProbeRealtime))
	defer server.Close()

	cases := []struct {
		origin string
		want   bool
	}{
		{"http://tauri.localhost", true},
		{"tauri://localhost", true},
		{"https://portal.example.org", true},
		{server.URL, true},
		{"https://evil.example.com", false},
		{"https://portal.example.org.evil.example.com", false},
		{"http://portal.example.org", false},
	}
	for _, c := range cases {
		t.Run(c.origin, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			conn, answer, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), //nolint:bodyclose // the library owns the handshake's body
				&websocket.DialOptions{
					Subprotocols: []string{realtime.Subprotocol},
					HTTPHeader:   http.Header{"Origin": []string{c.origin}},
				})
			if conn != nil {
				_ = conn.CloseNow()
			}
			if c.want && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.want && (err == nil || answer == nil || answer.StatusCode != http.StatusForbidden) {
				t.Fatalf("admitted or not a 403: %v", err)
			}
		})
	}
}
