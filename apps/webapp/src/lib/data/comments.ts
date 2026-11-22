// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The thread, and who may change what is in it. Pure, so both questions are testable.
 *
 * **One level of replies.** A comment carries `parent_comment_id` and the thread nests exactly
 * once. A reply whose parent is not on this page hangs at the top rather than disappearing: the
 * page boundary is not a statement about the conversation, and a comment nobody can see is a
 * comment nobody can answer.
 *
 * **A removed comment keeps its place.** The server sends the identifier, the author and the times
 * with `body: null` — the tombstone — and rendering it in place is what stops a reply
 * from dangling under a comment that vanished.
 *
 * **Who may change one is the author or an administrator**, which is the server's own rule
 * (`ChangeComment.ensureAuthorOrAdministrator`) mirrored here so a control the reader cannot use
 * is off with a reason rather than refused after they press it. Mirrored, not re-decided: when the
 * guess is wrong the server's `access.not_permitted` is what the reader sees.
 */

import type { Comment } from '@hubtask/sync-engine';

/** The longest body the contract accepts, in code points. */
export const BODY_LIMIT = 20_000;

/**
 * How long a body is, as the server counts it.
 *
 * Code points, not UTF-16 units: `i18n-l10n.md` §5 says length limits are in code points, and a
 * thread of emoji would otherwise be refused at half the length a person was promised.
 */
export function bodyLength(body: string): number {
  return [...body].length;
}

/** Whether a body is short enough to send. Checked before the request leaves, not after. */
export function isWithinLimit(body: string): boolean {
  return bodyLength(body) <= BODY_LIMIT;
}

/** A comment is removed exactly when its body is gone — the contract's own "null exactly when". */
export function isRemoved(comment: Comment): boolean {
  return comment.body === null || comment.body === undefined;
}

/** One comment with the replies underneath it. One level, and no more. */
export interface Thread {
  readonly comment: Comment;
  readonly replies: readonly Comment[];
}

/**
 * The page as a thread: the top-level comments in the order they came, each with its replies.
 *
 * A reply whose parent is not on the page is treated as top-level rather than dropped. Losing a
 * comment because its parent is on the next page would be the page boundary editing the
 * conversation.
 */
export function threadOf(comments: readonly Comment[]): readonly Thread[] {
  const byId = new Set(comments.map((comment) => comment.id));
  const replies = new Map<string, Comment[]>();

  for (const comment of comments) {
    const parent = comment.parent_comment_id;
    if (!parent || !byId.has(parent)) continue;
    replies.set(parent, [...(replies.get(parent) ?? []), comment]);
  }

  return comments
    .filter((comment) => !comment.parent_comment_id || !byId.has(comment.parent_comment_id))
    .map((comment) => ({ comment, replies: replies.get(comment.id) ?? [] }));
}

/** The roles this client treats as administrative, which is the server's `AtLeast(ADMIN)`. */
const ADMINISTRATIVE = new Set(['OWNER', 'ADMIN']);

/**
 * Whether this reader may edit or delete a comment.
 *
 * The author always may. Otherwise it takes a role the server would accept, and the roles the
 * actor holds along the path are what the client already reads. A role this client has never heard of
 * is **not** administrative: guessing in the permissive direction is what the manifest exists to
 * prevent, and the cost of guessing low is a control that is off and a server that would have
 * allowed it — which is a worse offer, not a wrong one.
 */
export function mayModerate(
  comment: Comment,
  actorId: string | undefined,
  roles: readonly string[],
): boolean {
  if (actorId !== undefined && comment.author_id === actorId) return true;
  return roles.some((role) => ADMINISTRATIVE.has(role));
}

/**
 * Whether a comment can be replied to.
 *
 * A removed one cannot: the server refuses it, and the refusal is a sentence. Offering the control
 * and letting the refusal explain would be offering an action that never works.
 */
export function mayReplyTo(comment: Comment): boolean {
  return !isRemoved(comment);
}

/** What a system comment's heading is rendered with: the catalogue, the moment and a device's words. */
export interface Readings {
  readonly say: (code: string, params?: Readonly<Record<string, string>>) => string;
  readonly moment: (iso: string) => string;
  /** The words for the device a displaced version was written on, by its identifier. */
  readonly device: (id: string) => string;
}

/** A system comment as the thread draws it: its mark word and its heading. */
export interface SystemNote {
  readonly label: string;
  readonly heading: string;
}

/**
 * The heading of a comment the server filed (`kind: SYSTEM`): `system_code` rendered through the
 * catalogue with `system_params`, the author's name and the moment as the reader writes it - the
 * server sends a code and its parameters, never a sentence (rule 8). Undefined for what somebody
 * wrote.
 */
export function systemNoteOf(comment: Comment, author: string, read: Readings): SystemNote | undefined {
  if (comment.kind !== 'SYSTEM' || !comment.system_code) return undefined;
  const params: Record<string, string> = { ...(comment.system_params ?? {}), author };
  if (params['at']) params['at'] = read.moment(params['at']);
  if (params['device']) params['device'] = read.device(params['device']);
  return { label: read.say('app.comments.kept_version'), heading: read.say(comment.system_code, params) };
}

/** The query parameter that addresses a comment of an entry: `/items/<id>?comment=<id>`. */
export const COMMENT_QUERY = 'comment';

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** The address of one comment in its entry's conversation. */
export function commentAddress(itemId: string, commentId: string): string {
  return `/items/${encodeURIComponent(itemId)}?${COMMENT_QUERY}=${encodeURIComponent(commentId)}`;
}

/** The comment an address names, or undefined where it names none a comment could have. */
export function addressedComment(value: string | undefined): string | undefined {
  const id = value?.trim();
  return id && UUID.test(id) ? id.toLowerCase() : undefined;
}

/**
 * How many threads, from the first, must be drawn for the comment to be among them - its own, or
 * the one it answers. Undefined where these threads hold no such comment.
 */
export function threadsToShow(threads: readonly Thread[], commentId: string): number | undefined {
  const at = threads.findIndex(({ comment, replies }) => comment.id === commentId || replies.some((reply) => reply.id === commentId));
  return at < 0 ? undefined : at + 1;
}
