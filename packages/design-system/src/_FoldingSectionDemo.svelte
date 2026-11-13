<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import FoldingSection from './FoldingSection.svelte';
  import StateLozenge from './StateLozenge.svelte';

  interface Props {
    /** A prefix for the section ids, so two demos on one page keep theirs apart. */
    prefix?: string;
    mode?: 'all' | 'short';
  }

  const { prefix = 'fold', mode = 'all' }: Props = $props();

  const LINKS = [
    ['TAMA-17', 'Passkey sign-in: recovery through a second device', 'In progress', 'info'],
    ['TBPA-1', 'Rate limiting per API key with a clear Retry-After header', 'Done', 'success'],
    ['TBPA-2', 'Webhooks: retry deliveries on 5xx with backoff', 'In progress', 'info'],
    ['TAMA-1', 'As a customer I want to sign in with a passkey', 'In progress', 'info'],
    ['SA-8', 'Since the update the customer portal no longer opens in Firefox', 'New', 'neutral'],
    ['TAMA-16', 'Checkout button hidden behind the keyboard', 'Done', 'success'],
    ['TAMA-10', 'Offline mode: orders are queued', 'To do', 'neutral'],
  ] as const;
  const COMMENTS = Array.from({ length: 13 }, (_, index) => `Comment ${index + 1}: the merchant certificate is in; the domain check waited on the CDN.`);
</script>

<div class="column">
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
        {#each LINKS.slice(shown.start, shown.end) as [key, title, state, tone] (key)}
          <li><span class="key">{key}</span><span class="title">{title}</span><StateLozenge label={state} tone={tone} /></li>
        {/each}
      </ul>
    {/snippet}
  </FoldingSection>

  <FoldingSection id={`${prefix}-conversation`} title="Conversation" count={String(COMMENTS.length)} fold={{ kind: 'rows', total: COMMENTS.length, limit: 3, from: 'end' }} showAllLabel={`${COMMENTS.length - 3} older comments`} showLessLabel="Only the newest three">
    {#snippet children(shown)}
      <ul class="rows">
        {#each COMMENTS.slice(shown.start, shown.end) as comment (comment)}
          <li>{comment}</li>
        {/each}
      </ul>
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
  .title { flex: 1; min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
