// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Every keyboard shortcut of the web app, once (UC-WRK-27 check 3), and which press means which of
 * the global ones.
 *
 * **One list.** *Help and shortcuts* draws it, the palette draws a row's key from it, and the frame's
 * listener asks `intentOf` - so the key a reader is shown and the key that works are the same
 * entry. The list and rank keys on a list or a board are handled where they act (`rankIntent` in
 * the design system, the selection in the list); they are listed here so a reader can find them.
 *
 * **`⌘K` and `Ctrl K` both, everywhere.** The web build cannot tell a Mac from anything else
 * without reading the platform at run time, which this client does not do (design-system.md §6,
 * "width is not platform"); so both work on every system and both are shown. `⌘K` acts in a text field too. `Ctrl K` does
 * not: in a field it is the field's own key on some systems, and that is the one thing given up.
 *
 * **A single character acts only where nothing is being typed** (WCAG 2.1.4): `/` and `?` in a
 * field type themselves, and so they do in a menu, a listbox or a grid, whose keys are their own.
 *
 * Pure, and tested beside itself; `shortcuts.svelte.ts` listens.
 */

/** Where a shortcut applies, which is how *Help and shortcuts* groups them. */
export type Scope = 'everywhere' | 'palette' | 'list' | 'selection';

export interface Shortcut {
  readonly id: string;
  readonly scope: Scope;
  /** What it does, as a message code. */
  readonly code: string;
  /** The keys as drawn, each a way to press it. Key names, not sentences. */
  readonly drawn: readonly string[];
  /** The same as `aria-keyshortcuts` writes them. */
  readonly aria: readonly string[];
}

export const SHORTCUTS: readonly Shortcut[] = [
  { id: 'palette', scope: 'everywhere', code: 'app.shortcuts.palette', drawn: ['⌘K', 'Ctrl K', '/'], aria: ['Meta+K', 'Control+K', '/'] },
  { id: 'help', scope: 'everywhere', code: 'app.shortcuts.help', drawn: ['?'], aria: ['Shift+?'] },
  { id: 'palette-move', scope: 'palette', code: 'app.shortcuts.palette_move', drawn: ['↑', '↓'], aria: ['ArrowUp', 'ArrowDown'] },
  { id: 'palette-enter', scope: 'palette', code: 'app.shortcuts.palette_enter', drawn: ['Enter'], aria: ['Enter'] },
  { id: 'palette-escape', scope: 'palette', code: 'app.shortcuts.palette_escape', drawn: ['Esc'], aria: ['Escape'] },
  { id: 'rank-step', scope: 'list', code: 'app.shortcuts.rank_step', drawn: ['Alt ↑', 'Alt ↓'], aria: ['Alt+ArrowUp', 'Alt+ArrowDown'] },
  { id: 'rank-end', scope: 'list', code: 'app.shortcuts.rank_end', drawn: ['Alt Shift ↑', 'Alt Shift ↓'], aria: ['Alt+Shift+ArrowUp', 'Alt+Shift+ArrowDown'] },
  { id: 'select-one', scope: 'selection', code: 'app.shortcuts.select_one', drawn: ['Ctrl click', '⌘ click'], aria: [] },
  { id: 'select-range', scope: 'selection', code: 'app.shortcuts.select_range', drawn: ['Shift click'], aria: [] },
  { id: 'select-end', scope: 'selection', code: 'app.shortcuts.select_end', drawn: ['Esc'], aria: ['Escape'] },
];

/** The keys of a shortcut as drawn; nothing for an id the list does not hold. */
export function drawnKeysOf(id: string): readonly string[] {
  return SHORTCUTS.find((each) => each.id === id)?.drawn ?? [];
}

/** The keys of a shortcut as `aria-keyshortcuts` writes them. */
export function ariaKeysOf(id: string): readonly string[] {
  return SHORTCUTS.find((each) => each.id === id)?.aria ?? [];
}

/** What a global press means: open or close the palette, help, or capture (UX-14's, when it exists). */
export type Intent = 'palette' | 'help' | 'capture';

/** The parts of a key press the decision reads - a `KeyboardEvent` is one. */
export interface Press {
  readonly key: string;
  readonly code?: string;
  readonly metaKey: boolean;
  readonly ctrlKey: boolean;
  readonly altKey: boolean;
  readonly shiftKey: boolean;
  readonly isComposing?: boolean;
  readonly defaultPrevented?: boolean;
  getModifierState?(key: string): boolean;
}

/** Where the press happened, as far as it matters. */
export interface Where {
  /** Focus is in something a person types into. */
  readonly isTextField: boolean;
  /** Focus is inside a menu, a listbox or a grid, whose keys are their own. */
  readonly isInWidget: boolean;
  /** A layer other than the palette is on top - a dialog, a menu, a popover. */
  readonly isLayerOpen: boolean;
  /** The palette is open, in the bar or in its dialog. */
  readonly isPaletteOpen: boolean;
}

/** A Latin letter, which is what `event.key` is for `K` on a Latin layout. */
const isLatin = (key: string) => /^[a-z]$/i.test(key);

/**
 * Whether the press is the letter: by `key` on a Latin layout, and by the physical key where the
 * layout is not Latin (Russian `л`, Arabic `ن` on the key that is `K` elsewhere).
 */
function isLetter(press: Press, letter: string): boolean {
  if (isLatin(press.key)) return press.key.toLowerCase() === letter;
  return press.code === `Key${letter.toUpperCase()}`;
}

/** Which global shortcut a press is, or `null` for every press that is not one here. */
export function intentOf(press: Press, where: Where): Intent | null {
  if (press.defaultPrevented || press.isComposing) return null;
  // AltGr arrives as Ctrl and Alt together on some systems, and it is how a German keyboard types
  // characters, not a modifier the reader meant.
  const isAltGraph = press.getModifierState?.('AltGraph') === true;
  const ctrl = press.ctrlKey && !isAltGraph;
  const alt = press.altKey && !isAltGraph;
  const meta = press.metaKey;

  if (isLetter(press, 'k') && !alt && !press.shiftKey && meta !== ctrl) {
    // `Ctrl K` in a field is the field's key; `⌘K` acts everywhere.
    if (ctrl && where.isTextField) return null;
    if (where.isPaletteOpen) return 'palette';
    if (where.isLayerOpen || where.isInWidget) return null;
    return 'palette';
  }

  if (ctrl || meta || alt) return null;
  if (where.isTextField || where.isInWidget || where.isLayerOpen || where.isPaletteOpen) return null;
  if (press.key === '/') return 'palette';
  if (press.key === '?' || press.key === '؟') return 'help';
  if (isLetter(press, 'q')) return 'capture';
  return null;
}
