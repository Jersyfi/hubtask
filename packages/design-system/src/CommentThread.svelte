<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What was said about an entry, oldest first, one level deep.
  //
  // **One level of replies and no more.** A comment carries `parentCommentId`, and a tree of
  // arbitrary depth is a tree nobody can indent on a phone and nobody can navigate by keyboard
  // without a treeview. One level is what the conversation actually has: somebody says something,
  // and somebody answers it.
  //
  // **"Removed" is a state, not a hole.** A deletion is soft, and the tombstone keeps the author,
  // the time and the place in the order — because a reply to a comment that vanished would dangle,
  // and a reader would be left working out who was answering what. So the row stays and says it
  // was removed; the body is the one thing that is gone, and it is gone from here because it was
  // never sent.
  //
  // **Edit and delete are the caller's knowledge.** Whether *this* reader is the author, or an
  // administrator, is not something a component can work out — so each affordance is switched on
  // per comment, and switched off with a reason (§4, `CapabilityGate`).
  //
  // Oldest first, because a conversation read from the bottom is a conversation read backwards.

  import Avatar from './Avatar.svelte';
  import Button from './Button.svelte';
  import LoadMore from './LoadMore.svelte';

  /** One comment, as the thread draws it. The shape `Comment` has, narrowed to what is rendered. */
  export interface ThreadComment {
    readonly id: string;
    /** Who wrote it. A removed comment keeps its author, which is why this is never absent. */
    readonly authorName: string;
    readonly authorSrc?: string;
    /** What they said. Absent on a removed comment: the body is what a soft delete takes away. */
    readonly body?: string;
    /** When, as a person reads it. Resolved by the caller, because a date is a locale (i18n-l10n.md). */
    readonly when: string;
    /** The machine-readable instant, for `<time datetime>`. */
    readonly at: string;
    /** Whether this comment was removed. The row stays; the body does not. */
    readonly isRemoved?: boolean;
    /** That it was changed after it was written. The model keeps `editedAt`; this is its sentence. */
    readonly editedLabel?: string;
    /** Why this reader cannot edit it — absent means they can, present means they cannot and why. */
    readonly editDisabledReason?: string;
    /** The same, for removing it. */
    readonly deleteDisabledReason?: string;
    /** The replies to this comment. One level: a reply carries none of its own. */
    readonly replies?: readonly Omit<ThreadComment, 'replies'>[];
  }

  interface Props {
    /** What the thread is called, for a reader who arrives by keyboard. */
    label: string;
    comments: readonly ThreadComment[];
    /** What a thread with nothing in it says. Resolved text (ADR-0011). */
    emptyLabel: string;
    /** What a removed comment says in place of its body. */
    removedLabel: string;
    /** The names of the two affordances. Shown per comment, switched on by the caller. */
    editLabel: string;
    deleteLabel: string;
    replyLabel: string;
    /** Whether there is another page of comments. */
    hasMore?: boolean;
    /** What the control that fetches it is called, and what it says once a page has landed. */
    loadMoreLabel?: string;
    arrivedLabel?: string;
    onLoadMore?: () => void;
    onEdit?: (id: string) => void;
    onDelete?: (id: string) => void;
    onReply?: (id: string) => void;
  }

  const {
    label,
    comments,
    emptyLabel,
    removedLabel,
    editLabel,
    deleteLabel,
    replyLabel,
    hasMore = false,
    loadMoreLabel,
    arrivedLabel,
    onLoadMore,
    onEdit,
    onDelete,
    onReply,
  }: Props = $props();
</script>

<section class="thread" aria-label={label}>
  {#if comments.length === 0}
    <p class="empty">{emptyLabel}</p>
  {:else}
    <ol class="list">
      {#each comments as comment (comment.id)}
        <li class="entry">
          {@render row(comment, true)}
          {#if comment.replies && comment.replies.length > 0}
            <ol class="replies">
              {#each comment.replies as reply (reply.id)}
                <li class="entry">{@render row(reply, false)}</li>
              {/each}
            </ol>
          {/if}
        </li>
      {/each}
    </ol>

    {#if hasMore && loadMoreLabel}
      <LoadMore label={loadMoreLabel} {hasMore} {arrivedLabel} {onLoadMore} />
    {/if}
  {/if}
</section>

{#snippet row(comment: Omit<ThreadComment, 'replies'>, canReply: boolean)}
  <article class="comment">
    <Avatar name={comment.authorName} src={comment.authorSrc} size="sm" />
    <div class="body">
      <p class="meta">
        <span class="author">{comment.authorName}</span>
        <!-- The order is the content, so the instant is machine-readable rather than only drawn -
             the same decision ActivityFeed made about a step. -->
        <time datetime={comment.at}>{comment.when}</time>
        {#if comment.editedLabel && !comment.isRemoved}<span class="edited">{comment.editedLabel}</span>{/if}
      </p>

      {#if comment.isRemoved}
        <!-- Rule 3 again: the sentence says it was removed. A greyed-out row alone would leave a
             reader guessing, and the guess "it failed to load" is the wrong one. -->
        <p class="removed">{removedLabel}</p>
      {:else}
        <p class="text">{comment.body}</p>
      {/if}

      {#if !comment.isRemoved}
        <p class="actions">
          {#if canReply}
            <Button size="sm" tone="subtle" onclick={() => onReply?.(comment.id)}>{replyLabel}</Button>
          {/if}
          <Button
            size="sm"
            tone="subtle"
            disabledReason={comment.editDisabledReason}
            onclick={() => onEdit?.(comment.id)}
          >{editLabel}</Button>
          <Button
            size="sm"
            tone="subtle"
            disabledReason={comment.deleteDisabledReason}
            onclick={() => onDelete?.(comment.id)}
          >{deleteLabel}</Button>
        </p>
      {/if}
    </div>
  </article>
{/snippet}

<style>
  .thread { display: flex; flex-direction: column; gap: var(--sp-150); min-width: 0; }

  .list,
  .replies {
    display: flex;
    flex-direction: column;
    gap: var(--sp-150);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .entry { min-width: 0; }

  /* One level, and the indent is what says so. `margin-inline-start` follows the writing
     direction, so a right-to-left thread indents the other way without a second rule. */
  .replies {
    margin-block-start: var(--sp-150);
    margin-inline-start: var(--sp-400);
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
    padding-inline-start: var(--sp-150);
  }

  .comment { display: flex; gap: var(--sp-100); min-width: 0; }

  .body { display: flex; flex-direction: column; gap: var(--sp-025); min-width: 0; }

  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--sp-100);
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .author { color: var(--text-primary); font-weight: var(--fw-medium); }

  .text { margin: 0; color: var(--text-primary); overflow-wrap: anywhere; }

  .removed { margin: 0; color: var(--text-subtle); font-style: italic; }

  .edited { color: var(--text-subtle); }

  .empty { margin: 0; padding: var(--sp-150); color: var(--text-subtle); font-size: var(--fs-075); }

  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: 0; }
</style>
