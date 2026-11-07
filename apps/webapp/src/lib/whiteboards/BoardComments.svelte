<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A board's comments beside the canvas (UC-CNV-09): the threads with their replies, each pinned to
  // an object or a place, a new one pinned at the chosen object or the middle of the view. A person
  // is named by choosing them; the words carry their address and draw their name. Whoever may draw
  // resolves a thread; only its author takes a comment back. A reader reads them and writes none.

  import { untrack } from 'svelte';
  import { Button, Inline, Stack, Switch, Textarea } from '@hubtask/design-system/components';
  import type { WhiteboardComment } from '@hubtask/sync-engine';

  import PersonPicker from '../people/PersonPicker.svelte';
  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { commentParts, mentionOf, threadsOf, whiteboards } from '../data/whiteboards.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
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

<Stack gap="050">
  <Inline gap="050" align="center" justify="between">
    <h3 class="heading">{t('app.whiteboards.comments', { count: String(threads.filter((each) => !each.thread.resolved).length) })}</h3>
    <Switch label={t('app.whiteboards.comments_resolved')} bind:checked={withResolved} />
  </Inline>
  <ol class="threads" aria-label={t('app.whiteboards.comments_list')}>
    {#each threads as { thread, replies } (thread.id)}
      <li class="thread" class:open={open === thread.id} class:resolved={thread.resolved}>
        {#each [thread, ...replies] as comment (comment.id)}
          <div class="comment">
            <p class="meta">{nameOf(comment.author_id)} · {formatDateTime(comment.created_at, messages.locale)}</p>
            <p class="text">
              {#if comment.deleted}
                {t('app.whiteboards.comment_removed')}
              {:else}
                {#each commentParts(comment.body) as part, i (i)}
                  {#if 'person' in part}<strong class="person">@{accounts.nameOf(part.person) ?? part.name}</strong>{:else}{part.text}{/if}
                {/each}
              {/if}
            </p>
            {#if comment.author_id === actor.account?.id && !comment.deleted && writable}
              <Button size="sm" tone="subtle" onclick={() => act(() => whiteboards.deleteComment(comment))}>{t('app.whiteboards.comment_delete')}</Button>
            {/if}
          </div>
        {/each}
        <Inline gap="050">
          <Button size="sm" tone="subtle" icon="eye" onclick={() => { open = thread.id; onshow(thread); }}>{t('app.whiteboards.comment_show')}</Button>
          {#if writable && !thread.deleted}
            <Button size="sm" tone="subtle" onclick={() => act(() => whiteboards.changeComment(thread, { resolved: !thread.resolved }))}>
              {thread.resolved ? t('app.whiteboards.comment_reopen') : t('app.whiteboards.comment_resolve')}
            </Button>
            <Button size="sm" tone="subtle" onclick={() => { replyTo = thread.id; replyDraft = ''; }}>{t('app.whiteboards.comment_reply')}</Button>
          {/if}
        </Inline>
        {#if replyTo === thread.id}
          <Stack gap="050">
            <Textarea label={t('app.whiteboards.comment_reply')} isLabelHidden rows={2} bind:value={replyDraft} />
            <PersonPicker label={t('app.whiteboards.comment_name')} noneLabel={t('app.person_picker.choose')} bind:value={namingReply} />
            <Button size="sm" disabledReason={replyDraft.trim() === '' ? t('app.whiteboards.comment_needs_words') : undefined} onclick={() => void reply(thread)}>{t('app.whiteboards.comment_send')}</Button>
          </Stack>
        {/if}
      </li>
    {:else}
      <li class="quiet">{t('app.whiteboards.no_comments')}</li>
    {/each}
  </ol>
  {#if writable}
    <Stack gap="050">
      <Textarea label={t('app.whiteboards.comment_new')} rows={3} bind:value={draft} />
      <PersonPicker label={t('app.whiteboards.comment_name')} noneLabel={t('app.person_picker.choose')} bind:value={naming} />
      <p class="quiet">{pinLabel}</p>
      <Button size="sm" disabledReason={draft.trim() === '' ? t('app.whiteboards.comment_needs_words') : undefined} onclick={() => void add()}>{t('app.whiteboards.comment_send')}</Button>
    </Stack>
  {/if}
</Stack>

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .threads { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .thread { display: grid; gap: var(--sp-050); padding: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); }
  .thread.open { border-color: var(--accent-primary); }
  .thread.resolved .text { color: var(--text-subtle); }
  .comment { display: grid; gap: var(--sp-025); }
  .meta { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
  .person { color: var(--text-brand); font-weight: var(--fw-semibold); }
</style>
