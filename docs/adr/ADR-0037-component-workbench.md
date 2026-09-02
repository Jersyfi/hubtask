# ADR-0037 — The component workbench is ours, and it renders every story through the rules

**Status:** accepted · **Date:** 2026-09-02

## Context

[ADR-0029](./ADR-0029-design-system-tokens.md) left `reference/foundations.html` as the visual
acceptance reference and said Storybook replaces it "once there is a framework".
[ADR-0030](./ADR-0030-svelte-frontend-framework.md) then decided the framework — Svelte 5 — and
deferred the tool itself: choosing it "is part of the component-layer work, not this ADR". This is
that choice.

Thirty-eight components arrive over `F1` to `F3`, and six of them are rules rather than pixels.
`design-system.md` §6 says focus is always visible, motion is confined to `opacity` and
`transform`, colour never stands alone, and **everything grows by 40 %** because German, Finnish
and Russian break any layout measured against English. `§3` adds that alignment is `start`/`end`
only, because RTL is a requirement and not a later port. None of these is checkable by a unit
test, and all of them are checkable by looking — *if there is somewhere to look, and if the thing
one has to look at is actually on screen.*

That is the real requirement. A workbench that shows a component in the one configuration its
author happened to develop in is a gallery. A workbench that shows the same component through
every axis the rules talk about is a checklist that renders itself.

There is a second constraint, and it is the one that decides the tool. This repository has kept
its dependency count deliberately small, and every dependency is a supply-chain decision
(`CLAUDE.md`). A development-only tool is not exempt: it is installed on every contributor's
machine and in every CI run, and it executes at build time.

## Options

Measured on 2026-09-02 by resolving each candidate into a throwaway lockfile, so that the numbers
below are counted rather than estimated.

```
the workspace as it stands today (pnpm-lock.yaml)             352 packages
svelte + vite + @sveltejs/vite-plugin-svelte + typescript     109 packages
the same, plus storybook 10.5.10 + @storybook/svelte-vite     371 packages
```

**A. Storybook 10.** Compatible: `@storybook/svelte-vite@10.5.10` declares `svelte ^5` and
`vite ^5 || ^6 || ^7 || ^8`, which the workspace satisfies. It is the tool everyone already knows,
it has autodocs, a viewport addon, a themes addon and an accessibility panel.

Rejected on the number: **+262 transitive packages**, against a whole-workspace tree of 352. A
75 % increase in what a contributor installs, for a tool no user ever runs. And the features that
justify the price are not the features this system needs — the accessibility panel is `axe-core`
in a tab that a human has to open, which is the opposite of how everything else here is checked
(`gate-selftest`, `lint-no-literals`, F1-02's measured contrast). The axes that matter to Hubtask —
RTL, a +40 % pseudo-locale, reduced motion, both themes at once — are custom decorators and globals
in Storybook too. We would pay 262 packages and still write them.

**B. Histoire.** Rejected on a fact rather than a preference: the current release is
`1.0.0-beta.1` (2026-01-07), and `@histoire/plugin-svelte@1.0.0-beta.1` declares
`peerDependencies: { "svelte": "^3.0.0 || ^4.0.0" }`. There is no Svelte 5 support. Adopting a
stalled tool for a system that has to live for years is the worst of the three outcomes.

**C. A Vite + Svelte page inside the package (chosen).**

## Decision

**The workbench is a small Svelte application in `packages/design-system/workbench/`, and it costs
zero new suppliers.** `vite`, `svelte`, `@sveltejs/vite-plugin-svelte`, `svelte-check` and
`typescript` are already in the lockfile through `apps/webapp`; adding them to this package's
`devDependencies` resolves to versions that are already there. The supply-chain surface grows by
nothing.

**The workbench renders every story through an axis matrix.** The axes are not a feature list;
each one exists because a rule in `design-system.md` is otherwise unobservable.

| Axis | Values | The rule it makes visible |
|---|---|---|
| `theme` | `light` · `dark` · `both` | §6 throughout. The stage always sets `data-theme`, because the stylesheet has no `:root` fallback on purpose |
| `dir` | `ltr` · `rtl` · `both` | §3's `start`/`end` rule. A component with a `left` in it is wrong here and nowhere else |
| `text` | `normal` · `long` | **Rule 4.** A pseudo-locale expands every string by ~40 % and brackets it, so clipping and reflow are visible rather than predicted |
| `motion` | `system` · `reduced` | Rule 6, and §7's requirement that a *user* can switch celebrations off |
| `zoom` | `100` · `200` | WCAG 2.2 SC 1.4.4. The type scale is in `px`, so this is the only way the question gets asked |
| `width` | the five `primitive.breakpoint` values, or the pane | Breadcrumb collapse and every other responsive decision |
| focus walk | a command, not a value | **Rule 5.** It steps through the stage in tab order and reports the sequence |

Two consequences of that table are decisions in their own right:

* **Reduced motion becomes an attribute as well as a media query.** Component and token CSS
  honours `@media (prefers-reduced-motion: reduce)` **and** `[data-motion="reduced"]`. Not for the
  workbench's convenience: `design-system.md` §7 already requires a per-user preference that
  switches celebrations off, and a preference that only the operating system can set is not one.
  The workbench sets the attribute and enforces nothing — a component that ignores it must look
  wrong here, or the axis proves nothing.
* **The workbench is not application code, and the lint agrees.** `lint-no-literals` bans bare
  lengths and durations under `apps/**` and `packages/design-system/src/**`. `workbench/` is
  neither — it is a tool, and its own chrome may carry the geometry that happened to look right,
  exactly as `reference/foundations.html` does today. Colour is still banned everywhere.

**The story format is CSF-shaped, deliberately.** A story module default-exports `title`,
`component` and named story exports carrying `args` — the same shape Storybook's Component Story
Format uses. Two fields are ours (`status`, `axes`). If option A ever becomes right — a second
contributor, or a published gallery — the migration is a rename, not a rewrite. That escape hatch
is what makes choosing C safe rather than merely cheap.

**A component without a story is a build failure.** `build/check-stories.js` runs in
`pnpm test` and is the design system's counterpart to the parity gate: every component in `src/`
has a story, every story names axes that exist, and every component in `src/` appears in one of
`design-system.md` §4's waves. The last rule is the one that matters in a year — it is what stops
the inventory in the specification and the components in the tree from quietly becoming two
different lists. Like every gate here it carries a `--selftest`, because a checker that cannot
fail proves nothing by passing.

**The workbench never reaches a user.** It has its own Vite config and its own scripts
(`pnpm workbench`, `pnpm workbench:build`). It is not part of `pnpm build`, so
`pnpm -r build` does not depend on it, nothing of it enters `apps/webapp`'s bundle, and the binary
that embeds that bundle (ADR-0028) is unchanged to the byte.

## Consequences

* Adding a component means adding a story, or the build goes red. That is the intended pressure:
  `design-system.md` §4 is an inventory, and an inventory nobody can see is a wish list.
* The axes are shared, so a rule added later — a density axis, a forced-colours axis — appears for
  all thirty-eight components at once rather than per component.
* We maintain roughly five hundred lines that Storybook would have maintained for us. That is the
  price of the 262 packages not installed, and it is the part of this decision most likely to be
  revisited.
* Forced visual states (`:hover`, `:active` galleries) are **not** possible without driving a
  browser through CDP, which is a dependency decision of its own. They are not faked here; the
  accessibility conformance work in `F5` is where a headless browser is decided, and `axe-core`
  (zero dependencies) is the obvious candidate when it is.
* `reference/foundations.html` stays for now. It is still what `README.md` and `CLAUDE.md` point
  at, and replacing it is worth doing once the foundations pages are generated from `tokens.json`
  rather than written by hand — which is a task, not a side effect. It is retired at the end of
  wave 1.

## Notes

Related: [ADR-0029](./ADR-0029-design-system-tokens.md) (the workbench it foresaw, and the rule
that no value is written twice), [ADR-0030](./ADR-0030-svelte-frontend-framework.md) (the framework
this waited for, and the deferral this closes),
[ADR-0028](./ADR-0028-embedded-web-ui.md) (the bundle the workbench must not touch),
[ADR-0027](./ADR-0027-monorepo-structure.md) (why nothing here is importable from Go),
[`design-system.md`](../design/design-system.md) (§3, §4, §5, §6, §7 — the rules the axes render).
