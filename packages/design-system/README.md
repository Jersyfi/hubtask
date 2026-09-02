# @hubtask/design-system

Design tokens and the CSS layer generated from them. Not a component library — not yet.

`tokens/tokens.json` is the **single origin** for every colour, spacing, radius, duration and
easing value in this product ([ADR-0029](../../docs/adr/ADR-0029-design-system-tokens.md)).
Everything else here is generated from it. The specification this package implements is
[`docs/design/design-system.md`](../../docs/design/design-system.md), and it is binding the way
`project-structure.md` is binding for the Go layout.

## The rule

**No hex value, no pixel count and no duration is written anywhere else.** If you need a value
that does not exist, add it to `tokens.json` — or you do not need it. `pnpm lint` fails on a
colour written outside that file, and on a bare length or duration written in application code.

A design system drifts at exactly the point where the same value is written twice. Nothing here
depends on anybody remembering that.

## What is generated

```
tokens/tokens.json                        the source, W3C DTCG
  │  pnpm build   (Style Dictionary)
  ├─→ dist/tokens.css                     custom properties, :root + [data-theme]
  ├─→ dist/tokens.ts                      typed constants for TypeScript consumers
  ├─→ dist/fonts.css + dist/fonts/        IBM Plex, self-hosted
  └─→ ../../core/domain/model/shared/LabelTokens.go
                                          the ten label token NAMES, for the backend
```

`dist/` is ignored by git. `LabelTokens.go` is **committed**, because `go build ./...` has to work
for somebody who has never installed Node.js — and because that makes a drift between the design
system and the domain show up as a diff rather than as a rendering bug. It is generated: never
edit it, and CI fails if you do.

Only the *names* reach Go, never a colour. The domain stores a `colorToken` on `Label` and on
`cover` precisely so that the backend holds no display information; a hex constant in `core/`
would undo that.

## Using it

```html
<html data-theme="dark">
  <link rel="stylesheet" href="…/tokens.css">
  <link rel="stylesheet" href="…/fonts.css">
```

There is deliberately **no** fallback for a document without `data-theme`: a page that forgets it
should look broken at once rather than quietly pick a mode nobody chose.

From TypeScript, bind to the custom property, not to the value:

```ts
import { tokens } from '@hubtask/design-system';
element.style.color = tokens.text.primary;   // 'var(--text-primary)'
```

`values.light` and `values.dark` hold the resolved literals, for the few consumers that cannot use
a custom property — a canvas, an image, a mail template. Anything that renders in a browser uses
`tokens`, because a component that writes the light-mode colour is a component that is wrong in
dark mode and no type can catch it.

## Commands

```bash
pnpm build            # regenerate all four targets
pnpm lint             # no value written outside tokens.json
pnpm test             # the properties ADR-0029 rests on, and the workbench's story gate
pnpm typecheck        # svelte-check over workbench/ and src/
pnpm workbench        # the component workbench on :5174 (ADR-0037)
pnpm workbench:build  # a static copy of it, into dist/workbench/
```

From the repository root, `make tokens` does the same as `pnpm build`.

`reference/foundations.html` is the visual acceptance reference for the **tokens**: open it after
a change and both modes must still look right. It imports the generated stylesheet and declares no
value of its own. It is retired at the end of wave 1, once the foundations pages are generated
from `tokens.json` rather than written by hand ([ADR-0037](../../docs/adr/ADR-0037-component-workbench.md)).

## The workbench

```bash
pnpm workbench     # http://localhost:5174
```

The component workbench of [ADR-0037](../../docs/adr/ADR-0037-component-workbench.md): a small
Svelte page in `workbench/`, built from dependencies the lockfile already carried, rather than
Storybook — which resolves to 262 further packages for a tool no user runs.

It renders every story through an **axis matrix**, and every axis is there because a rule in
`design-system.md` is otherwise unobservable:

| Axis | Values | What it makes visible |
|---|---|---|
| Theme | light · dark · both | §6 in both modes, side by side |
| Direction | ltr · rtl · both | §3's `start`/`end` — a `left` shows up here and nowhere else |
| Text | normal · +40 % | **Rule 4**, through a pseudo-locale. Clipping is a defect, not a rendering |
| Motion | system · reduced | Rule 6, via `[data-motion="reduced"]` as well as the media query |
| Zoom | 100 % · 200 % | WCAG 2.2 SC 1.4.4 — the type scale is in `px` |
| Width | the five breakpoints, or the pane | Responsive decisions at the widths the tokens declare |
| Keyboard order | a command | **Rule 5** — it walks focus through the stage and reports the order |

Every axis is in the query string, so
`?story=…&theme=dark&dir=rtl&text=long&zoom=200` is a link a reviewer can open. Use one in a pull
request instead of claiming a component was checked.

**A component without a story is a build failure.** `pnpm test` runs `build/check-stories.js`:
every component in `src/` has a story beside it, every story names axes that exist, and every
component in `src/` appears in one of `design-system.md` §4's waves — so the inventory and the
tree cannot become two different lists. It carries a `--selftest`, like every other gate here.

The workbench is a development tool. It is not part of `pnpm build`, `pnpm -r build` does not
depend on it, and nothing of it reaches `apps/webapp`'s bundle or the binary that embeds it
(ADR-0028).

## What is not here

Components. The framework is decided — Svelte 5
([ADR-0030](../../docs/adr/ADR-0030-svelte-frontend-framework.md)) — and there is now somewhere to
look at them, but `src/` stays empty until the component-layer work package builds it
deliberately, wave by wave — see [`src/README.md`](./src/README.md). The specimen the workbench
shows today lives in `workbench/fixtures/` on purpose: it is a fixture for checking the tool, not
a component pretending to exist.
