// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's posts as the screens draw them (work-surface concept B-1 … B-6, UC-DOC-17, UC-DOC-18):
 * the paths they read and the arrangements the server leaves to the client. Who sees which post is
 * the server's to judge; a list here only orders and narrows what it answered.
 */

import type { Post } from '@hubtask/sync-engine';

export const hubPostsPath = (hubId: string, filter: PostFilter = 'ALL') =>
  filter === 'ALL' ? `/hubs/${hubId}/posts` : `/hubs/${hubId}/posts?filter=${filter}`;
export const postPath = (pageId: string) => `/posts/${pageId}`;
export const postRepliesPath = (pageId: string) => `/posts/${pageId}/replies`;
export const postConfirmationsPath = (pageId: string) => `/posts/${pageId}/confirmations`;
/** The published posts the reader has not seen yet, across hubs: *New for you* on Today. */
export const unreadPostsPath = '/posts';

export type PostFilter = 'ALL' | 'UNREAD' | 'MINE' | 'SCHEDULED';

/** Where a post is read; the screen a mail's link opens. */
export const postHref = (pageId: string) => `/posts/${pageId}`;

/** The moment a post stands at: published, or for a writer the one it is scheduled for. */
export function postMoment(post: Pick<Post, 'published_at' | 'publish_at' | 'created_at'>): string {
  return post.published_at ?? post.publish_at ?? post.created_at;
}

/**
 * The hub column's section *Posts* (B-2): the published posts, newest first, a few of them - the
 * unread ones carry their dot; drafts and scheduled ones stay in the list for writers.
 */
export function sectionPosts(posts: readonly Post[], shown = 3): Post[] {
  return posts
    .filter((post) => post.state === 'PUBLISHED')
    .sort((a, b) => postMoment(b).localeCompare(postMoment(a)))
    .slice(0, shown);
}

/** How many of the published posts the reader has not seen. */
export function unreadCount(posts: readonly Post[]): number {
  return posts.filter((post) => post.state === 'PUBLISHED' && !post.seen).length;
}

/** The list below the featured post, by month, newest first (scene `posts` 3). */
export function postsByMonth(posts: readonly Post[]): { month: string; posts: Post[] }[] {
  const months = new Map<string, Post[]>();
  for (const post of [...posts].sort((a, b) => postMoment(b).localeCompare(postMoment(a)))) {
    const month = postMoment(post).slice(0, 7);
    months.set(month, [...(months.get(month) ?? []), post]);
  }
  return [...months].map(([month, rows]) => ({ month, posts: rows }));
}

/** The one post on top (scene `posts` 2): the newest featured one. */
export function featuredPost(posts: readonly Post[]): Post | undefined {
  return posts.filter((post) => post.featured).sort((a, b) => postMoment(b).localeCompare(postMoment(a)))[0];
}

/** Reading time in whole minutes, at least one: about two hundred words a minute. */
export function readingMinutes(text: string): number {
  const words = text.split(/\s+/).filter((word) => /[\p{L}\p{N}]/u.test(word)).length;
  return Math.max(1, Math.round(words / 200));
}

/** The four fixed reactions of a post's discussion (B-4), in their order, with their marks. */
export const REACTIONS = [
  { id: 'THANKS', mark: '🙏' },
  { id: 'AGREE', mark: '👍' },
  { id: 'CELEBRATE', mark: '🎉' },
  { id: 'IDEA', mark: '💡' },
] as const;

/** The post's own templates (B-1), in the order the writer is offered them, with their marks. */
export const POST_TEMPLATES = [
  { id: 'ANNOUNCEMENT', mark: '📣' },
  { id: 'RELEASE_NOTE', mark: '🚀' },
  { id: 'MONTHLY_REVIEW', mark: '📅' },
  { id: 'WELCOME', mark: '👋' },
] as const;
