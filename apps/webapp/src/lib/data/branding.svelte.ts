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
 *
 * **A stored picture is known to be stored before its bytes arrive.** Its read says so - being
 * read, ready, or unreadable and why - so the row offers *Replace* and *Remove* whatever became of
 * the bytes; one that could not be reached is asked again once the connection is back.
 */

import type { WorkspaceBranding, WorkspaceHeader, WorkspacePicture } from '@hubtask/sync-engine';

import { mayAskAgain, pictureFailureOf, type FailedRead, type FailureShape, type PictureFailure, type PictureStage } from '../workspace/branding.ts';
import { pixelsOf } from '../workspace/pixels.ts';
import { engine } from './engine.ts';
import { live } from './live.svelte.ts';
import { AVATAR_LIMIT, transferTimeoutFor } from './media.ts';
import { media } from './media.svelte.ts';
import { workspacePath } from './workspace.svelte.ts';

const PATH = `${workspacePath}/branding`;

/** A picture ready to draw: a local address, and its pixels where they could be read. */
export interface PictureLook {
  readonly url: string;
  readonly pixels?: Uint8ClampedArray;
}

/** Where a stored picture's look stands: its bytes being read, drawn, or unreadable and why. */
export type PictureRead =
  | { readonly status: 'reading' }
  | { readonly status: 'ready'; readonly look: PictureLook }
  | { readonly status: 'unreadable'; readonly failure: PictureFailure };

/** A local look of a file: what the setting draws while it is being sent, and after. */
export async function lookOf(blob: Blob): Promise<PictureLook> {
  return { url: URL.createObjectURL(blob), pixels: await pixelsOf(blob) };
}

class BrandingStore {
  /** Looks already made, by media identifier. */
  #looks = $state<Record<string, PictureLook>>({});
  #asking = new Set<string>();
  /** Why a picture could not be drawn, by media identifier; only an unreachable one is asked again. */
  #failures = $state<Record<string, FailedRead>>({});

  /**
   * Where a stored picture's look stands, asking for its bytes when there is none yet. Safe on
   * every render: a picture being asked for, or one that failed, is not asked for again - unless it
   * could not be reached and the connection is back (`mayAskAgain`).
   */
  read(mediaId: string | undefined): PictureRead | undefined {
    if (!mediaId) return undefined;
    const held = this.#looks[mediaId];
    if (held) return { status: 'ready', look: held };
    const failed = this.#failures[mediaId];
    if (!this.#asking.has(mediaId) && mayAskAgain(failed, live.state)) void this.#receive(mediaId);
    if (failed && !this.#asking.has(mediaId)) return { status: 'unreadable', failure: failed.failure };
    return { status: 'reading' };
  }

  /** Holds the look of a picture just set, made from the file that was sent. */
  hold(mediaId: string, look: PictureLook): void {
    const { [mediaId]: _past, ...failures } = this.#failures;
    this.#failures = failures;
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
    let stage: PictureStage = 'record';
    try {
      const { url, failure } = await media.downloadOf(mediaId, Date.now());
      if (!url) {
        this.#fail(mediaId, pictureFailureOf('record', failure, live.state === 'offline'));
        return;
      }
      stage = 'bytes';
      const size = media.recordOf(mediaId)?.size ?? AVATAR_LIMIT;
      const bytes = await engine.receive({ url, timeoutMs: transferTimeoutFor(size) });
      this.hold(mediaId, await lookOf(bytes));
    } catch (error) {
      // The row still offers to replace or remove it, and says why it cannot be shown.
      this.#fail(mediaId, pictureFailureOf(stage, error as FailureShape, live.state === 'offline'));
    } finally {
      this.#asking.delete(mediaId);
    }
  }

  #fail(mediaId: string, failure: PictureFailure): void {
    this.#failures = { ...this.#failures, [mediaId]: { failure, connection: live.state } };
  }
}

export const branding = new BrandingStore();
