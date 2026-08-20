// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	port "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// requestIDParam is the parameter `errors.internal` renders. The server puts the request ID in a
// field of its own rather than in the parameters, so it is merged in here - a message asking the
// reader to quote a reference has to have the reference in it.
const requestIDParam = "request_id"

// APIError is a refusal from the installation, already turned into a sentence.
//
// The rendering happens once, when the problem arrives, rather than in Error(): what a client
// shows a person is built from the code and the parameters (ADR-0011), and doing that at the
// point where the catalogue and the response are both in hand keeps the two from drifting apart.
type APIError struct {
	Status     int
	Code       string
	DetailCode string
	RequestID  string

	message string
	fields  []string
}

func (e APIError) Error() string {
	if len(e.fields) == 0 {
		return e.message
	}
	return e.message + "\n  " + strings.Join(e.fields, "\n  ")
}

// problem turns a 4xx or 5xx into an APIError.
//
// An answer that is not a problem document is reported by its status and nothing else. Printing
// the body would be printing whatever a proxy, a captive portal or a load balancer decided to
// send - which is HTML at best and misleading at worst.
func (c *Client) problem(response port.Response) error {
	contentType := header(response.Header, "Content-Type")
	if !strings.HasPrefix(contentType, "application/problem+json") {
		return fmt.Errorf("the installation answered %s (%s), which is not a problem document",
			statusText(response.Status), describe(contentType))
	}

	var document openapi.Problem
	if err := json.Unmarshal(response.Body, &document); err != nil {
		return fmt.Errorf("the installation answered %s with a problem document that cannot be read: %w",
			statusText(response.Status), err)
	}

	failure := APIError{
		Status:     document.Status,
		Code:       document.Code,
		DetailCode: value(document.DetailCode),
		RequestID:  value(document.RequestId),
	}

	params := stringParams(document.Params)
	if failure.RequestID != "" {
		if _, taken := params[requestIDParam]; !taken {
			params[requestIDParam] = failure.RequestID
		}
	}

	// The detail code is the specific one - `containers.parent_not_found` rather than
	// `not_found` - so it wins where the catalogue knows it. The contract code is the fallback,
	// and a code the catalogue has never heard of is printed as itself: a client that showed
	// nothing would leave the reader with a blank line and an exit code.
	message, known := c.message(failure.DetailCode, params)
	if !known {
		message, known = c.message("errors."+failure.Code, params)
	}
	if !known {
		message = firstNonEmpty(failure.DetailCode, failure.Code, statusText(document.Status))
	}
	failure.message = message

	if document.FieldErrors != nil {
		for _, field := range *document.FieldErrors {
			code := value(field.Code)
			text, ok := c.message(code, stringParams(field.Params))
			if !ok {
				text = code
			}
			failure.fields = append(failure.fields, strings.TrimPrefix(value(field.Path), "/")+": "+text)
		}
		sort.Strings(failure.fields)
	}
	return failure
}

// stringParams flattens the parameters of a problem document. They are strings on the wire - the
// domain's Params are map[string]string - and typed as `any` only because the generated schema
// says "additionalProperties". Anything else is formatted rather than dropped.
func stringParams(params *map[string]any) map[string]string {
	flattened := map[string]string{}
	if params == nil {
		return flattened
	}
	for name, raw := range *params {
		if text, ok := raw.(string); ok {
			flattened[name] = text
			continue
		}
		flattened[name] = fmt.Sprint(raw)
	}
	return flattened
}

func header(headers map[string][]string, name string) string {
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func statusText(status int) string {
	if text := http.StatusText(status); text != "" {
		return fmt.Sprintf("%d %s", status, text)
	}
	return fmt.Sprintf("%d", status)
}

func describe(contentType string) string {
	if contentType == "" {
		return "no content type"
	}
	return contentType
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func firstNonEmpty(candidates ...string) string {
	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}
