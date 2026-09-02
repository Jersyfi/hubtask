<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Rule 5's panel. It lists the tab order of the first pane and can step focus through it, so a
  // reader compares the list against the layout instead of trusting that the two agree.
  import { tabOrder, walk, type Stop } from '../lib/focus-walk.ts';

  interface Props {
    /** The panes on stage. Only the first is walked: four panes would fight over focus. */
    hosts: readonly HTMLElement[];
  }

  const { hosts }: Props = $props();

  let stops = $state<Stop[]>([]);
  let current = $state<Stop | null>(null);
  let stop: (() => void) | undefined;

  function inspect() {
    const host = hosts[0];
    stops = host ? tabOrder(host) : [];
    current = null;
  }

  function run() {
    stop?.();
    inspect();
    if (stops.length === 0) return;
    stop = walk(stops, (step) => (current = step));
  }

  // Re-read whenever the stage hands up a new set of hosts: a new render invalidates the order
  // and any walk through it.
  $effect(() => {
    void hosts;
    stop?.();
    inspect();
  });

  // Reads nothing, so it runs once and its cleanup runs on unmount. Putting the cancellation in
  // the effect above instead would cancel a running walk every time the stage re-renders - which
  // it does not, but the next axis will be the one that does.
  $effect(() => () => stop?.());
</script>

<section class="panel" aria-label="Keyboard order">
  <header>
    <h2>Keyboard order</h2>
    <button type="button" onclick={run} disabled={stops.length === 0}>Walk the focus</button>
    <button type="button" onclick={inspect}>Re-read</button>
  </header>

  {#if stops.length === 0}
    <p class="empty">Nothing in this story can take focus.</p>
  {:else}
    <ol>
      {#each stops as item (item.index)}
        <li class:current={current?.index === item.index}>
          <span class="index">{item.index}</span>
          <span class="label">{item.label}</span>
          {#if item.positiveTabIndex}
            <span class="flag">positive tabindex</span>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}

  <p class="caveat">
    <strong>Start the walk from the keyboard</strong> — focus <kbd>Walk the focus</kbd> and press
    <kbd>Enter</kbd>. Begun with a mouse the browser stays in pointer mode and
    <code>:focus-visible</code> does not match, so the ring rule 5 is about will not appear.
  </p>
  <p class="caveat">
    The order is read from the DOM: a synthetic <kbd>Tab</kbd> moves focus in no browser. This
    finds an order that disagrees with the layout. It does not find a focus trap — that needs a
    driven browser, which is F5's decision (ADR-0037).
  </p>
</section>

<style>
  .panel {
    padding: var(--sp-200) var(--sp-300);
    border-top: 1px solid var(--border-subtle);
    background: var(--bg-surface);
  }

  header {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
  }

  h2 {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    font-weight: var(--fw-text);
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-subtle);
  }

  button {
    padding: var(--sp-050) var(--sp-150);
    border: 1px solid var(--border-default);
    border-radius: var(--r-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  button:hover:not(:disabled) {
    background: var(--bg-surface-hover);
  }

  button:disabled {
    color: var(--text-subtle);
    cursor: not-allowed;
  }

  ol {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-100);
    margin: var(--sp-150) 0 0;
    padding: 0;
    list-style: none;
  }

  li {
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-025) var(--sp-100);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-sm);
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }

  li.current {
    border-color: var(--accent-primary);
    background: var(--accent-primary-subtle);
    color: var(--text-brand);
  }

  .index {
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    color: var(--text-subtle);
  }

  /* Rule 3 again: the flag is a word before it is a colour. */
  .flag {
    padding: 0 var(--sp-050);
    border-radius: var(--r-sm);
    background: var(--label-red-bg);
    color: var(--label-red-fg);
    font-family: var(--font-mono);
    font-size: var(--fs-050);
  }

  .empty,
  .caveat {
    margin: var(--sp-150) 0 0;
    color: var(--text-subtle);
    font-size: var(--fs-075);
    max-width: 80ch;
  }

  kbd,
  code {
    font-family: var(--font-mono);
  }
</style>
