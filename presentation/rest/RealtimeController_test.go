// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Jersyfi/hubtask/presentation/realtime"
)

// The probe switches to a WebSocket without a credential and closes it at once, normally and with
// its own reason, so whoever opened it can tell the server answered and not a proxy in front.
func TestTheRealtimeProbeUpgradesAndClosesAtOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(RealtimeController{}.ProbeRealtime))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"),
		&websocket.DialOptions{Subprotocols: []string{realtime.Subprotocol}})
	if err != nil {
		t.Fatalf("the probe did not upgrade: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()
	if conn.Subprotocol() != realtime.Subprotocol {
		t.Errorf("subprotocol %q, want %q", conn.Subprotocol(), realtime.Subprotocol)
	}
	_, _, err = conn.Read(ctx)
	var closed websocket.CloseError
	if !errors.As(err, &closed) || closed.Code != websocket.StatusNormalClosure || closed.Reason != realtime.ProbeReason {
		t.Errorf("read %v, want a normal closure with the reason %q", err, realtime.ProbeReason)
	}
}
