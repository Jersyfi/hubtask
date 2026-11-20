// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A post's writes (UC-DOC-17, UC-DOC-18), each invalidating what shows it: the hub's list, the
 * post, its discussion and *New for you*. Posts are read online - devices never hold them - so
 * every write goes straight to the server.
 */

import type { Post, PostCreate, PostFollowing, PostReminded, PostReply, PostReplyCreate, PostSettings } from '@hubtask/sync-engine';

import { engine, INTENT } from './engine.ts';
import { hubPostsPath, postPath, postRepliesPath, postConfirmationsPath, unreadPostsPath } from './posts.ts';

export * from './posts.ts';

/** Every list a change to one post may show in. */
function touched(post: Pick<Post, 'hub_id' | 'page_id'>): string[] {
  return [
    hubPostsPath(post.hub_id),
    hubPostsPath(post.hub_id, 'UNREAD'),
    hubPostsPath(post.hub_id, 'MINE'),
    hubPostsPath(post.hub_id, 'SCHEDULED'),
    postPath(post.page_id),
    unreadPostsPath,
  ];
}

/** The settings a post stands with, to be sent whole with one of them changed. */
export function settingsOf(post: Post): PostSettings {
  return {
    template: post.template,
    publish_at: post.publish_at,
    featured_until: post.featured_until,
    audience: post.audience,
    group_ids: [...post.group_ids],
    discussion: post.discussion,
    notify: post.notify,
    confirm_by: post.confirm_by,
  };
}

export const posts = {
  create(hubId: string, create: PostCreate): Promise<Post> {
    return engine.mutate<Post>('POST', hubPostsPath(hubId), create, {
      idempotencyKey: INTENT,
      invalidates: touched({ hub_id: hubId, page_id: '' }),
    });
  },

  /** A draft with the defaults a writer starts from (B-1), as *New post* and the hub column make one. */
  draft(hubId: string, title: string): Promise<Post> {
    return posts.create(hubId, { title, publish_at: null, audience: 'HUB', group_ids: [], discussion: true, notify: true });
  },

  /** Follows a hub's posts or stops (B-5): a follower is told when a post for them goes out. */
  follow(hubId: string, following: boolean): Promise<PostFollowing> {
    return engine.mutate<PostFollowing>('PUT', `${hubPostsPath(hubId)}/following`, { following }, {
      invalidates: [hubPostsPath(hubId), hubPostsPath(hubId, 'UNREAD'), hubPostsPath(hubId, 'MINE'), hubPostsPath(hubId, 'SCHEDULED')],
    });
  },

  settle(post: Post, settings: PostSettings): Promise<Post> {
    return engine.mutate<Post>('PUT', postPath(post.page_id), settings, { invalidates: touched(post) });
  },

  publish(post: Post): Promise<Post> {
    return engine.mutate<Post>('POST', `${postPath(post.page_id)}:publish`, {}, { invalidates: touched(post) });
  },

  /** Counts the reader among those who have seen it, once; the lists' dots go with it. */
  seen(post: Post): Promise<unknown> {
    return engine.mutate('POST', `${postPath(post.page_id)}:seen`, {}, { invalidates: touched(post) });
  },

  confirm(post: Post): Promise<Post> {
    return engine.mutate<Post>('POST', `${postPath(post.page_id)}:confirm`, {}, { invalidates: touched(post) });
  },

  remind(post: Post): Promise<PostReminded> {
    return engine.mutate<PostReminded>('POST', `${postPath(post.page_id)}:remind`, {}, {
      idempotencyKey: INTENT,
      invalidates: [postPath(post.page_id), postConfirmationsPath(post.page_id)],
    });
  },

  reply(post: Post, reply: PostReplyCreate): Promise<PostReply> {
    return engine.mutate<PostReply>('POST', postRepliesPath(post.page_id), reply, {
      idempotencyKey: INTENT,
      invalidates: [postRepliesPath(post.page_id), ...touched(post)],
    });
  },

  takeBack(post: Post, replyId: string): Promise<unknown> {
    return engine.mutate('DELETE', `${postRepliesPath(post.page_id)}/${replyId}`, undefined, {
      invalidates: [postRepliesPath(post.page_id), ...touched(post)],
    });
  },

  react(post: Post, replyId: string, reaction: string, isOn: boolean): Promise<unknown> {
    return engine.mutate(isOn ? 'PUT' : 'DELETE', `${postRepliesPath(post.page_id)}/${replyId}/reactions/${reaction}`, isOn ? {} : undefined, {
      invalidates: [postRepliesPath(post.page_id)],
    });
  },
};
