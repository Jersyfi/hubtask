# ADR-0048 — The browser job's driver: Playwright, pinned, in one workflow job

**Status:** proposed · **Date:** 2026-09-08

## Context

[`support-matrix.md`](../architecture/support-matrix.md) §1 defines `supported` as **"a CI job runs
the software on it"**. [ADR-0044](./ADR-0044-browser-support-row.md) decided *which* engines the
client is intended to work in — the current and previous major of Chromium, Gecko and WebKit — and
was explicit that the row cannot say `supported` for any of them, because no CI job runs a browser
at all. It named that job as option D and left it open, and its own consequences section calls it
"the only thing standing between this row and a real one".

Two things wait on it, and neither can move without it:

* **The row.** Three engines that say `best effort` — where a defect will be *fixed*, not where
  anybody has *looked*.
* **[ADR-0039](./ADR-0039-overlay-positioning.md)'s fallback.** Seventy-nine lines of script
  positioning in `packages/design-system/src/positioning.ts`, kept as insurance while nothing proves
  the row. Every engine on the row has CSS Anchor Positioning, so the script path is unreachable by
  any browser this project promises to work in — and ADR-0039 said when it was accepted that its
  lifetime is exactly this.

**A browser job needs a browser driver, and this workspace has none.** `pnpm-lock.yaml` carries no
Playwright, no Puppeteer, no WebDriver, no `@web/test-runner`. Adding one is a supply-chain decision
(`CLAUDE.md`: "every dependency is a supply chain decision"), which is why this is an ADR rather
than a pull request with a lockfile change in it.

**What the job is for is narrow, and that shapes the choice.** The issue says it plainly: not an
end-to-end suite. The question is whether the client *runs* in each engine — do the dialogs open, is
a refused control genuinely inert, is the focus ring where rule 5 puts it. ADR-0044's own feature
table is the list: `<dialog>` and `showModal()`, `inert`, `:has()`, `popover`, `clip-path`, logical
properties. Each of those is one assertion in a page that is already built.

## Options

**A. Playwright (chosen).** One dependency that drives all three engines, with the browser binaries
it needs pinned to the package version and downloaded by `playwright install`.

**B. WebDriverIO with a driver per engine.** The standards-track path: `chromedriver`,
`geckodriver`, `safaridriver`. Rejected on the shape of the dependency rather than the quality of
the tool: it is three drivers plus a runner, each versioned against a browser this project does not
install, and `safaridriver` needs macOS — which turns one Linux job into a matrix with a macOS
runner in it, for the one engine whose runner GitHub is withdrawing from the free tier
(`support-matrix.md` §4 records the same problem for the Intel macOS row). More moving parts, and
one of them cannot run where the rest of CI runs.

**C. Puppeteer.** Smaller, well made, and it drives Chromium and Firefox. It does not drive WebKit,
so the row's third engine would stay `best effort` — which is the engine most likely to differ and
the one a support row is most useful about. Rejected for covering two thirds of the question.

**D. `@web/test-runner` with browser launchers.** A test runner rather than a driver, and its
WebKit support is Playwright's launcher underneath — so it is option A with an extra layer, and the
layer buys a runner this project does not need: the assertions are `node:test`'s everywhere else,
and the job's job is to open a page and look.

**E. No job; leave the row `best effort` for ever.** Honest, and what the project has today. It
means the client's behavioural claims stay "checked once, by one person, in one Chromium" — which
is what F2's pull requests actually say — and it means F5 cannot make a WCAG conformance claim,
because such a claim is made *against a set of browsers* and this project would have proven none.
Rejected because ADR-0044 already weighed it and asked for the job.

## Decision

**Playwright, pinned to an exact version, running one job that loads the built bundle in Chromium,
Firefox and WebKit and asserts that the client runs.**

Five things this decides, each because the alternative is worse:

**1. One dependency, three engines.** Playwright is the only option that drives all three from one
Linux runner. `pnpm dlx playwright install --with-deps` fetches browsers pinned to the package
version, so the job's engines move when somebody upgrades the package and not when a runner image
changes underneath it.

**2. It is a `devDependency` of `apps/webapp`, and of nothing else.** The bundle it loads is that
workspace's, and no other package needs a browser. Nothing in the shipped image gains a dependency:
the container build's `ui` stage runs `pnpm build`, which this does not touch.

**3. The job serves the built bundle, not a dev server.** `pnpm build` already produces `dist/`, and
what ships is what is served — a job that tested a dev server would be testing a thing no reader
ever loads. A static file server over `dist/` is a few lines and needs no dependency.

**4. What it asserts is ADR-0044's feature table, not a user journey.** A dialog opens and traps
focus; a gated control is unreachable by keyboard; a focus ring lands on the element rule 5 names; a
visually-hidden label is not visible; an overlay is positioned by CSS rather than by the fallback.
Each is a fact about the engine, and each fails loudly in an engine that lacks the feature. Deeper
journeys are F5's and F6's, and building them here would make the job slow enough to be skipped.

**5. It becomes a required check, through `CI required`.** `ci-cd.md` §5 and the branch protection
memory: `main` lists exactly one context. A browser job that is not required proves nothing about
what is merged — and `supported` in §1 means a job that *gates*, not a job that runs.

## Consequences

* **The row becomes `supported` for the three engines**, and `support-matrix.md` §5's "Proven by"
  column names the job. `Anything older` stays `unsupported` for the reason it already gives.
* **ADR-0039's fallback can be deleted**, with the test that covers it. That is a separate change in
  the same task and should be a separate commit: it is a deletion whose justification is this job's
  existence, and a reviewer should be able to see it as one thing.
* **CI grows a job with a browser download in it.** Cached by Playwright's version key, which is why
  the version is pinned rather than floating: a floating version is a cache that misses and three
  browsers downloaded on every run.
* **A WebKit failure on a Linux runner is not a Safari failure.** Playwright's WebKit is a build of
  the same engine, not Safari, and the row should keep saying "WebKit (Safari)" while the job's
  column says what it actually ran. That is a smaller gap than having no job, and pretending
  otherwise would be the kind of overclaim §1 exists to prevent.
* **This ADR does not decide the assertions' content.** It decides the driver and the shape. The
  first job may reasonably start with three assertions and grow.

## Notes

Related: [ADR-0044](./ADR-0044-browser-support-row.md) (which engines, and this job as its open
option D), [ADR-0039](./ADR-0039-overlay-positioning.md) (the fallback whose lifetime this ends),
[ADR-0030](./ADR-0030-svelte-frontend-framework.md) and
[ADR-0028](./ADR-0028-embedded-web-ui.md) (what is built and how it is served),
[`support-matrix.md`](../architecture/support-matrix.md) §1 and §5 (the definition this satisfies),
`ci-cd.md` §5 (what makes a job a required check).

Nothing here is implemented: the dependency is the decision, and it waits.
