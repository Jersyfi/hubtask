# ADR-0035 — One product version for the server and every first-party client

**Status:** accepted · **Date:** 2026-08-25

## Context

The client track opens with `0.4.0`, and how its work is versioned has never been decided — only
assumed, in two sentences that no longer agree with the repository:

* [`roadmap.md`](../roadmap.md) phase 5 is headed "Frontend (in parallel from `0.4.0`, **with its
  own versioning**)". No decision carries that clause; it predates every client ADR.
* `onboarding.md` §2: "The frontend gets its own repository later, because it has its own
  versioning and works through the published API."
  [ADR-0027](./ADR-0027-monorepo-structure.md) reversed the repository half. The versioning half
  travelled on unexamined.

Four accepted decisions have since made the question far narrower than "how does one version a
frontend":

* [ADR-0028](./ADR-0028-embedded-web-ui.md): the web app is built into the server binary from the
  same commit. `presentation/webui/Embed.go` states the reason in its own doc comment — two
  artefacts would make it possible "to run a UI against an API of a different version".
* [ADR-0031](./ADR-0031-tauri-app-shell.md): the Tauri shells are the only first-party client
  artefacts distributed on their own — installers, updaters, store listings.
* [ADR-0033](./ADR-0033-shared-client-architecture.md): one product UI in `apps/webapp` builds
  every target. There is one client codebase, not three.
* [ADR-0027](./ADR-0027-monorepo-structure.md): the website is a separate deployment, never
  embedded, and it speaks no contract to anything.

A second input makes the question urgent rather than tidy. The client track is developed **in
parallel with a core that is still moving**, which means the client is deliberately *incomplete*
for most of the `0.x` phase — the alternative, a frontend that starts once the backend is finished,
delays `1.0.0` by its own whole duration. Whatever expresses that incompleteness has to be decided
together with the version, or the version number ends up carrying a meaning it cannot hold.

## Options

**A. A SemVer line per client** — `hubtask-webapp 0.3.0` next to `hubtask 0.6.0`, and a shell
version next to both. Rejected. A version answers "which artefact is this, and what is it
compatible with". The web app has no artefact of its own: it is bytes inside a binary that already
carries a version, and giving it a number invites back the exact question ADR-0028 removed — which
UI version is in this image? The number would also be maintained by hand, because nothing releases
it, and a hand-maintained version is wrong within two releases.

**B. Independent versions per workspace package** — the JavaScript-ecosystem default: changesets,
one version and one changelog per package. Rejected: that machinery exists for packages published
to a registry and consumed by strangers. Nothing here is published; every `apps/*` and `packages/*`
entry is `private: true` and its only consumer is this repository. It would buy a release ceremony
for an audience of none.

**C. One product version, with maturity stages instead of a second number (chosen).**

## Decision

### 1. The product has exactly one version, and every first-party client carries it

[`versioning-release.md`](../architecture/versioning-release.md) §1 gains the client rows:

| Artefact | Version | Rule |
|---|---|---|
| Web app bundle | none of its own | Built from the same commit and embedded (ADR-0028); it is not separately released, so it is not separately versioned |
| Tauri shells | `appVersion` = the product version, plus a platform build counter per build | The stores require a monotonic counter (`CFBundleVersion`, Android `versionCode`) that SemVer does not provide. A store-only fix — signing, metadata, a rejected listing — bumps the counter, never the product version. The same split the Helm chart already uses |
| Website | unversioned | Continuously deployed and identified by its commit. It carries no compatibility surface, so a version would state nothing |
| Workspace packages (`apps/*`, `packages/*`) | `private: true`; the `version` field is not a product statement | Nothing is published to a registry and the release automation does not read them |

The rule behind the table: **a version belongs to an artefact somebody can install by itself.**
The shells are that; the bundle and the website are not.

### 2. Client maturity, not a second version line

What a second number was wanted for is a real question — "can I rely on this yet?" — and it gets a
real answer that is not a number:

| Stage | What it promises | When |
|---|---|---|
| `experimental` | The client builds and runs. Screens appear, move and disappear without notice; nothing in it is a commitment | The first two client milestones |
| `preview` | What it shows is meant to stay and is usable for real work; the gaps against the capability matrix are expected and listed | Until convergence |
| `stable` | The capability matrix of [ADR-0032](./ADR-0032-client-capability-matrix.md) is met, and a regression in the client blocks a release exactly as a regression in the API does | From the convergence milestone onwards |

The stage is stated per release, in the release notes and `CHANGELOG.md`, and the application says
it about itself — a banner while it is not `stable`. It is deliberately **not** added to
`/meta/capabilities`: `web_ui` there is a runtime fact (is an interface being served?), and a
maturity stage is a statement about a release. Putting it in the manifest would make a product
adjective part of the API contract, which is exactly the kind of thing that then cannot be removed.

A deployment that does not want an unfinished interface has the switch it already has:
`HUBTASK_UI_ENABLED=false` (ADR-0028) answers `/` with 404 and leaves the API untouched.

### 3. Parallel development: incomplete is not broken

The two states are different and are treated differently, and this is what makes a client track
alongside a moving core affordable:

* **Incomplete** is the normal condition and costs nothing: a use case the core serves has no
  screen yet. It is visible in the maturity stage and in the coverage report the convergence
  milestone produces.
* **Broken** is a defect like any other: the client lane of CI — build, lint, typecheck, test —
  is green at **every** commit, on the same terms as the Go gates. A red client lane is not
  "the frontend catching up"; it is a build to be fixed before the next one lands.

### 4. A change to the contract carries its client fix, in the same pull request

`packages/api-client` is generated from `api/openapi.yaml`, and the web app typechecks against it.
A renamed or removed field therefore turns the client lane red **in the pull request that changed
the contract** — deliberately, because that is the one moment when the true cost of the change is
visible and still cheap.

So the fix belongs there too, not in a follow-up issue. A follow-up issue is how `main` acquires a
client that no longer builds, and how a rename that looked free turns out to cost four screens a
fortnight later. Additive changes break nothing and imply no client work; this rule is about the
ones [`versioning-release.md`](../architecture/versioning-release.md) §2 already calls a break.

This is also the answer to requirements that arrive late and touch both sides: they are ordinary
work — the contract moves first ([ADR-0004](./ADR-0004-api-first-openapi.md)), the client moves
with it — until the convergence milestone closes the scope window.

### 5. How a major is finished

The shape below is not a one-off arrangement for `1.0.0`. It is how a **completed major** is
reached in this project, `2.0` and every later one included:

1. **Parallel development.** Tracks run alongside each other and may lag each other; the client
   track runs one milestone window behind the core, so that it builds against a contract that has
   just settled rather than one still moving.
2. **Convergence.** A milestone of its own, whose whole purpose is that both tracks arrive: the
   client meets the capability matrix, the maturity stage goes to `stable`, the **scope window
   closes**, and everything with external lead time — store review above all — is set in motion.
   New requirements are accepted up to the day it opens; after it, defects only, and an exception
   is its own ADR.
3. **Stabilisation.** The major's prerequisites are demonstrated and only defects are fixed.

**A major is released when the server, the clients and the website are finished together, from one
commit.** That is the whole point of one version: there is no arrangement in which the product is
released and its client follows later, because there is no number in which that could be said.

## Consequences

* `roadmap.md` phase 5 loses "with its own versioning" and gains a schedule: the client track with
  its milestones, the convergence milestone before `1.0.0`, and the client half of the `1.0.0`
  prerequisites.
* The release automation is untouched — it computes one version from the Conventional Commits, as
  it does today. It gains exactly one thing when the shells arrive: the platform build counter.
* A client regression cannot be deferred to "the next client release", because there is none. Before
  `stable` that is what the maturity stage says out loud; after it, a client regression blocks the
  release.
* Anybody wanting `@hubtask/api-client` as a versioned dependency from outside this repository does
  not get one. ADR-0027 already deferred the SDK extraction; if it happens, the extracted SDK is
  published and versioned because it will then have an audience — a new decision, not a
  contradiction of this one.
* The two sentences that assumed otherwise go: the roadmap heading here, and `onboarding.md`, which
  is retired in the same change — its setup instructions live in `CONTRIBUTING.md`, its repository
  configuration in [`ci-cd.md`](../architecture/ci-cd.md) §4 and §6, and its working process in
  `CLAUDE.md`.

### Backlog impact

| Work package | Target |
|---|---|
| The maturity banner in the web app, and the stage stated in the release notes | client track, with the application frame |
| Platform build counters and the release lane for the shells | client track, with the desktop shell |

No issue is created now; both ride with the work packages they belong to.

## Notes

Related: [ADR-0028](./ADR-0028-embedded-web-ui.md) (why the bundle has no artefact of its own),
[ADR-0031](./ADR-0031-tauri-app-shell.md) (the only independently distributed clients),
[ADR-0033](./ADR-0033-shared-client-architecture.md) (one codebase, three targets),
[ADR-0032](./ADR-0032-client-capability-matrix.md) (what `stable` has to mean),
[ADR-0027](./ADR-0027-monorepo-structure.md) (the repository half of the assumption this replaces),
[ADR-0004](./ADR-0004-api-first-openapi.md) (contract first, which rule 4 depends on),
[`versioning-release.md`](../architecture/versioning-release.md) (where rules 1, 2 and 5 land).
