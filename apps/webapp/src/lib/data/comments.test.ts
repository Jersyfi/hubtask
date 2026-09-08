// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Comment } from '@hubtask/sync-engine';

import { bodyLength, isRemoved, isWithinLimit, mayModerate, mayReplyTo, threadOf } from './comments.ts';

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
