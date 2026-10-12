#!/usr/bin/env bash
# Is `main` green? The last step of `CI required` on a pull request (`make gate-main-green`,
# ADR-0083), and the state the after-CI script acts on.
#
# A pull request merges without catching up with `main`, so a commit that breaks `main` is found by
# `main`'s own run, after the merge. While that run is red, a pull request merging on top would hide
# a second break behind the first and leave nobody sure which commit to revert. So `CI required`
# fails while the newest deciding `ci.yml` run on `main` (below) is red - read at the end of the pull
# request's run, because auto-merge acts seconds after it turns green.
#
# The fix itself must pass: a revert (`revert: …`, `Revert "…"`) or a pull request whose
# description carries a line `Mends main: <sha>`.
#
#   main-green.sh            the guard; needs GITHUB_REPOSITORY and PR
#   main-green.sh --state    prints `green <sha> <run>`, `red <sha> <run>` or `unknown`
#
# Which run decides, newest commit first (by when the run was created, never by when it finished:
# every commit on `main` runs on its own and they finish out of order):
#   * finished `success` - green; `failure`, `timed_out` or `startup_failure` - red;
#   * finished `cancelled`, `skipped`, `neutral` - says nothing about the tree, passed over;
#   * still going on its first attempt - not known yet, passed over;
#   * still going on a later attempt - a re-run of a failure (after-ci.sh re-runs a red run once),
#     so red until it ends: otherwise pull requests would merge during the very re-run that the
#     "main is red" issue already announces.
# Only the runs of a push to `main` count, so a fork's branch named `main` is never read. A failing
# API call ends the script non-zero: the guard fails closed.
set -euo pipefail

repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY names the repository, owner/name}"

# AS_OF (an ISO time) looks back: the state `main` had then, as far as the runs' outcomes go now.
runs="$(gh api "repos/${repo}/actions/workflows/ci.yml/runs?branch=main&event=push&per_page=30${AS_OF:+&created=<=${AS_OF}}")"
newest="$(jq -r '[.workflow_runs[]
		| if .status == "completed" then
			(if .conclusion == "success" then . + {verdict: "success"}
			 elif (.conclusion == "failure" or .conclusion == "timed_out" or .conclusion == "startup_failure") then . + {verdict: "failure"}
			 else empty end)
		  elif (.run_attempt // 1) > 1 then . + {verdict: "failure"}
		  else empty end]
	| sort_by(.created_at) | reverse | first // empty
	| "\(.verdict) \(.head_sha) \(.id)"' <<<"$runs")"

if [[ "${1:-}" == "--state" ]]; then
	case "$newest" in
	success\ *) echo "green ${newest#success }" ;;
	failure\ *) echo "red ${newest#failure }" ;;
	*) echo "unknown" ;;
	esac
	exit 0
fi

pr="${PR:?PR names the pull request}"
read -r conclusion sha run <<<"${newest:-none - -}"
case "$conclusion" in
success)
	echo "main is green at ${sha:0:12} (run ${run})"
	exit 0
	;;
failure) ;;
*)
	echo "::notice::no deciding run on main to read - the guard has nothing to say"
	exit 0
	;;
esac

pull="$(gh api "repos/${repo}/pulls/${pr}")"
title="$(jq -r '.title // ""' <<<"$pull")"
body="$(jq -r '.body // ""' <<<"$pull")"

# The two revert titles tools/checkpr accepts: the Conventional one and GitHub's own button's.
conventional='^revert(\([a-z0-9 ,./_-]*\))?!?: '
button='^Revert "'
if [[ "$title" =~ $conventional || "$title" =~ $button ]]; then
	echo "::notice::main is red at ${sha:0:12}, and this pull request is a revert - it may pass"
	exit 0
fi
# A here-string, not a pipe: `grep -q` on a pipe under pipefail reports a match as a failure.
if grep -Eq '^Mends main: [0-9a-f]{7,40}[[:space:]]*$' <<<"$body"; then
	echo "::notice::main is red at ${sha:0:12}, and this pull request says it mends main - it may pass"
	exit 0
fi

server="${GITHUB_SERVER_URL:-https://github.com}"
echo "::error::main is red: its newest deciding run failed at ${sha:0:12} (${server}/${repo}/actions/runs/${run}). This pull request waits, so that nothing lands on a broken main; its run is re-run once main is green again. A pull request that fixes main says so on a line 'Mends main: ${sha:0:12}' in its description (ADR-0083)."
exit 1
