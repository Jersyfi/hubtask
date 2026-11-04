// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

func guardedForTest(host string) *http.Client {
	cfg := env.OutboundConfig{Timeout: 2 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 1 << 20}
	return httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg).ForOperatorService(host)).HTTPClient(RealtimeDependency)
}

// Through the guarded client, a public address whose server answers the probe is up; one whose
// front answers the upgrade with a page is down after two attempts in a row, with live editing
// named as what it takes away, and up again after the next success.
func TestTheRealtimeProbeSaysWhetherASocketGetsThroughThePublicAddress(t *testing.T) {
	refusing := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/realtime/probe" {
			http.NotFound(w, r)
			return
		}
		if refusing {
			_, _ = w.Write([]byte("<html>a proxy that drops the upgrade</html>"))
			return
		}
		// What presentation/realtime.Probe answers (its own test pins the reason).
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{realtimeSubprotocol}})
		if err == nil {
			_ = conn.Close(websocket.StatusNormalClosure, realtimeProbeReason)
		}
	}))
	defer server.Close()

	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	probe := NewRealtimeProbe(server.URL+"/", "/api/v1", guardedForTest("127.0.0.1"), func() time.Time { return at })
	ctx := t.Context()

	probe.Attempt(ctx)
	if got := probe.resultLocked(); got.Status != port.StatusOK {
		t.Fatalf("one failed attempt reads %s, want ok until the second", got.Status)
	}
	probe.Attempt(ctx)
	got := probe.resultLocked()
	if got.Status != port.StatusDown || got.ErrorCode != RealtimeRefusedCode || !got.Since.Equal(at) ||
		len(got.Impact) != 1 || got.Impact[0] != LiveEditingFeature {
		t.Fatalf("two failed attempts read %+v, want down for live editing since the first", got)
	}

	refusing = false
	probe.Attempt(ctx)
	if got := probe.resultLocked(); got.Status != port.StatusOK {
		t.Fatalf("a socket that got through reads %+v, want ok", got)
	}
}

func TestTheRealtimeProbeTargetsTheProbeUnderThePublicAddress(t *testing.T) {
	for base, want := range map[string]string{
		"https://tasks.example.org":       "wss://tasks.example.org/api/v1/realtime/probe",
		"http://localhost:8080/":          "ws://localhost:8080/api/v1/realtime/probe",
		"https://example.org/hubtask?x=1": "wss://example.org/hubtask/api/v1/realtime/probe",
		"":                                "",
		"ftp://example.org":               "",
	} {
		if got := realtimeTarget(base, "/api/v1"); got != want {
			t.Errorf("%q targets %q, want %q", base, got, want)
		}
	}
	if got := NewRealtimeProbe("", "/api/v1", nil, time.Now).Check(t.Context()); got.Status != port.StatusDisabled {
		t.Errorf("without a public address the probe reads %s, want disabled", got.Status)
	}
}
