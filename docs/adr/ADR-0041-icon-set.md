# ADR-0041 — Lucide, cut down to a declared subset, behind one `Icon`

**Status:** accepted · **Date:** 2026-09-02

## Context

`design-system.md` §9 named iconography as a gap and half-answered it: "24 px grid, 1.5 px stroke.
Base: Lucide or Phosphor (both MIT), plus roughly 15 custom ones" for the domain nouns. Two
decisions are left in that sentence, and one correction.

The correction first, because it is a fact and not a preference: **Lucide is ISC, not MIT.**
Phosphor is MIT. Both are permissive and both carry the same obligation — keep the notice — so
nothing about the choice changes, but the specification said something untrue about a licence and
this is where it stops saying it.

The two decisions are **which base set**, and **how an icon reaches a component**. The second is
the one with consequences, because of [ADR-0028](./ADR-0028-embedded-web-ui.md): the web UI ships
inside the binary, which carries the bundle byte for byte. An icon set is the kind of dependency
that is small in a web page and permanent in a binary.

## Which base set

Measured on 2026-09-02 by resolving each candidate into a throwaway lockfile, the way
[ADR-0037](./ADR-0037-component-workbench.md) measured Storybook:

```
@phosphor-icons/core@2.1.1   1 package, no transitive deps   5.9 MiB unpacked, 9072 SVGs
lucide-static@1.39.0         1 package, no transitive deps  46.5 MiB unpacked, 2050 SVGs
phosphor-svelte@3.1.0       31 packages (svelte among them)
lucide-svelte@1.0.1         29 packages (svelte among them)
```

**Lucide (chosen).** The deciding fact is not the licence or the package count — it is that Lucide
*is* what §9 asked for. It is drawn on a 24×24 grid as **stroked** paths with
`stroke="currentColor"`, so "1.5 px stroke" is one attribute on the wrapper, and the same file
renders correctly at 16 px with the stroke scaled back up.

**Phosphor (rejected).** A 256×256 grid of **filled** paths in six fixed weights. The stroke is
baked into the outline, so 1.5 px is not something a wrapper can ask for — the choice is one of
six weights, and each is a different drawing. Its 5.9 MiB is the smaller install and its licence is
the one §9 believed both had, and neither of those is worth a set that cannot honour the
specification's one measurable requirement.

The `-svelte` wrappers of both were rejected together: a component per glyph is a runtime
dependency that has to keep pace with Svelte's major versions, for an ergonomic gain the next
section gets for nothing.

## How an icon reaches a component

The usual trade is a single `Icon name="…"` against a direct import per icon: the first keeps call
sites short, the second keeps the bundle honest, because a component that can render any of 1792
glyphs has to carry 1792 glyphs.

**That trade does not exist here, and the reason is worth stating precisely.** `build/icons.js`
holds a *declared list*, and generates `src/icons/base.ts` from it. What is in the repository is
the subset — 46 icons — so there is nothing for a tree-shaker to remove and nothing a direct import
would save. An icon nobody declared is not in `src/` at all, which is a stronger guarantee than an
import a bundler may or may not drop.

So: **one `Icon` taking a name**, over one merged set of base icons and our own marks. Measured,
the whole set is **6,059 bytes of node data for 46 base icons**, plus 14 domain marks.

`lucide-static` is a **devDependency of the design system alone**, and the generated file is
committed — for the reason `LabelTokens.go` is ([ADR-0029](./ADR-0029-design-system-tokens.md)): a
checkout that never ran `pnpm install` still builds, `go build ./...` never learns that Node
exists, and drift shows up as a diff. `icons.test.js` compares the committed file against a fresh
render, so "committed" does not mean "unchecked".

Three smaller decisions follow from the same place:

* **Icon nodes, not markup strings.** `[tag, attributes]` rendered as elements, not a string
  through `{@html}`. The shapes are known at build time, so nothing needs a runtime markup parser —
  and the data form is what lets the test assert that no attribute names a colour.
* **`currentColor`, never a token.** An icon takes the colour of the text it sits in. It is the
  only thing in the package that may name a colour at all, and the two values it may name are
  `currentColor` and `none`.
* **The stroke is scaled with the box.** 1.5 at 24 px, 2.25 at 16 px. A 24-grid stroke shrunk to
  16 px reads as 1, which is a different set on the same page.

## The custom marks

Fourteen, and each is a noun from `domain-model.md` rather than a shape somebody liked: the three
levels that share one aggregate and differ by capability profile, the two container types, the
bucket, the jumble, the capability itself, and the four relationships the model has that a general
set has no word for.

The rule that keeps the set small is the one that keeps it recognisable: **where Lucide already
says a domain noun well, nothing is drawn.** A label is a `tag`, a comment a `message-square`, a
reminder a `bell`, a recurrence a `repeat`, an assignment a `user-check`. Drawing a second version
of an icon everybody already recognises is how a set stops being recognisable.

## Consequences

* `make icons` regenerates the subset; only a change to the declared list needs it. It is separate
  from `make tokens` for the reason `tokens` is separate from `make generate`.
* Adding an icon is one line in `build/icons.js` and a commit of the regenerated file. Removing one
  is the same, which is what keeps the set from growing by accident.
* An upstream rename stops the build rather than dropping a glyph: `check the name in
  build/icons.js` is the error, and Lucide renaming `filter` to `funnel` is how that path is known
  to work.
* The notice for Lucide is in `THIRD-PARTY-LICENSES.md`, in the assets section beside IBM Plex,
  because like the typeface it is an asset in the bundle rather than a linked Go dependency.
* `Icon` joins wave 1 in `design-system.md` §4, ahead of the rest of it: `IconButton` cannot be
  built before there is something to put in it.
