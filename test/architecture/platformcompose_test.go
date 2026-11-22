// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"regexp"
	"strings"
	"testing"
)

// deploy/coolify/compose.yaml is the reference stack arranged for one deployment platform, and no
// gate starts it (deployment.md §2.1): what holds it is this comparison with the reference, which
// does start. A difference the platform needs is marked [Cn] on its line; any other is drift.
const (
	referenceCompose = "../../deploy/docker/compose.yaml"
	platformCompose  = "../../deploy/coolify/compose.yaml"
)

// The services the platform file deliberately does not carry as the reference does: the database
// under the name the walked installation used, and no operator's tools.
var platformServiceDifferences = map[string]string{
	"db":  "postgres", // [C4]
	"ops": "",         // [C5]
}

// Every environment line the reference's services pass is passed here with the same value, and
// nothing else is - unless the line carries its [Cn] mark. The list is the reference itself.
func TestThePlatformComposeFilePassesWhatTheReferencePasses(t *testing.T) {
	reference := composeServices(t, referenceCompose)
	platform := composeServices(t, platformCompose)

	for name := range reference {
		want, differs := platformServiceDifferences[name]
		if !differs {
			want = name
		}
		if _, ok := platform[want]; want != "" && !ok {
			t.Errorf("%s has no service %s, which the reference runs as %s", platformCompose, want, name)
		}
	}
	for name := range platform {
		if _, ok := reference[name]; !ok && name != platformServiceDifferences["db"] {
			t.Errorf("%s runs %s, which the reference does not", platformCompose, name)
		}
	}

	for _, service := range []string{"db", "upgrade-dump", "migrate", "app"} {
		ref, plat := serviceEnvironment(reference[service]), serviceEnvironment(platform[service])
		if service == "db" {
			plat = serviceEnvironment(platform[platformServiceDifferences["db"]])
		}
		if len(ref) < 3 {
			t.Fatalf("the reference's %s yielded %d variables - the parsing has stopped matching", service, len(ref))
		}
		for key, value := range ref {
			line, ok := plat[key]
			switch {
			case !ok:
				t.Errorf("%s: %s does not pass %s, which the reference passes", platformCompose, service, key)
			case line.value != value.value && !line.marked:
				t.Errorf("%s: %s passes %s as %q, the reference as %q - align it, or mark the "+
					"difference [Cn] and say why in the file's header", platformCompose, service, key,
					line.value, value.value)
			}
		}
		for key := range plat {
			if _, ok := ref[key]; !ok {
				t.Errorf("%s: %s passes %s, which the reference does not", platformCompose, service, key)
			}
		}
	}

	app := serviceEnvironment(platform["app"])
	if got := app["HUBTASK_TRUSTED_PROXIES"].value; got != "${HUBTASK_TRUSTED_PROXIES:?}" {
		t.Errorf("app passes HUBTASK_TRUSTED_PROXIES as %q: every request arrives through the "+
			"platform's proxy, so the variable is required", got)
	}
}

// The shape the platform needs: no published port, the one-shots out of its health check, the
// dump script byte for byte the reference's, and a release that has to be chosen.
func TestThePlatformComposeFileKeepsItsMarkedShape(t *testing.T) {
	reference := composeServices(t, referenceCompose)
	platform := composeServices(t, platformCompose)

	for name, block := range platform {
		if regexp.MustCompile(`(?m)^    ports:`).MatchString(block) {
			t.Errorf("%s publishes ports on %s: the platform's proxy reaches the service itself", platformCompose, name)
		}
		excluded := regexp.MustCompile(`(?m)^    exclude_from_hc: true`).MatchString(block)
		oneShot := regexp.MustCompile(`(?m)^    restart: "no"`).MatchString(block)
		if excluded != oneShot {
			t.Errorf("%s: %s is one-shot %v and out of the health check %v - exactly the one-shots are "+
				"taken out", platformCompose, name, oneShot, excluded)
		}
		for _, m := range regexp.MustCompile(`(?m)^    image:\s*\$\{HUBTASK_IMAGE[^}]*\}:(\S+)`).FindAllStringSubmatch(block, -1) {
			if !strings.HasPrefix(m[1], "${HUBTASK_VERSION:?") {
				t.Errorf("%s: %s tags the Hubtask image %q, not a required release", platformCompose, name, m[1])
			}
		}
	}

	entrypoint := regexp.MustCompile(`(?s)\n    entrypoint:\n.*?\n    volumes:`)
	want := entrypoint.FindString(reference["upgrade-dump"])
	if want == "" {
		t.Fatal("the reference's upgrade-dump has no entrypoint before its volumes - the pattern has stopped matching")
	}
	if got := entrypoint.FindString(platform["upgrade-dump"]); got != want {
		t.Errorf("%s: upgrade-dump's entrypoint is not the reference's byte for byte", platformCompose)
	}
}

// A DSN and a PGHOST name the database service of their own file - `db` in the reference, the
// platform file's own name there. A name test alone passes a file that starts nothing, so the
// values are read.
func TestEveryComposeDatabaseAddressNamesItsOwnDatabaseService(t *testing.T) {
	dsnHost := regexp.MustCompile(`@([a-z0-9-]+):5432/`)
	for _, file := range []string{referenceCompose, platformCompose} {
		services := composeServices(t, file)
		database := databaseService(t, file, services)
		for _, service := range []string{"upgrade-dump", "migrate", "app", "ops"} {
			block, ok := services[service]
			if !ok {
				continue
			}
			env := serviceEnvironment(block)
			hosts := 0
			if v, ok := env["PGHOST"]; ok {
				hosts++
				if v.value != database {
					t.Errorf("%s: %s sets PGHOST %q, and the database service is %s", file, service, v.value, database)
				}
			}
			if v, ok := env["HUBTASK_DB_DSN"]; ok {
				hosts++
				if m := dsnHost.FindStringSubmatch(v.value); m == nil || m[1] != database {
					t.Errorf("%s: %s's HUBTASK_DB_DSN names host %v, and the database service is %s", file, service, m, database)
				}
			}
			if hosts == 0 {
				t.Errorf("%s: %s names no database host at all", file, service)
			}
		}
	}
}

// The password inside a DSN is the one its role was given: the owner's DSN carries the database's
// POSTGRES_PASSWORD (and the dump's PGPASSWORD), the application's DSN the password the migrator
// grants hubtask_app. In the platform file each is a generated SERVICE_PASSWORD_64_* value - the
// form the walked installation ran with in its DSNs (what the platform generates is stated in
// deployment.md §2.1, and is not verifiable from here).
func TestEveryComposeDSNCarriesThePasswordItsRoleWasGiven(t *testing.T) {
	variable := regexp.MustCompile(`^\$\{([A-Z][A-Z0-9_]*)`)
	password := regexp.MustCompile(`^postgres://(?:\$\{[^}]*\}|[^:$@]+):(\$\{[^}]*\})@`)
	secret := func(value string) string {
		if m := variable.FindStringSubmatch(value); m != nil {
			return m[1]
		}
		return ""
	}
	for _, file := range []string{referenceCompose, platformCompose} {
		services := composeServices(t, file)
		db := serviceEnvironment(services[databaseService(t, file, services)])
		dump := serviceEnvironment(services["upgrade-dump"])
		migrate := serviceEnvironment(services["migrate"])
		app := serviceEnvironment(services["app"])

		dsnSecret := func(env map[string]composeLine, service string) string {
			m := password.FindStringSubmatch(env["HUBTASK_DB_DSN"].value)
			if m == nil {
				t.Errorf("%s: %s's HUBTASK_DB_DSN carries no ${...} password", file, service)
				return ""
			}
			return secret(m[1])
		}
		owner, application := secret(db["POSTGRES_PASSWORD"].value), secret(migrate["HUBTASK_DB_APP_PASSWORD"].value)
		if owner == "" || application == "" || owner == application {
			t.Errorf("%s: the owner's password %q and the application role's %q must be two variables", file, owner, application)
		}
		if got := dsnSecret(migrate, "migrate"); got != owner {
			t.Errorf("%s: migrate's DSN carries %s, and the database's owner was given %s", file, got, owner)
		}
		if got := secret(dump["PGPASSWORD"].value); got != owner {
			t.Errorf("%s: upgrade-dump's PGPASSWORD is %s, and the database's owner was given %s", file, got, owner)
		}
		if got := dsnSecret(app, "app"); got != application {
			t.Errorf("%s: app's DSN carries %s, and the migrator grants hubtask_app %s", file, got, application)
		}
		if file == platformCompose {
			for _, name := range []string{owner, application, secret(app["HUBTASK_SECRET_KEY"].value)} {
				if !strings.HasPrefix(name, "SERVICE_PASSWORD_64_") {
					t.Errorf("%s: %s is not one of the platform's generated SERVICE_PASSWORD_64_* values", file, name)
				}
			}
		}
	}
}

type composeLine struct {
	value  string
	marked bool // the line carries a [Cn] mark
}

// composeServices cuts a compose file into its services' blocks, by indentation: a name at two
// spaces under `services:`, up to the next one or the next top-level key.
func composeServices(t *testing.T, file string) map[string]string {
	t.Helper()
	text := string(readFile(t, file))
	start := strings.Index(text, "\nservices:\n")
	if start < 0 {
		t.Fatalf("%s has no services", file)
	}
	text = text[start+len("\nservices:\n"):]
	if end := regexp.MustCompile(`(?m)^[a-z]`).FindStringIndex(text); end != nil {
		text = text[:end[0]]
	}
	header := regexp.MustCompile(`(?m)^  ([a-z][a-z0-9-]*):`)
	matches := header.FindAllStringSubmatchIndex(text, -1)
	services := map[string]string{}
	for i, m := range matches {
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		services[text[m[2]:m[3]]] = text[m[0]:end]
	}
	if len(services) < 4 {
		t.Fatalf("%s yielded %d services - the parsing has stopped matching", file, len(services))
	}
	return services
}

// serviceEnvironment reads the `environment:` map of one service block, a trailing comment off.
func serviceEnvironment(block string) map[string]composeLine {
	env := map[string]composeLine{}
	start := strings.Index(block, "\n    environment:\n")
	if start < 0 {
		return env
	}
	assignment := regexp.MustCompile(`^      ([A-Z][A-Z0-9_]*):\s*(.*?)\s*$`)
	for _, line := range strings.Split(block[start+len("\n    environment:\n"):], "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		m := assignment.FindStringSubmatch(line)
		if m == nil {
			if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
				break
			}
			continue
		}
		value, comment, _ := strings.Cut(m[2], " #")
		env[m[1]] = composeLine{
			value:  strings.TrimSpace(value),
			marked: regexp.MustCompile(`\[C\d+\]`).MatchString(comment),
		}
	}
	return env
}

// databaseService names the one service that holds the database's data directory.
func databaseService(t *testing.T, file string, services map[string]string) string {
	t.Helper()
	found := ""
	for name, block := range services {
		if strings.Contains(block, ":/var/lib/postgresql/data") {
			if found != "" {
				t.Fatalf("%s: both %s and %s hold the database's data", file, found, name)
			}
			found = name
		}
	}
	if found == "" {
		t.Fatalf("%s: no service holds the database's data", file)
	}
	return found
}
