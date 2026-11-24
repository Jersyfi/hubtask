# ADR-0114 — Icons carry our own names, with the source's name beside them

**Status:** accepted · **Date:** 2026-11-24 · **Decided:** 2026-11-24, by the owner

**Rule lives in:** [design-system.md](../design/design-system.md) §12

## Context

lucide-static 1.77 renamed five icons the product uses, `circle-alert` among them. The name of an
icon is not only in code: entry types store it in the database, and the shipped templates name it.
Taking Lucide's new names would have meant a rename across the clients and a migration of stored
data, again at every upstream rename.

## Decision

* **The product names an icon by its own name.** Code, templates and stored data use only that
  name, and it does not change when the source does.
* **Each declared icon names its source glyph beside it** in `build/icons.js`: our name, then
  Lucide's. Where both agree, both are written.
* Our names stay neutral: they say what the glyph means or shows, and no product's name. The names
  in use today are kept as they are.

This amends [ADR-0041](./ADR-0041-icon-set.md), Consequences: an upstream rename still stops the
build, and is answered by changing the source name in that one line.

## Consequences

* An upstream rename changes one line and the regenerated `base.ts`. It changes no client code and
  no stored data.
* Changing the icon source would change the right-hand column only.
