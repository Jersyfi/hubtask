// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package httpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// GuardedClient is the only way out of the process (rule 6). It is an http.Client with four
// things bolted on that the standard one does not do: the address guard at dial time, a
// re-check of every redirect hop, a hard cap on the response size, and a deadline that cannot
// be forgotten.
//
// It carries no circuit breaker of its own. A breaker belongs to a dependency, and this client
// serves all of them - one breaker here would let a broken AI provider cut off every webhook in
// the installation. The caller wraps its own call in resilience.Breaker; that is what the
// building blocks are for.
type GuardedClient struct {
	guard  *Guard
	cfg    env.OutboundConfig
	client *http.Client
	// observe records the duration. Injected rather than imported, so the adapter does not
	// depend on the metrics adapter (project-structure.md §2).
	observe func(ctx context.Context, targetClass string, seconds float64)
	// now is the clock, for the duration measurement.
	now func() time.Time
	// tracer produces the client span. Never nil: with tracing off it is the no-op tracer,
	// which still carries the incoming span context onwards.
	tracer trace.Tracer
}

// NewGuardedClient builds the client from the outbound configuration.
func NewGuardedClient(cfg env.OutboundConfig, guard *Guard) *GuardedClient {
	c := &GuardedClient{
		guard:   guard,
		cfg:     cfg,
		observe: func(context.Context, string, float64) {},
		now:     time.Now,
		tracer:  noop.NewTracerProvider().Tracer(""),
	}

	dialer := &net.Dialer{
		Timeout: cfg.ConnectTimeout,
		// Control runs after the name is resolved and before the socket connects, with the
		// concrete address. This is the check that survives DNS rebinding: whatever the
		// resolver said a moment ago, this is what the connection is about to use.
		Control: func(network, address string, _ syscall.RawConn) error {
			return guard.CheckControl(network, address)
		},
	}

	c.client = &http.Client{
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   cfg.ConnectTimeout,
			ResponseHeaderTimeout: cfg.Timeout,
			ExpectContinueTimeout: time.Second,
			// A handful of idle connections is enough for the traffic this client carries, and
			// it keeps a burst of webhook deliveries from holding sockets open to a target
			// that will not be called again for hours.
			MaxIdleConns:        32,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     30 * time.Second,
			ForceAttemptHTTP2:   true,
		},
		CheckRedirect: c.checkRedirect,
	}
	return c
}

// WithObserver returns a copy that reports the duration of every call. The composition root
// passes the metrics adapter's OutboundHTTP here.
func (c *GuardedClient) WithObserver(observe func(ctx context.Context, targetClass string, seconds float64)) *GuardedClient {
	copied := *c
	if observe != nil {
		copied.observe = observe
	}
	return &copied
}

// WithTracer returns a copy that opens a client span per call. The composition root passes the
// tracer of the observability adapter.
func (c *GuardedClient) WithTracer(tracer trace.Tracer) *GuardedClient {
	copied := *c
	if tracer != nil {
		copied.tracer = tracer
	}
	return &copied
}

// Do makes the call. It returns an error when the call could not be made or could not be
// completed; an answered request with a 500 is not an error here, because whether that is a
// failure depends on what was being asked (see port.Response).
func (c *GuardedClient) Do(ctx context.Context, req port.Request) (port.Response, error) {
	target, err := c.guard.CheckURL(req.URL)
	if err != nil {
		return port.Response{}, err
	}
	// Resolution before connecting, so that a target inside the network fails before a socket
	// is opened, and with an error that names the address rather than a dial failure. The
	// dial-time check runs regardless - this pass cannot be trusted to still hold.
	if _, err := c.guard.Resolve(ctx, target.Hostname()); err != nil {
		return port.Response{}, err
	}

	class := req.TargetClass
	if class == "" {
		class = "unclassified"
	}

	started := c.now()
	response, err := resilience.DoValue(ctx, class, c.cfg.Timeout,
		func(callCtx context.Context) (port.Response, error) {
			return c.send(callCtx, req, target.String(), class)
		})
	c.observe(ctx, class, c.now().Sub(started).Seconds())
	return response, err
}

func (c *GuardedClient) send(ctx context.Context, req port.Request, url, class string) (port.Response, error) {
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	// The span carries the method, the class and the host - never the URL. A query string can
	// hold a token, and a span is stored and read by more people than a log line
	// (security.md §12: traces with masked attributes).
	ctx, span := c.tracer.Start(ctx, "HTTP "+method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.request.method", method),
			attribute.String("hubtask.target_class", class),
		))
	defer span.End()

	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return port.Response{}, shared.ErrValidation.
			WithDetail(codeTargetMalformed).
			WithParams(map[string]string{"target": url}).
			WithCause(fmt.Errorf("building the request: %w", err))
	}
	for name, values := range req.Header {
		for _, value := range values {
			httpReq.Header.Add(name, value)
		}
	}
	span.SetAttributes(attribute.String("server.address", httpReq.URL.Hostname()))
	// The traceparent travels with the call, so that a webhook recipient that understands W3C
	// trace context can join the same trace - and so that our own installations calling each
	// other produce one trace rather than two (§3.3).
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(httpReq.Header))

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		failed := transportError(class, err)
		span.SetStatus(codes.Error, shared.AsError(failed).Code)
		return port.Response{}, failed
	}
	defer func() { _ = httpResp.Body.Close() }()

	// One byte more than the limit, so that "exactly at the limit" and "too much" can be told
	// apart. Without the extra byte a response of precisely the limit would be reported as
	// truncated for ever.
	span.SetAttributes(attribute.Int("http.response.status_code", httpResp.StatusCode))
	payload, err := io.ReadAll(io.LimitReader(httpResp.Body, c.cfg.MaxResponseBytes+1))
	if err != nil {
		failed := transportError(class, err)
		span.SetStatus(codes.Error, shared.AsError(failed).Code)
		return port.Response{}, failed
	}
	if int64(len(payload)) > c.cfg.MaxResponseBytes {
		// Truncating and carrying on would hand the caller half a JSON document. A target that
		// answers with more than the limit is misconfigured, and it will be next time too
		// (T-17).
		span.SetStatus(codes.Error, "response_too_large")
		return port.Response{}, shared.ErrValidation.
			WithDetail("dependency.response_too_large").
			WithParams(map[string]string{"limit_bytes": fmt.Sprint(c.cfg.MaxResponseBytes)})
	}

	return port.Response{
		Status: httpResp.StatusCode,
		Header: httpResp.Header,
		Body:   payload,
	}, nil
}

// Upload makes a call whose *request* body is a stream rather than a slice.
//
// The mirror of Stream, and it exists for the same kind of payload seen from the other side: an
// archive on its way to a backup target has no known length and does not fit in memory, and
// port.Request carries a []byte because everything else this system sends outwards is a small,
// already-rendered document (core/port/httpclient). Buffering an archive to satisfy that shape is
// the OOM kill observability-reliability.md §6 calls an architecture defect (T-17).
//
// Every guard applies unchanged - the URL check, the resolve before the connect, the dial-time
// control, the redirect budget and the response cap - and that is the point: a target whose
// address came from an API is exactly what rule 6 exists for. What is deliberately absent is the
// retry the resilient wrapper would otherwise add, because a stream can be read only once and a
// second attempt would send whatever was left of it.
func (c *GuardedClient) Upload(
	ctx context.Context, req port.Request, body io.Reader, length int64,
) (port.Response, error) {
	target, err := c.guard.CheckURL(req.URL)
	if err != nil {
		return port.Response{}, err
	}
	if _, err := c.guard.Resolve(ctx, target.Hostname()); err != nil {
		return port.Response{}, err
	}

	class := req.TargetClass
	if class == "" {
		class = "unclassified"
	}
	method := req.Method
	if method == "" {
		method = http.MethodPut
	}

	started := c.now()
	defer func() { c.observe(ctx, class, c.now().Sub(started).Seconds()) }()

	httpReq, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return port.Response{}, shared.ErrValidation.
			WithDetail(codeTargetMalformed).
			WithParams(map[string]string{"target": target.String()}).
			WithCause(fmt.Errorf("building the request: %w", err))
	}
	// A length of -1 means "nobody knows", which net/http turns into chunked transfer encoding.
	// That is the honest answer for an archive, and a server that cannot take it says so.
	httpReq.ContentLength = length
	for name, values := range req.Header {
		for _, value := range values {
			httpReq.Header.Add(name, value)
		}
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(httpReq.Header))

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return port.Response{}, transportError(class, err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(httpResp.Body, c.cfg.MaxResponseBytes+1))
	if err != nil {
		return port.Response{}, transportError(class, err)
	}
	if int64(len(payload)) > c.cfg.MaxResponseBytes {
		return port.Response{}, shared.ErrValidation.
			WithDetail("dependency.response_too_large").
			WithParams(map[string]string{"limit_bytes": fmt.Sprint(c.cfg.MaxResponseBytes)})
	}

	return port.Response{
		Status: httpResp.StatusCode,
		Header: httpResp.Header,
		Body:   payload,
	}, nil
}

// StreamResponse is an answer whose body is still arriving. The caller owns Body and must close
// it; closing it is also how the caller ends a stream the server would happily keep feeding.
type StreamResponse struct {
	Status int
	Header map[string][]string
	Body   io.ReadCloser
}

// Stream makes a call whose response body is handed to the caller instead of being read here -
// a server-sent event stream lives until somebody hangs up, and a client that buffered it whole
// would never hand over a byte.
//
// The guard is the same as Do's: the URL check, the resolve-before-connect, the dial-time
// control and the redirect re-check all apply, because a stream is exactly as capable of being
// pointed somewhere it should not go (T-07). What differs is the bounds. There is no overall
// deadline - the connection attempt, the TLS handshake and the wait for headers stay bounded by
// the transport's timeouts, and past the headers an open stream is the point, not a hang. And
// there is no response size cap - the body is unbounded by design, and bounding what one read
// may carry is the caller's job, since only the caller knows its framing. Liveness past the
// headers is also the caller's: it owns the read loop, so it is the one place a stalled stream
// can be noticed (rule 7 is satisfied by the caller's context, which cancels the body mid-read).
//
// No span and no duration metric, deliberately: the lifetime of a stream measures how long the
// caller stayed, not how the target behaved, and averaging it into the outbound histogram would
// bury the signal that histogram exists for.
func (c *GuardedClient) Stream(ctx context.Context, req port.Request) (StreamResponse, error) {
	target, err := c.guard.CheckURL(req.URL)
	if err != nil {
		return StreamResponse{}, err
	}
	if _, err := c.guard.Resolve(ctx, target.Hostname()); err != nil {
		return StreamResponse{}, err
	}
	class := req.TargetClass
	if class == "" {
		class = "unclassified"
	}
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return StreamResponse{}, shared.ErrValidation.
			WithDetail(codeTargetMalformed).
			WithParams(map[string]string{"target": target.String()}).
			WithCause(fmt.Errorf("building the request: %w", err))
	}
	for name, values := range req.Header {
		for _, value := range values {
			httpReq.Header.Add(name, value)
		}
	}

	httpResp, err := c.client.Do(httpReq) //nolint:bodyclose // handing the open body to the caller is the point; StreamResponse says the caller closes it
	if err != nil {
		return StreamResponse{}, transportError(class, err)
	}
	return StreamResponse{
		Status: httpResp.StatusCode,
		Header: httpResp.Header,
		Body:   httpResp.Body,
	}, nil
}

// checkRedirect re-checks every hop. The guard's dial-time control already refuses a private
// address, but a redirect also has to satisfy the scheme rule and the allowlist - "301 to
// file:///etc/passwd" and "301 to a host nobody put on the list" are both the point of the
// exercise (T-07).
//
// The standard client already drops Authorization and Cookie when a redirect changes host, so
// credentials do not travel to a target that was not the one addressed.
func (c *GuardedClient) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > c.cfg.MaxRedirects {
		return shared.ErrValidation.
			WithDetail("dependency.too_many_redirects").
			WithParams(map[string]string{"limit": fmt.Sprint(c.cfg.MaxRedirects)})
	}
	_, err := c.guard.CheckURL(req.URL.String())
	return err
}

// transportError turns what the transport reports into the project's error model. The text of a
// transport error can contain the address, the proxy, and the resolver's opinion of all three,
// so it travels as a cause for the log and never as a message for the client (security.md §9).
func transportError(class string, err error) error {
	// A guard refusal from the dial hook or from CheckRedirect arrives wrapped in a
	// *url.Error. It has to keep its own identity: the caller decides between "disable this
	// subscription" and "try again later" by exactly this distinction.
	var typed *shared.Error
	if errors.As(err, &typed) {
		return typed
	}
	// The context errors are handed back untouched: resilience.Do is the one place that knows
	// whether the deadline was our budget or the caller going away, and it must not be told
	// the answer twice.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// A timeout of the transport's own - the connection attempt, the TLS handshake, the wait
	// for response headers - never reaches resilience.Do as a deadline, so it is classified
	// here.
	if isTimeout(err) {
		return resilience.TimeoutError(class, err)
	}
	return shared.ErrUnavailable.
		WithDetail("dependency.unavailable").
		WithParams(map[string]string{"dependency": class}).
		WithCause(fmt.Errorf("outbound call: %w", err))
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// HTTPClient exposes the guard as a standard library client, for the one kind of dependency that
// insists on taking one (ADR-0036): a library that does its own discovery and key fetching
// cannot be handed `Do`, and forking it to make it would be worse than this.
//
// Nothing is given up. The allowlist and scheme check run in front of every request, the
// resolution check runs before the socket, the dial-time address check is the transport's own -
// so DNS rebinding is caught here exactly as it is in Do - and each redirect hop is checked
// again. The duration is reported under the class the caller names.
//
// What is deliberately absent is the circuit breaker `Do` carries. A breaker is keyed by
// dependency class, and the targets reached through this client are configured per workspace:
// one class per tenant's issuer would be an unbounded label (§3.2), and a single shared class
// would let one workspace's broken provider hold every other workspace's sign-in shut. An
// interactive sign-in fails fast on its own and says so with a code.
func (c *GuardedClient) HTTPClient(targetClass string) *http.Client {
	class := targetClass
	if class == "" {
		class = "unclassified"
	}
	return &http.Client{
		Transport:     guardedTransport{client: c, class: class},
		CheckRedirect: c.checkRedirect,
		Timeout:       c.cfg.Timeout,
	}
}

// guardedTransport is HTTPClient's round tripper: the checks of Do, in the shape the standard
// library calls.
type guardedTransport struct {
	client *GuardedClient
	class  string
}

func (g guardedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, err := g.client.guard.CheckURL(req.URL.String()); err != nil {
		return nil, err
	}
	if _, err := g.client.guard.Resolve(req.Context(), req.URL.Hostname()); err != nil {
		return nil, err
	}
	started := g.client.now()
	response, err := g.client.client.Transport.RoundTrip(req)
	g.client.observe(req.Context(), g.class, g.client.now().Sub(started).Seconds())
	if err != nil {
		return nil, transportError(g.class, err)
	}
	return response, nil
}
