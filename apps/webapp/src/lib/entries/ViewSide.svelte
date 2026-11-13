<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The panel *Ansicht* from `expanded` (§6.2, scene viewcfg): a side pane beside the page, not
  // over it - no scrim, no focus trap, the page narrowed and usable while each change shows at
  // once. `Escape` reaches it through the register as the weakest layer and gives focus back to
  // where it was opened from. A collection's panel and the plan's are this one pane.
  import type { Snippet } from 'svelte';
  import { tick } from 'svelte';
  import { Icon, IconButton, handleEscape, layers } from '@hubtask/design-system/components';

  interface Props {
    isOpen?: boolean;
    title: string;
    dismissLabel: string;
    /** The acts beside the title (*Wie die Vorlage*). */
    actions?: Snippet;
    children: Snippet;
  }

  let { isOpen = $bindable(false), title, dismissLabel, actions, children }: Props = $props();

  const titleId = $props.id();
  let heading = $state<HTMLElement | undefined>(undefined);
  /** Where focus was when the pane opened: the opener, as a rule. */
  let cameFrom: HTMLElement | undefined;
  function close() {
    isOpen = false;
    const back = cameFrom?.isConnected ? cameFrom : document.querySelector<HTMLElement>('[data-opener="view"]');
    back?.focus();
  }
  $effect(() => {
    if (!isOpen) return;
    const active = document.activeElement;
    cameFrom = active instanceof HTMLElement && active !== document.body ? active : undefined;
    void tick().then(() => heading?.focus());
    const handle = layers.open('overlay', close);
    // The same guard as every overlay's: the first handler to act marks the key, the rest stand down.
    const onKeydown = (event: KeyboardEvent) => {
      if (!event.defaultPrevented && handleEscape(event.key)) event.preventDefault();
    };
    document.addEventListener('keydown', onKeydown);
    return () => {
      document.removeEventListener('keydown', onKeydown);
      handle.release();
    };
  });
</script>

{#if isOpen}
  <aside class="side" aria-labelledby={titleId} data-view-side>
    <header class="side-head">
      <h2 id={titleId} tabindex="-1" bind:this={heading}><Icon name="sliders-horizontal" size="sm" />{title}</h2>
      {@render actions?.()}
      <IconButton icon="x" size="sm" label={dismissLabel} onclick={close} />
    </header>
    <div class="side-body">{@render children()}</div>
  </aside>
{/if}

<style>
  /* Beside the page, from the bar down to the window's end, scrolling on its own. The negative
     margins take back the main region's padding so its edge meets the window's, as the bar's does. */
  .side {
    box-sizing: border-box;
    position: sticky;
    inset-block-start: var(--layout-appbar-height);
    align-self: flex-start;
    flex: none;
    inline-size: var(--layout-pane-width);
    max-inline-size: 45%;
    block-size: calc(100vh - var(--layout-appbar-height));
    margin-block: calc(-1 * var(--sp-300));
    margin-inline-end: calc(-1 * var(--sp-300));
    margin-inline-start: var(--sp-300);
    overflow-y: auto;
    overscroll-behavior: contain;
    background: var(--bg-surface);
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
  }
  .side-head { position: sticky; inset-block-start: 0; display: flex; align-items: center; gap: var(--sp-100); padding: var(--sp-200) var(--sp-300); background: var(--bg-surface); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .side-head h2 { flex: 1; display: flex; align-items: center; gap: var(--sp-100); margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .side-head h2:focus { outline: none; }
  .side-body { padding: var(--sp-200) var(--sp-300) var(--sp-300); }
</style>
