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

printf '\n%d checks, %d failures\n' "$CHECKS" "$FAILURES"
test "$FAILURES" -eq 0
