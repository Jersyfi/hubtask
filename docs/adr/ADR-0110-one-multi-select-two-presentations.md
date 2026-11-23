# ADR-0110 — One multi-select, two presentations

**Status:** accepted · **Date:** 2026-11-22 · **Decided:** 2026-11-22, by the owner

**Rule lives in:** [design-system.md](../design/design-system.md) §4, wave 1 (`MultiSelect`)

## Context

Choosing several of many was drawn as a row of checkboxes wherever it occurred: the structure
panel's fifteen capabilities per level, up to forty-five boxes on one screen, as an unordered wall
that hid what was chosen; and a handful of other places that each built their own loop. The design
system had a fieldset of a few boxes (`CheckboxGroup`) and two pickers made for people and labels,
but nothing for a list whose length the data decides.

## Decision

* **One component, `MultiSelect`, with two presentations of the same option list.** `inline` is
  visible ticks that wrap, for up to five options; `field` is one closed button that says what is
  chosen and opens a checklist, for six or more. `auto` decides by the count; a screen may force
  one. The option shape is `CheckboxGroup`'s, so a call site moves between the two unchanged, and
  `CheckboxGroup` stays as the form's fieldset, its boxes drawn by the same ticks.
* **The closed field summarises.** The chosen as chips - a summary, not buttons - cut after two
  lines with the count of the rest; or the caller's sentence when nearly all are chosen; or
  its empty text. Removing happens in the list.
* **The open list is a multi-selectable listbox** (WAI-ARIA APG Listbox): groups by heading, a
  line under an option that describes it and is not its name, and from eight options a filter,
  *All* and *None* and a count. The list stays open while toggling and keeps the caller's order.
* **An option with a reason cannot change** - ticked, it is locked on and drawn with a lock; not
  ticked, it is refused - and stays focusable with its reason in the list. The whole control
  locked still opens, read-only.
* **At a phone's width the list is the `Popover`'s bottom sheet**, with *Done*; a popover become a
  sheet keeps the popover's rank for `Escape`, so inside a dialog the list closes first.

## Alternatives

* *A segmented control for several answers*: compact, but labels of uneven length wrap badly in
  German, it has no room for a reason or a line, and `ViewSwitcher` already means "pick one".
* *A transfer list* (two lists and move buttons): twice the width, no answer on a phone, many tab
  stops, and no place here needs an order.
* *Removable chips with a cross each*: a tab stop and a small target per chip, and a locked
  option would need a chip without one.

## Consequences

Every hand-made tick row and checkbox loop moves to `MultiSelect` or stays `CheckboxGroup`, screen
by screen. The people an entry is assigned to keep `AssigneeControl`, and its labels
`LabelPicker`; members or readers chosen from a list of people are `MultiSelect` (a page's readers
need a lock on one person, which `AssigneeControl` has not). What a screen reader
announces for a locked, selected option (`aria-disabled` with `aria-selected`) is not tested with
one.
