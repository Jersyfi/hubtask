<!-- SPDX-License-Identifier: Apache-2.0
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
  // administrator, is not something a component can work out — so the caller says per comment
  // whether the two are offered at all (`isChangeable`; absent where a role never may, P-05), and
  // switches one off with a reason only for something the reader could wait out (§4).
  //
  // Oldest first, because a conversation read from the bottom is a conversation read backwards.

  import type { Snippet } from 'svelte';

  import Avatar from './Avatar.svelte';
  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import LoadMore from './LoadMore.svelte';
  import Menu from './Menu.svelte';

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
    /**
     * Whether this reader may edit and remove it at all. False draws neither control and says
     * nothing: a person who may never change somebody else's comment is not shown the offer (P-05).
     * Absent means they may.
     */
    readonly isChangeable?: boolean;
    /** Why this reader cannot edit it now — absent means they can, present means they cannot and why. */
    readonly editDisabledReason?: string;
    /** The same, for removing it. */
    readonly deleteDisabledReason?: string;
    /** The replies to this comment. One level: a reply carries none of its own. */
    readonly replies?: readonly Omit<ThreadComment, 'replies'>[];
    /** Present: an internal note, drawn yellow with a lock and this word (*Internal*), never as a reply (ET-7). */
    readonly internalLabel?: string;
  }

  /**
   * Something that happened between the posts - a status change, a deadline met, a group changed:
   * a quiet row in time order among the comments (*All*, work-surface concept §6.9).
   */
  export interface ThreadEvent {
    readonly id: string;
    /** The sentence, resolved: *Clara Neumann moved it to In review*. */
    readonly text: string;
    readonly when: string;
    readonly at: string;
    readonly icon?: IconName;
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
    /** The ⋯ that holds edit and remove, beside *Reply*: *More for this comment*. */
    moreLabel: string;
    /** Whether there is another page of comments. */
    hasMore?: boolean;
    /** What the control that fetches it is called, and what it says once a page has landed. */
    loadMoreLabel?: string;
    arrivedLabel?: string;
    /** Why the last page of comments did not arrive, resolved by the caller (see `LoadMore`). */
    loadMoreFailedLabel?: string;
    onLoadMore?: () => void;
    onEdit?: (id: string) => void;
    onDelete?: (id: string) => void;
    /** Absent where the reader may not comment: no reply is offered then (P-05). */
    onReply?: (id: string) => void;
    /** Quiet rows placed among the comments by their instant. */
    events?: readonly ThreadEvent[];
    /** The caller's reading of a body - formatted text from the pages' editor (E-3); plain text without it. */
    body?: Snippet<[string]>;
  }

  const {
    body,
    label,
    comments,
    emptyLabel,
    removedLabel,
    editLabel,
    deleteLabel,
    replyLabel,
    moreLabel,
    hasMore = false,
    loadMoreLabel,
    arrivedLabel,
    loadMoreFailedLabel,
    onLoadMore,
    onEdit,
    onDelete,
    onReply,
    events = [],
  }: Props = $props();

  type Row = { kind: 'comment'; comment: ThreadComment } | { kind: 'event'; event: ThreadEvent };

  // Comments keep their own order (oldest first); an event goes before the first comment written
  // after it. Compared as instants: the history and the comments name them in different zones,
  // and as text every event of a later hour sorted after every post.
  const instant = (at: string): number => Date.parse(at);
  const rows = $derived.by((): Row[] => {
    const pending = [...events].sort((a, b) => instant(a.at) - instant(b.at));
    const out: Row[] = [];
    for (const comment of comments) {
      while (pending.length > 0 && instant(pending[0]?.at ?? '') < instant(comment.at)) out.push({ kind: 'event', event: pending.shift() as ThreadEvent });
      out.push({ kind: 'comment', comment });
    }
    for (const event of pending) out.push({ kind: 'event', event });
    return out;
  });
</script>

<section class="thread" aria-label={label}>
  {#if rows.length === 0}
    <p class="empty">{emptyLabel}</p>
  {:else}
    <ol class="list">
      {#each rows as each (each.kind === 'comment' ? each.comment.id : `event-${each.event.id}`)}
        {#if each.kind === 'event'}
          <li class="event" data-thread-event>
            <Icon name={each.event.icon ?? 'clock'} size="sm" />
            <span class="event-text">{each.event.text}</span>
            <time datetime={each.event.at}>{each.event.when}</time>
          </li>
        {:else}
        {@const comment = each.comment}
        <li class="entry">
          {@render row(comment, onReply !== undefined)}
          {#if comment.replies && comment.replies.length > 0}
            <ol class="replies">
              {#each comment.replies as reply (reply.id)}
                <li class="entry">{@render row(reply, false)}</li>
              {/each}
            </ol>
          {/if}
        </li>
        {/if}
      {/each}
    </ol>

    {#if hasMore && loadMoreLabel}
      <LoadMore isQuiet label={loadMoreLabel} {hasMore} {arrivedLabel} failedLabel={loadMoreFailedLabel} {onLoadMore} />
    {/if}
  {/if}
</section>

{#snippet row(comment: Omit<ThreadComment, 'replies'>, canReply: boolean)}
  <article class="comment" data-internal={comment.internalLabel ? '' : undefined}>
    <Avatar name={comment.authorName} src={comment.authorSrc} size="sm" />
    <div class="body">
      <p class="meta">
        <span class="author">{comment.authorName}</span>
        <!-- The order is the content, so the instant is machine-readable rather than only drawn -
             the same decision ActivityFeed made about a step. -->
        <time datetime={comment.at}>{comment.when}</time>
        {#if comment.internalLabel}<span class="internal"><Icon name="lock" size="sm" />{comment.internalLabel}</span>{/if}
        {#if comment.editedLabel && !comment.isRemoved}<span class="edited">{comment.editedLabel}</span>{/if}
      </p>

      {#if comment.isRemoved}
        <!-- Rule 3 again: the sentence says it was removed. A greyed-out row alone would leave a
             reader guessing, and the guess "it failed to load" is the wrong one. -->
        <p class="removed">{removedLabel}</p>
      {:else}
        {#if body && comment.body !== undefined}<div class="text">{@render body(comment.body)}</div>{:else}<p class="text">{comment.body}</p>{/if}
      {/if}

      {#if !comment.isRemoved && (canReply || comment.isChangeable !== false)}
        <p class="actions">
          {#if canReply}
            <Button size="sm" tone="subtle" onclick={() => onReply?.(comment.id)}>{replyLabel}</Button>
          {/if}
          <!-- *Reply ···* as the scene draws it: edit and remove wait in the ⋯, an unavailable one
               there with its reason (P-05). -->
          {#if comment.isChangeable !== false}
            <Menu
              label={moreLabel}
              items={[
                { id: 'edit', label: editLabel, icon: 'pencil', disabledReason: comment.editDisabledReason },
                { id: 'delete', label: deleteLabel, icon: 'trash', isDestructive: true, disabledReason: comment.deleteDisabledReason },
              ]}
              onselect={(id) => (id === 'edit' ? onEdit?.(comment.id) : onDelete?.(comment.id))}
            >
              {#snippet trigger(props)}<IconButton icon="ellipsis" label={moreLabel} size="sm" tone="subtle" {...props} />{/snippet}
            </Menu>
          {/if}
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
    gap: var(--sp-100);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .entry { min-width: 0; }

  /* One level, and the indent is what says so. `margin-inline-start` follows the writing
     direction, so a right-to-left thread indents the other way without a second rule. */
  .replies {
    margin-block-start: var(--sp-100);
    margin-inline-start: var(--sp-400);
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
    padding-inline-start: var(--sp-150);
  }

  .comment { display: flex; gap: var(--sp-100); min-width: 0; }

  /* An internal note is never mistaken for a reply (ET-7): yellow, with a lock and its word. */
  .comment[data-internal] {
    padding: var(--sp-100);
    border-radius: var(--r-md);
    border-inline-start: var(--bw-thick) solid var(--status-warning-accent);
    background: var(--status-warning-surface);
  }

  .internal { display: inline-flex; align-items: center; gap: var(--sp-025); color: var(--text-primary); font-weight: var(--fw-medium); }

  .event {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding-inline-start: var(--sp-050);
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .event-text { min-width: 0; overflow-wrap: anywhere; }

  .body { display: flex; flex: 1 1 auto; flex-direction: column; gap: var(--sp-025); min-width: 0; }

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

  /* A compact row under the text, as the scene draws *Reply ···*: the buttons at the 24 px a target
     needs (WCAG 2.5.8), the first one's words in line with the text above. */
  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: 0; margin-inline-start: calc(-1 * var(--sp-150)); }
  .actions :global(.button) { min-height: var(--sp-300); padding-block: 0; }
</style>
