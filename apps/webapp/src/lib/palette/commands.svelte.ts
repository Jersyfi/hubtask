// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the screens on view offer the palette, and what the palette asks a collection's screen to
 * do when it arrives there.
 *
 * Two small holders and no logic: which actions count is `actions.ts`'s, and what may be listed is
 * `sources.ts`'s. A screen offers under a source name for as long as its effect lives - the page's
 * head as `page`, an entry open in the detail pane as `pane` - and the cleanup takes the offer away,
 * the shape `page.offer` has.
 *
 * **The intent is one-shot.** *Open Kitchen as a board* and *New task in Kitchen* lead to a screen
 * that may not be mounted yet; the layout lives in that screen's memory and nowhere else. So the
 * palette leaves the wish here and the collection's screen takes it - once, and only for itself -
 * and decides with its own answers whether to act on it.
 */

import { untrack } from 'svelte';

import type { Act } from './actions.ts';

/** What the palette asks a collection's screen to do when it shows that collection. */
export interface Intent {
  readonly containerId: string;
  /** A layout to show it in, one the screen offers. */
  readonly layout?: string;
  /** Open the add form, where the screen says an entry may be created. */
  readonly add?: boolean;
  /**
   * One of the page menu's own items, by id - what the navigation's row menu asks of a hub or
   * collection it is not showing. Run only where the screen offers it and it is usable now.
   */
  readonly act?: string;
}

class Commands {
  #offers = $state<Record<string, readonly Act[]>>({});
  #intent = $state<Intent | undefined>(undefined);

  /** Offers a source's actions until the returned cleanup runs. */
  // Untracked: a screen calls this from an effect, and an effect that read the offers while
  // writing them would run again on its own write.
  offer(source: string, acts: readonly Act[]): () => void {
    untrack(() => {
      this.#offers = { ...this.#offers, [source]: acts };
    });
    return () =>
      untrack(() => {
        if (this.#offers[source] !== acts) return;
        const { [source]: _gone, ...rest } = this.#offers;
        this.#offers = rest;
      });
  }

  /** Everything offered now, in the order the sources offered it. */
  get all(): readonly Act[] {
    return Object.values(this.#offers).flat();
  }

  /** Leaves a wish for a collection's screen. A newer wish replaces an older one. */
  intend(intent: Intent): void {
    this.#intent = intent;
  }

  /** The wish for this collection, taken: a second call answers nothing. */
  take(containerId: string): Intent | undefined {
    return untrack(() => {
      const intent = this.#intent;
      if (!intent || intent.containerId !== containerId) return undefined;
      this.#intent = undefined;
      return intent;
    });
  }

  /** Whether a wish waits for this collection - what its screen's effect follows. */
  isWaitingFor(containerId: string): boolean {
    return this.#intent?.containerId === containerId;
  }
}

export const commands = new Commands();
