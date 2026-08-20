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

// Client is hubctl's half of the API contract.
//
// It goes through GuardedClient like everything else that leaves a process here (rule 6), with
// private networks allowed - the one place where that is not a hole but the point. The guard
// defends a server from targets its *users* named; here the user is the principal, and the most
// ordinary thing they do is run `hubctl --url http://localhost:8080` against `make run`.
type Client struct {
	base      string
	token     secret.Secret
	transport port.Port
	catalogue i18n.Catalogue
}

// NewClient builds the client for one invocation.
//
// Both the address and the credential are required here rather than discovered on a 401. A CLI
// that made a doomed call in order to report "unauthenticated" would be telling the user about
// the server's answer when the problem is on their own machine.
func NewClient(profile Profile, catalogue i18n.Catalogue, timeout time.Duration) (*Client, error) {
	if profile.BaseURL == "" {
		return nil, errors.New("no installation to talk to: run `hubctl auth login --url ...`, or set " + envURL)
	}
	if profile.Token.IsEmpty() {
		return nil, errors.New("not signed in: run `hubctl auth login`, or set " + envToken)
	}

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
		token:     profile.Token,
		transport: httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg)),
		catalogue: catalogue,
	}, nil
}

// Get calls an operation and decodes its answer into `into`, which may be nil.
func (c *Client) Get(ctx context.Context, path string, query url.Values, into any) error {
	return c.call(ctx, http.MethodGet, path, query, nil, nil, into)
}

// Post calls an action or a create. A nil body sends none, which is what the actions with an
// optional request body expect.
func (c *Client) Post(ctx context.Context, path string, body, into any) error {
	return c.call(ctx, http.MethodPost, path, nil, body, nil, into)
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
	target := c.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	request := port.Request{
		Method:      method,
		URL:         target,
		TargetClass: "hubtask-api",
		Header: map[string][]string{
			"Accept":        {"application/json, application/problem+json"},
			"Authorization": {"Bearer " + c.token.Reveal()},
			"User-Agent":    {"hubctl/" + version},
		},
	}
	for name, values := range header {
		request.Header[name] = values
	}
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("building the request: %w", err)
		}
		request.Body = encoded
		request.Header["Content-Type"] = []string{"application/json"}
	}

	response, err := c.transport.Do(ctx, request)
	if err != nil {
		return c.transportError(err)
	}
	if response.Status >= http.StatusBadRequest {
		return c.problem(response)
	}
	if into == nil || len(response.Body) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Body, into); err != nil {
		return fmt.Errorf("the installation answered with something that is not the documented payload: %w", err)
	}
	return nil
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
