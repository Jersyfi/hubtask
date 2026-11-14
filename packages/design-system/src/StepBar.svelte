<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A path of steps one object walks (IN-2): an inventory object's stage, *Planned › Ordered › In
  // stock › In use …*. The steps before the current one are done and carry a check, the current
  // one is filled, the later ones are quiet - the glyph and the fill say it, never the colour alone
  // (UX-7). It only shows where the object stands; changing it is the caller's act beside it. Below
  // `medium` the bar scrolls sideways, the current step brought into view.
  import Icon from './Icon.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';

  interface Step {
    readonly id: string;
    /** The step's word, resolved: *In use*. */
    readonly label: string;
  }

  interface Props {
    /** The list's name: *Stage of Portal-API*. */
    label: string;
    steps: readonly Step[];
    /** The step the object stands on; one that is no step marks none. */
    current: string;
    /** Said after a done step's word to a screen reader: *done*. */
    doneLabel: string;
  }

  const { label, steps, current, doneLabel }: Props = $props();

  const at = $derived(steps.findIndex((step) => step.id === current));
  const stateOf = (index: number) => (at < 0 || index > at ? 'later' : index < at ? 'done' : 'current');

  let list = $state<HTMLOListElement | null>(null);
  // Only where the bar overflows: a bar that fits moves nothing, and the page never scrolls for it.
  $effect(() => {
    void current;
    const node = list;
    if (!node || node.scrollWidth <= node.clientWidth) return;
    node.querySelector('[aria-current="step"]')?.scrollIntoView({ block: 'nearest', inline: 'center' });
  });
</script>

<ol class="steps" aria-label={label} bind:this={list}>
  {#each steps as step, index (step.id)}
    {@const state = stateOf(index)}
    <li class="step" data-state={state} aria-current={state === 'current' ? 'step' : undefined}>
      {#if state === 'done'}<Icon name="check" size="sm" />{/if}
      <span class="word">{step.label}</span>
      {#if state === 'done'}<VisuallyHidden>{doneLabel}</VisuallyHidden>{/if}
    </li>
  {/each}
</ol>

<style>
  .steps {
    display: flex;
    align-items: stretch;
    gap: var(--bw-thick);
    min-inline-size: 0;
    max-inline-size: 100%;
    margin: 0;
    padding: 0;
    list-style: none;
    overflow-x: auto;
    scrollbar-width: none;
  }

  /* Each step is an arrow pointing to the next; the first has no notch, the last no point. */
  .step {
    --notch: var(--sp-100);
    display: inline-flex;
    flex: none;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: calc(var(--sp-300) + var(--sp-050));
    padding-block: 0;
    padding-inline: calc(var(--notch) + var(--sp-100)) calc(var(--notch) + var(--sp-050));
    background: var(--bg-surface-sunken);
    color: var(--text-subtle);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    line-height: var(--lh-snug);
    white-space: nowrap;
    clip-path: polygon(0 0, calc(100% - var(--notch)) 0, 100% 50%, calc(100% - var(--notch)) 100%, 0 100%, var(--notch) 50%);
  }

  .step:first-child {
    padding-inline-start: var(--sp-100);
    border-start-start-radius: var(--r-sm);
    border-end-start-radius: var(--r-sm);
    clip-path: polygon(0 0, calc(100% - var(--notch)) 0, 100% 50%, calc(100% - var(--notch)) 100%, 0 100%);
  }

  .step:last-child {
    padding-inline-end: var(--sp-100);
    clip-path: polygon(0 0, 100% 0, 100% 100%, 0 100%, var(--notch) 50%);
  }

  .step:only-child { clip-path: none; }

  /* A polygon is drawn in physical space: right to left, every arrow is mirrored. */
  :global([dir='rtl']) .step { clip-path: polygon(100% 0, var(--notch) 0, 0 50%, var(--notch) 100%, 100% 100%, calc(100% - var(--notch)) 50%); }
  :global([dir='rtl']) .step:first-child { clip-path: polygon(100% 0, var(--notch) 0, 0 50%, var(--notch) 100%, 100% 100%); }
  :global([dir='rtl']) .step:last-child { clip-path: polygon(100% 0, 0 0, 0 100%, 100% 100%, calc(100% - var(--notch)) 50%); }
  :global([dir='rtl']) .step:only-child { clip-path: none; }

  .step[data-state='done'] {
    background: var(--status-success-surface);
    color: var(--status-success-text);
  }

  .step[data-state='current'] {
    background: var(--accent-primary);
    color: var(--text-inverse);
    font-weight: var(--fw-semibold);
  }

  .word { overflow: hidden; text-overflow: ellipsis; }
</style>
