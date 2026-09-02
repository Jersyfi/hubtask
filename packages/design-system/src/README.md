# Components live here — Svelte 5, built deliberately

This directory is still empty, now on different grounds.

The framework decision this directory waited for has been taken:
**Svelte 5** ([ADR-0030](../../../docs/adr/ADR-0030-svelte-frontend-framework.md)). Components
here are Svelte components, consumed by `apps/webapp` and `apps/website`.

They arrive through the component-layer work package (roadmap, frontend track), wave by wave in
the order `design-system.md` §4 proposes — not as a side effect of feature work. Three constraints
apply to every component:

* **A component here without a `<Name>.stories.ts` beside it fails the build**, and so does one
  that appears in no wave of `design-system.md` §4
  ([ADR-0037](../../../docs/adr/ADR-0037-component-workbench.md)). Run `pnpm workbench` to see it
  through every axis, and `pnpm test` to be told when you have not.
* The content security policy of [ADR-0028](../../../docs/adr/ADR-0028-embedded-web-ui.md) allows
  neither `'unsafe-inline'` nor `'unsafe-eval'`; styles compile to the external stylesheet.
* No colour, spacing, radius or duration is written anywhere but in `tokens/tokens.json`
  ([ADR-0029](../../../docs/adr/ADR-0029-design-system-tokens.md)).
