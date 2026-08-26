// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

// PG-6: with no configuration, **no** outbound connection occurs - not even to check for updates
// (ADR-0018 decision 6, data-protection.md §10).
//
// The promise is about the *application*, not about the guard: `GuardedClient` bounds where a
// configured call may go, and an installation that has configured nothing should make none at all.
// What a build can decide about that is where a destination could come from. Every outbound call
// this system makes names a target that arrived as data - a webhook subscription, an automation
// rule, a backup target, an OIDC issuer - and a URL written into the source is the one way a
// destination could exist without anybody configuring it.
//
// So this reads the source for addresses. The exceptions are named with their reason rather than
// pattern-matched away, which is the same discipline `outboundExceptions` in the architecture gate
// keeps: an exception somebody has to write a sentence for is an exception somebody thinks about.

// dialableExceptions are the address literals that exist and are not a phone home.
var dialableExceptions = map[string]string{
	"infrastructure/backupstorage/S3Store.go": "the default S3 endpoint for a region, used only once a target names that region",
	"infrastructure/storage/S3Storage.go":     "the same, for the media store",
	"presentation/rest/Problem.go":            "the documentation address in a problem document: rendered into a response, never dialled",
	"cmd/server/main.go":                      "the process's own health probe against 127.0.0.1, which is how a container asks itself whether it is ready",
	"cmd/hubctl/Calendar.go":                  "a prefix check on an address the person typed",
	"infrastructure/httpclient/Guard.go":      "the cloud metadata address, in a comment, as the thing the guard refuses",
}

func TestPG6NoAddressIsDialledWithoutConfiguration(t *testing.T) {
	forEachGoFile(t, func(path string, file *ast.File, fset *token.FileSet) {
		relative := strings.TrimPrefix(strings.TrimPrefix(path, "../../"), "./")
		if _, allowed := dialableExceptions[relative]; allowed {
			return
		}

		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value := strings.Trim(literal.Value, "`\"")
			if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
				return true
			}
			// A schema or namespace identifier is a name rather than a destination: it appears in
			// a document this system writes and nothing ever fetches it.
			if isIdentifierURL(value) {
				return true
			}

			t.Errorf("%s: the address %q is written into the source (PG-6). Every destination this "+
				"system calls arrives as configuration or as data; if this one is not dialled, "+
				"name it in dialableExceptions with the reason", at(path, fset, literal), value)
			return true
		})
	})
}

// isIdentifierURL recognises the addresses that are names: a JSON Schema dialect, an XML namespace,
// the licence a header points at. Nothing fetches them, and a system that refused to write one
// could not produce a valid document.
func isIdentifierURL(value string) bool {
	for _, identifier := range []string{
		"json-schema.org", "www.w3.org", "schema.org", "spdx.org", "opensource.org",
		"docs.hubtask.dev", "hubtask.eu", "example.org", "example.com",
	} {
		if strings.Contains(value, identifier) {
			return true
		}
	}
	return false
}

// The other half of decision 6 - that a call which *is* made goes through the guard - is rule 6 and
// is held by `test/architecture/outbound_test.go`, with its two documented exceptions: the
// container's health probe against its own loopback, and the operator-configured object storage
// endpoint. PG-6 does not repeat it. Two gates over one rule are two places to weaken it.
