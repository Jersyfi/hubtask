#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# Proves that the gates bite. For every configured rule this script writes a file that breaks
# exactly that rule, runs the gate that is supposed to catch it, and expects a red result.
#
# Why this exists: a rule that is configured but never triggered is indistinguishable from one
# that is silently disabled - a wrong glob pattern in depguard, a linter renamed by an upgrade,
# an exclusion that is too wide. The acceptance criterion of task A-01 is precisely "a deliberate
# violation of every configured rule demonstrably fails the build".
#
# The violating files are written into the work tree (the layer rules are path-dependent, so they
# cannot live in a temporary directory) and are removed again afterwards, including on abort.

set -euo pipefail

cd "$(dirname "$0")/.."

LINT=".tools/golangci-lint"
SCRATCH="gate_selftest"    # a package directory of this name is created and deleted per check
FAILURES=0
CHECKS=0
SKIPPED=0

cleanup() {
	find . -type d -name "$SCRATCH" -not -path './.git/*' -exec rm -rf {} + 2>/dev/null || true
}
trap cleanup EXIT INT TERM
cleanup

# write <package-dir> <file-name> <content>
write() {
	mkdir -p "$1/$SCRATCH"
	printf '%s\n' "$3" > "$1/$SCRATCH/$2"
}

# expect_lint_failure <name> <package-dir> <expected-linter> <content>
# The violation has to be reported by <expected-linter>: a build that goes red for another
# reason - a compile error, say - proves nothing about the rule under test.
expect_lint_failure() {
	local name="$1" dir="$2" linter="$3" content="$4"
	CHECKS=$((CHECKS + 1))

	write "$dir" "selftest.go" "$content"
	local out
	out="$($LINT run "./$dir/$SCRATCH/..." 2>&1 || true)"
	cleanup

	if grep -q "($linter)" <<<"$out"; then
		printf '  ok      %-44s caught by %s\n' "$name" "$linter"
	else
		printf '  FAILED  %-44s not caught by %s\n' "$name" "$linter"
		printf '%s\n' "$out" | sed 's/^/            /'
		FAILURES=$((FAILURES + 1))
	fi
}

# expect_gate_failure <name> <make target> <package-dir> <content>
expect_gate_failure() {
	local name="$1" target="$2" dir="$3" content="$4"
	CHECKS=$((CHECKS + 1))

	write "$dir" "selftest.go" "$content"
	if make --no-print-directory "$target" >/dev/null 2>&1; then
		printf '  FAILED  %-44s make %s stayed green\n' "$name" "$target"
		FAILURES=$((FAILURES + 1))
	else
		printf '  ok      %-44s caught by make %s\n' "$name" "$target"
	fi
	cleanup
}

header() { printf '\n%s\n' "$1"; }

test -x "$LINT" || { echo "$LINT is missing - run 'make tools'"; exit 1; }

header "Layer boundaries (depguard, ADR-0001)"

expect_lint_failure "domain imports infrastructure" core/domain depguard \
'package selftest

import _ "github.com/Jersyfi/hubtask/infrastructure/environment"'

expect_lint_failure "domain imports net/http" core/domain depguard \
'package selftest

import _ "net/http"'

expect_lint_failure "domain imports database/sql" core/domain depguard \
'package selftest

import _ "database/sql"'

expect_lint_failure "domain imports encoding/json" core/domain depguard \
'package selftest

import _ "encoding/json"'

expect_lint_failure "port imports presentation" core/port depguard \
'package selftest

import _ "github.com/Jersyfi/hubtask/presentation/rest"'

expect_lint_failure "core/shared imports infrastructure" core/shared depguard \
'package selftest

import _ "github.com/Jersyfi/hubtask/infrastructure/health"'

expect_lint_failure "application imports infrastructure" core/application depguard \
'package selftest

import _ "github.com/Jersyfi/hubtask/infrastructure/environment"'

expect_lint_failure "application imports net/http" core/application depguard \
'package selftest

import _ "net/http"'

expect_lint_failure "presentation imports infrastructure" presentation depguard \
'package selftest

import _ "github.com/Jersyfi/hubtask/infrastructure/health"'

expect_lint_failure "math/rand anywhere" infrastructure depguard \
'package selftest

import _ "math/rand"'

# T-06 and ADR-0026: the query builder is the one package that assembles SQL, and `fmt` is the
# tool that would put a value into it. The rule is scoped to that package, so the violation has
# to be written there.
expect_lint_failure "fmt in the query builder" infrastructure/postgres/query depguard \
'package selftest

import _ "fmt"'

# Four depguard rules cannot be proven through the linter yet, because the package they forbid
# does not exist: third-party libraries in the core, the database driver outside
# infrastructure/postgres, an adapter importing core/application/service, and the domain
# importing the application layer. golangci-lint reports typecheck for a missing package, which
# would prove nothing about depguard. All four are proven below through the architecture gate,
# which reads the source instead of compiling it.

header "Code rules (golangci-lint)"

expect_lint_failure "unchecked error" infrastructure errcheck \
'package selftest

import "os"

func Selftest() { os.Setenv("HUBTASK_SELFTEST", "1") }'

expect_lint_failure "unchecked type assertion" infrastructure errcheck \
'package selftest

func Selftest(v any) string { return v.(string) }'

expect_lint_failure "wrong Printf verb" infrastructure govet \
'package selftest

import "fmt"

func Selftest() string { return fmt.Sprintf("%d", "not a number") }'

expect_lint_failure "simplifiable code" infrastructure staticcheck \
'package selftest

func Selftest(b bool) bool {
	if b {
		return true
	}
	return false
}'

expect_lint_failure "unused function" infrastructure unused \
'package selftest

func selftestUnused() {}'

expect_lint_failure "ineffectual assignment" infrastructure ineffassign \
'package selftest

func Selftest(in int) int {
	n := in
	n = 2
	n = 3
	return n
}'

expect_lint_failure "HTTP call without a context" infrastructure noctx \
'package selftest

import "net/http"

func Selftest() (*http.Response, error) { return http.Get("http://example.invalid") }'

expect_lint_failure "response body not closed" infrastructure bodyclose \
'package selftest

import (
	"context"
	"net/http"
)

func Selftest(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	if err != nil {
		return err
	}
	_, err = http.DefaultClient.Do(req)
	return err
}'

expect_lint_failure "error compared instead of errors.Is" infrastructure errorlint \
'package selftest

import "os"

func Selftest(err error) bool { return err == os.ErrNotExist }'

expect_lint_failure "weak cryptographic primitive" infrastructure gosec \
'package selftest

import "crypto/md5"

func Selftest(b []byte) []byte {
	sum := md5.Sum(b)
	return sum[:]
}'

expect_lint_failure "dot import" infrastructure revive \
'package selftest

import . "strings"

var Selftest = TrimSpace'

expect_lint_failure "misspelling" infrastructure misspell \
'package selftest

// Selftest recieves nothing.
func Selftest() {}'

expect_lint_failure "suppression without a reason" infrastructure nolintlint \
'package selftest

import "os"

func Selftest() { os.Setenv("HUBTASK_SELFTEST", "1") } //nolint:errcheck'

expect_lint_failure "context not passed on" infrastructure contextcheck \
'package selftest

import (
	"context"
	"net/http"
)

func Selftest(ctx context.Context) error {
	_ = ctx
	return inner()
}

func inner() error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}'

header "Architecture rules (make gate-architecture)"

expect_gate_failure "bare goroutine" gate-architecture core/domain \
'package selftest

func Selftest() { go func() {}() }'

expect_gate_failure "time.Now in the domain" gate-architecture core/domain \
'package selftest

import "time"

func Selftest() time.Time { return time.Now() }'

expect_gate_failure "third-party library in the core" gate-architecture core/port \
'package selftest

import _ "github.com/pressly/goose/v3"'

expect_gate_failure "technology import in the core" gate-architecture core/domain \
'package selftest

import _ "database/sql"'

expect_gate_failure "repository method taking a tenant" gate-architecture core/application/repository \
'package selftest

import "context"

type Selftest interface {
	Read(ctx context.Context, tenantID string) error
}'

expect_gate_failure "adapter calls a use case" gate-architecture infrastructure \
'package selftest

import _ "github.com/Jersyfi/hubtask/core/application/service"'

expect_gate_failure "database driver outside the adapter" gate-architecture core/application \
'package selftest

import _ "github.com/jackc/pgx/v5"'

expect_gate_failure "domain imports the application layer" gate-architecture core/domain \
'package selftest

import _ "github.com/Jersyfi/hubtask/core/application/usecase"'

expect_gate_failure "outbound call without the guard" gate-architecture infrastructure \
'package selftest

import "net/http"

func Selftest() *http.Client { return http.DefaultClient }'

expect_gate_failure "a second HTTP client" gate-architecture infrastructure \
'package selftest

import "net/http"

func Selftest() *http.Client { return &http.Client{} }'

expect_gate_failure "a permission question that does not name its entry" gate-architecture core/application \
'package selftest

import "github.com/Jersyfi/hubtask/core/application/service/access"

const itemTarget = "item"

func Selftest() access.Request { return access.Request{TargetType: itemTarget} }'

expect_gate_failure "the per-entry matrix read in an adapter" gate-architecture infrastructure \
'package selftest

import "github.com/Jersyfi/hubtask/core/domain/service"

func Selftest() service.ItemAccess {
	return service.ItemAccessOf("GUEST", service.ItemChange)
}'

header "Formatting and generation (make gate-quick)"

expect_gate_failure "unformatted source" gate-quick infrastructure \
'package selftest
func  Selftest ()  {}'

expect_gate_failure "import order" gate-quick infrastructure \
'package selftest

import (
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"strings"
)

var _ = strings.TrimSpace
var _ = secret.New'

header "Coverage threshold (make gate-unit)"

expect_gate_failure "domain package without tests" gate-unit core/domain \
'package selftest

// Selftest is exported, uncovered, and therefore below the threshold.
func Selftest(n int) int {
	if n > 0 {
		return n
	}
	return -n
}'

header "Licence headers (make gate-architecture)"

expect_gate_failure "a source file without its licence header" gate-architecture core/domain \
'package selftest

// Selftest is a file nobody gave a licence.
func Selftest() {}'

header "Documentation (make gate-docs)"

# The documentation gate does not take a Go package, so it gets a probe of its own: a document
# that points at a file that is not there, at a heading that does not exist, and at an ADR nobody
# ever wrote. All three are mistakes that have been made in this repository already.
expect_docs_failure() {
	local name="$1" content="$2"
	CHECKS=$((CHECKS + 1))

	local probe="docs/gate_selftest_probe.md"
	printf '%s\n' "$content" > "$probe"
	if make --no-print-directory gate-docs >/dev/null 2>&1; then
		printf '  FAILED  %-44s make gate-docs stayed green\n' "$name"
		FAILURES=$((FAILURES + 1))
	else
		printf '  ok      %-44s caught by make gate-docs\n' "$name"
	fi
	rm -f "$probe"
}

expect_docs_failure "a link to a file that is not there" \
'# Probe

[nowhere](./nowhere-at-all.md)'

expect_docs_failure "a link to a heading that is not there" \
'# Probe

[a section that moved](./architecture/arc42.md#a-heading-nobody-wrote)'

expect_docs_failure "a citation of an ADR nobody wrote" \
'# Probe

The reasoning is in ADR-0099.'

header "Event schemas (make gate-contract)"

# The schemas under api/events/ are the contract a subscriber outside this repository writes
# against, and the conformance test is what keeps them from drifting from the code that produces
# the events. A schema that has drifted is exactly as bad as one that was never written, and both
# are invisible until somebody's integration breaks - so the gate has to catch a wrong field, not
# only a missing file.
#
# The probe edits a real schema and puts it back afterwards, because a schema nobody emits an
# event for would be caught by the orphan check rather than by the conformance check, and it is
# the conformance check under test.
expect_event_schema_failure() {
	local name="$1" schema="$2" edit="$3"
	CHECKS=$((CHECKS + 1))

	local original
	original="$(cat "$schema")"
	printf '%s\n' "$edit" > "$schema"
	if make --no-print-directory gate-contract >/dev/null 2>&1; then
		printf '  FAILED  %-44s make gate-contract stayed green\n' "$name"
		FAILURES=$((FAILURES + 1))
	else
		printf '  ok      %-44s caught by make gate-contract\n' "$name"
	fi
	printf '%s\n' "$original" > "$schema"
}

CONTAINER_SCHEMA="api/events/de.hubtask.work.container.created.v1.json"

if ! command -v jq >/dev/null 2>&1; then
	# Skipped rather than silently passed, on the same reasoning as the database probes below: a
	# check that quietly did nothing is worse than one that says it did not run.
	printf '  skipped %-44s jq is not installed\n' "the event schema probes"
	SKIPPED=$((SKIPPED + 3))
else

# The payload sits under `data`, so that is where a probe has to bite: a schema that described the
# envelope correctly and the payload wrongly would be exactly the drift nobody notices.

# A payload field the event does not carry, declared required. This is the drift that actually
# happens: a field is renamed in the code and the schema keeps the old name.
expect_event_schema_failure "a required payload field the event does not carry" "$CONTAINER_SCHEMA" \
"$(jq '.properties.data.required += ["a_field_that_was_renamed"]
      | .properties.data.properties.a_field_that_was_renamed = {"type": "string"}' "$CONTAINER_SCHEMA")"

# And the other direction: a field the event does carry, declared as the wrong type.
expect_event_schema_failure "a payload field declared as the wrong type" "$CONTAINER_SCHEMA" \
"$(jq '.properties.data.properties.name.type = "integer"' "$CONTAINER_SCHEMA")"

# The envelope half, which is what a broker routes on: an extension attribute the mapping stopped
# emitting has to be as loud as a payload field that changed.
expect_event_schema_failure "an extension attribute that is no longer emitted" "$CONTAINER_SCHEMA" \
"$(jq '.required += ["anextensionnobodyemits"]
      | .properties.anextensionnobodyemits = {"type": "string"}' "$CONTAINER_SCHEMA")"
fi

header "The decision list in arc42 §9 (make gate-docs)"

# arc42 §9 repeats what docs/adr/README.md owns, and a repetition nobody checks drifts - this one
# stood three decisions behind before the check existed. Both directions are shown to bite: a
# decision the index has and §9 does not, and a status the two disagree about.
ARC42="docs/architecture/arc42.md"

CHECKS=$((CHECKS + 1))
cp "$ARC42" "$ARC42.selftest-backup"
# Drop the last ADR row of the table, whichever it is - the probe must not name a number, or it
# rots the next time an ADR is written.
LAST_ADR_ROW=$(grep -n '^| [0-9]\{4\} |' "$ARC42" | tail -1 | cut -d: -f1)
sed -i.tmp "${LAST_ADR_ROW}d" "$ARC42" && rm -f "$ARC42.tmp"
if make --no-print-directory gate-docs >/dev/null 2>&1; then
	printf '  FAILED  %-44s make gate-docs stayed green\n' "an ADR missing from arc42 §9"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by make gate-docs\n' "an ADR missing from arc42 §9"
fi
mv "$ARC42.selftest-backup" "$ARC42"

CHECKS=$((CHECKS + 1))
cp "$ARC42" "$ARC42.selftest-backup"
LAST_ADR_ROW=$(grep -n '^| [0-9]\{4\} |' "$ARC42" | tail -1 | cut -d: -f1)
sed -i.tmp "${LAST_ADR_ROW}s/| accepted |/| superseded |/" "$ARC42" && rm -f "$ARC42.tmp"
if make --no-print-directory gate-docs >/dev/null 2>&1; then
	printf '  FAILED  %-44s make gate-docs stayed green\n' "a status arc42 and the index disagree on"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by make gate-docs\n' "a status arc42 and the index disagree on"
fi
mv "$ARC42.selftest-backup" "$ARC42"

header "Observability artefacts (make gate-observability)"

# An alert without a runbook is the one mistake §11 names explicitly, so it is the one the gate
# has to be shown catching. The probe adds a sixth alert pointing at a runbook nobody wrote.
CHECKS=$((CHECKS + 1))
RULES="deploy/observability/alerts/prometheus-rules.yaml"
cp "$RULES" "$RULES.selftest-backup"
cat >> "$RULES" <<'PROBE'

      - alert: HubtaskSelftestProbe
        expr: vector(1)
        labels:
          severity: ticket
          alert_id: A-99
        annotations:
          summary: "A probe that ships without a runbook"
          runbook: RB-A99-nobody-wrote-this.md
PROBE
if make --no-print-directory gate-observability >/dev/null 2>&1; then
	printf '  FAILED  %-44s make gate-observability stayed green
' "an alert without a runbook"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by make gate-observability
' "an alert without a runbook"
fi
mv "$RULES.selftest-backup" "$RULES"

header "Support matrix (make gate-docs)"

# The matrix is only worth anything if it cannot drift. Both directions are shown to bite: a row
# claiming a job nobody wrote, and a matrix job no row claims.
# The probe has to go into the matrix itself: the gate reads that one document, and a row in any
# other file is correctly none of its business.
CHECKS=$((CHECKS + 1))
MATRIX="docs/architecture/support-matrix.md"
cp "$MATRIX" "$MATRIX.selftest-backup"
printf '\n| Probe | `supported` | `nightly.yml:matrix-nobody-wrote-this` |\n' >> "$MATRIX"
if make --no-print-directory gate-docs >/dev/null 2>&1; then
	printf '  FAILED  %-44s make gate-docs stayed green\n' "a matrix row claiming a job nobody wrote"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by make gate-docs\n' "a matrix row claiming a job nobody wrote"
fi
mv "$MATRIX.selftest-backup" "$MATRIX"

CHECKS=$((CHECKS + 1))
WORKFLOW=".github/workflows/gate-selftest-probe.yml"
cat > "$WORKFLOW" <<'PROBE'
name: Selftest probe
on: workflow_dispatch
jobs:
  matrix-unclaimed:
    runs-on: ubuntu-latest
    steps:
      - run: echo "a matrix job no row in the support matrix claims"
PROBE
if make --no-print-directory gate-docs >/dev/null 2>&1; then
	printf '  FAILED  %-44s make gate-docs stayed green\n' "a matrix job without a row"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by make gate-docs\n' "a matrix job without a row"
fi
rm -f "$WORKFLOW"

header "The Go version (make gate-docs)"

# The Go version stands in seventeen places across eight files, and nothing kept them in step. A
# Dependabot pull request bumping only the base image (#107) would have left the released binary
# built by a compiler no gate had ever run - which is the probe: move the image and nothing else.
CHECKS=$((CHECKS + 1))
DOCKERFILE="deploy/docker/Dockerfile"
cp "$DOCKERFILE" "$DOCKERFILE.selftest-backup"
sed -i.tmp 's/^FROM golang:[0-9]*\.[0-9]*-alpine/FROM golang:1.99-alpine/' "$DOCKERFILE" && rm -f "$DOCKERFILE.tmp"
if make --no-print-directory gate-docs >/dev/null 2>&1; then
	printf '  FAILED  %-44s make gate-docs stayed green\n' "a base image ahead of go.mod"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by make gate-docs\n' "a base image ahead of go.mod"
fi
mv "$DOCKERFILE.selftest-backup" "$DOCKERFILE"

# The other direction: a statement reworded past the pattern would make the check silently stop
# covering it, which is the failure mode a "does everything agree" check is prone to.
CHECKS=$((CHECKS + 1))
MATRIX_DOC="docs/architecture/support-matrix.md"
cp "$MATRIX_DOC" "$MATRIX_DOC.selftest-backup"
sed -i.tmp 's/| Go (building from source) |/| Go (from source) |/' "$MATRIX_DOC" && rm -f "$MATRIX_DOC.tmp"
if make --no-print-directory gate-docs >/dev/null 2>&1; then
	printf '  FAILED  %-44s make gate-docs stayed green\n' "a version statement reworded away"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by make gate-docs\n' "a version statement reworded away"
fi
mv "$MATRIX_DOC.selftest-backup" "$MATRIX_DOC"

header "Workflow syntax (make gate-architecture)"

# A `${{ }}` expression is only parsed when GitHub runs it, and an invalid one fails the whole run
# before a single job starts - with an empty job list to look at. That is how `join(needs.*.result,
# " ")` got pushed: double quotes are not string delimiters in an expression, and nothing local
# said so. The probe is that exact mistake.
CHECKS=$((CHECKS + 1))
PROBE_WORKFLOW=".github/workflows/gate-selftest-expression.yml"
cat > "$PROBE_WORKFLOW" <<'PROBE'
name: Selftest probe
on: workflow_dispatch
jobs:
  bad-expression:
    runs-on: ubuntu-latest
    steps:
      - run: echo "${{ join(github.event.commits.*.id, ", ") }}"
PROBE
if make --no-print-directory gate-architecture >/dev/null 2>&1; then
	printf '  FAILED  %-44s make gate-architecture stayed green\n' "an invalid workflow expression"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by make gate-architecture\n' "an invalid workflow expression"
fi
rm -f "$PROBE_WORKFLOW"

header "Action pins (make gate-architecture)"

# The nightly script asks GitHub whether each pin resolves; this rule is the other half, and it is
# the half #16 walked through: two `uses:` of one repository at different versions, every pin
# resolving and every comment correct. A second pin of an action already used elsewhere is the
# whole probe.
CHECKS=$((CHECKS + 1))
PIN_PROBE=".github/workflows/gate-selftest-pin-probe.yml"
cat > "$PIN_PROBE" <<'PROBE'
name: Selftest probe
on: workflow_dispatch
jobs:
  probe:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@1111111111111111111111111111111111111111 # v4.2.2
PROBE
if make --no-print-directory gate-architecture >/dev/null 2>&1; then
	printf '  FAILED  %-44s make gate-architecture stayed green\n' "one repository pinned to two commits"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by make gate-architecture\n' "one repository pinned to two commits"
fi
rm -f "$PIN_PROBE"

header "Data protection (make gate-privacy)"

# PG-1 to PG-8 are the gates data-protection.md §10 and ADR-0018 assert. Four documents claimed
# them and nothing ran them until E-11, so what has to be shown here is not that they are clever -
# it is that each one goes red. Three take a probe package the way the layer rules do; the rest
# need a line of the real source moved, and are restored afterwards.

expect_gate_failure "an audit change without a classification" gate-privacy core/domain \
'package selftest

import "github.com/Jersyfi/hubtask/core/port/audit"

var probe = audit.Change{Field: "title", To: "the new title"}'

expect_gate_failure "a title in a log line" gate-privacy infrastructure \
'package selftest

import "log/slog"

func probe(logger *slog.Logger, title string) {
	logger.Info("saved", slog.String("title", title))
}'

expect_gate_failure "an address written into the source" gate-privacy infrastructure \
'package selftest

const collector = "https://collector.example.net/ingest"'

# PG-3, PG-5 and PG-8 read declarations of the real packages rather than a file the probe writes,
# so each is shown a real declaration changed. The first can be done additively - the map is a
# package-level variable, and an `init` in the same package is enough to make a decision about a
# table no archive carries.
CHECKS=$((CHECKS + 1))
EXPORT_PROBE="core/application/service/privacy/zz_gate_selftest_probe.go"
cat > "$EXPORT_PROBE" <<'PROBE'
package privacy

func init() { subjectColumns["nowhere_at_all"] = []string{"account_id"} }
PROBE
if make --no-print-directory gate-privacy >/dev/null 2>&1; then
	printf '  FAILED  %-44s make gate-privacy stayed green\n' "an export decision about no table"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by make gate-privacy\n' "an export decision about no table"
fi
rm -f "$EXPORT_PROBE"

# expect_privacy_failure_after <name> <file> <sed programme>
# The file is put back whatever happens - a probe that left the retention floor at zero would be
# worse than no probe at all.
expect_privacy_failure_after() {
	local name="$1" file="$2" programme="$3"
	CHECKS=$((CHECKS + 1))

	cp "$file" "$file.selftest-backup"
	sed -i.tmp "$programme" "$file" && rm -f "$file.tmp"
	if make --no-print-directory gate-privacy >/dev/null 2>&1; then
		printf '  FAILED  %-44s make gate-privacy stayed green\n' "$name"
		FAILURES=$((FAILURES + 1))
	else
		printf '  ok      %-44s caught by make gate-privacy\n' "$name"
	fi
	mv "$file.selftest-backup" "$file"
}

expect_privacy_failure_after "a retention kind without a lower bound" \
	core/domain/model/lifecycle/Catalogue.go '/KindTrash/ s/MinDays: [0-9]*/MinDays: 0/'

# PG-8 is a tripwire rather than a check, and a tripwire is proved by tripping it: the marker it
# watches for, in the file it watches.
expect_privacy_failure_after "an AI provider arriving in the configuration" \
	core/port/environment/Port.go '$a\
// AIProvider - the selftest probe for PG-8.'

header "Data protection with a database (make gate-privacy-full)"

# PG-2 and PG-7 need PostgreSQL, and this script runs where there may be none - so they are skipped
# rather than silently passed, and the nightly runs this script on the machine that has one.
if ! docker info >/dev/null 2>&1 && ! podman info >/dev/null 2>&1; then
	printf '  skipped %-44s no container runtime\n' "PG-2 and PG-7"
	SKIPPED=$((SKIPPED + 2))
else
	# expect_full_privacy_failure <name> <file>: the file is already broken; run the gate, expect
	# red, and put the file back.
	expect_full_privacy_failure() {
		local name="$1" file="$2"
		CHECKS=$((CHECKS + 1))

		if make --no-print-directory gate-privacy-full >/dev/null 2>&1; then
			printf '  FAILED  %-44s make gate-privacy-full stayed green\n' "$name"
			FAILURES=$((FAILURES + 1))
		else
			printf '  ok      %-44s caught by make gate-privacy-full\n' "$name"
		fi
		mv "$file.selftest-backup" "$file"
	}

	# PG-7: a table that holds personal content and no longer has a catalogue entry. The comments
	# row is taken out - one line, one table, and the table the catalogue would be most obviously
	# wrong to be missing.
	CATALOGUE="docs/privacy/data-catalog.md"
	cp "$CATALOGUE" "$CATALOGUE.selftest-backup"
	sed -i.tmp '/| `comment` |/d' "$CATALOGUE" && rm -f "$CATALOGUE.tmp"
	expect_full_privacy_failure "a personal table missing from the catalogue" "$CATALOGUE"

	# PG-2: an erasure that stops serving one storage location. The repository method keeps its
	# signature and does nothing, which is exactly the shape of the mistake - a step that reports
	# success and leaves the rows where they were.
	ERASER="infrastructure/postgres/PrivacyRepository.go"
	cp "$ERASER" "$ERASER.selftest-backup"
	awk '
		/func \(r PrivacyRepository\) DeleteAuthoredComments\(/ { found = 1 }
		{ print }
		found && /^\) \(int, error\) \{$/ { print "\treturn 0, nil // gate-selftest probe"; found = 0 }
	' "$ERASER.selftest-backup" > "$ERASER"
	expect_full_privacy_failure "an erasure that leaves a location behind" "$ERASER"
fi

header "Licences (make gate-licenses)"

# The licence gate cannot be shown a GPL dependency without adding one, so it is shown the other
# side of the same switch: with the permissive types declared disallowed, every dependency this
# project has becomes a finding. What that proves is that the tool runs, that the flag reaches it,
# and that a disallowed type turns the gate red - not that any particular licence is classified
# correctly.
CHECKS=$((CHECKS + 1))
if .tools/go-licenses check ./... --disallowed_types=notice >/dev/null 2>&1; then
	printf '  FAILED  %-44s a disallowed type stayed green\n' "a dependency of a refused licence type"
	FAILURES=$((FAILURES + 1))
else
	printf '  ok      %-44s caught by go-licenses\n' "a dependency of a refused licence type"
fi

printf '\n%d checks, %d failures, %d skipped\n' "$CHECKS" "$FAILURES" "$SKIPPED"
test "$FAILURES" -eq 0
