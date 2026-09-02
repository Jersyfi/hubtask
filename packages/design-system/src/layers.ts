// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// One register that knows which layer is on top.
//
// design-system.md §9 asked for a layering scale because wave 1b lands `Tooltip`, `Menu`,
// `Popover`, `Dialog` and `Toast` together with the rule that `Escape` closes one layer at a time.
// The scale in tokens.json answers *what paints over what*. This answers the other half - *what
// `Escape` reaches* - and the two are not the same question. A tooltip paints above a dialog and
// is not what `Escape` closes; a popover opened from inside a dialog is.
//
// It is plain TypeScript with no runes and no DOM on purpose. The whole point of writing it before
// the five components exist is that the ordering can be tested without any of them - and a module
// that reached for `document` could only be tested in a browser.

/**
 * The layers `Escape` can reach, weakest first. A tooltip is deliberately absent: it is dismissed
 * by the pointer or the blur that summoned it, never by a key, so putting it here would let it
 * swallow the `Escape` meant for the dialog underneath.
 */
export const DISMISSIBLE_LAYERS = ['overlay', 'dialog', 'popover'] as const;

export type DismissibleLayer = (typeof DISMISSIBLE_LAYERS)[number];

/** Where a layer sits in the paint order, mirroring `primitive.layer` in tokens.json. */
const RANK: Record<DismissibleLayer, number> = { overlay: 0, dialog: 1, popover: 2 };

export interface LayerEntry {
  readonly id: number;
  readonly layer: DismissibleLayer;
  /** What to run when this entry is the one dismissed. */
  readonly dismiss: () => void;
}

export interface LayerHandle {
  readonly id: number;
  /** Removes this entry. Idempotent: closing twice is not an error, it is a race. */
  readonly release: () => void;
}

/**
 * The order two open layers are dismissed in. Rank decides first - a popover opened from inside a
 * dialog closes before the dialog, whatever order they were opened in - and among equals the one
 * opened last goes first. Without the rank a dialog opened *after* a drawer would close before it,
 * which is not what a person pressing `Escape` means.
 */
function isAbove(candidate: LayerEntry, incumbent: LayerEntry): boolean {
  const byRank = RANK[candidate.layer] - RANK[incumbent.layer];
  return byRank === 0 ? candidate.id > incumbent.id : byRank > 0;
}

export class LayerRegister {
  #entries: LayerEntry[] = [];
  #next = 1;

  /** open registers a layer and returns the handle that closes it. */
  open(layer: DismissibleLayer, dismiss: () => void): LayerHandle {
    const id = this.#next++;
    this.#entries.push({ id, layer, dismiss });
    return {
      id,
      release: () => {
        this.#entries = this.#entries.filter((entry) => entry.id !== id);
      },
    };
  }

  /** top is the entry `Escape` reaches, or null when nothing is open. */
  top(): LayerEntry | null {
    let top: LayerEntry | null = null;
    for (const entry of this.#entries) if (!top || isAbove(entry, top)) top = entry;
    return top;
  }

  /** open layers, weakest first. For tests and for the frame that wants to know. */
  entries(): readonly LayerEntry[] {
    return [...this.#entries].sort((a, b) => (isAbove(a, b) ? 1 : -1));
  }

  get size(): number {
    return this.#entries.length;
  }

  /**
   * dismissTop closes exactly one layer and answers whether it closed anything. One, not all: the
   * rule §9 names is that `Escape` with two overlays open leaves one of them open. The entry is
   * removed before its callback runs, so a `dismiss` that synchronously opens something else
   * cannot be undone by its own release.
   */
  dismissTop(): boolean {
    const top = this.top();
    if (!top) return false;
    this.#entries = this.#entries.filter((entry) => entry.id !== top.id);
    top.dismiss();
    return true;
  }
}

/**
 * The register the application shares. A second one would be a second answer to "what is on top",
 * which is the thing this file exists to prevent - but the class is exported so a test, or a
 * second document such as an iframe or the workbench's stage, can hold its own.
 */
export const layers = new LayerRegister();

/**
 * handleEscape is the whole keyboard contract, kept away from the DOM so it can be tested.
 * The frame calls it from one `keydown` listener; the caller decides what to do with the answer,
 * which is `true` when a layer was closed and the event should go no further.
 */
export function handleEscape(key: string, register: LayerRegister = layers): boolean {
  return key === 'Escape' && register.dismissTop();
}
