# Components live here — Svelte 5, built deliberately

Wave 0 is built: `Box`, `Stack`, `Inline` and `VisuallyHidden`, the four primitives everything
else lays itself out with ([`design-system.md`](../../../docs/design/design-system.md) §4), and
`Icon`, which wave 1 brought forward because `IconButton` cannot be built before there is one.
Beside them are `space.ts` (the scale, as a type read from the generated tokens),
`layers.ts` (which layer `Escape` reaches), `icons/` (the set — `base.ts` is generated, `custom.ts`
is the domain marks) and `index.ts`, which is what `@hubtask/design-system/components` resolves
to.

Everything after them arrives through the component-layer work package (roadmap, frontend track),
wave by wave in the order §4 proposes — not as a side effect of feature work.

Five constraints apply to every component here:

* **A component without a `<Name>.stories.ts` beside it fails the build**, and so does one that
  appears in no wave of `design-system.md` §4
  ([ADR-0037](../../../docs/adr/ADR-0037-component-workbench.md)). Run `make workbench` to see it
  through every axis, and `pnpm test` to be told when you have not. A file whose name starts with
  `_` is a part rather than a component and owes neither.
* **No inline `style`.** [ADR-0028](../../../docs/adr/ADR-0028-embedded-web-ui.md)'s content
  security policy is `style-src 'self'` with no `'unsafe-inline'`, so a `style="gap: …"` written by
  a component is a rule the browser refuses — silently, in production, and not in a workbench
  served without the header. Wave 0 shows the shape that works: the value travels as a `data-`
  attribute and a stylesheet rule selects on it.
* **No colour, spacing, radius, duration or `z-index`** is written anywhere but in
  `tokens/tokens.json` ([ADR-0029](../../../docs/adr/ADR-0029-design-system-tokens.md)). A stacking
  order comes from `primitive.layer`; `pnpm lint` fails on the rest.
* **A primitive produces no visual style of its own** — no colour, no border, no shadow. One that
  decorates is a component, and belongs in a wave that plans it.
* **An icon takes the colour of the text it sits in.** `currentColor` and `none` are the only two
  colours anything under `icons/` may name ([ADR-0041](../../../docs/adr/ADR-0041-icon-set.md)).
  `base.ts` is generated from the declared list in `build/icons.js` — add a name there and run
  `make icons`, never edit the file.
