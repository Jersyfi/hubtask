# Contributing

Thanks for your interest. Hubtask's architecture is documented in full, and that is the basis you
work from. The rules for changing anything — for a person and for an AI coding agent of any make —
are in [`AGENTS.md`](AGENTS.md). This file is the human way in.

## Ways to contribute

- **Report a bug** with the bug form. It becomes a `finding`; the next milestone takes it in.
- **Propose a feature** with the feature form, or start with a
  [discussion](https://github.com/Jersyfi/hubtask/discussions) while it is an idea. A feature
  enters the product through a milestone cut ([docs/backlog/README.md](docs/backlog/README.md)).
- **Fix something** with a pull request from your fork. A fix without a task says
  `Readiness: n/a — <why>` and `No issue:` or `Closes #n` in its description.
- **Translate** — see below.
- **Work closely on a part of Hubtask** with your own tools: you take a milestone of your own, follow
  `AGENTS.md`, and put questions for the owner as `decision` issues.

## Setting up a machine

Any machine works; nothing depends on one in particular.

- Go (the version in `go.mod`), Docker, `git`, and the GitHub CLI `gh`.
- Node.js (the version in `.nvmrc`) only for `apps/` and `packages/`.

```bash
make tools          # the Go tools, pinned, into .tools
make tools-node     # pnpm into .tools, only for apps/ and packages/
make db-up && make migrate
make verify         # the fast gates
```

A new worktree has no `.tools`: run `make tools` (and `make tools-node`) in it. A copied
`.tools/pnpm` is a shim that breaks outside its checkout.

Access to the integration and production environments is the maintainers', and its credentials
are never in this repository.

## Signing in locally

`make db-up && make migrate` gives you a database and a schema, but no workspace and nobody to be.
Four things, once:

**1. The application role needs its login.** `make migrate` creates the role without a password;
the migrator binary grants it:

```bash
HUBTASK_DB_DSN=postgres://hubtask:hubtask-dev@localhost:5432/hubtask?sslmode=disable \
HUBTASK_DB_APP_PASSWORD=local-development-only \
go run ./cmd/migrate up
```

**2. Run the server as the application role, never as the database owner.** The owner role carries
`BYPASSRLS`: a server connected as it reads every workspace's rows, and the screens fill with other
people's data while looking entirely plausible. If a listing shows more than you created, check the
role before the code.

```bash
export HUBTASK_DB_DSN=postgres://hubtask_app:local-development-only@localhost:5432/hubtask?sslmode=disable
export HUBTASK_SECRET_KEY=local-development-only-not-a-secret-000000
export HUBTASK_TENANCY_MODE=multi
export HUBTASK_BASE_URL=http://localhost
make run
```

`multi` because single mode resolves "the only workspace", and a database with several resolves
nothing.

**3. Make a workspace and somebody who can sign in.**

```bash
export HUBTASK_PSQL="docker exec -i hubtask-dev-postgres-1 psql -U hubtask -d hubtask"
export HUBTASK_SECRET_KEY=local-development-only-not-a-secret-000000
scripts/dev-workspace.sh --bootstrap          # once per database; prints a token

export HUBTASK_ADMIN_TOKEN=…                  # the token it printed
export HUBTASK_DEMO_PASSWORD='seven blue lanterns above the harbour'
scripts/dev-workspace.sh
```

**4. Open the workspace, not the bare host.**

```bash
.tools/pnpm --filter @hubtask/webapp dev
# http://demo.localhost:5173/
```

An installation that serves several workspaces tells them apart by the subdomain, here as in
production; `*.localhost` resolves to your machine without configuration. The same script points
at the integration environment with `--api`.

## A pull request

- Start it as a draft (`gh pr create --draft`), its description copied from
  `.github/PULL_REQUEST_TEMPLATE.md`; `make gate-pr BODY=<file>` checks it.
- Small commits, one concern each, with a Conventional Commit title in English.
- `make verify-pr` before `gh pr ready`: a draft runs no CI; CI runs once when it is ready
  ([ci-cd.md](docs/architecture/ci-cd.md)). Most jobs skip themselves when their part of the tree
  did not change — that is normal. `CI required` is the one required check.

Everything else — the rules, the loop, the Definition of Done — is in [`AGENTS.md`](AGENTS.md).

## Merging a stack of pull requests (maintainers)

A squash merge rewrites the base of every pull request stacked on it. Merge in order, and after each
merge bring the next one up to date on the server — `gh api repos/Jersyfi/hubtask/merges -f
base=<branch> -f head=main` — rather than rebasing locally. Two sessions merging at once race on the
same `main`: merge from one place at a time. A `BEHIND` or `DIRTY` read right after a push is stale;
read it again half a minute later.

The server merge answers 409 on a conflict, which needs a checkout, and an empty answer when the
branch already contains `main`. Its commit exists only on the remote: a checkout of that branch
merges `origin/<branch>` before it pushes, or the push is rejected. A pull request whose base was
squash-merged is retargeted to `main` and moved with `git rebase --onto origin/main <old-base>
<branch>` and a force-push. Before calling a push done, `git status -sb` shows no divergence.

After a squash, git has no common base for the next branch: every file the previous pull request
touched comes back as a conflict.

- Check the cheap case first: if the previous branch was up to date with `main` before it was
  squashed, `git diff <previous tip> origin/main` is empty, and `git merge -s ours origin/main` on
  the next branch loses nothing. The previous tip is `refs/pull/<n>/head` — head branches are
  deleted on merge.
- Otherwise resolve each file three-way against the fork point (`git merge-file` with
  `git merge-base <branch> <previous tip>`), never "ours wins": that drops what `main` gained.
  `rerere` does not replay across a squash, because the base differs.
- Afterwards `git diff origin/main --stat` shows only the branch's own change.
- A change that lands on `main` mid-stack (a lint, a catalogue entry) is fixed on every remaining
  branch at once.
- `gh pr merge --delete-branch` fails in a worktree while `main` is checked out elsewhere, after
  the merge already happened: read the pull request's state, not the command's exit status.

## Translating

A translation is one file: `locales/<tag>.json`, named with a BCP 47 tag (`de`, `pt-BR`,
`zh-Hans`), mapping the message codes of `locales/en.json` to sentences. The binary embeds every
file in that directory, and `/meta/capabilities` lists a locale the moment its file exists
([i18n-l10n.md](docs/architecture/i18n-l10n.md) §2). An operator can lay a file over an
installation through `HUBTASK_LOCALE_DIR`; a pull request is how it reaches everybody.

**A partial file is welcome.** A missing code renders in English. Translate the families people
meet first (`email.*`, `errors.*`, `seed.*`, `app.*`). Two things hold a file to its source, both
in `make gate-architecture`: a key the source does not have fails, and so does a placeholder that
differs from the source's; a missing key is only reported.

```bash
make locales                              # how complete each translation is, per family
make gate-architecture                    # what is wrong with it, by key
.tools/pnpm --filter @hubtask/webapp test   # the browser's renderer parses every catalogue too
```

Use the ICU subset both renderers implement — simple arguments, `plural` with `offset:` and `=n`,
`selectordinal`, `select`, `#`, nesting, ICU's apostrophe rule — and nothing else. Write the
source's register ([voice-and-tone.md](docs/design/voice-and-tone.md)); German is *du*. A key
beginning with `_` is a note to translators and is never rendered. Edit the file by hand: it is
grouped, not sorted, and re-dumping it buries the change.

## Language

Everything is in English: documentation, code, identifiers, comments, commits. The backend never
contains display text; `locales/en.json` is the source for translations.

## Licence

Hubtask is licensed under the [Apache License 2.0](LICENSE)
([ADR-0080](docs/adr/ADR-0080-hubtask-is-apache-2-0.md)). Contributions are inbound = outbound:
what you submit is licensed under Apache-2.0, as section 5 of the licence says. There is no
Contributor License Agreement and no sign-off. You keep the copyright in your work. The name and
the logo are not covered by the licence — see [TRADEMARK.md](TRADEMARK.md).

## Security

Please do not report vulnerabilities as issues — see [SECURITY.md](SECURITY.md).
