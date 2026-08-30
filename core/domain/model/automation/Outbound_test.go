// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func validHTTPParams() map[string]any {
	return map[string]any{
		"method": "POST",
		"url":    "https://example.org/hook",
		"headers": map[string]any{
			"Content-Type": "application/json",
		},
		"secret_header_name":  "Authorization",
		"secret_header_value": "Bearer s3cret",
		"body_template":       "'static'",
	}
}

func TestAValidHTTPRequestIsReadWhole(t *testing.T) {
	request, err := ReadHTTPRequest(validHTTPParams(), "0")
	if err != nil {
		t.Fatalf("a valid request was refused: %v", err)
	}
	if request.Method != "POST" || request.URL != "https://example.org/hook" {
		t.Errorf("read %+v", request)
	}
	if request.Headers["Content-Type"] != "application/json" {
		t.Errorf("headers %v", request.Headers)
	}
	if request.SecretValue != "Bearer s3cret" || request.SecretHeaderName != "Authorization" {
		t.Errorf("secret half %+v", request)
	}
}

// Each refusal names the field it is about, so an editor can point at the line.
func TestAnHTTPRequestIsRefusedByField(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
		code   string
	}{
		{"no method", func(p map[string]any) { delete(p, "method") }, "automation.http_method_required"},
		{"a verb this system does not send", func(p map[string]any) { p["method"] = "TRACE" }, "automation.http_method_unknown"},
		{"no url", func(p map[string]any) { delete(p, "url") }, "automation.http_url_required"},
		{"a scheme that is not http", func(p map[string]any) { p["url"] = "ftp://example.org" }, "automation.http_url_invalid"},
		{"headers that are not an object", func(p map[string]any) { p["headers"] = "Authorization: x" }, "automation.http_headers_invalid"},
		{"a header value that is not text", func(p map[string]any) {
			p["headers"] = map[string]any{"X-N": 7}
		}, "automation.http_headers_invalid"},
		{"a secret nothing uses", func(p map[string]any) {
			delete(p, "secret_header_name")
		}, "automation.http_secret_unused"},
		{"a signature with no secret", func(p map[string]any) {
			delete(p, "secret_header_name")
			delete(p, "secret_header_value")
			p["signature_header"] = "X-Signature"
		}, "automation.http_secret_required"},
		{"a value beside a ciphertext", func(p map[string]any) {
			p["secret_header_sealed"] = map[string]any{
				"ciphertext": "abc", "key_id": "k1", "purpose": "x",
			}
		}, "automation.http_secret_conflict"},
		{"a stored secret this version cannot read", func(p map[string]any) {
			delete(p, "secret_header_value")
			p["secret_header_sealed"] = map[string]any{"ciphertext": "abc"}
		}, "automation.http_sealed_invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := validHTTPParams()
			tc.mutate(params)

			_, err := ReadHTTPRequest(params, "0")
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want a validation refusal", err)
			}
			coded := shared.AsError(err)
			found := coded.DetailCode == tc.code
			for _, field := range coded.Fields {
				if field.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Errorf("refused with %q / %+v, want %s", coded.DetailCode, coded.Fields, tc.code)
			}
		})
	}
}

// The mask means "keep the stored one", and the reader says so rather than reading it as a value.
func TestTheMaskReadsAsKeepNotAsAValue(t *testing.T) {
	params := validHTTPParams()
	params["secret_header_value"] = SecretMask

	request, err := ReadHTTPRequest(params, "0")
	if err != nil {
		t.Fatalf("the mask was refused: %v", err)
	}
	if !request.SecretMasked || request.SecretValue != "" {
		t.Errorf("the mask read as %+v", request)
	}
}

// The aggregate refuses a malformed HTTP_REQUEST at the write, exactly as it refuses a malformed
// WAIT: none of these fields can be supplied by the run later.
func TestARuleWithAMalformedHTTPRequestIsRefusedAtTheWrite(t *testing.T) {
	input := NewRuleInput{
		ID:       shared.ID("01936f2a-7c1e-7000-8000-0000000000b1"),
		TenantID: shared.ID("01936f2a-7c1e-7000-8000-0000000000b2"),
		Name:     "Call out", Scope: Scope{Type: ScopeTenant},
		RunAs:     shared.ID("01936f2a-7c1e-7000-8000-0000000000b3"),
		Trigger:   Trigger{Kind: TriggerManual},
		Actions:   []Action{{Kind: ActionHTTPRequest, Params: map[string]any{"method": "POST"}}},
		CreatedBy: shared.ID("01936f2a-7c1e-7000-8000-0000000000b4"),
	}

	_, err := NewRule(input)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation refusal", err)
	}
}
