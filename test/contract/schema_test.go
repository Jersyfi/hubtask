// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// A contract test that cannot fail proves nothing, so the validator is checked against
// deliberately wrong responses first - the same idea as make gate-selftest.
func TestTheValidatorCatchesABrokenResponse(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatalf("the specification could not be read: %v", err)
	}

	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "a required field is missing",
			body: `{"version":"0.1.0","dependencies":[]}`,
			want: `required field "status"`,
		},
		{
			name: "a value outside the enum",
			body: `{"status":"probably fine","version":"0.1.0","dependencies":[]}`,
			want: "outside the enum",
		},
		{
			name: "the wrong type",
			body: `{"status":"ok","version":7,"dependencies":[]}`,
			want: "is not string",
		},
		{
			name: "a field nobody declared",
			body: `{"status":"ok","version":"0.1.0","dependencies":[],"db_password":"hunter2"}`,
			want: `"db_password" is not in the specification`,
		},
		{
			name: "a broken dependency entry",
			body: `{"status":"ok","version":"0.1.0","dependencies":[{"name":"postgres"}]}`,
			want: `required field "required"`,
		},
		{
			name: "a circuit state that does not exist",
			body: `{"status":"ok","version":"0.1.0","dependencies":[` +
				`{"name":"s3","required":false,"status":"down","circuit_state":"melted"}]}`,
			want: "outside the enum",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems, err := spec.validateAgainst("HealthReport", []byte(tc.body))
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			if len(problems) == 0 {
				t.Fatalf("the validator accepted %s", tc.body)
			}
			if !strings.Contains(strings.Join(problems, "; "), tc.want) {
				t.Errorf("the findings %v do not mention %q", problems, tc.want)
			}
		})
	}
}

// And the reverse: a report exercising every optional field has to pass, or the validator is
// merely strict rather than correct.
func TestTheValidatorAcceptsAFullReport(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatalf("the specification could not be read: %v", err)
	}

	body := `{
	  "status": "degraded",
	  "version": "0.1.0",
	  "role": ["api", "worker"],
	  "migration": {"applied": 47, "expected": 47, "status": "ok"},
	  "dependencies": [
	    {"name": "postgres", "required": true, "status": "ok", "latency_ms": 3},
	    {"name": "object_storage", "required": false, "status": "down",
	     "since": "2026-08-14T09:12:04Z", "last_error_code": "storage.unreachable",
	     "circuit_state": "open", "impact": ["media.upload"]},
	    {"name": "ai_provider", "required": false, "status": "disabled",
	     "latency_ms": null, "since": null, "last_error_code": null, "circuit_state": null}
	  ],
	  "degraded_features": [
	    {"feature": "media", "reason_code": "dependency.unavailable", "since": "2026-08-14T09:12:04Z"}
	  ],
	  "backlogs": {"outbox_pending": 12, "outbox_lag_seconds": 1.5, "job_queue_depth": 3,
	               "dead_letter_total": 0, "webhook_retry_backlog": 0},
	  "warnings": [
	    {"code": "config.backup_not_configured", "severity": "warn"},
	    {"code": "config.smtp_without_tls", "severity": "warn", "params": {"host": "mail.example.org"}}
	  ]
	}`

	problems, err := spec.validateAgainst("HealthReport", []byte(body))
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	for _, p := range problems {
		t.Errorf("a valid report was rejected: %s", p)
	}
}

// A response the document describes by composition is judged whole: a field of either member of
// an allOf is declared, and a "this or null" takes both.
func TestTheValidatorComposesAllOfOneOfAndAnyOf(t *testing.T) {
	spec := &specification{}
	object := func(required []string, properties map[string]*schema) *schema {
		return &schema{Type: "object", Required: required, Properties: properties}
	}
	text := &schema{Type: "string"}
	spec.Components.Schemas = map[string]*schema{
		"Token": object([]string{"id"}, map[string]*schema{"id": text}),
		"TokenSecret": {AllOf: []*schema{
			{Ref: "#/components/schemas/Token"},
			object([]string{"token"}, map[string]*schema{"token": text}),
		}},
		"Holder": object(nil, map[string]*schema{
			"one": {OneOf: []*schema{{Ref: "#/components/schemas/Token"}, {Type: "null"}}},
			"any": {AnyOf: []*schema{{Ref: "#/components/schemas/Token"}, {Type: "null"}}},
		}),
	}

	cases := []struct {
		name, schema, body, want string
	}{
		{"both members' fields are declared", "TokenSecret", `{"id":"a","token":"t"}`, ""},
		{"a member's required field is missing", "TokenSecret", `{"id":"a"}`, `required field "token"`},
		{"a field neither member declares", "TokenSecret", `{"id":"a","token":"t","x":1}`, `"x" is not in the specification`},
		{"null where null is an alternative", "Holder", `{"one":null,"any":null}`, ""},
		{"the object where it is an alternative", "Holder", `{"one":{"id":"a"},"any":{"id":"a"}}`, ""},
		{"neither alternative", "Holder", `{"one":{"id":7}}`, "is not string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems, err := spec.validateAgainst(tc.schema, []byte(tc.body))
			if err != nil {
				t.Fatalf("%v", err)
			}
			got := strings.Join(problems, "; ")
			if (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// check judges a node with a oneOf or an anyOf by its alternatives alone. A node that declares a
// type, properties, items, an enum or required names beside them would have those ignored, so the
// contract may hold no such node - directly or once an allOf is composed - until check judges both.
func TestNoSchemaDeclaresAlternativesBesideItsOwnShape(t *testing.T) {
	spec := contractSpec(t)
	routes, err := spec.Routes("")
	if err != nil {
		t.Fatalf("%v", err)
	}
	seen := map[*schema]bool{}
	var visit func(where string, node *schema)
	visit = func(where string, node *schema) {
		if node == nil || seen[node] {
			return
		}
		seen[node] = true
		whole := node
		if len(node.AllOf) > 0 {
			composed, problems := spec.composed(node, 0)
			if len(problems) > 0 {
				t.Errorf("%s: %v", where, problems)
				return
			}
			whole = composed
		}
		if alternativesBesideAShape(whole) {
			t.Errorf("%s declares a oneOf or anyOf beside a shape of its own, which the validator would not judge", where)
		}
		for name, property := range node.Properties {
			visit(where+"."+name, property)
		}
		visit(where+"[]", node.Items)
		visit(where+".*", additionalSchema(node))
		for i, member := range slices.Concat(node.AllOf, node.OneOf, node.AnyOf) {
			visit(fmt.Sprintf("%s|%d", where, i), member)
		}
	}
	for name, node := range spec.Components.Schemas {
		visit(name, node)
	}
	for name, declared := range spec.Components.Responses {
		for mediaType, content := range declared.Content {
			visit("the response "+name+" "+mediaType, content.Schema)
		}
	}
	for route, op := range routes {
		for status, declared := range op.Responses {
			for mediaType, content := range declared.Content {
				visit(route+" "+status+" "+mediaType, content.Schema)
			}
		}
		if op.RequestBody != nil {
			for mediaType, content := range op.RequestBody.Content {
				visit(route+" request "+mediaType, content.Schema)
			}
		}
	}
	if len(seen) < 100 {
		t.Fatalf("visited %d schemas - the walk misses the contract", len(seen))
	}
}

func TestAlternativesBesideAShapeAreSeen(t *testing.T) {
	alternatives := []*schema{{Type: "string"}, {Type: "null"}}
	for name, tc := range map[string]struct {
		node *schema
		want bool
	}{
		"alternatives alone":         {&schema{OneOf: alternatives}, false},
		"a shape alone":              {&schema{Type: "object"}, false},
		"a oneOf beside a type":      {&schema{Type: "object", OneOf: alternatives}, true},
		"an anyOf beside properties": {&schema{AnyOf: alternatives, Properties: map[string]*schema{"a": {}}}, true},
		"a oneOf beside required":    {&schema{OneOf: alternatives, Required: []string{"a"}}, true},
	} {
		if got := alternativesBesideAShape(tc.node); got != tc.want {
			t.Errorf("%s: got %t, want %t", name, got, tc.want)
		}
	}
}
