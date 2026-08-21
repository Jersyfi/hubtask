#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# Every `uses:` in .github/workflows, checked against the repository it names.
#
# ci-cd.md asks for actions pinned by commit SHA with the version in a trailing comment. That
# rule has two halves, and only the first one is visible in a diff: a forty-character hex string
# looks equally correct whether it is the right commit, the wrong commit, or no commit at all.
# `aquasecurity/trivy-action@915b19b` was the third case for months. The job that used it failed
# in "Set up job" every single night, Dependabot could not bump a ref it was unable to resolve,
# and the vulnerability scan behind it had never run once (#64).
#
# So this asks GitHub what it actually sees: does the pinned commit exist, and does the tag named
# in the comment point at it? A comment is the only thing a reader can judge a hex string by, so
# a comment that has drifted is the same defect with a longer fuse.
#
# It needs the network and a token, which is why it runs in the nightly rather than in
# `make verify` - the local gates stay offline.
#
# The half of the rule that needs no network lives with the other repository-wide consistency
# rules: `test/architecture/actionpins_test.go` refuses two `uses:` of one repository at different
# commits. That one has to bite on the pull request, because #16 was a mixture in which every pin
# resolved and every comment was right.

set -euo pipefail

cd "$(dirname "$0")/.."

failures=0

fail() {
	printf '  FAIL  %s\n        %s\n' "$1" "$2"
	failures=$((failures + 1))
}

trim() {
	local s="$1"
	s="${s#"${s%%[![:space:]]*}"}"
	printf '%s' "${s%"${s##*[![:space:]]}"}"
}

# A tag resolves through a tag object when it is annotated and straight to the commit when it is
# not. Prints nothing and returns non-zero when the repository has no such tag.
resolve() {
	local tag="$1" head sha type
	head="$(gh api "repos/$repo/git/ref/tags/$tag" --jq '.object.sha + " " + .object.type' 2>/dev/null)" || return 1
	sha="${head%% *}"
	type="${head##* }"
	if [ "$type" = "tag" ]; then
		gh api "repos/$repo/git/tags/$sha" --jq '.object.sha' 2>/dev/null || return 1
	else
		printf '%s\n' "$sha"
	fi
}

# `uses: ./…` and `uses: docker://…` name no GitHub repository and carry no pin, so they are none
# of this script's business; the pattern below matches neither.
entries="$(
	grep -rhoE '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*[A-Za-z0-9._/-]+@[^[:space:]]+([[:space:]]*#.*)?$' .github/workflows |
		sed -E 's/^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*//' |
		sort -u
)"

if [ -z "$entries" ]; then
	echo "no 'uses:' found under .github/workflows - the grep has stopped matching, not the workflows stopped using actions"
	exit 1
fi

while IFS= read -r entry; do
	[ -n "$entry" ] || continue

	ref="$(trim "${entry%%#*}")"
	action="${ref%@*}"
	pin="${ref##*@}"
	repo="$(printf '%s' "$action" | cut -d/ -f1,2)"

	if [ "$entry" = "${entry%%#*}" ]; then
		comment=""
	else
		comment="$(trim "${entry#*#}")"
	fi

	if ! printf '%s' "$pin" | grep -qE '^[0-9a-f]{40}$'; then
		fail "$action" "pinned to '$pin', not to a commit SHA - see ci-cd.md"
		continue
	fi

	if [ -z "$comment" ]; then
		fail "$action" "pinned to $pin with no version comment - nobody can tell what that commit is"
		continue
	fi

	# `# v4.2.2` and `# 0.29.0` both occur in the wild; the tag is whatever the repository calls it.
	tagged="$(resolve "$comment" || resolve "v$comment" || resolve "${comment#v}" || true)"

	if [ -z "$tagged" ]; then
		fail "$action" "the comment says $comment, but $repo has no such tag"
		continue
	fi

	if [ "$tagged" != "$pin" ]; then
		if gh api "repos/$repo/commits/$pin" --jq '.sha' >/dev/null 2>&1; then
			fail "$action" "pinned to $pin, but $comment is $tagged - the comment has drifted"
		else
			fail "$action" "pinned to $pin, which is not a commit in $repo; $comment is $tagged"
		fi
		continue
	fi

	printf '  ok    %-46s %s\n' "$action" "$comment"
done < <(printf '%s\n' "$entries")

if [ "$failures" -gt 0 ]; then
	printf '\n%d action pin(s) do not hold up.\n' "$failures"
	exit 1
fi

echo
echo "Every action pin resolves, and every comment names the tag it is pinned to."
