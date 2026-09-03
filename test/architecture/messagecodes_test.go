// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const catalogue = "../../locales/en.json"

// TestEveryContractCodeIsInTheCatalogue covers the Definition of Done item "message codes are in
// locales/en.json". A code without an entry is not a small omission: the client has nothing to
// render, and the fallback is the key itself (i18n-l10n.md §3).
func TestEveryContractCodeIsInTheCatalogue(t *testing.T) {
	messages := loadCatalogue(t)

	// The sentinels of the error model. Listed explicitly, because that is the point: adding a
	// sentinel without a message should turn this test red.
	for _, err := range []*shared.Error{
		shared.ErrValidation, shared.ErrMalformedRequest, shared.ErrUnauthenticated,
		shared.ErrForbidden, shared.ErrNotFound, shared.ErrConflict, shared.ErrVersionConflict,
		shared.ErrGone, shared.ErrRateLimited, shared.ErrUnavailable, shared.ErrInternal,
		shared.ErrCapabilityNotSupported,
	} {
		key := "errors." + err.Code
		if _, ok := messages[key]; !ok {
			t.Errorf("the message code %s is missing from locales/en.json", key)
		}
	}

	// The contract codes with no domain error behind them: the router answers for itself when a
	// request reaches no route at all (presentation/rest/Router.go). They are part of the same
	// contract and need the same catalogue entry.
	for _, code := range []string{"method_not_allowed", "payload_too_large"} {
		key := "errors." + code
		if _, ok := messages[key]; !ok {
			t.Errorf("the message code %s is missing from locales/en.json", key)
		}
	}
}

// messageCode matches a string literal that looks like a message code of the configuration, the
// error model, the health report's degradation reasons, or the load shedder's capacity refusals.
// Narrow on purpose: only the prefixes that exist today, so that an example in a test comment
// does not turn into a false alarm.
var messageCode = regexp.MustCompile(`"((?:route|request|access|accounts|auth|oauth|admin|seed|groups|memberships|idempotency|config|errors|dependency|capacity|containers|items|buckets|labels|comments|fields|media|ordering|events|sync|storage|audit|usecase|shared|automation|lifecycle|activity|query|views|notifications|email|mail|jobs|crypto|calendar|backup)\.[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*)"`)

// TestEveryUsedMessageCodeIsInTheCatalogue reads the source rather than a registry: a code is
// used where it is written, and a registry would only be a second place to forget.
//
// Test files are left out. A code in a fixture is an example of the shape, not an answer any
// client will ever receive - Problem_test.go names `containers.not_found` to prove that a detail
// code reaches the wire, and demanding a catalogue entry for it would put invented sentences into
// the source language.
func TestEveryUsedMessageCodeIsInTheCatalogue(t *testing.T) {
	messages := loadCatalogue(t)
	used := map[string][]string{}

	forEachGoFile(t, []string{"../../core", "../../infrastructure", "../../presentation", "../../cmd"},
		func(path string, _ *ast.File, _ *token.FileSet) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			source, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("%s is not readable: %v", rel(path), err)
				return
			}
			for _, line := range strings.Split(string(source), "\n") {
				// An audit action is not a message code: nothing renders it, an auditor filters on
				// it (audit.md §2). Neither is a job kind: nothing renders that either, it is the
				// label of a queue metric (queue.Kind). The two namespaces only collide where a
				// resource's name is the same in the singular and the plural - `media.staged` and
				// `media.reconcile` beside `media.not_found` - which every other entity avoids by
				// accident rather than by design.
				// `port.Action` is the same declaration seen from inside the application's own
				// audit package, which imports the port under an alias because the two share a
				// name (core/application/service/audit, E-09).
				if strings.Contains(line, "audit.Action") || strings.Contains(line, "port.Action") ||
					strings.Contains(line, "Kind = \"") {
					continue
				}
				for _, match := range messageCode.FindAllStringSubmatch(line, -1) {
					used[match[1]] = append(used[match[1]], rel(path))
				}
			}
		})

	if len(used) == 0 {
		t.Fatal("no message code found at all - the pattern no longer matches")
	}
	for code, files := range used {
		if _, ok := messages[code]; !ok {
			t.Errorf("the message code %s (%s) is missing from locales/en.json",
				code, strings.Join(files, ", "))
		}
	}
}

// The other direction is a warning in CI, not an error (i18n-l10n.md §3) - a code may be
// prepared before its use lands. Reported so it does not rot unnoticed.
func TestUnusedCatalogueEntriesAreReported(t *testing.T) {
	messages := loadCatalogue(t)
	source := readAllSources(t)

	for key := range messages {
		if strings.HasPrefix(key, "_") {
			continue
		}
		// The error sentinels appear as bare codes in the source (validation_failed), not as
		// the catalogue key (errors.validation_failed).
		needle := key
		if bare, ok := strings.CutPrefix(key, "errors."); ok {
			needle = bare
		}
		// Both quote styles: Go writes "route.unknown" and the client writes 'route.unknown'.
		if !strings.Contains(source, `"`+needle+`"`) && !strings.Contains(source, `'`+needle+`'`) {
			t.Logf("note: %s is in the catalogue but used nowhere", key)
		}
	}
}

// A parameter in a message has to be filled by somebody. This catches the mismatch that produces
// a literal {variable} in a user's face.
func TestCataloguePlaceholdersAreWellFormed(t *testing.T) {
	placeholder := regexp.MustCompile(`\{([^}]*)\}`)

	for key, message := range loadCatalogue(t) {
		if strings.HasPrefix(key, "_") {
			continue
		}
		if strings.TrimSpace(message) == "" {
			t.Errorf("%s has an empty message", key)
		}
		if strings.Count(message, "{") != strings.Count(message, "}") {
			t.Errorf("%s has unbalanced braces: %q", key, message)
		}
		for _, m := range placeholder.FindAllStringSubmatch(message, -1) {
			name := m[1]
			if name == "" || strings.ContainsAny(name, " \t") {
				t.Errorf("%s has a malformed placeholder %q", key, m[0])
			}
		}
	}
}

func loadCatalogue(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(catalogue)
	if err != nil {
		t.Fatalf("locales/en.json is not readable: %v", err)
	}
	var messages map[string]string
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatalf("locales/en.json is not valid JSON: %v", err)
	}
	return messages
}

// readAllSources reads everything that can use a message code - which since F1-07 is both halves
// of the product. The client renders the same catalogue from the same file
// (apps/webapp/src/lib/i18n/catalogue.ts), so a key used only there is used, and a report that
// called it unused would be a report nobody trusts. Reading the client's source is all this does;
// nothing here builds it, and `go test ./...` still runs in a checkout where Node never was.
var codeUsingSources = []string{
	"../../core", "../../infrastructure", "../../presentation", "../../cmd", "../../apps/webapp/src",
}

func isCodeUsingSource(path string) bool {
	for _, extension := range []string{".go", ".ts", ".svelte"} {
		if strings.HasSuffix(path, extension) {
			return true
		}
	}
	return false
}

func readAllSources(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, root := range codeUsingSources {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !isCodeUsingSource(path) {
				return err
			}
			source, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			b.Write(source)
			return nil
		})
		if err != nil {
			t.Fatalf("directory %s is not readable: %v", root, err)
		}
	}
	return b.String()
}

// The history's verbs are message codes too, and derived ones: `item.completed` is stored and
// `activity.item_completed` is what a client renders (i18n-l10n.md §1). Derived means the literal
// never appears in the source, so the check above cannot see them - and a verb whose key is
// missing would reach a user as the key itself.
func TestEveryActivityVerbHasItsMessage(t *testing.T) {
	messages := loadCatalogue(t)

	for _, verb := range activity.Verbs() {
		if _, ok := messages[verb.MessageCode()]; !ok {
			t.Errorf("the history verb %s has no message %s in locales/en.json",
				verb, verb.MessageCode())
		}
	}
}
