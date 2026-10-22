// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Enter in a field submits its form, where the form's save is in a save bar.
 *
 * A browser submits a form on Enter only through a submit button inside it. A form whose save is
 * `SaveBar`'s keeps no such button - the bar's is an ordinary button, there only while something
 * changed - so a person who typed a value and pressed Enter would have nothing happen. This puts
 * that back: Enter in a single-line field asks the form to submit, which runs its own `onsubmit`
 * as a press of Save would. A text area keeps Enter for its new line, a button for itself.
 */

/** Whether a keydown is Enter, unmodified and not part of a composition. */
const isPlainEnter = (event: KeyboardEvent): boolean =>
  event.key === 'Enter' && !event.isComposing && !event.shiftKey && !event.altKey && !event.ctrlKey && !event.metaKey;

/** Enter in an input submits the form; for a keydown that reaches the form. */
export function submitOnEnter(event: KeyboardEvent): void {
  if (!isPlainEnter(event)) return;
  const field = event.target;
  if (!(field instanceof HTMLInputElement) || field.type === 'checkbox' || field.type === 'radio') return;
  const form = field.form;
  if (!form) return;
  event.preventDefault();
  form.requestSubmit();
}

/** The same as an action on the form: `use:enterSubmits`. */
export function enterSubmits(form: HTMLFormElement): { destroy(): void } {
  form.addEventListener('keydown', submitOnEnter);
  return { destroy: () => form.removeEventListener('keydown', submitOnEnter) };
}
