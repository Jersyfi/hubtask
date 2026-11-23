// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the state mark is to a screen reader and what it draws (design-system.md §4, wave 7): the
// APG Checkbox pattern wherever one press completes or reopens, a plain button named by its step
// where the press moves the entry on without completing it.

/** `check`: the press completes or reopens. `step`: it takes a step that does neither. */
export type MarkForm = 'check' | 'step';

export interface MarkLook {
  readonly role: 'checkbox' | undefined;
  readonly ariaChecked: 'true' | 'false' | undefined;
  readonly icon: 'check' | 'arrow-right';
}

export function markLook(form: MarkForm, checked: boolean): MarkLook {
  if (form === 'step') return { role: undefined, ariaChecked: undefined, icon: 'arrow-right' };
  return { role: 'checkbox', ariaChecked: checked ? 'true' : 'false', icon: 'check' };
}
