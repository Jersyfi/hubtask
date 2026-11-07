<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The bottom sheet of a phone: a menu, a picker or the filters of a list, risen over the page
  // (page-system §4.4, §6). It opens at most half the height, so the page stays in sight; the
  // grabber pulls it to full height, and a pull down, the close control or Escape puts it away.
  //
  // A native modal `<dialog>` for Dialog's reasons - the top layer, a real focus trap, `inert`
  // behind it - on the `overlay` rank of layers.ts, so a popover opened from inside it closes first
  // and a dialog opened over it closes before it. A list of actions is `items`: one row each, the
  // destructive one in the danger colour and never the first focused.
  //
  // It has two heights and no third: a sheet sized to the pixel is a drawer (Drawer, `block-end`),
  // and the two heights travel as a `data-` attribute rather than as a style (ADR-0028).

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import { escapeHandler, focusReturn } from './focus.ts';
  import { layers } from './layers.ts';
  import type { MenuItem } from './overlay.ts';

  interface Props {
    /** What the sheet holds, as a heading. Resolved text (ADR-0011). */
    title: string;
    isOpen?: boolean;
    /** The close control's name. A sheet is always dismissible. */
    dismissLabel: string;
    /** The grabber's name: it pulls the sheet to full height and back. */
    grabLabel: string;
    onClose?: () => void;
    /** A list of actions instead of a body: one row each, closing the sheet before it runs. */
    items?: readonly MenuItem[];
    onselect?: (id: string) => void;
    /** The buttons at its foot - *Clear* and *Show 9 people*. */
    actions?: Snippet;
    children?: Snippet;
  }

  let {
    title,
    isOpen = $bindable(false),
    dismissLabel,
    grabLabel,
    onClose,
    items,
    onselect,
    actions,
    children,
  }: Props = $props();

  const uid = $props.id();
  const titleId = `sheet-${uid}`;

  let node = $state<HTMLDialogElement | null>(null);
  let isFull = $state(false);
  let opener: Element | null = null;

  /** Where a pull began and how far it has gone; a quarter of the sheet decides. */
  let pullFrom: number | null = null;
  let pulled = 0;

  function close() {
    if (!isOpen) return;
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
    if (pulled < -reach) isFull = true;
    else if (pulled > reach) {
      if (isFull) isFull = false;
      else close();
    }
  }

  function byKey(event: KeyboardEvent) {
    if (event.key === 'ArrowUp') isFull = true;
    else if (event.key === 'ArrowDown') isFull = false;
    else return;
    event.preventDefault();
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
    isFull = false;
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
  data-full={isFull ? '' : undefined}
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
    <button
      type="button"
      class="grabber"
      aria-label={grabLabel}
      aria-expanded={isFull}
      onclick={() => (isFull = !isFull)}
      onpointerdown={grab}
      onpointermove={pull}
      onpointerup={letGo}
      onpointercancel={letGo}
      onkeydown={byKey}
    >
      <span class="bar"></span>
    </button>
    <header class="head">
      <h2 class="title" id={titleId}>{title}</h2>
      <IconButton icon="x" label={dismissLabel} size="sm" onclick={() => close()} />
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
  /* No `z-index`: the top layer is above every stacking context (Dialog says why). */
  .sheet {
    padding: 0;
    border: 0;
    inline-size: 100%;
    max-inline-size: 100%;
    margin: auto 0 0;
    max-block-size: 50%;
    border-start-start-radius: var(--r-xl);
    border-start-end-radius: var(--r-xl);
    background: var(--bg-raised);
    color: var(--text-primary);
    box-shadow: var(--shadow-overlay);
    overflow: hidden;
    animation: rise var(--motion-entrance-duration) var(--motion-entrance-easing) both;
  }

  .sheet[open] { display: flex; flex-direction: column; }

  .sheet[data-full] { max-block-size: calc(100% - var(--layout-appbar-height)); block-size: calc(100% - var(--layout-appbar-height)); }

  .sheet::backdrop { background: var(--bg-scrim); }

  .panel {
    display: flex;
    flex-direction: column;
    gap: var(--sp-150);
    flex: 1 1 auto;
    min-block-size: 0;
    padding: var(--sp-100) var(--sp-250) calc(var(--sp-300) + env(safe-area-inset-bottom, 0));
    box-sizing: border-box;
  }

  .grabber {
    align-self: center;
    display: grid;
    place-items: center;
    padding: var(--sp-050) var(--sp-200);
    margin: 0;
    border: 0;
    border-radius: var(--r-sm);
    background: none;
    cursor: ns-resize;
    touch-action: none;
  }

  .grabber .bar {
    display: block;
    inline-size: var(--sp-500);
    block-size: var(--sp-050);
    border-radius: var(--r-full);
    background: var(--border-strong);
  }

  .grabber:focus-visible,
  .item:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .head { display: flex; align-items: center; gap: var(--sp-150); }

  .title {
    flex: 1;
    min-width: 0;
    margin: 0;
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
  }

  .body {
    flex: 1 1 auto;
    min-block-size: 0;
    overflow: auto;
    display: flex;
    flex-direction: column;
    gap: var(--sp-200);
    font-size: var(--fs-100);
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

  .actions { display: flex; gap: var(--sp-150); }
  .actions :global(> :last-child) { flex: 1; }

  @keyframes rise {
    from { opacity: 0; transform: translateY(100%); }
    to { opacity: 1; transform: none; }
  }

  @media (prefers-reduced-motion: reduce) {
    .sheet { animation: none; }
  }

  :global([data-motion='reduced']) .sheet { animation: none; }
</style>
