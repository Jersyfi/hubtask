// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

// APIPath is the one major version of the interface this binary speaks. It is appended to the
// installation address rather than typed by the user (api-guidelines.md §8): which version of the
// contract hubctl speaks is a property of the binary, not of something somebody pasted once.
const APIPath = "/api/v1"

// maxResponseBytes caps what one answer may be. A page of items is kilobytes; this is far past
// anything the API returns and still bounded, which is what the cap is for.
const maxResponseBytes = 32 << 20

// connectTimeout bounds the connection attempt inside the overall budget, so that an address
// that accepts nothing fails quickly rather than consuming the whole timeout.
const connectTimeout = 10 * time.Second

// rateLimitRetries is how often a refused call waits and asks again.
//
// Retrying a 429 is safe in a way that retrying a 500 is not: the limiter refuses the request
// before anything runs it, so nothing was half-done and a repeated POST creates nothing twice.
// Bounded all the same - three waits and the answer stands, because a client that waits for ever
// is a client nobody can interrupt.
const rateLimitRetries = 3

// defaultRetryAfter is what a refusal without a Retry-After header is taken to mean. The server
// always sends one (presentation/rest/RateLimit.go); a proxy in between might not.
const defaultRetryAfter = time.Second

// Client is hubctl's half of the API contract.
//
// It goes through GuardedClient like everything else that leaves a process here (rule 6), with
// private networks allowed - the one place where that is not a hole but the point. The guard
// defends a server from targets its *users* named; here the user is the principal, and the most
// ordinary thing they do is run `hubctl --url http://localhost:8080` against `make run`.
type Client struct {
	base  string
	token secret.Secret
	// transport is the GuardedClient itself rather than the port, because the CLI is the one
	// consumer of the streaming half - Do for the calls, Stream for `hubctl watch` - and the port
	// deliberately does not know about streams (nothing in core consumes one).
	transport *httpclient.GuardedClient
	catalogue i18n.Catalogue
	// tenant is the workspace header, when this shell talks to an installation running more than
	// one. It confirms the resolution the credential already carries and never overrules it
	// (multi-tenancy.md §3), which is why sending it on every call is safe - and why a sign-in,
	// which has no credential yet, cannot resolve a workspace without it.
	tenant string

	// Notice reports something the user should know that is not the answer - so far, that a call
	// is waiting out a rate limit. Optional: nil says nothing.
	Notice func(format string, args ...any)
	// sleep is time.Sleep, injectable so a test of the retry does not take seconds.
	sleep func(ctx context.Context, d time.Duration) bool
}

// NewClient builds the client for one invocation.
//
// Both the address and the credential are required here rather than discovered on a 401. A CLI
// that made a doomed call in order to report "unauthenticated" would be telling the user about
// the server's answer when the problem is on their own machine.
func NewClient(profile Profile, catalogue i18n.Catalogue, timeout time.Duration) (*Client, error) {
	if profile.BaseURL == "" {
		return nil, errors.New("no installation to talk to: run `hubctl login --url ...`, or set " + envURL)
	}
	if profile.Credential().IsEmpty() {
		return nil, errors.New(
			"not signed in: run `hubctl login`, or `hubctl auth login` with a personal access token, or set " + envToken)
	}

	return newClient(profile, catalogue, timeout), nil
}

// NewAnonymousClient builds the client for the routes that carry no credential: the sign-in, its
// second step, the refresh, the token exchange.
//
// They are public by contract, and a credential presented beside the body would not be ignored -
// "a credential that was presented is always verified, even on a public route" (presentation/rest,
// Auth.go). So a refresh whose access token has just run out would be refused by the very
// expiry it exists to repair, which is precisely the call it has to survive.
func NewAnonymousClient(profile Profile, catalogue i18n.Catalogue, timeout time.Duration) (*Client, error) {
	if profile.BaseURL == "" {
		return nil, errors.New("no installation to talk to: run `hubctl login --url ...`, or set " + envURL)
	}
	profile.Token, profile.Session = secret.Secret{}, Session{}
	return newClient(profile, catalogue, timeout), nil
}

func newClient(profile Profile, catalogue i18n.Catalogue, timeout time.Duration) *Client {
	cfg := env.OutboundConfig{
		Timeout:          timeout,
		ConnectTimeout:   min(connectTimeout, timeout),
		MaxResponseBytes: maxResponseBytes,
		MaxRedirects:     0,
		// An installation on localhost, on a LAN, or behind a VPN is the normal case for a CLI.
		AllowPrivateNetworks: true,
	}
	return &Client{
		base:      profile.BaseURL + APIPath,
		token:     profile.Credential(),
		tenant:    profile.Tenant,
		transport: httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg)),
		catalogue: catalogue,
		sleep:     waitOrGiveUp,
	}
}

// identify puts the credential and the workspace on a request. An empty credential sends no
// header at all, which is what makes the anonymous client anonymous rather than a client with an
// empty bearer - a header the server would try to verify and refuse.
func (c *Client) identify(header map[string][]string) {
	if !c.token.IsEmpty() {
		header["Authorization"] = []string{"Bearer " + c.token.Reveal()}
	}
	if c.tenant != "" {
		header[restTenantHeader] = []string{c.tenant}
	}
}

// restTenantHeader is §3's third source of tenant resolution, spelled here rather than imported:
// a header name is part of the published contract (api/openapi.yaml, X-Hubtask-Tenant), and this
// binary reads the contract rather than the server's package.
const restTenantHeader = "X-Hubtask-Tenant"

// Get calls an operation and decodes its answer into `into`, which may be nil.
func (c *Client) Get(ctx context.Context, path string, query url.Values, into any) error {
	return c.call(ctx, http.MethodGet, path, query, nil, nil, into)
}

// Post calls an action or a create. A nil body sends none, which is what the actions with an
// optional request body expect.
func (c *Client) Post(ctx context.Context, path string, body, into any) error {
	return c.call(ctx, http.MethodPost, path, nil, body, nil, into)
}

// Patch moves part of a resource, which is what the cases of this API that have a state machine
// take: a data subject request is advanced by naming the field that changes rather than by sending
// the whole case back (api-guidelines.md).
func (c *Client) Patch(ctx context.Context, path string, body, into any) error {
	return c.call(ctx, http.MethodPatch, path, nil, body, nil, into)
}

// Put writes one addressed resource - a custom field's value, an attachment's membership. The
// operations behind it are idempotent by contract, which is what makes PUT the right verb.
func (c *Client) Put(ctx context.Context, path string, body, into any) error {
	return c.PutVersioned(ctx, path, body, "", into)
}

// PutVersioned is Put with the version the caller read as a precondition (ADR-0025). An empty
// version sends no If-Match, exactly as Delete's does: whether one is required is the server's
// answer rather than a guess made here.
func (c *Client) PutVersioned(
	ctx context.Context, path string, body any, ifMatch string, into any,
) error {
	header := map[string][]string{}
	if ifMatch != "" {
		header["If-Match"] = []string{ifMatch}
	}
	return c.call(ctx, http.MethodPut, path, nil, body, header, into)
}

// OpenStream connects to the change stream (C-10) and hands the open response back. The caller
// owns the body: it reads the events, it closes it, and its context is what bounds a connection
// that is deliberately unbounded past the headers.
func (c *Client) OpenStream(ctx context.Context, lastEventID string) (httpclient.StreamResponse, error) {
	request := port.Request{
		Method:      http.MethodGet,
		URL:         c.base + streamPath,
		TargetClass: "hubtask-api",
		Header: map[string][]string{
			"Accept":     {"text/event-stream"},
			"User-Agent": {"hubctl/" + version},
		},
	}
	c.identify(request.Header)
	if lastEventID != "" {
		request.Header["Last-Event-ID"] = []string{lastEventID}
	}
	response, err := c.transport.Stream(ctx, request)
	if err != nil {
		return httpclient.StreamResponse{}, c.transportError(err)
	}
	return response, nil
}

// Upload puts staged bytes where requestMediaUpload said to put them.
//
// The target is called as given: it is a capability URL - a presigned object-storage address, or
// this server's token-protected content route - and the token in it is the whole credential. No
// Authorization header travels with it, deliberately: a presigned target would refuse a request
// that carries a second credential, and the content route accepts none by contract.
func (c *Client) Upload(ctx context.Context, method, target string, data []byte) error {
	if method == "" {
		method = http.MethodPut
	}
	request := port.Request{
		Method:      method,
		URL:         target,
		TargetClass: "hubtask-media",
		Header: map[string][]string{
			"Content-Type": {"application/octet-stream"},
			"User-Agent":   {"hubctl/" + version},
		},
		Body: data,
	}
	response, err := c.send(ctx, request)
	if err != nil {
		return err
	}
	if response.Status >= http.StatusBadRequest {
		return c.problem(response)
	}
	return nil
}

// Delete removes, with the version the caller read as a precondition (ADR-0025). An empty
// version sends no If-Match, which the API answers with a precondition failure where it needs
// one - the refusal belongs to the server, not to a guess made here.
func (c *Client) Delete(ctx context.Context, path, ifMatch string) error {
	header := map[string][]string{}
	if ifMatch != "" {
		header["If-Match"] = []string{ifMatch}
	}
	return c.call(ctx, http.MethodDelete, path, nil, nil, header, nil)
}

func (c *Client) call(
	ctx context.Context, method, path string, query url.Values, body any,
	header map[string][]string, into any,
) error {
	_, answer, err := c.exchange(ctx, method, path, query, body, header)
	if err != nil {
		return err
	}
	if into == nil || len(answer) == 0 {
		return nil
	}
	if err := json.Unmarshal(answer, into); err != nil {
		return fmt.Errorf("the installation answered with something that is not the documented payload: %w", err)
	}
	return nil
}

// PostStatus posts and hands back the status beside the bytes, for the operations whose two
// successful answers are two different documents.
//
// The sign-in is the reason it exists: `201` is a session and `202` is the second step it owes
// (H-02), and nothing in either body reliably says which - a client that guessed by looking for a
// field would be inventing a contract the specification does not make. The status is the
// contract, so the status is what the caller gets.
func (c *Client) PostStatus(ctx context.Context, path string, body any) (int, []byte, error) {
	return c.exchange(ctx, http.MethodPost, path, nil, body, nil)
}

// exchange makes one call and turns a refusal into an error. What comes back is the status and
// the bytes; making sense of them is the caller's.
func (c *Client) exchange(
	ctx context.Context, method, path string, query url.Values, body any,
	header map[string][]string,
) (int, []byte, error) {
	target := c.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	request := port.Request{
		Method:      method,
		URL:         target,
		TargetClass: "hubtask-api",
		Header: map[string][]string{
			"Accept":     {"application/json, application/problem+json"},
			"User-Agent": {"hubctl/" + version},
		},
	}
	c.identify(request.Header)
	for name, values := range header {
		request.Header[name] = values
	}
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("building the request: %w", err)
		}
		request.Body = encoded
		request.Header["Content-Type"] = []string{"application/json"}
	}

	response, err := c.send(ctx, request)
	if err != nil {
		return 0, nil, err
	}
	if response.Status >= http.StatusBadRequest {
		return response.Status, nil, c.problem(response)
	}
	return response.Status, response.Body, nil
}

// Download posts a request whose answer is a file rather than a payload, and hands the bytes back
// with whatever the server said about the truncation.
//
// A file is not decoded: an export is CSV, JSON or a calendar document, and this is the one call
// whose answer is written where the caller says rather than reshaped by --json (D-08's row cap
// travels in the Export-Truncated header, which is why it comes back beside the bytes).
func (c *Client) Download(ctx context.Context, path string, body any) ([]byte, bool, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, false, fmt.Errorf("building the request: %w", err)
	}

	request := port.Request{
		Method:      http.MethodPost,
		URL:         c.base + path,
		TargetClass: "hubtask-api",
		Header: map[string][]string{
			"Accept":       {"text/csv, application/json, text/calendar, application/problem+json"},
			"Content-Type": {"application/json"},
			"User-Agent":   {"hubctl/" + version},
		},
		Body: encoded,
	}
	c.identify(request.Header)

	response, err := c.send(ctx, request)
	if err != nil {
		return nil, false, err
	}
	if response.Status >= http.StatusBadRequest {
		return nil, false, c.problem(response)
	}
	truncated := false
	for _, value := range response.Header["Export-Truncated"] {
		truncated = truncated || value == "true"
	}
	return response.Body, truncated, nil
}

// FetchPublic reads a URL that carries its own credential, with no Authorization header on it.
//
// The same trust model Upload uses for a content route, and for the same reason: a calendar feed
// URL *is* the credential, and a request that carried a second one would be claiming an identity
// the route does not accept (D-08).
func (c *Client) FetchPublic(ctx context.Context, target string) ([]byte, error) {
	response, err := c.send(ctx, port.Request{
		Method:      http.MethodGet,
		URL:         target,
		TargetClass: "hubtask-api",
		Header: map[string][]string{
			"Accept":     {"text/calendar, application/problem+json"},
			"User-Agent": {"hubctl/" + version},
		},
	})
	if err != nil {
		return nil, err
	}
	if response.Status >= http.StatusBadRequest {
		return nil, c.problem(response)
	}
	return response.Body, nil
}

// send makes the call and waits out a rate limit rather than handing it to the user.
//
// The server reports the budget on every answer and a Retry-After on a refusal, "so that a
// well-behaved client can slow down before it is refused rather than after"
// (presentation/rest/RateLimit.go). This is what being that client costs: a script doing a dozen
// things in a second is not abuse, and it should not have to know the burst size of the
// installation it is talking to.
func (c *Client) send(ctx context.Context, request port.Request) (port.Response, error) {
	for attempt := 0; ; attempt++ {
		response, err := c.transport.Do(ctx, request)
		if err != nil {
			return port.Response{}, c.transportError(err)
		}
		if response.Status != http.StatusTooManyRequests || attempt >= rateLimitRetries {
			return response, nil
		}

		wait := retryAfter(response.Header)
		if c.Notice != nil {
			c.Notice("the installation is limiting this credential; waiting %s", wait)
		}
		if !c.sleep(ctx, wait) {
			// The deadline would run out first, so the refusal is the answer.
			return response, nil
		}
	}
}

// retryAfter reads RFC 9110's Retry-After in its delay-seconds form, which is the one the server
// sends. A header that cannot be read is treated as absent rather than as zero: no wait at all
// would turn the retry into the hammering the limit exists to stop.
func retryAfter(headers map[string][]string) time.Duration {
	raw := ""
	for name, values := range headers {
		if strings.EqualFold(name, "Retry-After") && len(values) > 0 {
			raw = values[0]
			break
		}
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || seconds <= 0 {
		return defaultRetryAfter
	}
	return time.Duration(seconds) * time.Second
}

// waitOrGiveUp sleeps unless the context would end first, in which case there is no point.
func waitOrGiveUp(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// transportError turns the guard's own refusals into a sentence. They are domain errors carrying
// message codes, so they render from the same catalogue as anything the server says - a blocked
// address and a rejected token should not read as though they came from different programs.
func (c *Client) transportError(err error) error {
	domainErr := shared.AsError(err)
	if domainErr == nil {
		return err
	}
	if message, known := c.message(domainErr.DetailCode, nil); known {
		return errors.New(message)
	}
	message, _ := c.message("errors."+domainErr.Code, nil)
	return errors.New(message)
}

// message renders a code, treating an unknown one as unknown rather than as its own text: the
// caller has a second code to fall back to, and `errors.not_found` printed at a user would be
// this client failing at the one job the catalogue exists for.
func (c *Client) message(code string, params map[string]string) (string, bool) {
	if code == "" || !c.catalogue.Has(code) {
		return "", false
	}
	return c.catalogue.Message(code, params)
}
