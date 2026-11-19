<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import CommentThread, { type ThreadComment } from './CommentThread.svelte';
  import FoldingSection from './FoldingSection.svelte';
  import IconButton from './IconButton.svelte';
  import StateLozenge from './StateLozenge.svelte';
  import Tabs from './Tabs.svelte';
  import ComposerDemo from './_ComposerDemo.svelte';

  interface Props {
    /** A prefix for the section ids, so two demos on one page keep theirs apart. */
    prefix?: string;
    mode?: 'all' | 'short';
  }

  const { prefix = 'fold', mode = 'all' }: Props = $props();

  // Grouped by relation, as the entry's mockup draws them (ET-3): the order is the groups'.
  const LINKS = [
    ['Blocked by', 'TBPA-1', 'Rate limiting per API key with a clear Retry-After header', 'Done', 'success'],
    ['Blocked by', 'TBPA-2', 'Webhooks: retry deliveries on 5xx with backoff', 'In progress', 'info'],
    ['Relates to', 'TAMA-17', 'Passkey sign-in: recovery through a second device', 'In progress', 'info'],
    ['Relates to', 'TAMA-1', 'As a customer I want to sign in with a passkey', 'In progress', 'info'],
    ['Relates to', 'TAMA-16', 'Checkout button hidden behind the keyboard', 'Done', 'success'],
    ['Relates to', 'TAMA-10', 'Offline mode: orders are queued', 'To do', 'neutral'],
    ['Mentioned in', 'SA-8', 'Since the update the customer portal no longer opens in Firefox', 'New', 'neutral'],
  ] as const;
  const countOf = (relation: string) => LINKS.filter((link) => link[0] === relation).length;

  const PEOPLE = ['Amelia Hughes', 'Mehmet Yılmaz', 'Clara Neumann'];
  const SAID = [
    'The merchant certificate is in; the domain check waited on the CDN.',
    'Google Pay works on the test devices. Apple Pay still needs the domain file.',
    'Can we show the wallet button only when a card is set up? Otherwise it is a dead end.',
  ];
  // Newest first; the older ones load at the foot.
  const COMMENTS: ThreadComment[] = SAID.map((body, index) => ({
    id: `c${index}`,
    authorName: PEOPLE[index % PEOPLE.length]!,
    body,
    when: `${index + 1} h ago`,
    at: `2026-11-12T${String(10 - index).padStart(2, '0')}:00:00Z`,
    isChangeable: index === 0,
    replies: index === 1 ? [{ id: 'c1r', authorName: 'Jana Krüger', body: 'Domain file is uploaded.', when: '50 min ago', at: '2026-11-12T10:10:00Z' }] : undefined,
  }));
  const HISTORY = ['Mehmet moved it to In progress · 2 h ago', 'Amelia set the due date to 27 Nov · yesterday', 'Clara linked TBPA-1 · 3 Nov'];
  let conversation = $state('all');
</script>

<div class="column">
  <FoldingSection id={`${prefix}-files`} title="Files" count="0" emptyLabel="Nothing attached yet.">
    {#snippet actions()}<Button tone="secondary" size="sm" icon="plus">Attach</Button>{/snippet}
  </FoldingSection>
  <FoldingSection id={`${prefix}-description`} title="Description" fold={{ kind: 'lines', lines: 12 }} showAllLabel="Show more · Background, Out of scope, Open questions, Risks" showLessLabel="Show less">
    {#snippet actions()}<Button tone="subtle" size="sm" icon="pencil">Edit</Button>{/snippet}
    {#snippet children()}
      <div class="text">
        <h3>Goal</h3>
        <p>Customers pay in the app’s checkout with Apple Pay (iOS) and Google Pay (Android), without typing card details. Today <b>31 %</b> of mobile purchases end at the payment step.</p>
        {#if mode === 'all'}
          <h3>Acceptance criteria</h3>
          <ol>
            <li>The wallet button appears only when the device has a wallet with at least one card.</li>
            <li>The amount in the system dialog matches the basket, shipping and discount included.</li>
            <li>A refused payment shows an understandable message.</li>
            <li>Accessibility: VoiceOver and TalkBack name the button correctly.</li>
          </ol>
          <h3>Background</h3>
          <p>The payment provider supports both wallets through one token endpoint; the backend accepts the token and authorises it like a card.</p>
          <h3>Out of scope</h3>
          <p>Wallets in the web shop; recurring payments; <a href="#fold">the loyalty card</a>.</p>
        {/if}
      </div>
    {/snippet}
  </FoldingSection>

  <FoldingSection id={`${prefix}-links`} title="Links" count={String(LINKS.length)} fold={{ kind: 'rows', total: LINKS.length, limit: 5 }} showAllLabel={`Show all ${LINKS.length}`} showLessLabel="Show the first 5">
    {#snippet actions()}<Button tone="subtle" size="sm" icon="link">Link</Button>{/snippet}
    {#snippet children(shown)}
      <ul class="rows">
        {#each LINKS.slice(shown.start, shown.end) as [relation, key, title, state, tone], index (key)}
          {#if index === 0 || LINKS[shown.start + index - 1]?.[0] !== relation}
            <li class="relation" aria-hidden="true">{relation}<span class="count">{countOf(relation)}</span></li>
          {/if}
          <li><span class="key">{key}</span><span class="title">{title}</span><StateLozenge label={state} tone={tone} /><IconButton icon="ellipsis" label={`More for ${key}`} tone="subtle" size="sm" /></li>
        {/each}
      </ul>
    {/snippet}
  </FoldingSection>

  <FoldingSection id={`${prefix}-conversation`} title="Conversation" count="21">
    {#snippet children()}
      <Tabs label="Show in the conversation" tabs={[{ id: 'all', label: 'All', count: 21 }, { id: 'comments', label: 'Comments', count: 13 }, { id: 'history', label: 'History', count: 8 }]} bind:selected={conversation} />
      <ComposerDemo mode="comment" />
      {#if conversation === 'history'}
        <ul class="rows history">{#each HISTORY as line (line)}<li>{line}</li>{/each}</ul>
      {:else}
        <CommentThread
          label="Comments on TAMA-18"
          comments={COMMENTS}
          emptyLabel="No comments yet."
          removedLabel="Comment removed"
          editLabel="Edit"
          deleteLabel="Remove"
          moreLabel="More for this comment"
          replyLabel="Reply"
          onReply={() => {}}
          hasMore
          loadMoreLabel="10 older"
          onLoadMore={() => {}}
        />
        {#if conversation === 'all'}<ul class="rows history">{#each HISTORY.slice(0, 1) as line (line)}<li>{line}</li>{/each}</ul>{/if}
      {/if}
    {/snippet}
  </FoldingSection>
</div>

<style>
  .column { min-inline-size: 0; }
  .text { font-size: var(--fs-100); line-height: var(--lh-normal); }
  .text h3 { margin: var(--sp-100) 0 var(--sp-050); font-family: var(--font-display); font-size: var(--fs-200); }
  .text p, .text ol { margin: 0 0 var(--sp-100); }
  .text a { color: var(--text-brand); }
  .text a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .rows { display: flex; flex-direction: column; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; font-size: var(--fs-090); }
  .rows li { display: flex; align-items: center; gap: var(--sp-100); min-block-size: var(--sp-400); }
  .key { color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--fs-075); }
  .relation { min-block-size: auto; padding-block-start: var(--sp-100); color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .relation .count { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-weight: var(--fw-regular); }
  .history { color: var(--text-subtle); font-size: var(--fs-075); }
  .title { flex: 1; min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
