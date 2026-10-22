// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The frame's one listener for the global shortcuts: it reads where a press happened and hands it
 * to `intentOf` (`lib/shortcuts.ts`), which decides. What each intent does is the caller's; an
 * intent nobody handles - capture, until it exists - is left alone, so a key the list does not show
 * does nothing either.
 */

import { layers } from '@hubtask/design-system/components';

import { intentOf, type Intent } from '../shortcuts.ts';

/** Text inputs: every input but the ones that are pressed rather than typed into. */
const PRESSED = new Set(['button', 'submit', 'reset', 'checkbox', 'radio', 'range', 'color', 'file', 'image']);

function isTextField(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  if (target instanceof HTMLTextAreaElement || target instanceof HTMLSelectElement) return true;
  return target instanceof HTMLInputElement && !PRESSED.has(target.type);
}

function isInWidget(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest('[role="menu"], [role="listbox"], [role="grid"]') !== null;
}

/**
 * Listens on the document until the returned cleanup runs. `isPaletteOpen` is asked per press,
 * because the palette may be open in its dialog, which is a layer of its own.
 */
export function listenForShortcuts(
  isPaletteOpen: () => boolean,
  handlers: Partial<Record<Intent, () => void>>,
): () => void {
  const onKeydown = (event: KeyboardEvent) => {
    const intent = intentOf(event, {
      isTextField: isTextField(event.target),
      isInWidget: isInWidget(event.target),
      isLayerOpen: layers.top() !== null,
      isPaletteOpen: isPaletteOpen(),
    });
    const handler = intent ? handlers[intent] : undefined;
    if (!handler) return;
    event.preventDefault();
    handler();
  };
  document.addEventListener('keydown', onKeydown);
  return () => document.removeEventListener('keydown', onKeydown);
}
