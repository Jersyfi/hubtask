#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The inner loop (`make check`, ADR-0089): the checks of what this branch changed, in minutes and
# without a lock another worktree could hold. It checks what the branch changed against where it
# left main, committed or not:
#
#   Go          gofmt, vet, golangci-lint and the tests of the changed packages, and the
#               architecture tests whenever any Go changed;
#   generated   `make generate` without a diff, when the contract, a query or a migration changed;
#   locales     the catalogue's own tests, when a message catalogue changed;
#   JavaScript  lint, typecheck and test of each changed workspace package;
#   documents   the documentation gate, when a document changed.
#
# Everything else - the containers, integration, the other packages - is CI's, once the pull
# request is ready. BASE names what the branch is compared with (default origin/main).
set -euo pipefail

cd "$(dirname "$0")/.."
# The pinned tools, pnpm among them (`make tools-node`), before whatever else is installed.
export PATH="$PWD/.tools:$PATH"

base="${BASE:-origin/main}"
fork="$(git merge-base "$base" HEAD)"
changed="$({
	git diff --name-only "$fork"
	git ls-files --others --exclude-standard
} | sort -u | while read -r f; do if [ -e "$f" ]; then echo "$f"; fi; done)"

if [ -z "$changed" ]; then
	echo "check: nothing changed against $base"
	exit 0
fi

lint="golangci-lint"
step() { printf '\ncheck: %s\n' "$1"; }

go_files="$(grep -E '\.go$' <<<"$changed" || true)"
# A package is a directory with Go files; a changed fixture or golden file counts for the package
# beside it, because its test reads it.
go_pkgs="$(grep -E '^(cmd|core|infrastructure|presentation|test|tools|sdk/go)/' <<<"$changed" |
	while read -r f; do
		d="$(dirname "$f")"
		while [ "$d" != "." ] && ! ls "$d"/*.go >/dev/null 2>&1; do d="$(dirname "$d")"; done
		if [ "$d" != "." ]; then echo "./$d"; fi
	done | sort -u || true)"

if [ -n "$go_files" ]; then
	step "gofmt"
	unformatted="$(gofmt -l $go_files)"
	if [ -n "$unformatted" ]; then
		echo "not formatted - run make fmt:"
		echo "$unformatted"
		exit 1
	fi
fi

# A package whose every file carries a build tag - the integration, contract, resilience, load and
# evidence suites - builds only with its tag: it is vetted and linted with them, and its tests are
# CI's like the containers they need.
suite_tags="integration,contract,resilience,load,evidence"
plain_pkgs=""
suite_pkgs=""
for p in $go_pkgs; do
	if go list "$p" >/dev/null 2>&1; then
		plain_pkgs="$plain_pkgs $p"
	else
		suite_pkgs="$suite_pkgs $p"
	fi
done

if [ -n "$go_pkgs" ]; then
	step "vet, lint and test: $(wc -l <<<"$go_pkgs" | tr -d ' ') package(s)"
	command -v "$lint" >/dev/null || make --no-print-directory tools-ensure >/dev/null
	if [ -n "$plain_pkgs" ]; then
		# shellcheck disable=SC2086 # one package per word
		go vet $plain_pkgs
		# Another worktree's run holds golangci-lint's lock; wait for it rather than fail.
		# shellcheck disable=SC2086
		"$lint" run --allow-serial-runners $plain_pkgs
		# shellcheck disable=SC2086
		go test $plain_pkgs
	fi
	if [ -n "$suite_pkgs" ]; then
		# shellcheck disable=SC2086
		go vet -tags "$suite_tags" $suite_pkgs
		# shellcheck disable=SC2086
		"$lint" run --allow-serial-runners --build-tags "$suite_tags" $suite_pkgs
	fi
	step "architecture"
	go test ./test/architecture/...
fi

if grep -qE '^(api/|db/queries/|db/migrations/)' <<<"$changed"; then
	step "generate"
	before="$(git status --porcelain)"
	make --no-print-directory generate >/dev/null
	if [ "$before" != "$(git status --porcelain)" ]; then
		echo "make generate produces a diff - commit it:"
		git status --porcelain
		exit 1
	fi
fi

if grep -qE '^locales/' <<<"$changed"; then
	step "locales"
	go test ./infrastructure/i18n/...
fi

packages="$(grep -oE '^(apps/[^/]+|packages/[^/]+|sdk/typescript)/' <<<"$changed" | sort -u || true)"
# A fresh worktree has neither the pinned pnpm nor the workspace installed.
if [ -n "$packages" ]; then
	command -v pnpm >/dev/null || make --no-print-directory tools-node >/dev/null
	[ -d node_modules ] || pnpm install --frozen-lockfile >/dev/null
fi
for dir in $packages; do
	[ -f "$dir/package.json" ] || continue
	name="$(sed -n 's/^  "name": "\(.*\)",$/\1/p' "$dir/package.json")"
	step "$name"
	# Its workspace dependencies are built first: its typecheck reads their dist/, which another
	# branch's build may have left behind.
	pnpm --filter "$name^..." build >/dev/null
	pnpm --filter "$name" lint
	pnpm --filter "$name" typecheck
	pnpm --filter "$name" test
done

if grep -qE '(\.md$|^docs/)' <<<"$changed"; then
	step "documents"
	go run ./tools/checkdocs
fi

printf '\ncheck: green - CI runs the rest once the pull request is ready\n'
