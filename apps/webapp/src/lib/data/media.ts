// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What this client decides about bytes, pure so that each decision is testable without a network.
 *
 * **The size limit is the installation's, read from the manifest.** `max_upload_bytes` is in
 * `limits`, and the check happens before a byte leaves — the alternative is a person watching a
 * hundred megabytes upload and being told at confirmation that the installation never accepts
 * that much. Zero means unlimited, the same convention every other quota in `limits` uses.
 *
 * **A deadline sized by the bytes.** `ByteTransfer` refuses to default to "forever" and the API's
 * own deadline is far too short for a file, so the timeout is computed from the size against a
 * floor rate. It is deliberately generous: the deadline exists to stop a transfer that has died,
 * not to enforce a bandwidth.
 *
 * **A download target is minted when it is clicked.** `GET /items/{id}/attachments` lists media
 * records and mints nothing; asking `GET /media/{id}` is what produces a URL, and that URL expires.
 * So a list of twenty attachments is one read and no capabilities, and a target that has been
 * sitting on a screen since breakfast is checked against the clock rather than followed.
 *
 * **A cover is drawn only from a READY object's download URL.** That is the one path where the
 * sniffed inline allowlist (C-05) has already judged the bytes. A PENDING object has a claimed type
 * and nothing more, and drawing from it would be this client trusting a claim the server refuses to.
 */

import type { Capabilities, Cover, CoverInput, MediaObject, MediaTransfer } from '@hubtask/sync-engine';

/** Where the installation states what it accepts. One key, named once. */
const LIMIT_KEY = 'max_upload_bytes';

/**
 * The largest upload this installation takes, or `undefined` where it states no ceiling.
 *
 * `undefined` also stands for "not read yet", and the two are deliberately one answer: a client
 * that has not read the manifest cannot honestly refuse a file, and the server refuses it a moment
 * later with a sentence of its own (`media.too_large`).
 */
export function uploadLimitOf(capabilities: Capabilities | undefined): number | undefined {
  const limits = capabilities?.limits as Record<string, unknown> | undefined;
  const declared = limits?.[LIMIT_KEY];
  if (typeof declared !== 'number' || !Number.isFinite(declared) || declared <= 0) return undefined;
  return declared;
}

/** Whether a file may be offered at all. No limit known means yes, and the server decides. */
export function isWithinUploadLimit(size: number, limit: number | undefined): boolean {
  if (limit === undefined) return true;
  return size <= limit;
}

/** How long the bytes may take, in milliseconds. */
export function transferTimeoutFor(size: number): number {
  // A floor of 32 kB/s — worse than a bad mobile connection, which is the point: anything faster
  // finishes well inside the deadline, and anything slower than this has stopped rather than
  // slowed. The base covers the round trip on a file small enough for the rate to be noise.
  const FLOOR_BYTES_PER_SECOND = 32 * 1000;
  const BASE_MS = 30_000;
  const CEILING_MS = 2 * 60 * 60 * 1000;
  const bytes = Number.isFinite(size) && size > 0 ? size : 0;
  return Math.min(CEILING_MS, BASE_MS + Math.ceil((bytes / FLOOR_BYTES_PER_SECOND) * 1000));
}

/** Only a sealed object may cover or be attached — the contract's own rule, mirrored. */
export function isReady(object: MediaObject | undefined): boolean {
  return object?.status === 'READY';
}

/**
 * Whether a transfer target can still be used.
 *
 * A URL that expired is not followed: a bucket answers a signature error and this server answers
 * `media.token_invalid`, and neither is a sentence about what actually happened, which is that the
 * page has been open a while. Asking for the record again mints a new one.
 */
export function isUsable(transfer: MediaTransfer | null | undefined, now: number): boolean {
  if (!transfer?.url || !transfer.expires_at) return false;
  const expires = Date.parse(transfer.expires_at);
  return Number.isNaN(expires) ? false : expires > now;
}

/** Where to fetch the bytes of a sealed object, or nothing when there is no usable target. */
export function downloadUrlOf(object: MediaObject | undefined, now: number): string | undefined {
  if (!isReady(object)) return undefined;
  return isUsable(object?.download, now) ? object?.download?.url : undefined;
}

/** The image a cover names, or nothing when it is a colour or is not set. */
export function coverImageIdOf(cover: Cover | null | undefined): string | undefined {
  if (cover?.kind !== 'IMAGE') return undefined;
  return cover.media_id ?? undefined;
}

/** The document that sets an image cover. Exactly one of the two fields, matching the kind. */
export function imageCover(mediaId: string): CoverInput {
  return { kind: 'IMAGE', media_id: mediaId, color_token: null };
}

/**
 * What the file dialog offers first for a usage.
 *
 * A hint to the platform and never a check: the judgement is the server's, made from the bytes at
 * confirmation (T-11), and a client that filtered here would only be refusing files the server
 * would have taken. A cover is an image because nothing else can be drawn as one; an attachment is
 * anything.
 */
export function acceptFor(usage: 'COVER' | 'ATTACHMENT'): string | undefined {
  return usage === 'COVER' ? 'image/*' : undefined;
}
