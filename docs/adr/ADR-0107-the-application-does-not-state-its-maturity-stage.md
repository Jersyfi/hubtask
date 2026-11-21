# ADR-0107 — The application does not state its maturity stage

**Status:** accepted · **Date:** 2026-11-21 · **Decided:** 2026-11-21, by the owner ·
**Supersedes:** the in-application notice of [ADR-0035](./ADR-0035-one-product-version.md) §2 and
the stage row of [ADR-0065](./ADR-0065-the-second-walk-of-the-shell.md) decision 4

**Rule lives in:** [versioning-release.md](../architecture/versioning-release.md) §5 (client
maturity); [design-system.md](../design/design-system.md) §11.6 (the notice mark)

## Context

ADR-0035 §2 answers "can I rely on this yet?" with a maturity stage, stated per release in the
release notes and `CHANGELOG.md`, and by the application about itself while it is not `stable`.
ADR-0065 decision 4 moved that statement from a banner into the notice mark in the app bar. Since
then every installation shows the quiet mark on every screen, and pressed it says "Hubtask is a
preview". The owner decided that the notice goes on every installation.

## Decision

* **The application does not state its maturity stage.** The notice mark carries only what is
  wrong: the health report where it says something is wrong, and a manifest that could not be
  read. With neither, no mark is drawn.
* **The release notes and `CHANGELOG.md` keep naming the stage**, per release, as ADR-0035 §2 says.
  The stages and what each promises are unchanged, and the stage still goes to `stable` at
  convergence (ADR-0035 §5).
* Everything else in ADR-0035 and ADR-0065 decision 4 holds.

## Alternatives

* *A setting to hide the notice per installation*: a switch for a sentence nobody wanted kept,
  and a statement about a release that differs per installation is no statement about the release.
* *Keep the notice until `stable`*: what ADR-0035 decided, and what the owner reversed.

## Consequences

`apps/webapp/src/lib/maturity.ts` and the `app.maturity.*` texts go, since nothing else reads them.
A deployment that does not want an unfinished interface keeps the switch it had:
`HUBTASK_UI_ENABLED=false` (ADR-0028).
