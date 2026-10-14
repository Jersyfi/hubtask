// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Jersyfi/hubtask/presentation/rest"
)

// credentialFields are the answer fields that carry a credential shown once (security.md §8). `code`
// counts only at the top of an answer, where it is an OAuth authorization code; nested, it is a
// finding's or a rule's code.
var credentialFields = []string{
	"token", "secret", "client_secret", "access_token", "refresh_token", "pending_token",
	"step_up_token", "recovery_codes", "start_password", "owner_redemption_token", "invitation_link",
	"otpauth_uri",
}

// Every operation whose successful answer carries a credential keeps no body in its idempotency
// record (T-18): it is in rest.ShownOnceRoutes, or its handler marks the answers that carry one
// (rest.ShownOnceByHandler), or it is named with the reason its record still keeps it
// (rest.ShownOnceKept). A new route that answers a token and is in neither is red here - and
// an entry the router does not serve is a line that never matches.
func TestEveryCredentialAnswerKeepsNoBodyForAReplay(t *testing.T) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("reading the specification: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the specification: %v", err)
	}
	schemas, _ := dig(doc, "components", "schemas").(map[string]any)
	paths, _ := doc["paths"].(map[string]any)

	found := 0
	for path, item := range paths {
		methods, _ := item.(map[string]any)
		for method, op := range methods {
			if !slices.Contains(httpMethods, method) {
				continue
			}
			responses, _ := dig(op, "responses").(map[string]any)
			var carried []string
			for status, response := range responses {
				if !strings.HasPrefix(status, "2") {
					continue
				}
				content, _ := dig(response, "content").(map[string]any)
				for _, media := range content {
					carried = append(carried, credentials(dig(media, "schema"), schemas, 0, map[string]bool{})...)
				}
			}
			if len(carried) == 0 {
				continue
			}
			found++
			route := strings.ToUpper(method) + " " + rest.APIBasePath + path
			if !rest.ShownOnceRoutes[route] && rest.ShownOnceByHandler[route] == "" && rest.ShownOnceKept[route] == "" {
				t.Errorf("%s answers %v, and its idempotency record would keep it: add it to rest.ShownOnceRoutes",
					route, carried)
			}
		}
	}
	if found == 0 {
		t.Fatal("no answer carries a credential - the walk no longer matches the specification")
	}

	served := rest.NewRestController().Routes().Routes()
	for route := range rest.ShownOnceRoutes {
		if !slices.Contains(served, route) {
			t.Errorf("rest.ShownOnceRoutes names %q, which the router does not serve", route)
		}
	}
	for route := range rest.ShownOnceKept {
		if !slices.Contains(served, route) {
			t.Errorf("rest.ShownOnceKept names %q, which the router does not serve", route)
		}
	}
	for route := range rest.ShownOnceByHandler {
		if !slices.Contains(served, route) {
			t.Errorf("rest.ShownOnceByHandler names %q, which the router does not serve", route)
		}
	}
}

// credentials walks a schema - references, compositions, arrays and nested objects - and answers the
// credential fields it declares.
func credentials(node any, schemas map[string]any, depth int, seen map[string]bool) []string {
	s, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	if ref, ok := s["$ref"].(string); ok {
		name := ref[strings.LastIndex(ref, "/")+1:]
		if seen[name] {
			return nil
		}
		seen[name] = true
		defer delete(seen, name)
		return credentials(schemas[name], schemas, depth, seen)
	}
	var out []string
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		list, _ := s[key].([]any)
		for _, member := range list {
			out = append(out, credentials(member, schemas, depth, seen)...)
		}
	}
	out = append(out, credentials(s["items"], schemas, depth, seen)...)
	properties, _ := s["properties"].(map[string]any)
	for name, property := range properties {
		if slices.Contains(credentialFields, name) || (name == "code" && depth == 0) {
			out = append(out, name)
		}
		out = append(out, credentials(property, schemas, depth+1, seen)...)
	}
	return out
}

func dig(node any, keys ...string) any {
	for _, key := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		node = m[key]
	}
	return node
}
