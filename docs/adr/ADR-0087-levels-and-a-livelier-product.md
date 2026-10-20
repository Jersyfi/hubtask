# ADR-0087 — Levels, and a livelier product

**Status:** accepted · **Date:** 2026-10-20 · **Decided:** 2026-10-20, by the owner (#1619–#1631)

**Rule lives in:** [domain-model.md](../architecture/domain-model.md) §2,
[design-system.md](../design/design-system.md) §3, §6, §7, §9,
[voice-and-tone.md](../design/voice-and-tone.md) §7.4,
[ai-first.md](../architecture/ai-first.md) §2

## Context

The owner's walk of the product on 2026-10-20 found it plain, low in contrast and slow to use:
canvas and cards differed by 1.07–1.08:1, one desaturated blue carried the whole interface,
creating an entry with a date and a label took about seven clicks, and nothing stayed visible after
work was done. The same walk named the product's actual promise — one model from a grocery list to
a client project — and found that a grocery list offers work packages, labels, members and dates it
never needs, while the system profile could only be narrowed by the installation, and that path had
no writer.

`design-system.md` §7 excluded levels, points, badges, streaks and leaderboards permanently and
asked for "never playful"; §6 kept shadows for overlays only; §3 promised coverage for Arabic,
Hebrew, Devanagari, Thai and CJK that the build did not ship.

## Decision

1. **A collection chooses its structure from a preset, and every setting under it stays free**
   (#1619, #1621, #1622). Presets are data; settings narrow the system profile and never widen it;
   inheritance runs installation → workspace → hub → collection with locks (P-06, P-07).
2. **Narrowing hides, never deletes** (#1620): the cost of keeping hidden values is storage; the
   cost of deleting them would be P-03.
3. **An activity may sit directly under a task** (#1627): the strict ladder made people create a
   work package to hold one step.
4. **Bounded gamification** (#1623): progress, rhythm without penalty, outcome milestones and
   kudos — evidence of competence and relatedness — while points, leaderboards and breaking streaks
   stay excluded because they reward activity over outcome and invite farming. Figures about one
   person are that person's.
5. **A vivid palette, soft depth, and the fonts §3 promised** (#1624, #1625); colour belongs to
   hubs and collections, never to a level, because a level is a preset its collection may outgrow.
6. **AI in place** (#1628, #1629): a single sourced answer is not a chat; proposals may wait on
   arrival when a workspace switches it on, without counts.
7. **A provisional mark** (#1630).

The concept as a whole, its use cases and its order of work are milestone UX (#1631).

## Consequences

* Presets and the hidden-value mark are new in the contract; existing collections keep the
  installation's profile until somebody changes them.
* Every screen changes; `docs/backlog/milestone-UX.md` holds the inventory that proves no control
  is lost.
* The font payload grows; `unicode-range` keeps it out of every page whose text does not need it.
