// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package connector is the adapters of the CMDB's connectors (research itsm-cmdb-ai §4, §6.3):
// one per program, each turning that program's API into pages of the reconciliation's payload.
// Every call leaves through the guarded client (rule 6) with a deadline of its own (rule 7), is
// refused unless its host is one the connector may reach (C6), and answers a rate limit with the
// wait the source asked for (429, Retry-After). The answers' contents - names, serials, addresses
// - travel only into the payload, never into a log or an error (rule 10).
package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/connector"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
)

// targetClass is the outbound metric's label for every connector call (a small fixed set).
const targetClass = "connector"

// The bounds of one call and one page.
const (
	callTimeout     = 30 * time.Second
	defaultPageSize = 100
)

// Breaker is the slice of a circuit breaker a pull runs inside.
type Breaker interface {
	Do(ctx context.Context, call func(context.Context) error) error
}

// BreakerPool hands out one breaker per connector, made on first use: a breaker belongs to a
// dependency, and one source down must not cut off the others.
type BreakerPool struct {
	New func(name string) Breaker

	mu   sync.Mutex
	held map[shared.ID]Breaker
}

func (p *BreakerPool) of(id shared.ID) Breaker {
	if p == nil || p.New == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.held == nil {
		p.held = map[shared.ID]Breaker{}
	}
	b, ok := p.held[id]
	if !ok {
		b = p.New("connector")
		p.held[id] = b
	}
	return b
}

// Pullers pulls a page from any of the shipped kinds.
type Pullers struct {
	Client   httpport.Port
	Breakers *BreakerPool
	// PageSize is how many records one call asks for; zero is 100.
	PageSize int
}

var _ port.Puller = Pullers{}

// Pull answers one page of the connector's source.
func (p Pullers) Pull(ctx context.Context, pull port.Pull) (port.Page, error) {
	if pull.Credential.IsEmpty() {
		return port.Page{}, shared.ErrUnavailable.WithDetail(port.CodeNoCredential)
	}
	var fetch func(context.Context, session) (port.Page, error)
	switch pull.Kind {
	case cmdb.ConnectorGLPI:
		fetch = glpi
	case cmdb.ConnectorNetBox:
		fetch = netbox
	case cmdb.ConnectorIntune:
		fetch = intune
	case cmdb.ConnectorEntra:
		fetch = entra
	case cmdb.ConnectorHetzner:
		fetch = hetzner
	case cmdb.ConnectorOpenStack:
		fetch = openstack
	default:
		return port.Page{}, shared.ErrUnavailable.WithDetail(port.CodeUnsupported)
	}
	s := session{p: p, pull: pull, size: p.PageSize}
	if s.size <= 0 {
		s.size = defaultPageSize
	}
	breaker := p.Breakers.of(pull.Connector)
	if breaker == nil {
		return fetch(ctx, s)
	}
	var page port.Page
	err := breaker.Do(ctx, func(ctx context.Context) error {
		var err error
		page, err = fetch(ctx, s)
		return err
	})
	return page, err
}

// session is one pull's calls.
type session struct {
	p    Pullers
	pull port.Pull
	size int
}

func (s session) base(path string) string { return strings.TrimRight(s.pull.BaseURL, "/") + path }

func (s session) setting(key string) string { return s.pull.Settings[key] }

func (s session) secret() string { return s.pull.Credential.Reveal() }

func unavailable(code string, params ...string) error {
	e := shared.ErrUnavailable.WithDetail(code)
	if len(params) > 1 {
		m := map[string]string{}
		for i := 0; i+1 < len(params); i += 2 {
			m[params[i]] = params[i+1]
		}
		e = e.WithParams(m)
	}
	return e
}

// call makes one request to an allowed host and judges the answer's status.
func (s session) call(ctx context.Context, method, rawURL string, header map[string][]string, body []byte) (httpport.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return httpport.Response{}, unavailable(port.CodeMalformed)
	}
	if !slices.Contains(s.pull.Hosts, strings.ToLower(u.Hostname())) {
		return httpport.Response{}, unavailable(port.CodeHostNotAllowed)
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	resp, err := s.p.Client.Do(ctx, httpport.Request{Method: method, URL: rawURL, Header: header, Body: body, TargetClass: targetClass})
	if err != nil {
		return httpport.Response{}, shared.ErrUnavailable.WithDetail(port.CodeUnreachable).WithCause(err)
	}
	switch {
	case resp.Status >= 200 && resp.Status < 300:
		return resp, nil
	case resp.Status == 401 || resp.Status == 403:
		return resp, unavailable(port.CodeUnauthorised, "status", strconv.Itoa(resp.Status))
	case resp.Status == 429:
		return resp, unavailable(port.CodeRateLimited, "retry_after", retryAfter(resp.Header))
	}
	return resp, unavailable(port.CodeUnreachable, "status", strconv.Itoa(resp.Status))
}

// retryAfter reads a Retry-After header in seconds; a date or nothing is a minute.
func retryAfter(header map[string][]string) string {
	for k, v := range header {
		if strings.EqualFold(k, "Retry-After") && len(v) > 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(v[0])); err == nil && n >= 0 {
				return strconv.Itoa(n)
			}
		}
	}
	return "60"
}

// getJSON reads an answer into a document.
func (s session) getJSON(ctx context.Context, rawURL string, header map[string][]string, into any) (httpport.Response, error) {
	resp, err := s.call(ctx, "GET", rawURL, header, nil)
	if err != nil {
		return resp, err
	}
	return resp, decode(resp.Body, into)
}

func decode(body []byte, into any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if err := d.Decode(into); err != nil {
		return unavailable(port.CodeMalformed)
	}
	return nil
}

// form is a token request's body.
func form(values map[string]string) []byte {
	v := url.Values{}
	for k, x := range values {
		v.Set(k, x)
	}
	return []byte(v.Encode())
}

var formHeader = map[string][]string{"Content-Type": {"application/x-www-form-urlencoded"}}

// token answers an OAuth 2 client-credentials access token.
func (s session) token(ctx context.Context, endpoint string, values map[string]string) (string, error) {
	values["grant_type"] = "client_credentials"
	resp, err := s.call(ctx, "POST", endpoint, formHeader, form(values))
	if err != nil {
		return "", err
	}
	var t struct {
		AccessToken string `json:"access_token"`
	}
	if err := decode(resp.Body, &t); err != nil || t.AccessToken == "" {
		return "", unavailable(port.CodeUnauthorised)
	}
	return t.AccessToken, nil
}

func bearer(token string) map[string][]string {
	return map[string][]string{"Authorization": {"Bearer " + token}, "Accept": {"application/json"}}
}

// attrs keeps the non-empty values only, so that a source never clears what it does not know.
func attrs(pairs ...string) map[string]any {
	out := map[string]any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		if v := strings.TrimSpace(pairs[i+1]); v != "" {
			out[pairs[i]] = v
		}
	}
	return out
}

// offsetCursor is a walk through a listing changed since a moment, page by page: where the walk
// stands and the newest change it saw, which the next walk starts from.
type offsetCursor struct {
	since  string
	offset int
	newest string
}

func readOffset(raw string) offsetCursor {
	parts := strings.Split(raw, "|")
	c := offsetCursor{since: parts[0]}
	if len(parts) > 1 {
		c.offset, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		c.newest = parts[2]
	}
	return c
}

func (c offsetCursor) String() string {
	return c.since + "|" + strconv.Itoa(c.offset) + "|" + c.newest
}

// saw keeps the newest of the changes' moments; the sources write them in one sortable form.
func (c *offsetCursor) saw(at string) {
	if at > c.newest {
		c.newest = at
	}
}

// next answers the cursor after a page of n records: on to the next page, or - the walk done -
// a fresh walk from the newest change seen.
func (c offsetCursor) next(n, size int) (string, bool) {
	if n >= size {
		c.offset += n
		return c.String(), true
	}
	since := c.newest
	if since == "" {
		since = c.since
	}
	return since, false
}

func text(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case map[string]any:
		return text(x["name"])
	}
	return ""
}
