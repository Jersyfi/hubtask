// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Comment } from '@hubtask/sync-engine';

import { format } from '../i18n/format.ts';
import { SOURCE } from '../i18n/source.ts';
import {
  addressedComment, bodyLength, commentAddress, isRemoved, isWithinLimit, mayModerate, mayReplyTo, systemNoteOf, threadOf, threadsToShow, type Readings,
} from './comments.ts';

const AMELIE = 'a-1';
const JONAS = 'a-2';

function comment(over: Partial<Comment> & Pick<Comment, 'id'>): Comment {
  return {
    item_id: 'i-1',
    author_id: AMELIE,
    body: 'text',
    created_at: '2026-09-04T07:12:00Z',
    version: 1,
    ...over,
  } as Comment;
}

test('a body is counted in code points, as the server counts it', () => {
  // UTF-16 would make this four, and a thread of emoji would be refused at half the length the
  // reader was promised (i18n-l10n.md §5).
  assert.equal(bodyLength('👍🏽'), 2);
  assert.equal(isWithinLimit('x'.repeat(20_000)), true);
  assert.equal(isWithinLimit('x'.repeat(20_001)), false);
});

test('a comment is removed exactly when its body is gone', () => {
  assert.equal(isRemoved(comment({ id: 'c1', body: null, deleted_at: '2026-09-04T08:00:00Z' })), true);
  assert.equal(isRemoved(comment({ id: 'c1' })), false);
});

test('a reply sits under its parent, and the order is the page order', () => {
  const page = [
    comment({ id: 'c1' }),
    comment({ id: 'c1r1', parent_comment_id: 'c1' }),
    comment({ id: 'c2' }),
    comment({ id: 'c1r2', parent_comment_id: 'c1' }),
  ];

  const threads = threadOf(page);
  assert.deepEqual(threads.map((thread) => thread.comment.id), ['c1', 'c2']);
  assert.deepEqual(threads[0]?.replies.map((reply) => reply.id), ['c1r1', 'c1r2']);
});

test('a reply whose parent is on another page hangs at the top rather than vanishing', () => {
  // The page boundary is not a statement about the conversation, and a comment nobody can see is
  // a comment nobody can answer.
  const threads = threadOf([comment({ id: 'orphan', parent_comment_id: 'gone' })]);
  assert.deepEqual(threads.map((thread) => thread.comment.id), ['orphan']);
});

test('a removed comment keeps its place and cannot be replied to', () => {
  const removed = comment({ id: 'c1', body: null, deleted_at: '2026-09-04T08:00:00Z' });
  const threads = threadOf([removed, comment({ id: 'c1r1', parent_comment_id: 'c1' })]);

  // The tombstone is what stops the reply dangling, so it is still a thread with a reply under it.
  assert.equal(threads.length, 1);
  assert.equal(threads[0]?.replies.length, 1);
  assert.equal(mayReplyTo(removed), false);
});

test('the author may change their own comment, and a stranger may not', () => {
  const mine = comment({ id: 'c1', author_id: AMELIE });
  assert.equal(mayModerate(mine, AMELIE, ['VIEWER']), true);
  assert.equal(mayModerate(mine, JONAS, ['MEMBER']), false);
});

test('an administrator may change somebody else’s, and an unknown role may not', () => {
  const theirs = comment({ id: 'c1', author_id: AMELIE });
  assert.equal(mayModerate(theirs, JONAS, ['ADMIN']), true);
  assert.equal(mayModerate(theirs, JONAS, ['OWNER']), true);
  // Guessing in the permissive direction is what the manifest exists to prevent. The cost of
  // guessing low is a control that is off where the server would have allowed it.
  assert.equal(mayModerate(theirs, JONAS, ['MODERATOR']), false);
  assert.equal(mayModerate(theirs, undefined, []), false);
});

// The heading of a kept version (UC-SYN-02 check 7): the code and its
// parameters rendered here, through the catalogue, never a sentence from the server (rule 8).
const read: Readings = {
  say: (code, params) => format(SOURCE[code] ?? code, params ?? {}),
  moment: (iso) => `moment ${iso}`,
  device: (id) => (id === 'dev-1' ? 'this device' : 'another device'),
};

test('a kept version is headed by its code, naming whose version it holds and when', () => {
  const displaced = comment({ id: 'k1', kind: 'SYSTEM', system_code: 'sync.displaced_version', system_params: { field: 'notes', device: 'dev-1', at: '2026-11-22T09:00:00Z' } });
  assert.deepEqual(systemNoteOf(displaced, 'Amelie Fischer', read), {
    label: 'Kept version',
    heading: 'Diverging version from Amelie Fischer, written on this device at moment 2026-11-22T09:00:00Z. This is the text that lost the merge; the entry carries the other one.',
  });
});

test('a displaced version names the device in words, never by its identifier', () => {
  const displaced = comment({ id: 'k3', kind: 'SYSTEM', system_code: 'sync.displaced_version', system_params: { field: 'notes', device: 'dev-2', at: '2026-11-22T09:00:00Z' } });
  assert.equal(systemNoteOf(displaced, 'Jonas Brandt', read)?.heading,
    'Diverging version from Jonas Brandt, written on another device at moment 2026-11-22T09:00:00Z. This is the text that lost the merge; the entry carries the other one.');
});

test('what somebody wrote has no heading', () => {
  assert.equal(systemNoteOf(comment({ id: 'c1' }), 'Amelie Fischer', read), undefined);
  assert.equal(systemNoteOf(comment({ id: 'c2', kind: 'USER' }), 'Amelie Fischer', read), undefined);
  assert.equal(systemNoteOf(comment({ id: 'c3', kind: 'SYSTEM', system_code: null }), 'Amelie Fischer', read), undefined);
});

// The address of a comment: `/items/<id>?comment=<id>`, read back leniently in
// case and strictly in shape.
const KEPT = '01a0e2e0-0000-7000-8000-0000000c0f01';

test('a comment is addressed in its entry, and the address names it again', () => {
  assert.equal(commentAddress('i-1', KEPT), `/items/i-1?comment=${KEPT}`);
  assert.equal(addressedComment(KEPT), KEPT);
  assert.equal(addressedComment(KEPT.toUpperCase()), KEPT);
});

test('an address that names no comment is no address', () => {
  assert.equal(addressedComment(undefined), undefined);
  assert.equal(addressedComment(''), undefined);
  assert.equal(addressedComment('comment-1'), undefined);
  assert.equal(addressedComment(`${KEPT}x`), undefined);
});

test('an addressed comment is drawn with as many threads as reach it, its own or its parent', () => {
  const threads = threadOf([comment({ id: 'a' }), comment({ id: 'b' }), comment({ id: 'b1', parent_comment_id: 'b' }), comment({ id: 'c' })]);
  assert.equal(threadsToShow(threads, 'a'), 1);
  assert.equal(threadsToShow(threads, 'b1'), 2);
  assert.equal(threadsToShow(threads, 'c'), 3);
  assert.equal(threadsToShow(threads, 'gone'), undefined);
});
