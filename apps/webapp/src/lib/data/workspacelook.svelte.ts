// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where a member's mark of the workspace comes from: the header, the switcher, the phone's sheet
 * (UC-INS-22 checks 5 and 6; ADR-0112). The sign-in card's is `cardLookOf` and `cardSourcesOf`,
 * from the rules at the address, so the first screen does not carry this store.
 *
 * **A picture is drawn as an avatar is**: its media record is read once
 * (`GET /media/{id}`, which every member may read for a picture the workspace shows) and the image
 * is drawn from the download address it carries. Not from its bytes, as the setting draws them to
 * judge their pixels: that needs a read of the object store from this page, which a bucket on
 * another origin without a CORS rule refuses, and the header has no pixels to judge. A picture
 * whose record cannot be read - refused, gone, offline with nothing cached - is left out, so the
 * mark falls back by G8.
 *
 * **The look is its own read**, `GET /tenant/look`, which every member may make: a guest of one hub
 * may not read the workspace's set-up (`/tenant`) and still sees its name and look.
 */

import type { ResourceState, WorkspaceLook as Look } from '@hubtask/sync-engine';

import { memberLookOf, PICTURES, type PictureSources, type ShownLook } from '../workspace/branding.ts';
import { engine } from './engine.ts';
import { media } from './media.svelte.ts';

class WorkspaceLook {
  #state = $state<ResourceState<Look>>({ status: 'idle' });
  /** Download addresses, by media identifier. */
  #urls = $state<Record<string, string>>({});
  /** Asked once and refused or unreadable: never asked again in this session. */
  #gone = $state<Record<string, true>>({});
  #asking = new Set<string>();

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<Look>({ path: '/tenant/look' }, (next) => {
      this.#state = next;
    });
  }

  /** Whether the look's answer is in: until then nothing is drawn in the mark's place. */
  get isRead(): boolean {
    return this.#state.status === 'ready' || this.#state.status === 'failed';
  }

  /** The workspace's name, as its members read it. */
  get name(): string | undefined {
    return this.#look?.display_name;
  }

  /** What a member sees: the server answers the product's where the workspace may not show its own. */
  get shown(): ShownLook {
    return memberLookOf(this.#look);
  }

  get #look(): Look | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /**
   * The addresses of the pictures a member's mark draws: `null` while a record is being read, none
   * for a picture that cannot be drawn. Safe on every render, as `media.coverUrl` is.
   */
  sourcesOf(ids: ShownLook['ids']): PictureSources {
    const sources: PictureSources = {};
    for (const picture of PICTURES) {
      const id = ids[picture];
      if (!id || this.#gone[id]) continue;
      const url = this.#urls[id];
      if (url === undefined && !this.#asking.has(id)) void this.#ask(id);
      sources[picture] = url ?? null;
    }
    return sources;
  }

  async #ask(id: string): Promise<void> {
    this.#asking.add(id);
    try {
      const url = await media.downloadUrl(id, Date.now());
      if (url) this.#urls = { ...this.#urls, [id]: url };
      else this.#gone = { ...this.#gone, [id]: true };
    } catch {
      // A picture that cannot be read is a mark without it; a sentence about media has no place in a header.
      this.#gone = { ...this.#gone, [id]: true };
    } finally {
      this.#asking.delete(id);
    }
  }
}

export const workspaceLook = new WorkspaceLook();
