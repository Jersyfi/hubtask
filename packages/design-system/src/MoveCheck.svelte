<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  /** One line of the pre-check, worded by the caller from a `MoveFinding`'s code and params. */
  export interface MoveCheckItem {
    readonly id: string;
    /** `ok` is a check that passed, `obstacle` holds *Move* back, `warning` only says what follows. */
    readonly tone: 'ok' | 'obstacle' | 'warning';
    readonly text: string;
    /** The one step that resolves an obstacle: "Lift the type into the workspace". */
    readonly way?: { readonly label: string; readonly onselect: () => void };
    /** What follows the step: "or choose a destination that has it." */
    readonly rest?: string;
  }
</script>

<script lang="ts">
  // The move pre-check's list (design-system.md §4, wave 5; the navigation concept's §5.3): each
  // check with its mark - passed, an obstacle with the one step that resolves it, a warning that
  // is no obstacle. The mark is a shape as well as a colour (rule 3), and the words say it
  // without either. A live region, so a re-check after a step is heard. *Move* itself is the
  // dialog's: it is enabled once `obstacles` is zero, which the caller reads from the same list.
  import Icon from './Icon.svelte';

  interface Props {
    /** What the list is called: "Before moving". */
    label: string;
    items: readonly MoveCheckItem[];
    /** The words of each mark for a screen reader: "Passed", "Obstacle", "Warning". */
    toneLabels: { readonly ok: string; readonly obstacle: string; readonly warning: string };
  }

  const { label, items, toneLabels }: Props = $props();
</script>

<ul class="checks" aria-label={label} aria-live="polite">
  {#each items as item (item.id)}
    <li class="check" data-tone={item.tone}>
      <span class="mark" title={toneLabels[item.tone]}>
        {#if item.tone === 'ok'}
          <Icon name="check" size="sm" label={toneLabels.ok} />
        {:else if item.tone === 'obstacle'}
          <Icon name="x" size="sm" label={toneLabels.obstacle} />
        {:else}
          <span class="bang" role="img" aria-label={toneLabels.warning}>!</span>
        {/if}
      </span>
      <span class="words">
        {#if item.tone === 'obstacle'}<strong>{item.text}</strong>{:else}{item.text}{/if}
        {#if item.way}
          <button type="button" class="way" onclick={item.way.onselect}>{item.way.label}</button>
        {/if}
        {#if item.rest}{item.rest}{/if}
      </span>
    </li>
  {/each}
</ul>

<style>
  .checks {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .check {
    display: flex;
    align-items: flex-start;
    gap: var(--sp-100);
    margin: calc(var(--sp-050) + var(--sp-025)) 0;
    color: var(--text-secondary);
    font-size: var(--fs-090);
  }

  .check strong { color: var(--text-primary); font-weight: var(--fw-semibold); }

  .mark {
    display: grid;
    place-items: center;
    flex: none;
    inline-size: calc(var(--sp-200) + var(--sp-025));
    block-size: calc(var(--sp-200) + var(--sp-025));
    border-radius: var(--r-full);
  }

  .mark :global(.icon) {
    inline-size: var(--layout-nav-twistglyph);
    block-size: var(--layout-nav-twistglyph);
  }

  [data-tone='ok'] .mark { background: var(--status-success-surface); color: var(--status-success-text); }
  [data-tone='obstacle'] .mark { background: var(--status-danger-surface); color: var(--status-danger-text); }
  [data-tone='warning'] .mark { background: var(--status-warning-surface); color: var(--status-warning-text); }

  .bang { font-size: var(--fs-075); font-weight: var(--fw-bold); line-height: 1; }

  .way {
    display: inline;
    margin: 0;
    padding: 0;
    border: 0;
    background: none;
    color: var(--text-brand);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .way:hover { text-decoration: underline; }

  .way:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }
</style>
