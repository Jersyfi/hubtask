#!/usr/bin/env bash
# A Claude Code PreToolUse hook on Bash (.claude/settings.json, ADR-0078). It guards the two moments
# a pull request changes what CI does:
#
#   gh pr create   - only as a draft: a draft runs no CI, and a session checks it locally.
#   gh pr ready    - only when `make verify-pr` passed for HEAD, and HEAD is pushed: leaving draft
#                    starts the pipeline, on the commit GitHub has. `--undo` is always allowed.
#
# Exit 2 refuses the call and hands this script's stderr to the session; exit 0 lets it run. Every
# other command leaves at the first test, so the hook costs a Bash call nothing it can notice.
set -uo pipefail

input="$(cat)"
case "$input" in
*"gh pr"* | *'gh  pr'*) ;;
*) exit 0 ;;
esac

command="$(printf '%s' "$input" | python3 -c 'import json, sys; print(json.load(sys.stdin).get("tool_input", {}).get("command", ""))' 2>/dev/null)" || exit 0

refuse() {
	printf '%s\n' "$1" >&2
	exit 2
}

# A word boundary on both sides: `gh pr ready` as a command, not `echo "gh pr readyness"`.
boundary='(^|[;&|(`[:space:]])'
if [[ "$command" =~ ${boundary}gh[[:space:]]+pr[[:space:]]+create([[:space:]]|$) ]]; then
	if [[ ! "$command" =~ [[:space:]](--draft|-d)([[:space:]=]|$) ]]; then
		refuse "Refused: a pull request starts as a draft (ADR-0078). Run the same command with --draft."
	fi
fi

if [[ "$command" =~ ${boundary}gh[[:space:]]+pr[[:space:]]+ready([[:space:]]|$) && ! "$command" =~ [[:space:]]--undo([[:space:]]|$) ]]; then
	head="$(git rev-parse HEAD 2>/dev/null)" || refuse "Refused: not inside a git checkout, so make verify-pr cannot have passed here."
	stamp="$(git rev-parse --absolute-git-dir)/hubtask-verify-pr"
	if [[ ! -f "$stamp" || "$(head -n 1 "$stamp")" != "$head" ]]; then
		refuse "Refused: make verify-pr has not passed for ${head:0:12} (ADR-0078). Run make verify-pr, push, then gh pr ready."
	fi
	if ! upstream="$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null)" ||
		! git merge-base --is-ancestor "$head" "$upstream" 2>/dev/null; then
		refuse "Refused: ${head:0:12} is not pushed, and CI would run on the commit GitHub has. Run git push on its own first, then gh pr ready."
	fi
fi

exit 0
