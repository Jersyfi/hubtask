// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package stream

import (
	"net/http/httptest"
	"testing"

	"github.com/Jersyfi/hubtask/presentation/realtime"
)

// A browser's socket carries its bearer as a subprotocol; it is counted under the same key as the
// same token in a header, so a browser's live documents meet the per-credential cap too.
func TestABrowserSocketIsCountedUnderItsBearer(t *testing.T) {
	header := httptest.NewRequestWithContext(t.Context(), "GET", "/api/v1/stream", nil)
	header.Header.Set("Authorization", "Bearer hbt_pat_abc")
	socket := httptest.NewRequestWithContext(t.Context(), "GET", "/api/v1/realtime/documents", nil)
	socket.Header.Set("Upgrade", "websocket")
	socket.Header.Set("Sec-WebSocket-Protocol", "hubtask.v1, "+realtime.BearerPrefix+"hbt_pat_abc")

	key := Credential(socket)
	if key == "" || key != Credential(header) {
		t.Errorf("the socket's key is %q, the header's %q: want the same, not empty", key, Credential(header))
	}
	plain := httptest.NewRequestWithContext(t.Context(), "GET", "/api/v1/realtime/documents", nil)
	plain.Header.Set("Sec-WebSocket-Protocol", realtime.BearerPrefix+"hbt_pat_abc")
	if got := Credential(plain); got != "" {
		t.Errorf("a request that is no upgrade is keyed %q", got)
	}
}
