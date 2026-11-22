// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The arithmetic of choosing several of many, kept out of the DOM so that it can be tested
// (multiselect.test.js) - the bargain focus.ts and structure.ts make. `MultiSelect` draws what this
// answers: which presentation, what a press emits, what the closed field says, what the filter
// leaves (design-system.md §4, wave 1; ADR-0110).

/** One answer. The shape `CheckboxOption` has, so that a call site moves between the two unchanged. */
export interface ChoiceOption {
  readonly value: string;
  /** Display text: a resolved message code (ADR-0011), or a name the reader gave. */
  readonly label: string;
  /** One line under the label. It describes the option; it is never part of its name. */
  readonly hint?: string;
  /**
   * Why this option cannot change. Ticked, it is locked on; unticked, it is refused. Either way it
   * stays focusable and says why.
   */
  readonly disabledReason?: string;
}

/** Answers under one heading - the four groups of what a level carries. */
export interface ChoiceGroup {
  readonly label: string;
  readonly options: readonly ChoiceOption[];
}

/** What `presentation` may ask for; `auto` decides by the number of options. */
export type Presentation = 'auto' | 'inline' | 'field';

/**
 * The most options still drawn as visible ticks. Up to five a person reads every answer at a
 * glance; beyond that a wall of boxes hides what is chosen (ADR-0110).
 */
export const INLINE_MOST = 5;

/** From this many options the open list has a filter, and *All* and *None*. */
export const FILTER_FROM = 8;

export function presentationOf(requested: Presentation, count: number): 'inline' | 'field' {
  if (requested !== 'auto') return requested;
  return count <= INLINE_MOST ? 'inline' : 'field';
}

export function hasFilter(count: number): boolean {
  return count >= FILTER_FROM;
}

/** Cannot change by a press, *All*, *None* or Ctrl+A. */
export function isFixed(option: ChoiceOption): boolean {
  return option.disabledReason !== undefined;
}

/** Ticked and cannot be cleared - an option that is always on. */
export function isLocked(option: ChoiceOption, selected: readonly string[]): boolean {
  return isFixed(option) && selected.includes(option.value);
}

/**
 * The selection after one press. The caller's order is kept and a new value goes to its end, as
 * `CheckboxGroup` does; a value no option names stays, because the caller may know more than
 * this list draws.
 */
export function toggled(selected: readonly string[], value: string, options: readonly ChoiceOption[]): readonly string[] {
  const option = options.find((each) => each.value === value);
  if (!option || isFixed(option)) return selected;
  return selected.includes(value) ? selected.filter((held) => held !== value) : [...selected, value];
}

/** Every option of `options` that can change, ticked; the rest as they were. */
export function withAll(selected: readonly string[], options: readonly ChoiceOption[]): readonly string[] {
  const added = options.filter((each) => !isFixed(each) && !selected.includes(each.value)).map((each) => each.value);
  return [...selected, ...added];
}

/** Every option of `options` that can change, cleared; a locked one stays. */
export function withNone(selected: readonly string[], options: readonly ChoiceOption[]): readonly string[] {
  const cleared = new Set(options.filter((each) => !isFixed(each)).map((each) => each.value));
  return selected.filter((held) => !cleared.has(held));
}

/** A run of options under a heading, or under none - the options given outside a group. */
export interface Section {
  readonly label?: string;
  readonly options: readonly ChoiceOption[];
}

/** The options without a group first, then each group in its order; a group of none is not drawn. */
export function sectionsOf(options: readonly ChoiceOption[], groups: readonly ChoiceGroup[]): readonly Section[] {
  const out: Section[] = [];
  if (options.length > 0) out.push({ options });
  for (const group of groups) if (group.options.length > 0) out.push({ label: group.label, options: group.options });
  return out;
}

/**
 * What the filter leaves: options whose label or hint contains the term, folded under the reader's
 * locale (under `tr` a capital I lowers to a dotless ı); a group with no match is left out.
 */
export function filterSections(sections: readonly Section[], term: string, locale?: string): readonly Section[] {
  const needle = term.trim().toLocaleLowerCase(locale);
  if (needle === '') return sections;
  const matches = (text: string | undefined) => text !== undefined && text.toLocaleLowerCase(locale).includes(needle);
  return sections
    .map((section) => ({ ...section, options: section.options.filter((each) => matches(each.label) || matches(each.hint)) }))
    .filter((section) => section.options.length > 0);
}

/** One chosen option as the closed field shows it. */
export interface Chip {
  readonly value: string;
  readonly label: string;
  readonly isLocked: boolean;
}

export type Closed =
  | { readonly kind: 'empty'; readonly text: string }
  | { readonly kind: 'summary'; readonly text: string }
  | { readonly kind: 'chips'; readonly chips: readonly Chip[] };

/** A sentence for the closed field, or `undefined` to show the chips. */
export type Summary = (chosen: readonly ChoiceOption[], total: number) => string | undefined;

/**
 * What the closed field says: the caller's empty text, the caller's sentence, or the chosen as
 * chips - in the options' order, a locked one first, because it is the one the reader cannot
 * change and the first a reader asks about.
 */
export function closedOf(
  all: readonly ChoiceOption[],
  selected: readonly string[],
  { emptyLabel, summary }: { readonly emptyLabel: string; readonly summary?: Summary },
): Closed {
  const chosen = all.filter((each) => selected.includes(each.value));
  if (chosen.length === 0) return { kind: 'empty', text: emptyLabel };
  const sentence = summary?.(chosen, all.length);
  if (sentence !== undefined) return { kind: 'summary', text: sentence };
  const chip = (each: ChoiceOption): Chip => ({ value: each.value, label: each.label, isLocked: isLocked(each, selected) });
  return { kind: 'chips', chips: [...chosen.filter((each) => isLocked(each, selected)), ...chosen.filter((each) => !isLocked(each, selected))].map(chip) };
}

/**
 * The chips drawn when only `fitting` fit in the field's two lines: one place goes to the count of
 * the rest, and one chip is always drawn, so the field never shows a bare number.
 */
export function cutChips<T>(chips: readonly T[], fitting: number): { readonly shown: readonly T[]; readonly more: number } {
  if (fitting >= chips.length) return { shown: chips, more: 0 };
  const shown = chips.slice(0, Math.max(1, fitting - 1));
  return { shown, more: chips.length - shown.length };
}
