# ADR-0027 — One repository for the core, the clients, and the design system

**Status:** accepted · **Date:** 2026-08-22

## Context

Until now this repository has held one deployable thing: the Go binary and everything that
describes it. `apps/` and `packages/` did not exist because there was no client to put in them.
C-14 (arc42 §2.2) states that frontend design and frontend feature set are *not* decided, and R-08
records the cost of that: without a client, user feedback arrives late.

That is now changing. Three first-party clients are foreseeable — the to-do application in the
browser, the project website at hubtask.eu, and the design system both of them consume — and none
of them exists yet. Where they are put is decided once, cheaply, and is expensive to revisit later,
because a repository split has to be undone commit by commit.

The question is not "monorepo or polyrepo" in the abstract. It is what happens to a change that
touches both sides of the OpenAPI contract, which is the change this project will make most often.
[ADR-0004](./ADR-0004-api-first-openapi.md) makes `api/openapi.yaml` the source: the specification
changes, `make generate` runs, and both the server types and every client's types are regenerated
from it. Across two repositories that is two pull requests with an ordering constraint, a window in
which `main` of one repository does not satisfy the contract of the other, and a version pin to
carry the coupling that the file system would otherwise carry for free.

Four further facts constrain the answer:

* **Milestones and releases are repository-scoped on GitHub.** [ADR-0022](./ADR-0022-github-platform.md)
  puts the process on GitHub, and `docs/backlog/` plus the `task` issues are the ledger. A milestone
  cannot span repositories. A second repository means a second milestone list to keep in step by
  hand, and CLAUDE.md's "the issue is the task" stops having one answer.
* **The ADRs govern the whole product, not the backend.** This very file is an example: it decides
  where the frontend lives. An ADR set that governs code in another repository is either duplicated
  or ignored.
* **Development here is AI-assisted.** An assistant reasons about a contract change from what is in
  its working tree. Both halves of the change in one tree is the difference between reading the
  producer and the consumer and guessing at one of them.
* **One maintainer.** Every boundary that has to be maintained by hand — a version pin, a
  synchronised milestone, a mirrored CI configuration — is paid for out of the same budget that
  would otherwise implement features.

## Options

**A. One repository per application.** `hubtask`, `hubtask-webapp`, `hubtask-website`,
`hubtask-design-system`. Rejected: it loses the atomic contract change, which is the change that
happens most; it multiplies milestones and issue ledgers by four when there is one maintainer to
keep them consistent; and the design system, whose entire purpose is to be consumed by two clients,
would reach them through a published version rather than through the workspace, so every token
change becomes a release.

**B. The core plus a single `hubtask-clients` repository.** The same problem with one repository
fewer. The cut still falls exactly across the OpenAPI contract, which is the one place it must not
fall. It would additionally put the design system in the clients repository, where the generated Go
constant list of ADR-0029 could not be committed alongside the domain that validates against it.

**C. Splitting the website out immediately.** The website is genuinely independent — it is not
embedded, it has no API contract with the core, and it will eventually deploy on its own cadence.
Rejected for now as premature: with one maintainer the coupling that matters is the design system,
which the website consumes exactly as the webapp does, and a separate repository would make that a
published dependency before there is anything to publish. This is the split most likely to become
right later, and nothing here prevents it.

**D. One repository for the core, the clients, and the design system (chosen).**

## Decision

**One repository holds the Go core, all first-party clients, and the design system.**

The layout gains two top-level directories:

```text
apps/
  webapp/     # the Hubtask to-do application in the browser — the product UI
  website/    # the project website (hubtask.eu) — information only, no task management
packages/
  design-system/   # tokens, CSS layer, the visual reference (ADR-0029)
  api-client/      # generated from api/openapi.yaml, never hand-edited
```

The names are load-bearing. **`apps/web` is not a name in this project** — it does not say which of
the two it means, and the two have nothing in common beyond a design system.

The rules that keep the addition from costing anything:

1. **The Go module path stays `github.com/Jersyfi/hubtask`.** No organisation transfer is planned
   and a module path is not a directory layout.
2. **`core/` does not learn that a frontend exists.** The UI is an inbound adapter like REST or MCP
   ([ADR-0028](./ADR-0028-embedded-web-ui.md) decides how), and rule 1 of CLAUDE.md is untouched:
   the dependencies still point inwards.
3. **`apps/*` may depend on `packages/*`; never the reverse, and never `apps/*` on `apps/*`.**
4. **Nothing under `apps/` or `packages/` is importable from Go.** With one deliberate exception in
   the other direction: ADR-0029 generates a Go constant list *out of* the design system *into* the
   core, and no Go file is committed under `packages/` or `apps/`.
5. **A backend-only contributor never installs Node.js.** `go build ./...`, `go test ./...` and
   `golangci-lint run` work in a checkout with no JavaScript toolchain present. What makes that true
   in the presence of an embedded bundle is ADR-0028's committed placeholder.
6. **JavaScript builds are pnpm workspaces plus the existing `Makefile`.** No monorepo build
   orchestrator. Nx, Turborepo and Bazel each solve a problem — task graphs across dozens of
   packages, remote caching — that four packages and one maintainer do not have, and each is a
   second build system to keep in step with `make verify`.

**Deferred, to be revisited before `1.0.0`:** extracting the generated SDKs into a separately
licensed repository. [ADR-0013](./ADR-0013-licensing.md) puts this repository under BSL 1.1, and a
client library nobody may use in commercial production is a client library nobody builds on — which
defeats the purpose of shipping one. Until that is decided, generated clients live in
`packages/api-client` under the repository licence, and `packages/api-client` is deliberately kept
free of anything but generated output so that the extraction stays a move rather than a rewrite.

## Consequences

* A contract change is one pull request: `api/openapi.yaml`, `make generate`, server and client,
  reviewed together. The window in which the two halves disagree does not exist.
* One milestone list, one issue ledger, one ADR set, one CLAUDE.md hierarchy. The backlog keeps
  meaning what it says.
* Every pull request now touches a repository in which most jobs are irrelevant to it. CI must
  therefore run per changed area rather than per repository — decided and built separately, with
  the specific trap that a skipped required check blocks a merge forever.
* `git log` and `git blame` cover the whole product. Moves into the new layout use `git mv`, so the
  history of a moved file survives.
* The repository gets larger and a clone gets slower. At this size that is measured in seconds and
  is the cheapest thing being traded.
* The webapp's frontend framework is **not** decided here. This ADR decides where it will live;
  the choice itself needs its own ADR and stays reversible until it is made. C-14 continues to hold
  in the meantime: the backend makes no assumptions about it.

## Notes

Related: [ADR-0004](./ADR-0004-api-first-openapi.md) (the contract that makes the cut expensive),
[ADR-0022](./ADR-0022-github-platform.md) (milestones and issues are repository-scoped),
[ADR-0013](./ADR-0013-licensing.md) (why the SDK extraction is deferred rather than dismissed),
[ADR-0028](./ADR-0028-embedded-web-ui.md) (how the webapp reaches a user),
[ADR-0029](./ADR-0029-design-system-tokens.md) (what `packages/design-system` contains),
[ADR-0001](./ADR-0001-hexagonal-architecture.md) (the dependency direction the new directories do
not touch), [project-structure.md](../architecture/project-structure.md) §1 and §2, arc42 §2.2 C-14.
