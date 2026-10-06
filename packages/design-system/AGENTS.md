# packages/design-system — one origin for every value

`tokens/tokens.json`, what is generated from it, the components both clients use, and the
workbench. It implements [`docs/design/design-system.md`](../../docs/design/design-system.md);
[`README.md`](./README.md) describes the package.

## What must not happen here

* **No second place for a value** (rule 15). `pnpm lint` fails on a colour outside `tokens.json`,
  on a bare length or duration, and on a `var(--x)` that nothing declares — it paints nothing,
  silently. A new value goes into `tokens.json`, then `make tokens`.
* **No sentence in a component.** A component takes resolved text as a prop; every string is a
  message code in `locales/en.json`, written as
  [`voice-and-tone.md`](../../docs/design/voice-and-tone.md) says.
* **No component added or changed as a side effect of other work.** `src/` grows wave by wave in
  the order of `design-system.md` §4; a component an application needs is a design system task of
  its own.
* **No component without a story.** A `<Name>.svelte` in `src/` needs a `<Name>.stories.ts` beside
  it and a place in one of §4's waves, or `pnpm test` fails.
* **No inline `style`.** The content security policy refuses one in production, never in the
  workbench. A value travels as a `data-` attribute a stylesheet rule selects on.
* **No `z-index` at a call site, and no overlay that measures its anchor.** Layers come from
  `primitive.layer` in `tokens.json`, what `Escape` reaches from `src/layers.ts`, and where an
  overlay is drawn from `src/anchor.ts`.
* **No `disabled` boolean.** `disabledReason` is what disables. `test/conventions.test.js` also
  fails on a physical `left`/`right`, state as a prop, an animated length, a boolean that does not
  ask a question, a native input hidden from the accessibility tree, and an interactive component
  without a focus ring.
* **No colour pair below its floor.** `pnpm test` measures the WCAG 2.2 contrast of every declared
  pair in both modes — 4.5:1 for text, 3:1 for a control boundary and the focus ring. A new
  semantic colour token needs a role in `test/contrast.test.js`; an unclassified one fails.
* **No icon that names a colour** — `currentColor` and `none` only. `src/icons/base.ts` is
  generated from the list in `build/icons.js`: add a name there and run `make icons`.
* **No hand edit of `dist/` or of `core/domain/model/shared/LabelTokens.go`**, and no colour value
  in the Go output — it carries the ten label token names only.
* **Nothing loaded from a foreign domain**, fonts included, and **no `:root` fallback** for the
  semantic layer: a document without `data-theme` must look broken at once.
* **No workbench in the product.** `workbench/` is not part of `pnpm build`, and nothing of it
  reaches `apps/webapp` or the binary. It is published, so the page is public: it promises nothing
  about the product, contacts no foreign domain, and has no form that submits. A control that only
  narrows what is already on the page is not a form.

**The gates read comments.** `test/conventions.test.js` matches element names in the whole Svelte
source and `build/lint-no-literals.js` matches values in comments. In a comment, name an element
without angle brackets ("a native select list") and describe a measurement without its number.

## How to check a change

```bash
make tokens                                       # or: pnpm --filter @hubtask/design-system build
pnpm --filter @hubtask/design-system lint
pnpm --filter @hubtask/design-system test         # tokens, contrast, layers, icons, stories, conventions
pnpm --filter @hubtask/design-system typecheck    # svelte-check over src/ and workbench/, stories included
make icons                                        # only after changing the declared icon list
make workbench                                    # look at it on :5174
git diff --exit-code core/domain/model/shared/LabelTokens.go
```

A pull request that says a component is right links the workbench state that shows it
(`?story=…&theme=dark&dir=rtl&zoom=200`). After a token change, look at `Foundations/Tokens` with
Theme set to Both.
