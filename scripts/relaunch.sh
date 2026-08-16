#!/usr/bin/env bash
# Relaunch: publishes this working tree as a fresh repository under a new owner.
#
# What it does, in order:
#   1. Rewrites the owner everywhere (Go module path, imports, image names, links).
#   2. Discards the old git history and creates ONE clean initial commit.
#   3. Creates the GitHub repository and pushes.
#   4. Sets up labels, milestones, environments, branch protection, repo settings.
#   5. Creates the ten real milestone-0.1.0 issues from docs/backlog/.
#
# Idempotent from step 4 onwards. Steps 1-3 run once.
# Prerequisite: gh CLI authenticated as the NEW account (gh auth login).
set -euo pipefail

OLD_OWNER="Jersyfi"
NEW_OWNER=""
REPO="hubtask"
AUTHOR_NAME="Jérôme Bastian Winkel"
AUTHOR_EMAIL=""
VISIBILITY="--public"
SKIP_HISTORY=0

usage() {
  cat <<'EOF'
Usage:
  ./scripts/relaunch.sh --owner <github-user> --email <commit-email> [options]

  --owner    GitHub user or organisation of the NEW account   (required)
  --email    Email for the initial commit author              (required)
  --repo     Repository name                                  (default: hubtask)
  --private  Create the repository private instead of public
  --resume   Skip the rewrite and the fresh history; only run the GitHub setup
             (use this if the push worked but the setup failed halfway)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --owner)   NEW_OWNER="${2:-}"; shift 2 ;;
    --email)   AUTHOR_EMAIL="${2:-}"; shift 2 ;;
    --repo)    REPO="${2:-}"; shift 2 ;;
    --private) VISIBILITY="--private"; shift ;;
    --resume)  SKIP_HISTORY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

[[ -n "$NEW_OWNER"   ]] || { echo "ERROR: --owner is missing" >&2; usage; exit 1; }
[[ -n "$AUTHOR_EMAIL" ]] || { echo "ERROR: --email is missing" >&2; usage; exit 1; }

cd "$(dirname "$0")/.."
command -v gh >/dev/null || { echo "ERROR: gh CLI not found (https://cli.github.com)" >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "ERROR: not signed in - run 'gh auth login'" >&2; exit 1; }

ACTIVE="$(gh api user -q .login)"
echo "gh is authenticated as: $ACTIVE"
echo "Target repository:      $NEW_OWNER/$REPO"
echo "Commit author:          $AUTHOR_NAME <$AUTHOR_EMAIL>"
echo
read -r -p "Correct? [y/N] " a
[[ "$a" == "y" || "$a" == "Y" ]] || exit 1

# ---------------------------------------------------------------- 1. rewrite

if [[ $SKIP_HISTORY -eq 0 && "$NEW_OWNER" != "$OLD_OWNER" ]]; then
  echo
  echo "== Rewriting owner $OLD_OWNER -> $NEW_OWNER =="
  # -I skips binary files; the module path lives in imports, so this must be exact.
  grep -rlI --exclude-dir=.git "$OLD_OWNER" . | while IFS= read -r f; do
    sed -i '' "s|${OLD_OWNER}|${NEW_OWNER}|g" "$f"
    echo "  $f"
  done
  echo
  echo "== Verifying the rewrite =="
  go build ./... || { echo "ERROR: build broken after the rewrite - stopping." >&2; exit 1; }
  go test ./... >/dev/null || { echo "ERROR: tests broken after the rewrite - stopping." >&2; exit 1; }
  echo "  build and tests green"
elif [[ $SKIP_HISTORY -eq 0 ]]; then
  echo "Owner unchanged ($OLD_OWNER) - no rewrite needed."
fi

# ------------------------------------------------------- 2./3. history + push

if [[ $SKIP_HISTORY -eq 0 ]]; then
  echo
  echo "== Fresh history =="
  rm -rf .git
  git init -q -b main
  git config user.name  "$AUTHOR_NAME"
  git config user.email "$AUTHOR_EMAIL"
  git add -A
  git commit -q -m "feat: initial public release

Architecture documentation, repository skeleton, licence construct (BSL 1.1
with a conversion to Apache-2.0) and the operating material for milestone 0.1.0."
  echo "  1 commit: $(git log --format='%an <%ae>' -1)"

  echo
  echo "== Creating the repository and pushing =="
  gh repo create "$NEW_OWNER/$REPO" $VISIBILITY --source=. --remote=origin --push
fi

FULL="$NEW_OWNER/$REPO"

# ------------------------------------------------------------- 4. repo set-up

echo
echo "== Labels =="
label() {
  gh label create "$1" --repo "$FULL" --color "$2" --description "$3" --force >/dev/null 2>&1 \
    && echo "  $1" || echo "  $1 (unchanged)"
}
label "task"         "0e8a16" "An actionable implementation task"
label "claude:task"  "5319e7" "Triggers Claude Code"
label "adr"          "1d76db" "Architecture decision"
label "discussion"   "cfd3d7" "Needs clarification"
label "security"     "b60205" "Security relevant"
label "privacy"      "b60205" "Data protection relevant"
label "breaking"     "d93f0b" "Breaking change"
label "blocked"      "e4e669" "Waiting on a decision"

echo
echo "== Milestones =="
for m in "0.1.0|Walking skeleton" \
         "0.2.0|Hierarchy and items" \
         "0.3.0|Collaboration and content" \
         "0.4.0|Time" \
         "0.4.5|Backup, retention, audit"; do
  gh api "repos/$FULL/milestones" -f title="${m%%|*}" -f description="${m#*|}" >/dev/null 2>&1 \
    && echo "  ${m%%|*}" || echo "  ${m%%|*} (exists)"
done

echo
echo "== Environments =="
gh api -X PUT "repos/$FULL/environments/integration" >/dev/null 2>&1 && echo "  integration"
USER_ID="$(gh api user -q .id)"
gh api -X PUT "repos/$FULL/environments/production" \
  -F "reviewers[][type]=User" -F "reviewers[][id]=$USER_ID" >/dev/null 2>&1 \
  && echo "  production (with approval)" || echo "  production (add the approver by hand)"

echo
echo "== Repository settings =="
gh api -X PATCH "repos/$FULL" \
  -F allow_merge_commit=false -F allow_squash_merge=true -F allow_rebase_merge=false \
  -F delete_branch_on_merge=true -F has_wiki=false -F has_projects=true \
  -F has_discussions=true >/dev/null 2>&1 && echo "  squash merge, branch deletion, discussions"

echo
echo "== Branch protection for main =="
# The contexts must match the job names in .github/workflows/ci.yml exactly.
gh api -X PUT "repos/$FULL/branches/main/protection" --input - >/dev/null 2>&1 <<'JSON' \
  && echo "  rule set" || echo "  rule NOT set (private repo without a suitable plan?)"
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "Format, lint, generation",
      "Unit and domain tests",
      "Architecture rules",
      "Security gates SG-1..SG-12"
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": { "required_approving_review_count": 1, "dismiss_stale_reviews": true },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": true
}
JSON

# ------------------------------------------------------------------ 5. issues

echo
echo "== Issues from docs/backlog/milestone-0.1.0.md =="
BACKLOG="docs/backlog/milestone-0.1.0.md"
if [[ ! -f "$BACKLOG" ]]; then
  echo "  backlog not found - skipped"
else
  WORK="$(mktemp -d)"
  trap 'rm -rf "$WORK"' EXIT
  # One file per "## A-xx" section. Writing to files instead of piping through
  # `read` is deliberate: the section bodies contain newlines, which is exactly
  # what broke the previous script.
  awk -v dir="$WORK" '
    /^## A-[0-9]+ / { n++; file = sprintf("%s/%02d.md", dir, n); title = $0; sub(/^## /, "", title);
                      printf "%s\n", title > (file ".title"); next }
    /^## /         { file = ""; next }
    file           { print >> file }
  ' "$BACKLOG"

  for tf in "$WORK"/*.title; do
    [[ -e "$tf" ]] || { echo "  no sections found"; break; }
    body="${tf%.title}"
    title="$(head -1 "$tf")"
    if gh issue list --repo "$FULL" --search "\"$title\" in:title" --json title -q '.[].title' 2>/dev/null | grep -qxF "$title"; then
      echo "  exists: $title"
      continue
    fi
    { cat "$body"; printf '\n---\nSource: `%s`\n' "$BACKLOG"; } > "$WORK/body.md"
    gh issue create --repo "$FULL" --title "$title" --body-file "$WORK/body.md" \
      --label task --milestone "0.1.0" >/dev/null && echo "  created: $title"
  done
fi

echo
echo "Done. Remaining manual steps:"
echo "  1. gh secret set ANTHROPIC_API_KEY --repo $FULL"
echo "  2. Settings -> Actions -> General -> Workflow permissions: 'Read repository contents'"
echo "  3. Settings -> Code security: enable secret scanning + push protection"
echo "  4. Check that exactly 10 issues exist: gh issue list --repo $FULL"
