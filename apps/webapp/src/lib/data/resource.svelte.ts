// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The whole Svelte binding: runes wrapping the engine's subscription API (ADR-0033 §2).
 *
 * It is this short on purpose. The engine already holds the state, shares it between subscribers
 * and publishes every change; what a framework adds is the way a component *observes* that, and
 * nothing more. A binding that also cached, or retried, or transformed would be a second engine —
 * and the second one is the one that disagrees.
 *
 * Nothing else in the application subscribes: a component reads `resource(...).state` and never
 * learns that a Transport exists. With a queue and a local store behind `SyncEngine`, this
 * file does not change and neither does any component.
 */
import { untrack } from 'svelte';

import type { ResourceRequest, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

export interface Resource<T> {
  /** Every state the resource can be in, as one value a template can switch on. */
  readonly state: ResourceState<T>;
  /** Reads it again. What a retry button calls. */
  refresh(): Promise<void>;
  /**
   * Asks for the next page and adds it to what is already held. What `LoadMore` calls.
   *
   * Does nothing on the last page, so a component may bind it to a button without asking first —
   * and it throws only when the page it asked for failed, which leaves the pages that arrived
   * where the reader can still see them.
   */
  loadMore(): Promise<void>;
}

/**
 * resource subscribes for as long as the component that called it is alive.
 *
 * `$effect` is what ties the two together: it runs when the component mounts and its cleanup runs
 * when the component is destroyed, so there is no unsubscribe for a caller to forget. That is the
 * reason this is a rune-aware module (`.svelte.ts`) rather than a plain function.
 */
export function resource<T>(request: string | ResourceRequest): Resource<T> {
  // A path is the common case and a document is the other one: `/items:query` and `/search` read
  // by `POST`, and a component asking one of them is asking for a resource like any other. The
  // string form stays because most call sites have nothing to say beyond where to look.
  const asked: ResourceRequest = typeof request === 'string' ? { path: request } : request;
  let state = $state<ResourceState<T>>(engine.peek<T>(asked));

  $effect(() => engine.subscribe<T>(asked, (next) => (state = next)));

  return {
    get state() {
      return state;
    },
    async refresh() {
      await engine.refresh<T>(asked);
    },
    async loadMore() {
      await engine.loadMore<T>(asked);
    },
  };
}

/**
 * resourceWhen is `resource` for what only some readers may read: `request` answers nothing while
 * the reader may not (or it is not yet known), and nothing is asked then - a request the server
 * would refuse is a failure in the reader's console, not a quiet absence. The state is `idle`
 * until a request is answered, and follows a change of request.
 */
export function resourceWhen<T>(request: () => string | ResourceRequest | undefined): Resource<T> {
  const asked = $derived.by(() => {
    const wanted = request();
    return wanted === undefined ? undefined : typeof wanted === 'string' ? { path: wanted } : wanted;
  });
  const key = $derived(asked === undefined ? '' : JSON.stringify(asked));
  let state = $state<ResourceState<T>>({ status: 'idle' });

  $effect(() => {
    void key;
    const wanted = untrack(() => asked);
    if (wanted === undefined) {
      state = { status: 'idle' };
      return;
    }
    return engine.subscribe<T>(wanted, (next) => (state = next));
  });

  return {
    get state() {
      return state;
    },
    async refresh() {
      if (asked !== undefined) await engine.refresh<T>(asked);
    },
    async loadMore() {
      if (asked !== undefined) await engine.loadMore<T>(asked);
    },
  };
}
