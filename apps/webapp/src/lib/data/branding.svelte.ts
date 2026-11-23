// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's own logo, symbol and header choice: the three writes, and the pictures as the
 * setting draws them (UC-INS-22 checks 1 to 4, ADR-0112).
 *
 * **Each write answers the branding as it stands and invalidates the workspace**, whose read
 * carries it: removing the last picture under "the logo alone" returns the header to the product's
 * name in the same write, so the screen reads the answer rather than predicting it.
 *
 * **A picture is drawn from its bytes, not its download address.** The setting needs the pixels to
 * say whether a picture reads on a surface, and an image drawn from another origin cannot be read
 * back; so the bytes are received once and drawn from a local address. A picture just uploaded is
 * held from the file that was sent - the server has judged those very bytes.
 */

import type { WorkspaceBranding, WorkspaceHeader, WorkspacePicture } from '@hubtask/sync-engine';

import { pixelsOf } from '../workspace/pixels.ts';
import { engine } from './engine.ts';
import { AVATAR_LIMIT, transferTimeoutFor } from './media.ts';
import { media } from './media.svelte.ts';
import { workspacePath } from './workspace.svelte.ts';

const PATH = `${workspacePath}/branding`;

/** A picture ready to draw: a local address, and its pixels where they could be read. */
export interface PictureLook {
  readonly url: string;
  readonly pixels?: Uint8ClampedArray;
}

/** A local look of a file: what the setting draws while it is being sent, and after. */
export async function lookOf(blob: Blob): Promise<PictureLook> {
  return { url: URL.createObjectURL(blob), pixels: await pixelsOf(blob) };
}

class BrandingStore {
  /** Looks already made, by media identifier. */
  #looks = $state<Record<string, PictureLook>>({});
  #asking = new Set<string>();
  /** Refused or unreadable: asked once, never again, and drawn as nothing. */
  #unknown = new Set<string>();

  /**
   * The look of a stored picture, asking for its bytes when there is none yet. Safe on every
   * render: a picture being asked for, or one refused, is not asked for again.
   */
  look(mediaId: string | undefined): PictureLook | undefined {
    if (!mediaId) return undefined;
    const held = this.#looks[mediaId];
    if (held) return held;
    if (!this.#asking.has(mediaId) && !this.#unknown.has(mediaId)) void this.#receive(mediaId);
    return undefined;
  }

  /** Holds the look of a picture just set, made from the file that was sent. */
  hold(mediaId: string, look: PictureLook): void {
    this.#looks = { ...this.#looks, [mediaId]: look };
  }

  /** Makes an uploaded, confirmed picture the workspace's logo or symbol. */
  async setPicture(picture: WorkspacePicture, mediaId: string): Promise<WorkspaceBranding> {
    return engine.mutate<WorkspaceBranding>('PUT', `${PATH}/${picture}`, { media_id: mediaId }, { invalidates: [workspacePath] });
  }

  /** Takes a picture away; the last one under "the logo alone" returns the header to the product's name. */
  async removePicture(picture: WorkspacePicture): Promise<WorkspaceBranding> {
    return engine.mutate<WorkspaceBranding>('DELETE', `${PATH}/${picture}`, undefined, { invalidates: [workspacePath] });
  }

  /** Chooses what the header and the sign-in card show. */
  async setHeader(header: WorkspaceHeader): Promise<WorkspaceBranding> {
    return engine.mutate<WorkspaceBranding>('PUT', PATH, { header }, { invalidates: [workspacePath] });
  }

  async #receive(mediaId: string): Promise<void> {
    this.#asking.add(mediaId);
    try {
      const url = await media.downloadUrl(mediaId, Date.now());
      if (!url) {
        this.#unknown.add(mediaId);
        return;
      }
      const size = media.recordOf(mediaId)?.size ?? AVATAR_LIMIT;
      const bytes = await engine.receive({ url, timeoutMs: transferTimeoutFor(size) });
      this.hold(mediaId, await lookOf(bytes));
    } catch {
      // A picture that cannot be fetched is drawn as nothing; the row still offers to replace it.
      this.#unknown.add(mediaId);
    } finally {
      this.#asking.delete(mediaId);
    }
  }
}

export const branding = new BrandingStore();
