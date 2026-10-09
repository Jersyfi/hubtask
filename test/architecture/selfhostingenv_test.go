// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A variable the self-hosting example offers has to arrive somewhere. The operator copies
// .env.example, sets a line, and believes the installation is configured (P-11). Two hops can
// lose it: Compose reads .env only to interpolate compose.yaml - there is deliberately no
// env_file, so a variable compose.yaml does not name never reaches a container - and a container
// variable no code reads changes nothing either. Both are checked against their source rather
// than against a list, because a list kept by hand beside them is how the example went stale.
func TestEveryVariableTheSelfHostingExampleOffersIsRead(t *testing.T) {
	const (
		example = "../../deploy/docker/.env.example"
		compose = "../../deploy/docker/compose.yaml"
	)

	offered := map[string]bool{}
	for _, line := range strings.Split(string(readFile(t, example)), "\n") {
		// Commented lines count: a commented assignment is an offer the operator uncomments.
		if m := regexp.MustCompile(`^#?\s*([A-Z][A-Z0-9_]*)=`).FindStringSubmatch(line); m != nil {
			offered[m[1]] = true
		}
	}
	if len(offered) == 0 {
		t.Fatalf("%s offers no variable at all - the assignment pattern has stopped matching", example)
	}

	// Which .env variables compose.yaml interpolates, and into which container variables.
	interpolated := map[string]bool{}
	passedAs := map[string][]string{}
	reference := regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)`)
	assignment := regexp.MustCompile(`^\s+([A-Z][A-Z0-9_]*):\s*(.*)$`)
	for _, line := range strings.Split(string(readFile(t, compose)), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key := ""
		if m := assignment.FindStringSubmatch(line); m != nil {
			key = m[1]
		}
		for _, m := range reference.FindAllStringSubmatch(line, -1) {
			interpolated[m[1]] = true
			if key != "" {
				passedAs[m[1]] = append(passedAs[m[1]], key)
			}
		}
	}

	read := variablesTheCodeReads(t)
	for name := range offered {
		if !interpolated[name] {
			t.Errorf("%s offers %s, and %s never names it: Compose reads .env only to fill "+
				"compose.yaml, so the value reaches no container - pass it through, or take it out "+
				"of the example", example, name, compose)
			continue
		}
		for _, key := range passedAs[name] {
			// The database image's own variables (POSTGRES_*) are that image's to read.
			if strings.HasPrefix(key, "HUBTASK_") && !read(key) {
				t.Errorf("%s offers %s, which compose.yaml passes as %s, and no code under cmd/ or "+
					"infrastructure/ reads %s", example, name, key, key)
			}
		}
	}
}

// variablesTheCodeReads answers whether a process of this repository reads an environment
// variable: its name as a string literal in non-test code under cmd/ or infrastructure/, or a
// literal ending in "_" that is its prefix (a keyring's HUBTASK_ENCRYPTION_KEY_<ID>).
func variablesTheCodeReads(t *testing.T) func(string) bool {
	t.Helper()
	literal := regexp.MustCompile(`"(HUBTASK_[A-Z0-9_]*)"`)
	names := map[string]bool{}
	var prefixes []string
	for _, root := range []string{"../../cmd", "../../infrastructure"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			for _, m := range literal.FindAllStringSubmatch(string(readFile(t, path)), -1) {
				if strings.HasSuffix(m[1], "_") {
					prefixes = append(prefixes, m[1])
				} else {
					names[m[1]] = true
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if !names["HUBTASK_SECRET_KEY"] {
		t.Fatal("HUBTASK_SECRET_KEY is not among the variables found - the literal pattern has stopped matching")
	}
	return func(name string) bool {
		if names[name] {
			return true
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
		return false
	}
}
