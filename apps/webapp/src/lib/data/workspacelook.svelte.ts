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
 * mark falls back by G8; one that could not be reached is asked again once the connection is back.
 *
 * **The look is its own read**, `GET /tenant/look`, which every member may make: a guest of one hub
 * may not read the workspace's set-up (`/tenant`) and still sees its name and look. Whoever may
 * read `/tenant` takes the look from that answer instead, so the frame asks one of the two.
 */

import type { ResourceState, WorkspaceLook as Look } from '@hubtask/sync-engine';

import {
  lookOfWorkspace, mayAskAgain, memberLookOf, pictureFailureOf, PICTURES, type FailedRead, type PictureSources, type ShownLook,
} from '../workspace/branding.ts';
import { engine } from './engine.ts';
import { live } from './live.svelte.ts';
import { media } from './media.svelte.ts';
import { workspace } from './workspace.svelte.ts';

class WorkspaceLook {
  #state = $state<ResourceState<Look>>({ status: 'idle' });
  /** Download addresses, by media identifier. */
  #urls = $state<Record<string, string>>({});
  /** Why a record could not be read; only one that could not be reached is asked again. */
  #failed = $state<Record<string, FailedRead>>({});
  #asking = new Set<string>();

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    this.#fromWorkspace = false;
    return engine.subscribe<Look>({ path: '/tenant/look' }, (next) => {
      this.#state = next;
    });
  }

  /**
   * Starts the read for whoever may read `/tenant`: the look is taken from that one answer, so
   * the frame asks one of the two (`lookOfWorkspace`). **From `untrack`**, as `open`.
   */
  openFromWorkspace(): () => void {
    this.#fromWorkspace = true;
    return workspace.open();
  }

  #fromWorkspace = $state(false);

  get #read(): ResourceState<unknown> {
    return this.#fromWorkspace ? workspace.state : this.#state;
  }

  /** Whether the look's answer is in: until then nothing is drawn in the mark's place. */
  get isRead(): boolean {
    return this.#read.status === 'ready' || this.#read.status === 'failed';
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
    if (this.#fromWorkspace) return lookOfWorkspace(workspace.workspace);
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
      if (!id) continue;
      const url = this.#urls[id];
      const failed = this.#failed[id];
      if (url === undefined && !this.#asking.has(id) && mayAskAgain(failed, live.state)) void this.#ask(id);
      // Asked again or not, a failed picture keeps the fallback until its address is in.
      if (failed && url === undefined) continue;
      sources[picture] = url ?? null;
    }
    return sources;
  }

  async #ask(id: string): Promise<void> {
    this.#asking.add(id);
    try {
      const { url, failure } = await media.downloadOf(id, Date.now());
      if (url) {
        const { [id]: _past, ...failed } = this.#failed;
        this.#failed = failed;
        this.#urls = { ...this.#urls, [id]: url };
      } else {
        this.#fail(id, pictureFailureOf('record', failure, live.state === 'offline'));
      }
    } catch {
      // A picture that cannot be read is a mark without it; a sentence about media has no place in a header.
      this.#fail(id, 'unknown');
    } finally {
      this.#asking.delete(id);
    }
  }

  #fail(id: string, failure: FailedRead['failure']): void {
    this.#failed = { ...this.#failed, [id]: { failure, connection: live.state } };
  }
}

export const workspaceLook = new WorkspaceLook();
