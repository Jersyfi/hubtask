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
 */

import { PICTURES, shownLookOf, type PictureSources, type ShownLook } from '../workspace/branding.ts';
import { media } from './media.svelte.ts';
import { workspace } from './workspace.svelte.ts';

class WorkspaceLook {
  /** Download addresses, by media identifier. */
  #urls = $state<Record<string, string>>({});
  /** Asked once and refused or unreadable: never asked again in this session. */
  #gone = $state<Record<string, true>>({});
  #asking = new Set<string>();

  /** Whether the workspace's answer is in: until then nothing is drawn in the mark's place. */
  get isRead(): boolean {
    return workspace.state.status === 'ready' || workspace.state.status === 'failed';
  }

  /** The workspace's name, as its members read it. */
  get name(): string | undefined {
    return workspace.workspace?.display_name;
  }

  /** What a member sees: the stored choice where the workspace may show it, else the product. */
  get shown(): ShownLook {
    return shownLookOf(workspace.workspace?.branding);
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
