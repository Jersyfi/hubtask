// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities, MediaObject } from '@hubtask/sync-engine';

import {
  acceptFor,
  colorCover,
  coverImageIdOf,
  downloadUrlOf,
  imageCover,
  isReady,
  isUsable,
  isWithinUploadLimit,
  transferTimeoutFor,
  uploadLimitOf,
} from './media.ts';

const NOW = Date.parse('2026-09-08T10:00:00Z');

const manifest = (limits: Record<string, unknown>) => ({ limits } as unknown as Capabilities);

function object(over: Partial<MediaObject> = {}): MediaObject {
  return {
    id: 'm-1',
    usage: 'ATTACHMENT',
    status: 'READY',
    content_type: 'application/pdf',
    size: 1_024,
    ref_count: 1,
    created_by: 'a-1',
    created_at: '2026-09-08T09:00:00Z',
    ...over,
  } as MediaObject;
}

test('the limit is the installation’s, and an absent one is not a limit of zero', () => {
  assert.equal(uploadLimitOf(manifest({ max_upload_bytes: 25_000_000 })), 25_000_000);
  // 0 is the "unlimited" every other quota in `limits` uses, and a manifest nobody has read yet
  // is the same answer: this client cannot honestly refuse a file it has no ceiling for.
  assert.equal(uploadLimitOf(manifest({ max_upload_bytes: 0 })), undefined);
  assert.equal(uploadLimitOf(manifest({})), undefined);
  assert.equal(uploadLimitOf(undefined), undefined);
});

test('a file over the limit is refused, and no limit refuses nothing', () => {
  assert.equal(isWithinUploadLimit(25_000_000, 25_000_000), true);
  assert.equal(isWithinUploadLimit(25_000_001, 25_000_000), false);
  assert.equal(isWithinUploadLimit(9e15, undefined), true);
});

test('the deadline grows with the bytes, from a base and up to a ceiling', () => {
  // A transfer needs a positive timeout and there is no default of "forever" (ByteTransfer), so
  // the number is computed rather than picked. A small file gets the round trip; a huge one is
  // bounded, because a deadline that is hours long is not a deadline.
  assert.equal(transferTimeoutFor(0), 30_000);
  assert.ok(transferTimeoutFor(32_000_000) > transferTimeoutFor(32_000));
  assert.equal(transferTimeoutFor(9e15), 2 * 60 * 60 * 1000);
});

test('only a sealed object may be used, and only through a target that has not expired', () => {
  assert.equal(isReady(object()), true);
  assert.equal(isReady(object({ status: 'PENDING' })), false);

  const fresh = { url: 'https://s/o?sig=1', method: 'GET' as const, expires_at: '2026-09-08T10:05:00Z' };
  const stale = { ...fresh, expires_at: '2026-09-08T09:55:00Z' };
  assert.equal(isUsable(fresh, NOW), true);
  assert.equal(isUsable(stale, NOW), false);
  assert.equal(isUsable(null, NOW), false);
});

test('a PENDING object has no download URL, whatever it carries', () => {
  // A claimed type is a claim. The sniffed allowlist judges at confirmation, so drawing or
  // downloading before it would be this client trusting what the server refuses to.
  const target = { url: 'https://s/o?sig=1', method: 'GET' as const, expires_at: '2026-09-08T10:05:00Z' };
  assert.equal(downloadUrlOf(object({ download: target }), NOW), target.url);
  assert.equal(downloadUrlOf(object({ status: 'PENDING', download: target }), NOW), undefined);
  assert.equal(downloadUrlOf(object(), NOW), undefined);
});

test('a cover names an image only when it is one', () => {
  assert.equal(coverImageIdOf({ kind: 'IMAGE', media_id: 'm-9' }), 'm-9');
  assert.equal(coverImageIdOf({ kind: 'COLOR', color_token: 'amber' }), undefined);
  assert.equal(coverImageIdOf(null), undefined);
});

test('a cover document carries exactly one of the two fields', () => {
  // The contract says "exactly one of the two, matching the kind", and sending both is
  // `items.cover_contradictory` rather than a preference the server resolves.
  assert.deepEqual(imageCover('m-9'), { kind: 'IMAGE', media_id: 'm-9', color_token: null });
  assert.deepEqual(colorCover('amber'), { kind: 'COLOR', color_token: 'amber', media_id: null });
});

test('the accept hint offers images for a cover and everything for an attachment', () => {
  assert.equal(acceptFor('COVER'), 'image/*');
  assert.equal(acceptFor('ATTACHMENT'), undefined);
});
