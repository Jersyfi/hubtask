// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Post } from '@hubtask/sync-engine';

import { POSTS_SHOWN, featuredPost, hubPostsPath, postsByMonth, publishedPosts, readingMinutes, sectionPosts, unreadCount } from './posts.ts';

const post = (id: string, at: string, more: Partial<Post> = {}): Post => ({
  page_id: id, hub_id: 'h', title: id, icon: null, template: null, state: 'PUBLISHED', publish_at: at, published_at: at,
  featured_until: null, featured: false, audience: 'HUB', group_ids: [], everyone_in_workspace: false, discussion: true,
  notify: true, confirm_by: null, created_by: null, created_at: at, seen_count: 0, seen: true, confirmed: false,
  may_write: false, cover: null, excerpt: '', reading_minutes: 1, reply_count: 0, labels: [], ...more,
});

// UC-DOC-17 check 7: the hub column's section is the published posts, newest first, a few, the
// unread ones counted; a scheduled one or a draft is not among them.
test('the hub column shows the newest published posts and counts the unread', () => {
  const rows = [
    post('older', '2026-11-02T08:00:00Z'),
    post('newest', '2026-11-14T08:00:00Z', { seen: false }),
    post('planned', '2026-11-16T08:00:00Z', { state: 'SCHEDULED', published_at: null, seen: false }),
    post('middle', '2026-11-06T08:00:00Z'),
    post('oldest', '2026-10-30T08:00:00Z'),
  ];
  assert.deepEqual(sectionPosts(rows).map((row) => row.page_id), ['newest', 'middle', 'older']);
  assert.equal(unreadCount(rows), 1);
});

// UC-DOC-17 check 7: the section holds every published post, bounded as *Contents* is - the first
// three shown, the rest behind *n more* - so the count and *n more* agree with what it holds.
test('the hub column holds every published post, three of them shown first', () => {
  const rows = ['a', 'b', 'c', 'd', 'e'].map((id, at) => post(id, `2026-11-0${at + 1}T08:00:00Z`));
  rows.push(post('draft', '2026-11-09T08:00:00Z', { state: 'DRAFT', published_at: null }));
  assert.deepEqual(publishedPosts(rows).map((row) => row.page_id), ['e', 'd', 'c', 'b', 'a']);
  assert.equal(POSTS_SHOWN, 3);
  assert.deepEqual(sectionPosts(rows).map((row) => row.page_id), ['e', 'd', 'c']);
  assert.deepEqual(publishedPosts([post('draft', '2026-11-09T08:00:00Z', { state: 'DRAFT', published_at: null })]), [], 'none yet: the empty part');
});

test('the list stands by month, newest first, with the newest featured post on top', () => {
  const rows = [post('a', '2026-10-30T08:00:00Z'), post('b', '2026-11-14T08:00:00Z', { featured: true }), post('c', '2026-11-02T08:00:00Z')];
  assert.deepEqual(postsByMonth(rows).map((group) => [group.month, group.posts.map((row) => row.page_id)]), [
    ['2026-11', ['b', 'c']], ['2026-10', ['a']],
  ]);
  assert.equal(featuredPost(rows)?.page_id, 'b');
  assert.equal(featuredPost([rows[0]!]), undefined);
});

test('reading time is at least a minute, about two hundred words each', () => {
  assert.equal(readingMinutes(''), 1);
  assert.equal(readingMinutes(Array(820).fill('Wort').join(' ')), 4);
  assert.equal(readingMinutes('# — * '), 1);
});

test('a filter other than all is asked of the server', () => {
  assert.equal(hubPostsPath('h'), '/hubs/h/posts');
  assert.equal(hubPostsPath('h', 'UNREAD'), '/hubs/h/posts?filter=UNREAD');
});
