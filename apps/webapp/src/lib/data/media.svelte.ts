// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The bytes: staging an upload, sending them, sealing the object, and what an entry then carries.
 *
 * **Three steps, and the middle one does not go to the API.** `POST /media` stages the object and
 * answers where to put the bytes; the bytes go there through the seam's `transfer`, with no bearer
 * and no cookie, because the URL is its own credential; `POST /media/{id}:confirm` reads them back,
 * judges them and seals the object. Nothing may use an object before that judgement (C-06, T-11),
 * so this store never hands a PENDING object to a caller as if it were usable.
 *
 * **A cancelled upload leaves a PENDING object behind, and that is by design.** The reconciliation
 * job takes an unconfirmed staging after its grace period. The client says so rather than deleting
 * anything: `DELETE /media/{id}` is for an object that exists and is unreferenced, and calling it
 * on a staging whose bytes may or may not have landed would be this client guessing at a state the
 * server has not looked at yet.
 *
 * **The record cache mints no target per row.** `GET /items/{id}/attachments` lists media records
 * and no download URLs; a URL comes from `GET /media/{id}` and expires. A download therefore asks
 * when it is clicked, and a cover asks once and asks again when what it holds has expired.
 */

import type { CoverInput, ItemAttachments, MediaObject, MediaPage, WorkItem } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';
import { downloadUrlOf, transferTimeoutFor } from './media.ts';

/** The entry's attachments — a page of media records, and the only path a write here touches. */
export const attachmentsPath = (itemId: string) => `/items/${itemId}/attachments`;

/** One media record. Where a usable transfer target comes from, and the only place one does. */
export const mediaPath = (mediaId: string) => `/media/${mediaId}`;

/** What an upload needs to know from its caller, beyond the file itself. */
export interface UploadOptions {
  /** The intent's key. A retry of the same upload is the same key, so the caller mints it. */
  readonly idempotencyKey: string;
  /** Bytes as they leave, and once more at the end. */
  readonly onProgress?: (sent: number, total: number) => void;
  /** How the person abandons it. */
  readonly signal?: AbortSignal;
  /** Told the staged object as soon as there is one, so a cancel can say what it left behind. */
  readonly onStaged?: (object: MediaObject) => void;
}

class Media {
  /** Records already read, by identifier. A record is not something a screen watches change. */
  #records = $state<Record<string, MediaObject>>({});
  /** Being read, so two rows asking for one object make one request. */
  #asking = new Set<string>();
  /** Refused or gone: asked once, never again. */
  #unknown = new Set<string>();

  /** The record, when it has been read. `undefined` means "not yet" or "not for this reader". */
  recordOf(mediaId: string | null | undefined): MediaObject | undefined {
    return mediaId ? this.#records[mediaId] : undefined;
  }

  /** The name the file arrived under, when the record has been read. */
  fileNameOf(mediaId: string | null | undefined): string | undefined {
    return mediaId ? (this.#records[mediaId]?.file_name ?? undefined) : undefined;
  }

  /**
   * Asks for the records a screen names and has not got.
   *
   * Safe on every render, like the account cache it mirrors: an identifier already held, already
   * being asked for, or already refused is skipped. A history of twenty steps about three files is
   * three reads, once.
   */
  resolve(ids: readonly (string | null | undefined)[]): void {
    for (const id of ids) {
      if (!id || id in this.#records || this.#asking.has(id) || this.#unknown.has(id)) continue;
      void this.#read(id);
    }
  }

  /**
   * Where to draw a cover from, asking for the record if there is none or if its target expired.
   *
   * Safe on every render: an object being asked for is not asked for again, and one that was
   * refused is never asked for again at all — a cover the reader may not see is a card without
   * one, not a request per frame.
   */
  coverUrl(mediaId: string | null | undefined, now: number): string | undefined {
    if (!mediaId) return undefined;
    const url = downloadUrlOf(this.#records[mediaId], now);
    if (url) return url;
    if (!this.#unknown.has(mediaId) && !this.#asking.has(mediaId)) void this.#read(mediaId);
    return undefined;
  }

  /**
   * A download target, minted now.
   *
   * Always a fresh read: the list carries none, and one that has been on screen since breakfast
   * has expired. What comes back is a URL or nothing — nothing being an object that is not READY
   * or a reader who may not have it, and in both cases the caller says so rather than following a
   * link that answers a signature error.
   */
  async downloadUrl(mediaId: string, now: number): Promise<string | undefined> {
    const state = await engine.refresh<MediaObject>({ path: mediaPath(mediaId) });
    if (state.status !== 'ready') return undefined;
    this.#records = { ...this.#records, [mediaId]: state.data };
    return downloadUrlOf(state.data, now);
  }

  /**
   * The three steps, in order, and the object they produce.
   *
   * The deadline on the bytes is sized by the bytes; the API's own is not, and a file on a bad
   * connection would be cut off by a timeout meant for a JSON document. The claimed content type
   * is passed on as a claim — the server judges from the bytes at confirmation and answers
   * `media.type_mismatch` when the two disagree, which is a sentence the reader gets to read.
   */
  async upload(file: File, usage: 'COVER' | 'ATTACHMENT', options: UploadOptions): Promise<MediaObject> {
    const staged = await engine.mutate<MediaObject>(
      'POST',
      '/media',
      {
        file_name: file.name || null,
        content_type: file.type || null,
        size: file.size,
        usage,
      },
      { idempotencyKey: options.idempotencyKey },
    );
    options.onStaged?.(staged);

    const target = staged.upload;
    if (!target?.url) {
      // The contract says the staging answer carries the target. Without one there is nowhere to
      // put the bytes, and going on would send them to `undefined`.
      throw new Error('media: the staged object carries no upload target');
    }

    await engine.transfer({
      url: target.url,
      method: 'PUT',
      body: file,
      contentType: file.type || undefined,
      timeoutMs: transferTimeoutFor(file.size),
      signal: options.signal,
      onProgress: options.onProgress,
    });

    // The same key: the idempotency record is per endpoint, and confirming is idempotent by state
    // anyway — a second confirmation of a READY object answers with what is there.
    const sealed = await engine.mutate<MediaObject>(
      'POST',
      `${mediaPath(staged.id)}:confirm`,
      undefined,
      { idempotencyKey: options.idempotencyKey },
    );
    this.#records = { ...this.#records, [sealed.id]: sealed };
    return sealed;
  }

  /** Puts a cover on the entry. The entry's own row moves, so this carries the version. */
  async setCover(itemId: string, cover: CoverInput, version: number): Promise<WorkItem> {
    return engine.mutate<WorkItem>('PUT', `/items/${itemId}/cover`, cover, {
      ifMatch: etagFor(version),
      invalidates: ['/items'],
    });
  }

  /** Takes it off. Idempotent on the server: an entry with no cover succeeds and announces nothing. */
  async clearCover(itemId: string, version: number): Promise<WorkItem> {
    return engine.mutate<WorkItem>('DELETE', `/items/${itemId}/cover`, undefined, {
      ifMatch: etagFor(version),
      invalidates: ['/items'],
    });
  }

  /**
   * Attaches an object to the entry.
   *
   * Only the attachment list is invalidated, for the reason a comment only touches its thread:
   * attaching raises the object's reference count and does not move the entry's own row, so naming
   * `/items` would reload every list on screen for a change none of them renders.
   */
  async attach(itemId: string, mediaId: string): Promise<ItemAttachments> {
    return engine.mutate<ItemAttachments>(
      'PUT',
      `${attachmentsPath(itemId)}/${mediaId}`,
      undefined,
      { invalidates: [attachmentsPath(itemId)] },
    );
  }

  /** Drops the reference. The object stays until nothing points at it and the job comes round. */
  async detach(itemId: string, mediaId: string): Promise<ItemAttachments> {
    return engine.mutate<ItemAttachments>(
      'DELETE',
      `${attachmentsPath(itemId)}/${mediaId}`,
      undefined,
      { invalidates: [attachmentsPath(itemId)] },
    );
  }

  async #read(mediaId: string): Promise<void> {
    this.#asking.add(mediaId);
    try {
      const state = await engine.refresh<MediaObject>({ path: mediaPath(mediaId) });
      if (state.status === 'ready') this.#records = { ...this.#records, [mediaId]: state.data };
      else this.#unknown.add(mediaId);
    } catch {
      // A refusal is an answer: this reader may not have the object, and a card without a cover is
      // what that looks like. Rethrowing would put a sentence about media on a screen about work.
      this.#unknown.add(mediaId);
    } finally {
      this.#asking.delete(mediaId);
    }
  }
}

export const media = new Media();
export type { MediaObject, MediaPage };
