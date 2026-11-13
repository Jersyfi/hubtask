// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A key acts on the selection the browser holds. The browser reports a moved selection a moment
 * after the key that moved it (`selectionchange`), and ProseMirror flushes only a change it has
 * already been told of: the line's end and Enter pressed quickly acted on the old selection, and
 * Enter replaced the words still selected. Reading the browser's selection first closes that gap.
 */

import { Plugin, PluginKey } from '@tiptap/pm/state';

const MODIFIERS = new Set(['Shift', 'Meta', 'Control', 'Alt', 'AltGraph', 'CapsLock']);

/** The part of ProseMirror's view this reads; its DOM observer is not in the published types. */
interface Observed {
  readonly domObserver?: { flush(): void };
}

export const readSelectionFirst = new Plugin({
  key: new PluginKey('readSelectionFirst'),
  props: {
    handleKeyDown(view, event) {
      if (event.isComposing || MODIFIERS.has(event.key)) return false;
      (view as unknown as Observed).domObserver?.flush();
      return false;
    },
  },
});
