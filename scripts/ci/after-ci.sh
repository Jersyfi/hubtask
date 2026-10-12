#!/usr/bin/env bash
# What happens after a `ci.yml` run (the workflow after-ci.yml, ADR-0083).
#
#   1. A failed run is re-run once - its failed jobs and what depends on them: known-traps.md's rule
#      for flakes ("re-run the job once before reading code"), made automatic so that pull requests
#      merging on their own do not stop at a registry's 403. Not a pull request's run that failed at
#      the guard alone: nothing in it is flaky, it waits for `main` (step 3 re-runs it).
#   2. A red `main` opens the one issue "CI: main is red" (label `finding`), or comments on it: the
#      commit, its pull request, the failed jobs and how to revert. And every ready pull request whose
#      `CI required` is green has that job re-run, so the guard (main-green.sh) turns it red: a pull
#      request that went green before `main` turned red would otherwise still merge onto it.
#   3. A green `main` closes the issue, and every ready pull request whose `CI required` failed at the
#      guard alone has its failed jobs re-run.
#
# A re-run repeats the run's own merge commit: it never tests a newer `main` - only a push does.
#
# MODE=run (the default) acts on one finished run: RUN_ID, ATTEMPT, CONCLUSION, EVENT, BRANCH and
# HEAD_SHA. Nothing guarantees it hears of every run - GitHub starts no workflow for what its own
# token caused, a re-run included - so MODE=sweep, on a schedule, reads `main`'s state and does 2 or 3
# by it. Every value arrives through the environment and is compared, never executed: a branch name
# is a pull request author's text. A write GitHub refuses (a run already re-running, the read-only
# token of a Dependabot run) is a warning, not a failure: the sweep comes again.
set -euo pipefail

repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY names the repository, owner/name}"
server="${GITHUB_SERVER_URL:-https://github.com}"
here="$(cd "$(dirname "$0")" && pwd)"
title="CI: main is red"

try() {
	if ! gh "$@"; then
		echo "::warning::gh $1 $2 was refused; the next sweep tries again"
	fi
}

# The open or closed issue this script files under, by its exact title and its label. The issue
# list, not the search: the search index lags, and two runs ending together would file twice.
issue() {
	gh api "repos/${repo}/issues?state=all&labels=finding&per_page=100" |
		jq -r --arg title "$title" '[.[] | select(.title == $title and (.pull_request | not))] | first // empty | "\(.number) \(.state)"'
}

# The names of a run's failed jobs, comma-separated.
failed_jobs() {
	gh api "repos/${repo}/actions/runs/$1/jobs?per_page=100" |
		jq -r '[.jobs[] | select(.conclusion == "failure") | .name] | join(",")'
}

# Every ready pull request with the newest CI run of its head: "<number> <run id> <conclusion>".
ready_runs() {
	local number head runs
	while read -r number head; do
		[[ -z "$number" ]] && continue
		runs="$(gh api "repos/${repo}/actions/workflows/ci.yml/runs?head_sha=${head}&event=pull_request&per_page=1")"
		jq -r --arg n "$number" '.workflow_runs[0] | select(.status == "completed") | "\($n) \(.id) \(.conclusion)"' <<<"$runs"
	done < <(gh api "repos/${repo}/pulls?state=open&per_page=100" | jq -r '.[] | select(.draft | not) | "\(.number) \(.head.sha)"')
}

file_red() {
	local run="$1" attempt="$2" sha="$3"
	local subject pr jobs found number state body retry=""
	subject="$(gh api "repos/${repo}/commits/${sha}" | jq -r '.commit.message | split("\n")[0]')"
	pr="$(sed -nE 's/.*\(#([0-9]+)\)$/\1/p' <<<"$subject")"
	jobs="$(gh api "repos/${repo}/actions/runs/${run}/attempts/${attempt}/jobs?per_page=100" |
		jq -r '.jobs[] | select(.conclusion == "failure") | "* `\(.name)`"')"
	if [[ "$attempt" == "1" ]]; then
		retry="The failed jobs are being re-run once; a flake ends there, and the next green run closes this issue."
	fi
	body="$(
		cat <<EOF
\`main\` is red at ${sha} - ${subject}${pr:+ (#${pr})}.

* Run: ${server}/${repo}/actions/runs/${run} (attempt ${attempt})
* Failed jobs:
${jobs:-* (none listed)}

Until a run on \`main\` is green again, \`CI required\` fails on every pull request that is not the fix (ADR-0083). ${retry}

If the re-run fails too, revert the commit, or fix it forward with a line \`Mends main: ${sha:0:12}\` in the pull request's description. Either merges on the owner's word, like every pull request.

\`\`\`bash
git fetch origin && git switch -c revert-${sha:0:12} origin/main
git revert --no-edit ${sha} && git push -u origin HEAD
# then a draft from a copy of .github/PULL_REQUEST_TEMPLATE.md, titled 'revert: …', saying "No issue: main is red at ${sha:0:12}"
\`\`\`
EOF
	)"
	found="$(issue)"
	if [[ -z "$found" ]]; then
		try issue create --repo "$repo" --title "$title" --label finding --body "$body"
		return
	fi
	read -r number state <<<"$found"
	try issue comment "$number" --repo "$repo" --body "$body"
	if [[ "$state" == "closed" ]]; then
		try issue reopen "$number" --repo "$repo"
	fi
}

# Re-run `CI required` alone on every ready pull request whose run is green: the guard reads the red
# `main` and holds it until step 3.
hold() {
	local number id conclusion job
	while read -r number id conclusion; do
		[[ "$conclusion" == "success" ]] || continue
		job="$(gh api "repos/${repo}/actions/runs/${id}/jobs?per_page=100" | jq -r '[.jobs[] | select(.name == "CI required") | .id] | first // empty')"
		[[ -z "$job" ]] && continue
		try run rerun --job "$job" --repo "$repo"
		echo "held #${number}: re-ran CI required of run ${id} against the red main"
	done < <(ready_runs)
}

mended() {
	local sha="$1" run="$2" found number state id conclusion
	found="$(issue)"
	if [[ -n "$found" ]]; then
		read -r number state <<<"$found"
		if [[ "$state" == "open" ]]; then
			try issue close "$number" --repo "$repo" --comment "\`main\` is green again at ${sha} (${server}/${repo}/actions/runs/${run}). The next red run reopens this issue."
		fi
	fi
	while read -r number id conclusion; do
		[[ "$conclusion" == "failure" ]] || continue
		if [[ "$(failed_jobs "$id")" == "CI required" ]]; then
			try run rerun "$id" --failed --repo "$repo"
			echo "re-ran #${number}'s run ${id}: it waited only for main"
		fi
	done < <(ready_runs)
}

state_of_main() {
	"$here/main-green.sh" --state
}

case "${MODE:-run}" in
sweep)
	now="$(state_of_main)"
	read -r state sha run <<<"$now"
	case "$state" in
	green) mended "$sha" "$run" ;;
	red)
		found="$(issue)"
		if [[ "$found" != *" open" ]]; then
			attempt="$(gh api "repos/${repo}/actions/runs/${run}" | jq -r '.run_attempt')"
			file_red "$run" "$attempt" "$sha"
		fi
		hold
		;;
	esac
	echo "main is ${state}"
	;;
run)
	: "${RUN_ID:?}" "${ATTEMPT:?}" "${CONCLUSION:?}" "${EVENT:?}" "${BRANCH:?}" "${HEAD_SHA:?}"
	on_main=false
	[[ "$EVENT" == "push" && "$BRANCH" == "main" ]] && on_main=true
	if [[ "$CONCLUSION" == "failure" ]]; then
		if [[ "$ATTEMPT" == "1" ]]; then
			if [[ "$on_main" == true ]]; then
				try run rerun "$RUN_ID" --failed --repo "$repo"
			elif [[ "$EVENT" == "pull_request" && "$(failed_jobs "$RUN_ID")" != "CI required" ]]; then
				try run rerun "$RUN_ID" --failed --repo "$repo"
			fi
		fi
		if [[ "$on_main" == true ]]; then
			file_red "$RUN_ID" "$ATTEMPT" "$HEAD_SHA"
			hold
		fi
	elif [[ "$CONCLUSION" == "success" && "$on_main" == true ]]; then
		now="$(state_of_main)"
		read -r state sha run <<<"$now"
		if [[ "$state" == "green" ]]; then
			mended "$sha" "$run"
		fi
	fi
	;;
*)
	echo "after-ci: MODE is run or sweep, not ${MODE}" >&2
	exit 2
	;;
esac
