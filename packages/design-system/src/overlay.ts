// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What it means to open a layer, in one place.
//
// `Menu` and `Popover` do the same four things when they open - take a place in the register, tie
// themselves to their trigger, listen for `Escape`, and close when the pointer goes somewhere else
// - and a component that did three of them would be the one that leaves an entry in the register
// after it is gone. Writing it twice is how the two come to disagree about what dismisses them,
// which is a difference a reader feels and nobody can name.
//
// It returns the function that undoes all of it. Focus is deliberately *not* part of that: where
// focus goes when an overlay closes depends on why it closed - back to the trigger after `Escape`,
// onwards after a menu item that opened a dialog - and only the component knows.

import { anchorTo, type Placement } from './positioning.ts';
import { escapeHandler } from './focus.ts';
import { layers, type DismissibleLayer } from './layers.ts';

export interface OverlayOptions {
  /** Which rank this takes in the register - what `Escape` reaches first (design-system.md §6). */
  readonly layer: DismissibleLayer;
  readonly trigger: HTMLElement;
  readonly surface: HTMLElement;
  readonly placement: Placement;
  /** Close this overlay. Called by `Escape`, and by a pointer that went elsewhere. */
  readonly onDismiss: () => void;
}

export function openOverlay({ layer, trigger, surface, placement, onDismiss }: OverlayOptions): () => void {
  const entry = layers.open(layer, onDismiss);
  const unanchor = anchorTo(trigger, surface, { placement });
  const onKeydown = escapeHandler();

  // Capture, and `pointerdown` rather than `click`: a menu that closed on click would still be
  // open while the pointer was pressed on something behind it, and a click on a control that
  // re-renders never reaches the document at all.
  const onPointerDown = (event: Event) => {
    const target = event.target as Node | null;
    if (!target || surface.contains(target) || trigger.contains(target)) return;
    onDismiss();
  };

  document.addEventListener('keydown', onKeydown);
  document.addEventListener('pointerdown', onPointerDown, true);

  return () => {
    document.removeEventListener('keydown', onKeydown);
    document.removeEventListener('pointerdown', onPointerDown, true);
    unanchor();
    // Idempotent on purpose: `Escape` took the entry out before calling `onDismiss`, and this runs
    // afterwards either way.
    entry.release();
  };
}
