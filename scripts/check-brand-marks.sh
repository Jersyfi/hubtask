#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# Every brand mark the sign-in buttons draw, held to its row in THIRD-PARTY-LICENSES.md and to a
# preset the server can send (ADR-0069 §2, §3; design-system.md §12).
#
# A mark is somebody else's, permitted for one use by one guideline, and the row is the only place
# that says which. So three things are refused, each of which has happened or would happen
# silently: a mark drawn without a row (shipped with nobody having read its guideline), a row for
# a mark nobody draws (a permission recorded for something that is not shipped), and a mark for a
# kind the contract does not have (a branch no provider can reach, kept alive only by not being
# looked at - four of them were).
#
# Shell and grep only: `make licenses` and this gate run where Node.js was never installed. The
# three paths can be pointed elsewhere so that `make gate-selftest` can show each refusal without
# editing the tree.

set -euo pipefail

cd "$(dirname "$0")/.."

component="${BRAND_MARKS_COMPONENT:-apps/webapp/src/lib/signin/ProviderMark.svelte}"
licences="${BRAND_MARKS_LICENSES:-THIRD-PARTY-LICENSES.md}"
contract="${BRAND_MARKS_CONTRACT:-api/openapi.yaml}"

drawn="$(grep -oE "preset === '[A-Z_]+'" "$component" | sed -E "s/.*'([A-Z_]+)'/\1/" | sort -u || true)"
# The rows of the "Brand marks" table, by the preset in their second column.
listed="$(sed -n '/^## Brand marks$/,/^## /p' "$licences" \
	| grep -oE '^\|[^|]*\| `[A-Z_]+` \|' | sed -E 's/.*`([A-Z_]+)`.*/\1/' | sort -u || true)"
presets="$(awk '/^    IdentityProviderKind:/ { inside = 1; next } inside && /enum:/ { print; exit }' "$contract" \
	| grep -oE '[A-Z_]+' | sort -u || true)"

failures=0
fail() {
	printf '%s\n' "$1"
	failures=$((failures + 1))
}

if [ -z "$drawn" ]; then
	fail "brand marks: no mark found in $component - the check no longer reads where the marks are"
fi
if [ -z "$presets" ]; then
	fail "brand marks: no IdentityProviderKind enum found in $contract"
fi

for mark in $drawn; do
	grep -qx "$mark" <<<"$listed" ||
		fail "brand marks: $mark is drawn but $licences has no row for it - add one to tools/licenses.md.tpl with its source, guideline and permitted use"
	grep -qx "$mark" <<<"$presets" ||
		fail "brand marks: $mark is drawn but is no IdentityProviderKind - a mark without a preset does not ship (ADR-0069 §3)"
done
for mark in $listed; do
	grep -qx "$mark" <<<"$drawn" ||
		fail "brand marks: $licences has a row for $mark, which is not drawn - remove it from tools/licenses.md.tpl"
done

if [ "$failures" -ne 0 ]; then
	exit 1
fi
printf 'licences: brand marks %s - each drawn, listed with its source and guideline, and a preset\n' \
	"$(printf '%s' "$drawn" | paste -sd, - | sed 's/,/, /g')"
