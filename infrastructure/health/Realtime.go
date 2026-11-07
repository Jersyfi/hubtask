// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	port "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// RealtimeDependency names the probe in the report and in hubtask_dependency_up; LiveEditingFeature
// is what a socket that does not get through takes away.
const (
	RealtimeDependency = "realtime_public"
	LiveEditingFeature = "live_editing"
	// RealtimeRefusedCode says the public address does not let a WebSocket through.
	RealtimeRefusedCode = "health.realtime_unreachable"
)

// The probe's pace. A socket is opened through the public address at most every realtimeEvery;
// realtimeTimeout bounds one attempt (rule 7). Down takes realtimeFailures attempts in a row, so
// one lost packet does not page anyone.
const (
	realtimeEvery    = 5 * time.Minute
	realtimeTimeout  = 10 * time.Second
	realtimeFailures = 2
)

// realtimeSubprotocol and realtimeProbeReason are what presentation/realtime answers its probe
// with; repeated here because an outbound adapter does not import an inbound one.
const (
	realtimeSubprotocol = "hubtask.v1"
	realtimeProbeReason = "realtime.probe"
)

// RealtimeProbe opens a WebSocket to the server's own probe through the installation's public
// address - the way a browser comes in, through whatever proxy is in front. A proxy that drops
// the upgrade breaks live editing for everyone while every other check stays green.
//
// Optional: without live editing every other path still works. The socket is opened in the
// background, never inside a report, so neither /meta/health nor the sampler waits on it; until
// its first attempt has ended the probe has nothing to say and reports ok rather than guess.
type RealtimeProbe struct {
	target string
	client *http.Client
	now    func() time.Time

	mu        sync.Mutex
	running   bool
	checkedAt time.Time
	failures  int
	since     time.Time
}

// NewRealtimeProbe probes baseURL's probe path with client, which must be the guarded client
// (rule 6). An empty or unusable baseURL disables the probe: there is no public address to try.
func NewRealtimeProbe(baseURL, apiBasePath string, client *http.Client, now func() time.Time) *RealtimeProbe {
	return &RealtimeProbe{target: realtimeTarget(baseURL, apiBasePath), client: client, now: now}
}

// realtimeTarget is the probe's WebSocket address under the public base URL, or "" when that is
// not an http(s) address.
func realtimeTarget(baseURL, apiBasePath string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" {
		return ""
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return ""
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + apiBasePath + "/realtime/probe"
	parsed.RawQuery, parsed.Fragment = "", ""
	return parsed.String()
}

var _ port.Probe = (*RealtimeProbe)(nil)

func (p *RealtimeProbe) Name() string   { return RealtimeDependency }
func (p *RealtimeProbe) Required() bool { return false }

// Check answers the last attempts' verdict and starts the next attempt when the last is old.
func (p *RealtimeProbe) Check(ctx context.Context) port.Result {
	if p.target == "" {
		return port.Result{Status: port.StatusDisabled}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running && (p.checkedAt.IsZero() || p.now().Sub(p.checkedAt) >= realtimeEvery) {
		p.running = true
		concurrency.Go(context.WithoutCancel(ctx), "health.realtime", func(ctx context.Context) {
			p.Attempt(ctx)
		})
	}
	return p.resultLocked()
}

func (p *RealtimeProbe) resultLocked() port.Result {
	if p.failures < realtimeFailures {
		return port.Result{Status: port.StatusOK}
	}
	return port.Result{
		Status: port.StatusDown, ErrorCode: RealtimeRefusedCode, Since: p.since,
		Impact: []string{LiveEditingFeature},
	}
}

// Attempt opens one socket now and records how it went. Exported for the tests, which cannot wait
// for the background.
func (p *RealtimeProbe) Attempt(ctx context.Context) {
	err := p.dial(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running, p.checkedAt = false, p.now()
	if err == nil {
		p.failures, p.since = 0, time.Time{}
		return
	}
	if p.failures == 0 {
		p.since = p.checkedAt
	}
	p.failures++
}

// errNotTheProbe is a socket that opened but did not end the way the server's probe ends it -
// something in front answered instead.
var errNotTheProbe = errors.New("health: the socket was not closed by the realtime probe")

func (p *RealtimeProbe) dial(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, realtimeTimeout)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, p.target, &websocket.DialOptions{ //nolint:bodyclose // the library owns the handshake's body
		HTTPClient: p.client, Subprotocols: []string{realtimeSubprotocol},
	})
	if err != nil {
		return err
	}
	defer func() { _ = conn.CloseNow() }()
	_, _, err = conn.Read(ctx)
	var closed websocket.CloseError
	if errors.As(err, &closed) && closed.Code == websocket.StatusNormalClosure && closed.Reason == realtimeProbeReason {
		return nil
	}
	return errNotTheProbe
}
