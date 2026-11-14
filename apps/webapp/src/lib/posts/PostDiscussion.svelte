<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The discussion under a post (B-4, UC-DOC-18): write, reply once, four fixed reactions. A page
  // has only comments at the text; a post has both, under two names.

  import { Avatar, Button, Composer, Menu, VisuallyHidden } from '@hubtask/design-system/components';
  import type { Post, PostReply, PostReplyList } from '@hubtask/sync-engine';
  import { untrack } from 'svelte';

  import { accounts } from '../data/accounts.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { REACTIONS, postRepliesPath, posts } from '../data/posts.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatRelative } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    post: Post;
  }

  const { post }: Props = $props();

  const read = untrack(() => resource<PostReplyList>(postRepliesPath(post.page_id)));
  const replies = $derived(read.state.status === 'ready' ? read.state.data.data : []);
  $effect(() => accounts.resolve(replies.map((reply) => reply.author_id)));
  const nameOf = (id: string | null) => (id ? accounts.nameOf(id) : undefined) ?? t('app.people.unnamed');

  let isNewestFirst = $state(true);
  const firsts = $derived(
    replies
      .filter((reply) => !reply.parent_id)
      .sort((a, b) => (isNewestFirst ? b.created_at.localeCompare(a.created_at) : a.created_at.localeCompare(b.created_at))),
  );
  const answersTo = (id: string) => replies.filter((reply) => reply.parent_id === id).sort((a, b) => a.created_at.localeCompare(b.created_at));
  const live = $derived(replies.filter((reply) => !reply.deleted).length);

  let draft = $state('');
  let answerTo = $state<string | undefined>(undefined);
  let answer = $state('');
  let isBusy = $state(false);

  async function act(run: () => Promise<unknown>): Promise<boolean> {
    isBusy = true;
    try {
      await run();
      await read.refresh();
      return true;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return false;
    } finally {
      isBusy = false;
    }
  }

  async function send(body: string, parent?: string) {
    if (body.trim() === '') return;
    if (await act(() => posts.reply(post, { body: body.trim(), parent_id: parent ?? null }))) {
      if (parent) { answer = ''; answerTo = undefined; } else draft = '';
    }
  }

  const mark = (id: string) => REACTIONS.find((each) => each.id === id)?.mark ?? '';
</script>

{#snippet one(reply: PostReply, isAnswer: boolean)}
  <article class="reply" class:answer={isAnswer} data-post-reply={reply.id}>
    <Avatar name={nameOf(reply.author_id)} size={isAnswer ? 'sm' : 'md'} />
    <div class="words">
      <span class="who"><strong>{nameOf(reply.author_id)}</strong> · {formatRelative(reply.created_at, messages.locale)}</span>
      {#if reply.deleted}
        <span class="gone">{t('app.posts.reply_taken_back')}</span>
      {:else}
        <span class="body">{reply.body}</span>
        <span class="acts">
          {#each reply.reactions.filter((each) => each.count > 0) as each (each.reaction)}
            <button type="button" class="react" aria-pressed={each.mine} data-post-reaction={each.reaction}
              onclick={() => void act(() => posts.react(post, reply.id, each.reaction, !each.mine))}>
              <span aria-hidden="true">{mark(each.reaction)}</span> {each.count}<VisuallyHidden>{t(`app.posts.reaction_${each.reaction}`)}</VisuallyHidden>
            </button>
          {/each}
          <Menu label={t('app.posts.react')} items={REACTIONS.map((each) => ({ id: each.id, label: `${each.mark} ${t(`app.posts.reaction_${each.id}`)}` }))}
            onselect={(id) => void act(() => posts.react(post, reply.id, id, !reply.reactions.some((each) => each.reaction === id && each.mine)))}>
            {#snippet trigger(props)}
              <button type="button" class="react add" aria-label={t('app.posts.react')} data-post-react={reply.id} {...props}>+<span aria-hidden="true">🙂</span></button>
            {/snippet}
          </Menu>
          {#if !isAnswer}
            <button type="button" class="link" data-post-answer={reply.id} onclick={() => { answerTo = reply.id; answer = ''; }}>{t('app.posts.answer')}</button>
          {/if}
          {#if reply.author_id === actor.account?.id || post.may_write}
            <button type="button" class="link" data-post-take-back={reply.id} onclick={() => void act(() => posts.takeBack(post, reply.id))}>{t('app.posts.take_back')}</button>
          {/if}
        </span>
      {/if}
      {#if !isAnswer}
        {#each answersTo(reply.id) as each (each.id)}{@render one(each, true)}{/each}
        {#if answerTo === reply.id}
          <div class="answering">
            <Composer label={t('app.posts.answer_to', { name: nameOf(reply.author_id) })} bind:value={answer} sendLabel={t('app.posts.send')}
              {isBusy} onsend={(value) => void send(value, reply.id)} />
            <Button size="sm" tone="subtle" onclick={() => (answerTo = undefined)}>{t('app.posts.cancel')}</Button>
          </div>
        {/if}
      {/if}
    </div>
  </article>
{/snippet}

<section class="discussion" aria-labelledby="post-discussion" data-post-discussion>
  <div class="head">
    <h2 id="post-discussion">{t('app.posts.discussion', { count: String(live) })}</h2>
    <span class="order" role="radiogroup" aria-label={t('app.posts.order')}>
      <button type="button" role="radio" aria-checked={isNewestFirst} onclick={() => (isNewestFirst = true)}>{t('app.posts.newest_first')}</button>
      <button type="button" role="radio" aria-checked={!isNewestFirst} onclick={() => (isNewestFirst = false)}>{t('app.posts.oldest_first')}</button>
    </span>
  </div>
  <Composer label={t('app.posts.say_something')} placeholder={t('app.posts.say_something_hint')} bind:value={draft} sendLabel={t('app.posts.send')}
    {isBusy} onsend={(value) => void send(value)} />
  <div class="replies">
    {#each firsts as reply (reply.id)}{@render one(reply, false)}{/each}
  </div>
</section>

<style>
  .discussion { margin-block-start: var(--sp-500); padding-block-start: var(--sp-300); border-block-start: var(--bw-hairline) solid var(--border-subtle); display: flex; flex-direction: column; gap: var(--sp-200); }
  .head { display: flex; align-items: baseline; gap: var(--sp-200); }
  h2 { flex: 1; margin: 0; font: var(--fw-semibold) var(--fs-300) / 1.3 var(--font-display); }
  .order { display: inline-flex; gap: var(--sp-050); font-size: var(--fs-075); }
  .order button, .link { border: none; background: none; padding: 0; color: var(--text-subtle); font: inherit; font-size: var(--fs-075); cursor: pointer; }
  .order button[aria-checked='true'] { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .link { color: var(--text-brand); }
  .order button:focus-visible, .link:focus-visible, .react:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .replies { display: flex; flex-direction: column; gap: var(--sp-300); }
  .reply { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: var(--sp-150); }
  .reply.answer { margin-block-start: var(--sp-150); }
  .words { display: flex; flex-direction: column; gap: var(--sp-050); min-inline-size: 0; }
  .who { font-size: var(--fs-075); color: var(--text-secondary); }
  .who strong { color: var(--text-primary); }
  .body { white-space: pre-wrap; overflow-wrap: anywhere; }
  .gone { color: var(--text-subtle); font-style: italic; }
  .acts { display: flex; align-items: center; gap: var(--sp-150); flex-wrap: wrap; }
  .react {
    display: inline-flex; align-items: center; gap: var(--sp-050); padding: 0 var(--sp-100); min-block-size: var(--sp-300);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-full); background: var(--bg-surface); color: var(--text-primary);
    font: inherit; font-size: var(--fs-075); cursor: pointer;
  }
  .react[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary-subtle); }
  .react.add { color: var(--text-subtle); }
  .answering { display: flex; flex-direction: column; gap: var(--sp-100); margin-block-start: var(--sp-150); align-items: flex-start; }
</style>
