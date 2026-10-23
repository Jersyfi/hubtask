// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The three ways to an entry's menu besides its ⋯ (UC-WRK-28 check 1, design-system.md §11.9,
 * ADR-0088): a right click, the keyboard's context-menu key or Shift+F10, and a long press.
 *
 * One listener set on the surface rather than one per row, for the reason the list's key handler
 * gives: a static element carrying an interaction is a control nobody announces. Each event finds
 * its entry through `idAt`.
 *
 * **The long press opens on release.** A coarse pointer held still for the hold opens the menu
 * when it lifts; one that moves first is scrolling, and one that moves after the hold is a drag
 * the drag helper takes (it starts on its own handle, which `ignore` names). The click the lift
 * sends is swallowed, so the entry under the finger does not also open. A browser that answers a
 * long press with its own `contextmenu` is told no and left to the release, so the gesture means
 * one thing on every engine.
 */

import { HOLD_MS, hasLeftTheHandle } from '@hubtask/design-system/components';

export interface EntryGestureOptions {
  /** The entry an event happened on, or nothing where it was not on one. */
  readonly idAt: (target: EventTarget | null) => string | undefined;
  /** Opens that entry's menu with the first item focused. */
  readonly open: (id: string) => void;
  /** Where a press belongs to something else - a drag's grip, which takes its own hold. */
  readonly ignore?: string;
}

const isContextKey = (event: KeyboardEvent) => event.key === 'ContextMenu' || (event.key === 'F10' && event.shiftKey);

/** Swallows the one click a lifted hold sends; a lift that sends none must not eat a later one. */
function swallowNextClick() {
  const swallow = (click: MouseEvent) => {
    click.preventDefault();
    click.stopPropagation();
  };
  window.addEventListener('click', swallow, { capture: true, once: true });
  setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), HOLD_MS);
}

/** Installs the gestures on `node`; the returned function removes them. Call it from an `$effect`. */
export function attachEntryGestures(node: HTMLElement, options: EntryGestureOptions): () => void {
  let held: ReturnType<typeof setTimeout> | undefined;
  let holding: { id: string; from: { x: number; y: number }; isStill: boolean } | undefined;

  const forget = () => {
    if (held !== undefined) clearTimeout(held);
    held = undefined;
    holding = undefined;
  };

  const onContextMenu = (event: MouseEvent) => {
    const id = options.idAt(event.target);
    if (id === undefined) return;
    // A finger's long press: the release decides, and the browser's own menu is not wanted.
    if ((event as PointerEvent).pointerType === 'touch' || holding !== undefined) {
      event.preventDefault();
      return;
    }
    // Shift with the right button keeps the browser's menu - for a pointer only.
    if (event.shiftKey) return;
    event.preventDefault();
    options.open(id);
  };

  const onKeydown = (event: KeyboardEvent) => {
    if (!isContextKey(event)) return;
    const id = options.idAt(event.target);
    if (id === undefined) return;
    event.preventDefault();
    options.open(id);
  };

  const onPointerDown = (event: PointerEvent) => {
    forget();
    if (event.pointerType === 'mouse' || event.button !== 0) return;
    const target = event.target as Element | null;
    if (options.ignore && target?.closest?.(options.ignore)) return;
    if (target?.closest?.('button, input, select, textarea, [role="menuitem"]') && !target.closest('a')) return;
    const id = options.idAt(event.target);
    if (id === undefined) return;
    holding = { id, from: { x: event.clientX, y: event.clientY }, isStill: false };
    held = setTimeout(() => {
      held = undefined;
      if (holding) holding.isStill = true;
    }, HOLD_MS);
  };

  const onPointerMove = (event: PointerEvent) => {
    if (!holding) return;
    if (hasLeftTheHandle(holding.from, { x: event.clientX, y: event.clientY })) forget();
  };

  const onPointerUp = () => {
    const ended = holding;
    forget();
    if (!ended?.isStill) return;
    swallowNextClick();
    options.open(ended.id);
  };

  node.addEventListener('contextmenu', onContextMenu);
  node.addEventListener('keydown', onKeydown);
  node.addEventListener('pointerdown', onPointerDown);
  window.addEventListener('pointermove', onPointerMove);
  window.addEventListener('pointerup', onPointerUp);
  window.addEventListener('pointercancel', forget);
  return () => {
    forget();
    node.removeEventListener('contextmenu', onContextMenu);
    node.removeEventListener('keydown', onKeydown);
    node.removeEventListener('pointerdown', onPointerDown);
    window.removeEventListener('pointermove', onPointerMove);
    window.removeEventListener('pointerup', onPointerUp);
    window.removeEventListener('pointercancel', forget);
  };
}
