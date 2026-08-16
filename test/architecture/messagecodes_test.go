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
	} {
		key := "errors." + err.Code
		if _, ok := messages[key]; !ok {
			t.Errorf("the message code %s is missing from locales/en.json", key)
		}
	}
}

// messageCode matches a string literal that looks like a message code of the configuration, the
// error model, or the health report's degradation reasons. Narrow on purpose: only the prefixes
// that exist today, so that an example in a test comment does not turn into a false alarm.
var messageCode = regexp.MustCompile(`"((?:config|errors|dependency)\.[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*)"`)

// TestEveryUsedMessageCodeIsInTheCatalogue reads the source rather than a registry: a code is
// used where it is written, and a registry would only be a second place to forget.
func TestEveryUsedMessageCodeIsInTheCatalogue(t *testing.T) {
	messages := loadCatalogue(t)
	used := map[string][]string{}

	forEachGoFile(t, []string{"../../core", "../../infrastructure", "../../presentation", "../../cmd"},
		func(path string, _ *ast.File, _ *token.FileSet) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("%s is not readable: %v", rel(path), err)
				return
			}
			for _, match := range messageCode.FindAllStringSubmatch(string(source), -1) {
				used[match[1]] = append(used[match[1]], rel(path))
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
		if !strings.Contains(source, `"`+needle+`"`) {
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

func readAllSources(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, root := range []string{"../../core", "../../infrastructure", "../../presentation", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
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
