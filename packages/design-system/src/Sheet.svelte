<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A bottom sheet on compact (design-system.md §4, wave 5; the navigation concept's §9): the
  // view bar's views, the context sheet of a long press, *New*, the account. A modal dialog
  // rising from the bottom, with a grabber, a heading and *Close* - every sheet has both - over a
  // scrim; `Escape` and a press on the scrim close it, and the focus goes back where it was.
  //
  // A list of actions is `items` (page-system §4.4): one row each, the destructive one in the
  // danger colour and never the first focused.
  //
  // **One height for every sheet**: as tall as what it holds, at most 85 % of the screen; beyond
  // that its body scrolls under a head that stays. Every sheet has its grabber, and pulling it down
  // closes the sheet; with `grabLabel` the grabber is also a button that closes it.
  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import type { IconName } from './icons/index.ts';
  import { escapeHandler, focusReturn } from './focus.ts';
  import { layers } from './layers.ts';
  import type { MenuItem } from './overlay.ts';

  interface Props {
    /** The heading and the dialog's name: "Views", "Create under Team". */
    title: string;
    closeLabel: string;
    /** The mark of the thing the sheet is about, before its heading: a long press's row. */
    icon?: IconName;
    isOpen?: boolean;
    onClose?: () => void;
    /** The grabber's name; given, the grabber is a button that closes the sheet as a pull does. */
    grabLabel?: string;
    /** A list of actions instead of a body: one row each, closing the sheet before it runs. */
    items?: readonly MenuItem[];
    onselect?: (id: string) => void;
    /** The buttons at its foot - *Clear* and *Show 9 people*. */
    actions?: Snippet;
    children?: Snippet;
  }

  let {
    title,
    closeLabel,
    icon,
    isOpen = $bindable(false),
    onClose,
    grabLabel,
    items,
    onselect,
    actions,
    children,
  }: Props = $props();

  const instanceId = $props.id();
  const titleId = `sheet-${instanceId}`;
  let node = $state<HTMLDialogElement | null>(null);
  let opener: Element | null = null;

  /** Where a pull began and how far it has gone; a quarter of the sheet decides. */
  let pullFrom: number | null = null;
  let pulled = 0;
  /** A pull ends in a click on the grabber; that click is the pull's, not a press of its own. */
  let wasPulled = false;

  function close() {
    isOpen = false;
  }

  function grab(event: PointerEvent) {
    pullFrom = event.clientY;
    pulled = 0;
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  }

  function pull(event: PointerEvent) {
    if (pullFrom !== null) pulled = event.clientY - pullFrom;
  }

  function letGo(event: PointerEvent) {
    if (pullFrom === null) return;
    (event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId);
    pullFrom = null;
    const reach = (node?.getBoundingClientRect().height ?? 0) / 4;
    wasPulled = pulled !== 0;
    if (pulled > reach) close();
    pulled = 0;
  }

  function choose(item: MenuItem) {
    if (item.disabledReason !== undefined) return;
    close();
    onselect?.(item.id);
  }

  $effect(() => {
    const sheet = node;
    if (!sheet) return;
    if (!isOpen) {
      if (sheet.open) sheet.close();
      return;
    }
    opener = document.activeElement;
    if (!sheet.open) sheet.showModal();
    const handle = layers.open('overlay', close);
    const onKeydown = escapeHandler();
    document.addEventListener('keydown', onKeydown);
    return () => {
      document.removeEventListener('keydown', onKeydown);
      handle.release();
      if (sheet.open) sheet.close();
      focusReturn(opener);
      onClose?.();
    };
  });
</script>

<dialog
  class="sheet"
  bind:this={node}
  aria-labelledby={titleId}
  oncancel={(event) => event.preventDefault()}
  onclose={(event) => {
    if (!event.currentTarget.open) close();
  }}
  onclick={(event) => {
    if (event.target === node) close();
  }}
>
  <div class="panel">
    {#if grabLabel !== undefined}
      <button
        type="button"
        class="grabber"
        aria-label={grabLabel}
        onclick={() => {
          if (wasPulled) wasPulled = false;
          else close();
        }}
        onpointerdown={grab}
        onpointermove={pull}
        onpointerup={letGo}
        onpointercancel={letGo}
      >
        <span class="grip"></span>
      </button>
    {:else}
      <span class="grabber" aria-hidden="true" onpointerdown={grab} onpointermove={pull} onpointerup={letGo} onpointercancel={letGo}><span class="grip"></span></span>
    {/if}
    <header class="head">
      <h2 class="title" id={titleId}>{#if icon}<Icon name={icon} size="sm" />{/if}{title}</h2>
      <!-- A thumb's target (§9: 44 px): the regular size, not the small one. -->
      <IconButton icon="x" label={closeLabel} tone="subtle" onclick={() => close()} />
    </header>
    <div class="body">
      {#if items}
        <ul class="items">
          {#each items as item (item.id)}
            <li>
              <button
                type="button"
                class="item"
                data-destructive={item.isDestructive ? '' : undefined}
                data-separated={item.hasSeparatorBefore ? '' : undefined}
                aria-disabled={item.disabledReason !== undefined ? 'true' : undefined}
                aria-describedby={item.disabledReason !== undefined ? `${titleId}-${item.id}` : undefined}
                onclick={() => choose(item)}
              >
                {#if item.icon}<Icon name={item.icon} size="sm" />{/if}
                <span class="item-text">
                  <span>{item.label}</span>
                  {#if item.disabledReason !== undefined}
                    <span class="reason" id={`${titleId}-${item.id}`}>{item.disabledReason}</span>
                  {/if}
                </span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
      {@render children?.()}
    </div>
    {#if actions}<footer class="actions">{@render actions()}</footer>{/if}
  </div>
</dialog>

<style>
  /* In the top layer as a modal dialog: no z-index, the overlay rank decides what Escape reaches. */
  .sheet {
    inline-size: 100%;
    max-inline-size: 100%;
    max-block-size: 85%;
    margin: auto 0 0;
    padding: 0;
    border: 0;
    border-start-start-radius: var(--layout-sheet-radius);
    border-start-end-radius: var(--layout-sheet-radius);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
    color: var(--text-primary);
    overflow: hidden;
    animation: rise var(--motion-entrance-duration) var(--motion-entrance-easing) both;
  }

  @keyframes rise {
    from { transform: translateY(100%); }
    to { transform: none; }
  }

  /* The head and the foot stay; only the body scrolls once the sheet is at its most. */
  .sheet[open] { display: flex; flex-direction: column; }
  .panel { flex: 1 1 auto; min-block-size: 0; box-sizing: border-box; }
  .body { flex: 1 1 auto; min-block-size: 0; overflow: auto; }

  .sheet::backdrop { background: var(--bg-scrim); }

  .panel {
    display: flex;
    flex-direction: column;
    padding: var(--sp-100) var(--sp-150) calc(var(--sp-250) + env(safe-area-inset-bottom, 0));
  }

  /* The whole strip above the head is the grabber, so a thumb finds it anywhere along the edge. */
  .grabber {
    align-self: stretch;
    display: grid;
    place-items: center;
    padding: var(--sp-050) var(--sp-200) var(--sp-100);
    margin: 0;
    border: 0;
    border-radius: var(--r-sm);
    background: none;
    cursor: grab;
    touch-action: none;
  }

  .grabber .grip {
    display: block;
    inline-size: var(--layout-sheet-grabber);
    block-size: var(--layout-sheet-grabberheight);
    border-radius: var(--r-full);
    background: var(--border-strong);
  }

  .grabber:focus-visible,
  .item:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-100);
    margin: 0 var(--sp-050) var(--sp-100);
  }

  .title {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    margin: 0;
    font-family: var(--font-ui);
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
  }

  .body :global(ul) {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  /* The sheet is the surface: its rows stand plain, without a card or a hairline between them,
     and a second list is set off by one hairline (the navigation concept's phone screens). */
  .body :global(.row[data-size='touch']) {
    border-block-end: 0;
    border-radius: var(--layout-nav-radius);
  }

  .body :global(ul:has(> .row) + ul:has(> .row)) {
    margin-block-start: var(--sp-050);
    padding-block-start: var(--sp-050);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  .items { margin: 0; padding: 0; list-style: none; }

  .item {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
    inline-size: 100%;
    min-block-size: var(--density-control-md-min);
    padding: var(--sp-150) 0;
    border: 0;
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    background: none;
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-200);
    text-align: start;
    cursor: pointer;
  }

  .item[data-separated] { border-block-start-color: var(--border-default); }
  .item[data-destructive] { color: var(--text-danger); }
  .item[aria-disabled='true'] { color: var(--text-subtle); cursor: not-allowed; }
  .item-text { display: flex; flex-direction: column; gap: var(--sp-025); }
  .reason { font-size: var(--fs-075); color: var(--text-secondary); }

  .actions { display: flex; gap: var(--sp-150); margin-block-start: var(--sp-150); }
  .actions :global(> :last-child) { flex: 1; }

  @media (prefers-reduced-motion: reduce) {
    .sheet { animation: none; }
  }

  :global([data-motion='reduced']) .sheet { animation: none; }
</style>
