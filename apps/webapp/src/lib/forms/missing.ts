// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a form still needs, found when it is submitted - never announced in advance
 * (design-system.md §4, case 3; UC-WRK-32 check 3).
 *
 * A form's submit is never switched off because something is empty. Pressed, the form asks this
 * module what is missing, marks each such field with its error, sends nothing, and moves focus to
 * the first one; changing a marked field takes its mark away. The words are message codes the
 * caller renders (rule 8: this module holds none).
 *
 * Three shapes of requirement, because forms have three:
 * * **a field** that must hold something - a name;
 * * **at least one** of a group - the scopes of a token, the types a field applies to;
 * * **exactly one way** of several - a reminder counted from the due date *or* at a fixed time,
 *   where the first way cannot be filled and the second is the way out.
 *
 * The caller lists them in document order, and the first missing one is the one focus goes to.
 */

export type Requirement =
  | { readonly kind: 'field'; readonly key: string; readonly code: string; readonly isFilled: boolean }
  | { readonly kind: 'at-least-one'; readonly key: string; readonly code: string; readonly chosen: number }
  | { readonly kind: 'one-of'; readonly key: string; readonly code: string; readonly ways: readonly boolean[] };

/** The missing ones, by key, each with the code of its error; empty means the form may be sent. */
export type Missing = Readonly<Record<string, string>>;

/** Whether one requirement is met. */
export function isMet(requirement: Requirement): boolean {
  switch (requirement.kind) {
    case 'field':
      return requirement.isFilled;
    case 'at-least-one':
      return requirement.chosen > 0;
    case 'one-of':
      return requirement.ways.filter(Boolean).length === 1;
  }
}

/** What is missing, in the order the caller listed it. */
export function check(requirements: readonly Requirement[]): Missing {
  const missing: Record<string, string> = {};
  for (const requirement of requirements) {
    if (!isMet(requirement)) missing[requirement.key] = requirement.code;
  }
  return missing;
}

/** Whether nothing is missing. */
export const isComplete = (missing: Missing): boolean => Object.keys(missing).length === 0;

/** The marks without one key's: what a change to that field or group does. */
export function without(missing: Missing, key: string): Missing {
  if (!(key in missing)) return missing;
  const { [key]: _gone, ...rest } = missing;
  return rest;
}

/**
 * The selector every marked field answers to: the control itself where its role takes
 * `aria-invalid`, or the field's root marked `data-invalid` where it does not (a fieldset, an
 * upload's button, an input's shell) - the design system's one convention.
 */
export const MARKED = '[aria-invalid="true"], [data-invalid]';

const FOCUSABLE = 'input, select, textarea, button, [tabindex]:not([tabindex="-1"]), a[href]';

/**
 * Moves focus to the first marked field inside `root`, in document order: the element itself
 * where it takes focus, else the first control inside it. Called after the marks are drawn.
 * Answers whether it found one.
 */
export function focusFirst(root: ParentNode | null | undefined): boolean {
  const first = root?.querySelector<HTMLElement>(MARKED);
  if (!first) return false;
  const target = first.matches(FOCUSABLE) && first.tabIndex >= 0 ? first : first.querySelector<HTMLElement>(FOCUSABLE);
  if (!target) return false;
  target.focus();
  return true;
}
