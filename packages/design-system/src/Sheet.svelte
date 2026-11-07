<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A bottom sheet on compact (design-system.md §4, wave 5; the navigation concept's §9): the
  // view bar's views, the context sheet of a long press, *New*, the account. A modal dialog
  // rising from the bottom, with a grabber, a heading and *Close* - every sheet has both - over a
  // scrim; `Escape` and a press on the scrim close it, and the focus goes back where it was.
  import type { Snippet } from 'svelte';

  import IconButton from './IconButton.svelte';
  import { escapeHandler, focusReturn } from './focus.ts';
  import { layers } from './layers.ts';

  interface Props {
    /** The heading and the dialog's name: "Views", "Create under Team". */
    title: string;
    closeLabel: string;
    isOpen?: boolean;
    onClose?: () => void;
    children: Snippet;
  }

  let { title, closeLabel, isOpen = $bindable(false), onClose, children }: Props = $props();

  const instanceId = $props.id();
  const titleId = `sheet-${instanceId}`;
  let node = $state<HTMLDialogElement | null>(null);
  let opener: Element | null = null;

  function close() {
    isOpen = false;
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
    <span class="grabber" aria-hidden="true"></span>
    <header class="head">
      <h2 class="title" id={titleId}>{title}</h2>
      <IconButton icon="x" label={closeLabel} size="sm" tone="subtle" onclick={() => close()} />
    </header>
    <div class="body">{@render children()}</div>
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
    overflow: auto;
    animation: rise var(--motion-entrance-duration) var(--motion-entrance-easing) both;
  }

  @keyframes rise {
    from { transform: translateY(100%); }
    to { transform: none; }
  }

  .sheet::backdrop { background: var(--bg-scrim); }

  .panel {
    display: flex;
    flex-direction: column;
    padding: var(--sp-100) var(--sp-150) calc(var(--sp-250) + env(safe-area-inset-bottom, 0));
  }

  .grabber {
    display: block;
    inline-size: var(--layout-sheet-grabber);
    block-size: var(--layout-sheet-grabberheight);
    margin: 0 auto calc(var(--sp-100) + var(--sp-025));
    border-radius: var(--r-full);
    background: var(--border-strong);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-100);
    margin: 0 var(--sp-050) var(--sp-100);
  }

  .title {
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

  @media (prefers-reduced-motion: reduce) {
    .sheet { animation: none; }
  }

  :global([data-motion='reduced']) .sheet { animation: none; }
</style>
