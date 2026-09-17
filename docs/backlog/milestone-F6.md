# Milestone F6 — Offline, the shells, the moments

**This file is a partial cut.** F6's scope is what `roadmap.md` phase 5's table says — the sync
engine's offline half, the Tauri desktop and mobile shells, the celebration kit and the onboarding
tour — and none of that is cut yet; the owner cuts it, in the manner `0.9.0` and `F5` were cut,
and the numbering continues from `F6-03`. What is here today are the two tasks the owner
scheduled on 2026-09-17, when the review of every `proposed` ADR found two decisions that were
accepted, consistent with the product, and built by nobody: [ADR-0047](../adr/ADR-0047-media-origin-in-the-interface-policy.md)
and [ADR-0048](../adr/ADR-0048-browser-job-driver.md). Their origin tasks — F3-09 and F3-19 —
wrote the records and closed; the building half was owed to no milestone. Both go before `0.9.5`
(`roadmap.md`, rule 4 of "new requirements"): the first because a chart installation on its
default `storage.kind: s3` cannot upload a cover from the browser today, the second because the
conformance claim convergence makes is a claim against browsers a job has run.

**F6 is not a version.** It is a planning milestone; nothing is released by it and the product
version stays the single line ADR-0035 decided. The client's maturity stage stays `preview`.

Every task is one pull request. Both tasks below depend on nothing and may run in either order.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue. Both are **[L]** during the initial phase (`CLAUDE.md`).

---

## F6-01 — The policy names the media origin **[L]**

*Depends on: nothing. Issue #736.*

ADR-0047, accepted 2026-09-17, built here. Under `s3` the transfer URL `POST /media` answers is a
presigned URL on the bucket's origin and the cover's download URL is another; the interface's
policy is `connect-src 'self'` and `img-src 'self' data: blob:` (`security.md` §9, ADR-0028), so
the browser refuses the `PUT` before it is sent and refuses to draw the cover, in front of the
person, with no server involvement. The chart defaults `storage.kind: s3` (`k8s/values.yaml`), so
this is the default Kubernetes installation, not a corner; Compose and the integration environment
run `local`, which is why no walk has found it.

The change is the server's and small. The composition root derives the **origin** — scheme, host,
port, nothing else — from the storage configuration it already parsed, the same way `NewS3Storage`
resolves an empty endpoint to `https://s3.<region>.amazonaws.com`; `webui.NewHandler` takes it as a
parameter and composes the policy once at construction, beside the entity tags it already computes;
under `local` the produced string **equals** `ContentSecurityPolicy` byte for byte, and a test
compares rather than contains, which is what keeps the default installation from widening. Exactly
one origin, never a list, never a wildcard; no other directive moves. `security.md` §9's interface
row gains the sentence, and `deployment.md` gains the operator's half — the bucket's CORS rule:
the interface's origin exactly, method `PUT`, header `Content-Type`, no credentials — because
Hubtask does not own the bucket policy and cannot set it.

Then the proof the record could not have: the integration environment, or a local stack with
MinIO, run under `s3`, a cover uploaded and drawn from the browser, and the evidence in the pull
request. That is the walk F3-09 was proved against `local` for want of.

**Acceptance:** `webui.NewHandler` takes the media origin and the policy under `s3` names it in
`connect-src` and `img-src` and nowhere else; under `local` the policy equals the ADR-0028 constant
and a test asserts equality; the origin is derived from `StorageConfig` in `cmd/server/main.go`
and nowhere in `apps/`; `security.md` §9 and `deployment.md` say what the code does; a cover
uploaded from the browser against an `s3` installation is shown in the pull request; `make verify`
green.

**Read:** ADR-0047 (all of it, including the amendment); ADR-0028; `security.md` §9;
`presentation/webui/Handler.go`; `infrastructure/storage/S3Storage.go` (how the endpoint is
resolved); F3-09 in `milestone-F3.md`; `milestone-F4.md`'s export paragraph (why a backup target
is *not* a second origin)

---

## F6-02 — The browser job **[L]**

*Depends on: nothing. Issue #737.*

ADR-0048, accepted 2026-09-17, built here: Playwright, pinned to an exact version, a
`devDependency` of `apps/webapp` and of nothing else, one job in `ci.yml` that serves the built
`dist/` over a static server of a few lines and loads it in Chromium, Firefox and WebKit. The
dependency is the one CLAUDE.md reserves to the owner, and the owner took it in the record; the
pull request carries the lockfile change with the count it costs, in F1-01's manner.

What the job asserts is ADR-0044's feature table and not a journey: a dialog opens and traps focus;
a gated control is unreachable by keyboard; the focus ring lands where rule 5 puts it; a
visually-hidden label is not visible; an overlay is positioned by CSS and not by the fallback. Each
is a fact about the engine and fails loudly in one that lacks the feature. Three assertions are a
fine first job; the list grows in the pull requests that need it. The job is required through
`CI required` — `main` lists exactly one context, and a browser job that does not gate proves
nothing about what is merged (`ci-cd.md` §5).

Then the two things that waited on it, each in its own commit. **The row**: `support-matrix.md` §5
reads `supported` for the three engines and names the job, with the column saying what actually
ran — Playwright's WebKit is the engine, not Safari, and the row says so rather than overclaiming.
**The fallback**: `packages/design-system/src/positioning.ts` and its test are deleted, and
ADR-0039's status line records that its lifetime ended here. A reviewer sees the deletion as one
thing whose justification is the commit before it.

**Acceptance:** `pnpm-lock.yaml` carries Playwright at an exact version and nothing else new;
`ci.yml` has the job, `CI required` depends on it, and a deliberately broken assertion goes red on a
branch before the job is merged green; `support-matrix.md` §5 names the job and reads `supported`
for the engines it runs; `positioning.ts` is gone and ADR-0039 says so; `ci-cd.md` names the job;
the pull request states the dependency's transitive count.

**Read:** ADR-0048 (all of it, including the amendment); ADR-0044; ADR-0039; `support-matrix.md`
§1, §5; `ci-cd.md` §5; F3-19 in `milestone-F3.md`; F1-01 in `milestone-F1.md` (how a tool decision
is recorded in a pull request)
