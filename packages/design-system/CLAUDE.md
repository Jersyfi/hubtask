# packages/design-system — one origin for every value

`tokens/tokens.json` is the **only** place a colour, spacing, radius, duration or easing value is
defined in this product. Everything else here is generated from it
([ADR-0029](../../docs/adr/ADR-0029-design-system-tokens.md)). The specification this implements
is [`docs/design/design-system.md`](../../docs/design/design-system.md), and it is binding.

## What must not happen here

* **No second place for a value.** A design system drifts at exactly the point where the same
  number is written twice. `pnpm lint` fails on a colour written outside `tokens.json`, and on a
  bare length or duration in application code.
* **No sentence in a component.** Every string is a message code in `locales/en.json`
  ([ADR-0011](../../docs/adr/ADR-0011-i18n-message-codes.md)); a component takes resolved text as a
  prop. How that text is written is
  [`voice-and-tone.md`](../../docs/design/voice-and-tone.md) — sentence case, the verb of what
  happens on a button, an error that names the fix.
* **No component out of turn.** `src/` holds wave 0 — `Box`, `Stack`, `Inline`,
  `VisuallyHidden` — and all of wave 1, and grows wave by wave per `design-system.md` §4, through
  the component-layer work package. No component arrives here as a side effect of other work.
* **No inline `style`.** ADR-0028's policy is `style-src 'self'` with no `'unsafe-inline'`, so a
  `style="gap: …"` is a rule the browser refuses — in production only, never in a workbench served
  without the header. The value travels as a `data-` attribute and a stylesheet rule selects on it;
  wave 0 is the worked example.
* **No `z-index` written at a call site.** It comes from `primitive.layer` in `tokens.json`, and
  what `Escape` reaches comes from `src/layers.ts` — one register, not one per overlay. Where an
  overlay is *drawn* comes from `src/positioning.ts`
  ([ADR-0039](../../docs/adr/ADR-0039-overlay-positioning.md)): no component measures an anchor
  itself, and neither path writes an inline style, because the CSP refuses one.
* **No component without a story.** A `<Name>.svelte` in `src/` needs a `<Name>.stories.ts` beside
  it and a place in one of `design-system.md` §4's waves; `pnpm test` fails otherwise. That is the
  design system's parity gate — the specification's inventory and the tree may not become two
  different lists.
* **No workbench in the product.** `workbench/` is a development tool. It is not part of
  `pnpm build`, `pnpm -r build` does not depend on it, and nothing of it may reach `apps/webapp`'s
  bundle or the binary that embeds it (ADR-0028). Being published at `workbench.hubtask.eu`
  (ADR-0038) does not change any of that — but it does mean **the page is public**: nothing there
  may promise anything about the product, contact a foreign domain, or carry a form.
* **No pair below its floor.** `pnpm test` measures the WCAG 2.2 contrast of every pair
  `tokens.json` declares, in both modes: 4.5:1 for text, 3:1 for a control boundary and the focus
  ring. A new semantic colour token needs a role in `test/contrast.test.js` — an unclassified token
  fails the suite rather than being skipped, which is what stops the check from shrinking.
* **No control that switches off without saying why.** There is no `disabled` boolean in this
  package: `disabledReason` is what disables, and `test/conventions.test.js` fails on the other
  spelling. Same file: no physical `left`/`right`, no state as a prop, no animated length, no
  boolean that does not ask a question, and no native input hidden from the accessibility tree.
* **No icon that names a colour.** The set takes `currentColor` from the text it sits in
  ([ADR-0041](../../docs/adr/ADR-0041-icon-set.md)); `currentColor` and `none` are the only values
  a mark may carry. `src/icons/base.ts` is generated from the declared list in `build/icons.js` —
  add a name there and run `make icons`; the test fails on a file that is out of step.
* **No colour value in the Go output.** `LabelTokens.go` carries the ten *names* and nothing else,
  so the core keeps the vocabulary while staying colour-blind. A hex constant in `core/domain`
  would be display information in the backend.
* **No hand-edited `dist/`.** It is generated. So is
  `core/domain/model/shared/LabelTokens.go` — that one is committed, which makes it editable and
  therefore worth saying twice: **never edit it**, CI regenerates it and fails on a diff.
* **No font loaded from a foreign domain.** IBM Plex ships in the bundle. A self-hosted Hubtask
  must contact nobody on load.
* **No `:root` fallback for the semantic layer.** A document without `data-theme` should look
  broken at once rather than quietly pick a mode nobody chose.

## Adding a value

Edit `tokens.json`, run `make tokens`, commit the regenerated `LabelTokens.go` if the label set
changed. Never the other way round.

## How to check a change

```bash
make tokens                                       # or: pnpm --filter @hubtask/design-system build
pnpm --filter @hubtask/design-system lint         # no value outside tokens.json
pnpm --filter @hubtask/design-system test         # tokens, contrast, layers, icons, stories
make icons                                        # only after changing the declared icon list
pnpm --filter @hubtask/design-system typecheck    # svelte-check over workbench/ and src/
make workbench                                    # look at it: :5174
git diff --exit-code core/domain/model/shared/LabelTokens.go   # must be empty after committing
```

Every axis of the workbench is in the query string. A pull request that says a component is right
in dark RTL at 200 % links to it — `?story=…&theme=dark&dir=rtl&zoom=200` — rather than asserting
it (ADR-0037).

`reference/foundations.html` is the **visual acceptance reference**. Open it after any token
change and check both modes. It imports the generated stylesheet and declares no value of its own —
if you find yourself adding one there, that is the bug.
