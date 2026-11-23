// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A difference of the platform file from the reference outside the environment, named by the mark
// its header gives it. The reference rewritten by every difference is the platform file - key for
// key, service for service; anything else is drift.
type platformDifference struct {
	mark    string
	service string // the reference's service name; "" for every service that sets the key
	key     string
	rewrite func(reference string) string // "" in and out stands for the key being absent
}

var (
	requiredWithMessage = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*):\?[^}]*\}`)
	dependsOnDatabase   = regexp.MustCompile(`(?m)^      db:$`)
	mountedVolume       = regexp.MustCompile(`(?m)^(\s*- )([a-z][a-z0-9-]*):`)
	declaredVolume      = regexp.MustCompile(`(?m)^(  )([a-z][a-z0-9-]*):`)
)

// The volumes under the walked installation's names [C9].
var platformVolumeNames = map[string]string{
	"db-data":       "hubtask-db",
	"media":         "hubtask-media",
	"backups":       "hubtask-backups",
	"restore-drill": "hubtask-restore-drill",
	"upgrade-dumps": "hubtask-upgrade-dumps",
}

func renameBy(pattern *regexp.Regexp) func(string) string {
	return func(value string) string {
		return pattern.ReplaceAllStringFunc(value, func(m string) string {
			parts := pattern.FindStringSubmatch(m)
			if name, ok := platformVolumeNames[parts[2]]; ok {
				return parts[1] + name + ":"
			}
			return m
		})
	}
}

func requiredWithoutMessage(value string) string {
	return requiredWithMessage.ReplaceAllString(value, "$${${1}:?}")
}

// The marked differences outside the environment, each with the mark that explains it.
var platformShapeDifferences = []platformDifference{
	{"[C1]", "app", "ports", func(string) string { return "" }},
	{"[C3]", "upgrade-dump", "exclude_from_hc", func(string) string { return " true" }},
	{"[C3]", "migrate", "exclude_from_hc", func(string) string { return " true" }},
	{"[C4]", "", "depends_on", func(v string) string { return dependsOnDatabase.ReplaceAllString(v, "      postgres:") }},
	{"[C7]", "migrate", "image", requiredWithoutMessage},
	{"[C7]", "app", "image", requiredWithoutMessage},
	{"[C9]", "", "volumes", renameBy(mountedVolume)},
}

// What a mark on an environment line allows its value to be: the reference's value rewritten by
// the marks the line carries, in their order, and nothing else - a literal in place of the
// variable fails.
var platformEnvironmentRewrites = map[string]func(string) string{
	"[C2]": strings.NewReplacer(
		"${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}", "${SERVICE_PASSWORD_64_PGOWNER}",
		"${POSTGRES_PASSWORD}", "${SERVICE_PASSWORD_64_PGOWNER}",
		"${HUBTASK_DB_APP_PASSWORD:?HUBTASK_DB_APP_PASSWORD must be set}", "${SERVICE_PASSWORD_64_PGAPP}",
		"${HUBTASK_DB_APP_PASSWORD}", "${SERVICE_PASSWORD_64_PGAPP}",
		"${HUBTASK_SECRET_KEY:?HUBTASK_SECRET_KEY must be set}", "${SERVICE_PASSWORD_64_HUBTASKSECRET}",
	).Replace,
	"[C4]": func(v string) string {
		if v == "db" {
			return "postgres"
		}
		return strings.ReplaceAll(v, "@db:5432/", "@postgres:5432/")
	},
	"[C6]": func(v string) string {
		return strings.ReplaceAll(v, "${HUBTASK_TRUSTED_PROXIES:-}", "${HUBTASK_TRUSTED_PROXIES:?}")
	},
	"[C7]": requiredWithoutMessage,
}

// Every key a service of the reference sets other than its environment - health check, restart,
// read-only, security options, tmpfs, shared memory, entrypoint, dependencies with their condition,
// volumes, whatever it is - is set the same in the platform file, and nothing more, unless the
// difference is one of platformShapeDifferences. The environment is held line by line elsewhere.
func TestThePlatformComposeFileSetsWhatTheReferenceSets(t *testing.T) {
	reference := composeServices(t, referenceCompose)
	platform := composeServices(t, platformCompose)
	used := map[int]bool{}

	for name, refBlock := range reference {
		platName, differs := platformServiceDifferences[name]
		if !differs {
			platName = name
		}
		if platName == "" {
			continue
		}
		ref, plat := serviceKeys(refBlock), serviceKeys(platform[platName])
		if len(ref) < 3 {
			t.Fatalf("the reference's %s yielded %d keys - the parsing has stopped matching", name, len(ref))
		}
		for _, key := range unionKeys(ref, plat) {
			if key == "environment" {
				continue
			}
			want, set := ref[key]
			if !set {
				want = ""
			}
			for i, d := range platformShapeDifferences {
				if d.key == key && (d.service == "" || d.service == name) {
					if got := d.rewrite(want); got != want {
						used[i] = true
						want = got
					}
				}
			}
			got, has := plat[key]
			switch {
			case has && got == want:
			case !has && want == "":
			case !has:
				t.Errorf("%s: %s does not set %s, which the reference sets as %q", platformCompose, platName, key, want)
			case want == "":
				t.Errorf("%s: %s sets %s, which the reference does not", platformCompose, platName, key)
			default:
				t.Errorf("%s: %s sets %s as %q, the reference as %q - align it, or name the difference "+
					"with its mark in platformShapeDifferences and the file's header", platformCompose, platName, key, got, want)
			}
		}
	}
	for i, d := range platformShapeDifferences {
		if !used[i] {
			t.Errorf("the difference %s on %q %s changes nothing - the list has drifted from the files", d.mark, d.service, d.key)
		}
	}

	// The top level: the volumes the services mount, renamed [C9], and no project name [C8].
	refTop, platTop := topLevelKeys(t, referenceCompose), topLevelKeys(t, platformCompose)
	if want := renameBy(declaredVolume)(refTop["volumes"]); platTop["volumes"] != want {
		t.Errorf("%s declares the volumes %q, the reference's renamed by [C9] %q", platformCompose, platTop["volumes"], want)
	}
	if _, ok := platTop["name"]; ok {
		t.Errorf("%s sets a top-level name: the platform names the project itself [C8]", platformCompose)
	}
	for key := range platTop {
		if _, ok := refTop[key]; !ok {
			t.Errorf("%s sets the top-level %s, which the reference does not", platformCompose, key)
		}
	}

	header := platformHeaderMarks(t)
	for _, d := range platformShapeDifferences {
		if !header[d.mark] {
			t.Errorf("the difference %s on %q %s is not explained in %s's header", d.mark, d.service, d.key, platformCompose)
		}
	}
	for mark := range platformEnvironmentRewrites {
		if !header[mark] {
			t.Errorf("the environment mark %s is not explained in %s's header", mark, platformCompose)
		}
	}
}

// Each marked environment line holds exactly the form its marks allow: a required variable
// ${NAME:?}, a generated secret, the database's service name - never a literal in their place.
func TestEveryMarkedPlatformEnvironmentLineHoldsTheFormItsMarksAllow(t *testing.T) {
	reference := composeServices(t, referenceCompose)
	platform := composeServices(t, platformCompose)
	mark := regexp.MustCompile(`\[C\d+\]`)
	marked := 0
	for name, refBlock := range reference {
		platName, differs := platformServiceDifferences[name]
		if !differs {
			platName = name
		}
		if platName == "" {
			continue
		}
		ref, plat := serviceEnvironment(refBlock), serviceEnvironment(platform[platName])
		for key, line := range plat {
			if !line.marked {
				continue
			}
			marked++
			refLine, ok := ref[key]
			if !ok {
				continue // the environment test reports a variable the reference does not pass
			}
			want := refLine.value
			for _, m := range mark.FindAllString(line.comment, -1) {
				rewrite, ok := platformEnvironmentRewrites[m]
				if !ok {
					t.Errorf("%s: %s marks %s %s, a mark that allows no environment value", platformCompose, platName, key, m)
					continue
				}
				want = rewrite(want)
			}
			if line.value != want {
				t.Errorf("%s: %s passes %s as %q; its marks %s allow exactly %q", platformCompose, platName, key,
					line.value, strings.TrimSpace(line.comment), want)
			}
			if want == refLine.value {
				t.Errorf("%s: %s marks %s, which its marks leave as the reference's %q", platformCompose, platName, key, want)
			}
		}
	}
	if marked < 8 {
		t.Fatalf("%s yielded %d marked environment lines - the parsing has stopped matching", platformCompose, marked)
	}
}

// serviceKeys reads the keys of one service block at four spaces, each with its value: the rest of
// its line and the lines under it, comments and blank lines off - except inside a block scalar, a
// script, which is kept as written.
func serviceKeys(block string) map[string]string {
	_, body, _ := strings.Cut(block, "\n")
	return yamlKeys(body, 4)
}

func topLevelKeys(t *testing.T, file string) map[string]string {
	t.Helper()
	keys := yamlKeys(string(readFile(t, file)), 0)
	if _, ok := keys["services"]; !ok {
		t.Fatalf("%s yielded no top-level services - the parsing has stopped matching", file)
	}
	return keys
}

func yamlKeys(text string, indent int) map[string]string {
	keyLine := regexp.MustCompile(`^` + strings.Repeat(" ", indent) + `([a-z_]+):(.*)$`)
	keys := map[string]string{}
	current := ""
	var lines []string
	scalarIndent := -1
	flush := func() {
		if current != "" {
			keys[current] = strings.Join(lines, "\n")
		}
	}
	for _, line := range strings.Split(text, "\n") {
		lead := len(line) - len(strings.TrimLeft(line, " "))
		if scalarIndent >= 0 {
			if strings.TrimSpace(line) == "" || lead > scalarIndent {
				lines = append(lines, strings.TrimRight(line, " "))
				continue
			}
			scalarIndent = -1
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if before, _, found := strings.Cut(line, " #"); found {
			line = before
		}
		line = strings.TrimRight(line, " ")
		switch m := keyLine.FindStringSubmatch(line); {
		case m != nil:
			flush()
			current, lines = m[1], []string{strings.TrimRight(m[2], " ")}
		case lead <= indent:
			flush()
			current, lines = "", nil
			continue
		default:
			lines = append(lines, line)
		}
		if strings.HasSuffix(line, "|") || strings.HasSuffix(line, "|-") {
			scalarIndent = lead
		}
	}
	flush()
	return keys
}

func unionKeys(a, b map[string]string) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// platformHeaderMarks names the marks the platform file's header explains, `#   [Cn] ...`.
func platformHeaderMarks(t *testing.T) map[string]bool {
	t.Helper()
	header, _, _ := strings.Cut(string(readFile(t, platformCompose)), "\nservices:\n")
	marks := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^#   (\[C\d+\]) `).FindAllStringSubmatch(header, -1) {
		marks[m[1]] = true
	}
	return marks
}
