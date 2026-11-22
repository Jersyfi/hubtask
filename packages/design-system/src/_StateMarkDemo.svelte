<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import StateLozenge from './StateLozenge.svelte';
  import StateMark from './StateMark.svelte';

  interface Props {
    mode?: 'forms' | 'rows' | 'unavailable';
  }

  const { mode = 'forms' }: Props = $props();

  let isDone = $state(false);
  let at = $state<'review' | 'approved' | 'sent'>('review');
  const AT = { review: 'In review', approved: 'Approved', sent: 'Sent' } as const;
</script>

{#if mode === 'forms'}
  <div class="line">
    <StateMark label={isDone ? 'Mark “Book the electrician” as not done' : 'Mark “Book the electrician” as done'} checked={isDone} onclick={() => { isDone = !isDone; }} />
    <span>{isDone ? 'Done' : 'Open'} - one press completes, another reopens</span>
  </div>
  <div class="line">
    <StateMark
      form={at === 'review' ? 'step' : 'check'}
      checked={at === 'sent'}
      label={at === 'review' ? 'Choose the paint: Move on – Approve' : at === 'approved' ? 'Mark “Choose the paint” as done' : 'Mark “Choose the paint” as not done'}
      onclick={() => { at = at === 'review' ? 'approved' : at === 'approved' ? 'sent' : 'review'; }}
    />
    <StateLozenge vocabulary="category" state={at === 'sent' ? 'done' : 'progress'} label={AT[at]} />
    <span>a step that does not complete is an arrow; the one into done is the tick</span>
  </div>
{:else if mode === 'rows'}
  <ul class="rows">
    <li><StateMark label="Mark “Order the tiles” as done" /><span class="title">Order the tiles</span></li>
    <li><StateMark form="step" label="Choose the paint: Move on – Approve" /><StateLozenge vocabulary="category" state="progress" label="In review" /><span class="title">Choose the paint</span></li>
    <li><StateMark label="Mark “Measure the worktop” as done" /><StateLozenge vocabulary="category" state="todo" label="Draft" /><span class="title">Measure the worktop</span></li>
    <li><StateMark checked label="Mark “Clear the room” as not done" /><StateLozenge vocabulary="category" state="done" label="Sent" /><span class="title done">Clear the room</span></li>
  </ul>
{:else}
  <div class="line"><StateMark label="Mark “Clear the room” as done" disabledReason="The collection is archived." /><span>read only</span></div>
  <div class="line"><StateMark form="step" label="Choose the paint: Move on – Approve" disabledReason="Steps need a connection." /><span>a step offline</span></div>
  <div class="line"><StateMark checked label="Mark “Clear the room” as not done" disabledReason="No step leads on from “Sent”." /><span>no step out of done</span></div>
  <div class="line"><StateMark label="Mark “Order the tiles” as done" isBusy busyLabel="Completing" /><span>busy</span></div>
{/if}

<style>
  .line { display: flex; align-items: center; gap: var(--sp-150); margin-block-end: var(--sp-200); color: var(--text-secondary); font-size: var(--fs-090); }
  .rows { display: grid; margin: 0; padding: 0; list-style: none; }
  .rows li { display: flex; align-items: center; gap: var(--sp-150); min-block-size: var(--sp-500); padding: var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .title { color: var(--text-primary); }
  .title.done { color: var(--text-subtle); text-decoration: line-through; }
</style>
