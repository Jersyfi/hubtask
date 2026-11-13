<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A board's comments beside the canvas (UC-CNV-09): the threads with their replies, each pinned to
  // an object or a place, a new one pinned at the chosen object or the middle of the view. A person
  // is named by choosing them; the words carry their address and draw their name. Whoever may draw
  // resolves a thread; only its author takes a comment back. A reader reads them and writes none.

  import { untrack } from 'svelte';
  import { Avatar, Button, IconButton, Inline, Stack, Switch, Textarea } from '@hubtask/design-system/components';
  import type { WhiteboardComment } from '@hubtask/sync-engine';

  import PersonPicker from '../people/PersonPicker.svelte';
  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { commentParts, mentionOf, threadsOf, whiteboards } from '../data/whiteboards.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime, formatRelative } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    boardId: string;
    writable: boolean;
    comments: readonly WhiteboardComment[];
    /** Where a new thread is pinned: the chosen object, or a place. */
    pin: () => { element_id?: string; x: number; y: number };
    /** What the new thread stands at, in words: the object chosen, or the view. */
    pinLabel: string;
    /** The thread opened from its pin on the canvas. */
    open: string | undefined;
    withResolved: boolean;
    onshow: (comment: WhiteboardComment) => void;
  }

  let { boardId, writable, comments, pin, pinLabel, open = $bindable(), withResolved = $bindable(), onshow }: Props = $props();

  const threads = $derived(threadsOf(comments, withResolved));
  let draft = $state('');
  let replyTo = $state<string | undefined>(undefined);
  let replyDraft = $state('');
  let naming = $state('');
  let namingReply = $state('');

  $effect(() => {
    const ids = comments.map((each) => each.author_id);
    untrack(() => accounts.resolve(ids));
  });

  /** A person chosen is named in the words being written, and the picker is free again. */
  const named = (words: string, person: string) =>
    `${words}${words && !words.endsWith(' ') ? ' ' : ''}${mentionOf(person, accounts.nameOf(person) ?? '')} `;
  $effect(() => {
    const person = naming;
    if (!person) return;
    untrack(() => {
      draft = named(draft, person);
      naming = '';
    });
  });
  $effect(() => {
    const person = namingReply;
    if (!person) return;
    untrack(() => {
      replyDraft = named(replyDraft, person);
      namingReply = '';
    });
  });

  async function act(run: () => Promise<unknown>): Promise<boolean> {
    try {
      await run();
      return true;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return false;
    }
  }

  async function add() {
    if (draft.trim() === '') return;
    if (await act(() => whiteboards.addComment(boardId, { body: draft.trim(), ...pin() }))) draft = '';
  }

  async function reply(thread: WhiteboardComment) {
    if (replyDraft.trim() === '') return;
    if (await act(() => whiteboards.addComment(boardId, { body: replyDraft.trim(), parent_id: thread.id }))) {
      replyTo = undefined;
      replyDraft = '';
    }
  }

  const nameOf = (id: string | null) => (id ? (accounts.nameOf(id) ?? t('app.people.unnamed')) : t('app.people.unnamed'));
</script>

<!-- The sheet as scene wboard draws a conversation: a thread a card, each comment its face, name and
     time, the thread's verbs in one quiet row, the composer at the foot. -->
<Stack gap="100">
  <Switch label={t('app.whiteboards.comments_resolved')} bind:checked={withResolved} />
  <ol class="threads" aria-label={t('app.whiteboards.comments_list')}>
    {#each threads as { thread, replies } (thread.id)}
      <li class="thread" class:open={open === thread.id} class:resolved={thread.resolved} data-board-thread={thread.id}>
        {#each [thread, ...replies] as comment (comment.id)}
          <div class="comment" class:reply={comment.parent_id !== null}>
            <Avatar name={nameOf(comment.author_id)} size="sm" />
            <div class="body">
              <p class="meta"><strong class="name">{nameOf(comment.author_id)}</strong> <span title={formatDateTime(comment.created_at, messages.locale)}>{formatRelative(comment.created_at, messages.locale)}</span></p>
              <p class="text">
                {#if comment.deleted}
                  {t('app.whiteboards.comment_removed')}
                {:else}
                  {#each commentParts(comment.body) as part, i (i)}
                    {#if 'person' in part}<strong class="person">@{accounts.nameOf(part.person) ?? part.name}</strong>{:else}{part.text}{/if}
                  {/each}
                {/if}
              </p>
            </div>
            {#if comment.author_id === actor.account?.id && !comment.deleted && writable}
              <IconButton size="sm" tone="subtle" icon="trash" label={t('app.whiteboards.comment_delete')} onclick={() => act(() => whiteboards.deleteComment(comment))} />
            {/if}
          </div>
        {/each}
        <div class="verbs">
          <Button size="sm" tone="subtle" icon="eye" onclick={() => { open = thread.id; onshow(thread); }}>{t('app.whiteboards.comment_show')}</Button>
          {#if writable && !thread.deleted}
            <Button size="sm" tone="subtle" icon="message-square" onclick={() => { replyTo = thread.id; replyDraft = ''; }}>{t('app.whiteboards.comment_reply')}</Button>
            <Button size="sm" tone="subtle" icon={thread.resolved ? 'rotate-ccw' : 'check'} onclick={() => act(() => whiteboards.changeComment(thread, { resolved: !thread.resolved }))}>
              {thread.resolved ? t('app.whiteboards.comment_reopen') : t('app.whiteboards.comment_resolve')}
            </Button>
          {/if}
        </div>
        {#if replyTo === thread.id}
          <Stack gap="050">
            <Textarea label={t('app.whiteboards.comment_reply')} isLabelHidden rows={2} bind:value={replyDraft} />
            <PersonPicker label={t('app.whiteboards.comment_name')} noneLabel={t('app.person_picker.choose')} bind:value={namingReply} />
            <Inline gap="050" justify="end">
              <Button size="sm" tone="subtle" onclick={() => (replyTo = undefined)}>{t('app.links.cancel')}</Button>
              <Button size="sm" tone="primary" disabledReason={replyDraft.trim() === '' ? t('app.whiteboards.comment_needs_words') : undefined} onclick={() => void reply(thread)}>{t('app.whiteboards.comment_send')}</Button>
            </Inline>
          </Stack>
        {/if}
      </li>
    {:else}
      <li class="quiet none">{t('app.whiteboards.no_comments')}</li>
    {/each}
  </ol>
  {#if writable}
    <div class="composer">
      <Textarea label={t('app.whiteboards.comment_new')} isLabelHidden placeholder={t('app.whiteboards.comment_new')} rows={2} bind:value={draft} />
      <PersonPicker label={t('app.whiteboards.comment_name')} noneLabel={t('app.person_picker.choose')} bind:value={naming} />
      <Inline gap="050" align="center" justify="between">
        <p class="quiet small">{pinLabel}</p>
        <Button size="sm" tone="primary" disabledReason={draft.trim() === '' ? t('app.whiteboards.comment_needs_words') : undefined} onclick={() => void add()}>{t('app.whiteboards.comment_send')}</Button>
      </Inline>
    </div>
  {/if}
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .none { padding: var(--sp-200) 0; text-align: center; }
  .threads { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .thread { display: grid; gap: var(--sp-100); padding: var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .thread.open { border-color: var(--accent-primary); box-shadow: 0 0 0 var(--bw-hairline) var(--accent-primary); }
  .thread.resolved { background: var(--bg-surface-sunken); }
  .thread.resolved .text { color: var(--text-subtle); }
  .comment { display: grid; grid-template-columns: auto 1fr auto; align-items: start; gap: var(--sp-100); }
  .comment.reply { padding-inline-start: var(--sp-300); }
  .body { display: grid; gap: var(--sp-025); min-inline-size: 0; }
  .meta { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .name { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
  .person { color: var(--text-brand); font-weight: var(--fw-semibold); }
  .verbs { display: flex; flex-wrap: wrap; gap: var(--sp-025); margin-inline-start: calc(-1 * var(--sp-050)); }
  .composer { display: grid; gap: var(--sp-100); padding-block-start: var(--sp-150); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
</style>
