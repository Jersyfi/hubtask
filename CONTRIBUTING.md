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

## Linux, macOS, Windows

Develop on whichever of the three you like. Every gate is a make target and the recipes are bash,
so what a platform needs is bash, GNU make, and the Go toolchain. The `Toolchain (…)` job in CI runs
`make tools`, `gate-quick`, `gate-architecture` and `gate-unit` on macOS and Windows on every pull
request, which is what keeps this true rather than merely intended.

**Linux** — nothing to arrange. This is what the other CI jobs run.

**macOS** — the `make` from the Xcode command line tools is enough (`xcode-select --install`), as is
its bash 3.2; nothing here needs bash 4. The container suites want a Docker endpoint: Docker Desktop
works as is, and for Colima or Podman set `DOCKER_HOST` so Testcontainers finds the socket.

**Windows** — install make (`winget install ezwinports.make`) and run it **from Git Bash**, not from
cmd or PowerShell. The Makefile looks bash up on PATH rather than at `/bin/bash`, and both scripts
under `scripts/` are bash. For `gate-unit` you also need a C compiler, because the race detector
needs cgo — `winget install BrechtSanders.WinLibs.POSIX.UCRT` provides one. Without it, run the same
packages without the detector and leave `-race` to CI:

```bash
go test ./core/... ./infrastructure/... ./presentation/...
```

**Everywhere** — line endings are pinned to LF by `.gitattributes`. A clone made before that file
existed still has a CRLF work tree, and `gofmt -l .` then flags every file in the repository; on a
clean tree, `git rm --cached -r .` followed by `git reset --hard` refreshes it.

What you genuinely cannot run locally, CI runs on the pull request — say which gate you skipped
rather than ticking "`make verify` green locally" for one you did not.

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
