---
name: tidy
description: Shorten one document without changing a rule, or remove code a tool reports as unused, each in a pull request of its own. Use it at a cut, or when the owner asks for a tidy pass. Do not use it inside a change in behaviour.
---

# Tidy

Tidying is kept apart from every change in behaviour, so that a removed sentence or function can
be judged by itself.

## What a worker removes, and when

| What | When |
|---|---|
| What the change itself made unused: a function whose last caller it removed, a flag, a translation key, a stub answer | In that change, in a commit of its own. Not a tidy pass. |
| What a tool reports as unused across the repository | In a tidy pull request, below |
| A true sentence that is said only in one place and looks unnecessary | In a tidy pass for that document, below |
| What a model judges unnecessary — a test, an abstraction, a sweep for neatness | Not on a worker's own initiative. It needs a concept or a `finding` the owner accepted. |

Only the owner's decision removes a contract field, a merged migration, a database column or
anything under `docs/archive/`.

## A tidy pass for a document

1. One document per pull request. No rule changes.
2. Before editing, list the document's numbered sections and its rule sentences.
3. Edit by `docs/README.md`, "How to write": leave things out; do not compress what stays.
4. Compare before and after:
   - every numbered section is still there, under its number;
   - every rule sentence is kept, or moved with a link from its old place, or listed in the
     description as removed, with the reason.
5. The independent review makes the same comparison on its own
   (`.agents/skills/review-change/SKILL.md`).

## Removing what a tool reports

1. Take the tool's report: unreachable Go functions, unused translation keys, unused files,
   exports or dependencies of the workspace. A report is valid for one build configuration and can
   be wrong about code reached by other means.
2. One pull request per package. It changes no behaviour.
3. Remove only what the tool reported, and say so in the description, item by item.
4. What is kept on purpose goes on a short list in the description, each item with its reason.
5. Every test passes afterwards.

## Tests

A test is deleted or weakened only when the behaviour it asserts was removed or changed on
purpose, or another test asserts the same thing. The description names each such test and the
reason. A test that a use case cites in `checked_by` leaves only together with the change to that
use case.
