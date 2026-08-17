# Contributing

Thanks for your interest. This project has a fully documented architecture — that is not an
accident, it is the basis you work from.

## Before you write code

1. Read `CLAUDE.md`. The thirteen rules it lists apply to humans just the same.
2. Read the document under `docs/architecture/` that matches what you are doing, and the ADRs it
   links to.
3. Open an issue before starting anything substantial. For architectural changes open an ADR
   issue, not a pull request presenting a decision as already made.

## The loop

```bash
make tools
make db-up && make migrate
# work
make verify          # must be green
```

Branch names: `feat/short-description`, `fix/…`, `docs/…`, `chore/…`.

**Conventional Commits**, in English:

```
feat(workmanagement): add container archiving

Archived containers remain restorable indefinitely (F-10).
Refs #42
```

Scopes correspond to the bounded contexts in `docs/architecture/arc42.md` §5.

## Running the gates on Windows

The gates assume a POSIX shell and CI runs them on Linux. Two things a stock Windows machine does
not have:

* **`make` itself** — `winget install ezwinports.make`, and invoke it from Git Bash: the Makefile
  sets `SHELL := /bin/bash`, and `scripts/gate-selftest.sh` is a bash script.
* **A C compiler** — the race detector needs cgo, which is why `gate-unit` is the one target that
  overrides `CGO_ENABLED=1`. Without a toolchain that target cannot run as written. Run the same
  packages without the detector and leave `-race` to CI:

  ```bash
  go test ./core/... ./infrastructure/... ./presentation/...
  ```

`gate-quick`, `gate-architecture` and `gate-security` run on Windows unchanged. Line endings are
pinned to LF by `.gitattributes`; a clone made before that file existed still has a CRLF work tree,
and `gofmt -l .` then flags every file in the repository. On a clean tree, `git rm --cached -r .`
followed by `git reset --hard` refreshes it.

What you cannot run locally, CI runs on the pull request — say which gate you skipped rather than
ticking "`make verify` green locally" for one you did not.

## One pull request, one issue

Every task in `docs/backlog/` has an issue (label `task`). Your pull request closes it by naming it
in the `Closes #` line of the template — otherwise the work is merged and the issue stays open,
which makes the board lie about what is done. A question that blocks the task belongs in the issue
rather than in the pull request: the issue outlives the branch.

## One pull request, several commits

A task is one pull request — but not one commit. Split the work into steps of roughly one commit
each and record the split as a checklist in the pull request body, before you start. Open the pull
request as a **draft** at that point; it takes the checklist and marks the work as running.

Per step: one commit, pushed straight away, and its box ticked. `make gate-quick` stays green at
every commit and `make verify` at the last one; only then does the pull request leave draft.

A step is one concern. Tests travel with the code they test, not in a trailing "add tests" commit.
No `wip` or `fixup` commits: while the pull request is a draft you may rewrite history, once it is
in review only new commits. The squash merge collapses the chain into a single commit on `main`
(`docs/architecture/versioning-release.md` §3), so the steps are read in the pull request.

Two reasons this matters more than tidiness: a reviewer follows five small steps and rarely follows
one large diff, and pushed work survives an interrupted session — including one where an AI
assistant loses its context mid-task.

## What makes a pull request acceptable

The template lists the Definition of Done. Two items are missed most often:

* **A cross-tenant negative test** for every new repository method. Without it, gate SG-3 fails.
* **A merge rule** for every new field on `WorkItem` (LWW, OR-set, fractional index, or
  server-side) — otherwise the behaviour on offline conflicts is undefined.

## Language

Everything is in English: documentation, code, identifiers, code comments, commit titles, and
commit bodies. Message codes and `locales/en.json` are the source for translations — the backend
never contains display text.

## Licence and CLA

Contributions are published under the project licence ([LICENSE](LICENSE), BSL 1.1, converting to
Apache-2.0 three years after each release). Contributors sign a
[Contributor License Agreement](CLA.md) on their first pull request; a bot posts the link
automatically. It exists because the conversion to Apache-2.0 and the sale of commercial licences
both require the Licensor to hold sufficient rights in the whole codebase. You keep full ownership
of your work. The reasoning, and what it costs you, is in [ADR-0013](docs/adr/ADR-0013-licensing.md).

## Security

Please do not report vulnerabilities as issues — see [SECURITY.md](SECURITY.md).
