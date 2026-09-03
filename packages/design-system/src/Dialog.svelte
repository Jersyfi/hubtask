<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one overlay that is a claim: nothing else matters until this is answered.
  //
  // It is a native `<dialog>` opened with `showModal`, and that is a decision rather than a
  // shortcut. The browser gives four things a hand-written modal has to reimplement and usually
  // reimplements badly - the top layer, so no `z-index` can lose to a stacking context; a real
  // focus trap; `inert` on everything behind it; and a backdrop that is not an element anybody can
  // click through. Writing those by hand is how a modal ends up half-trapping focus.
  //
  // What the browser does *not* do correctly for this project is `Escape`, and that is the only
  // place this component takes over. The platform closes the topmost dialog; design-system.md §6
  // says `Escape` closes the topmost *layer*, which may be a popover opened from inside this
  // dialog. So `cancel` is refused and the register decides - which is exactly what
  // `layers.ts` was written before this component to make possible.

  import type { Snippet } from 'svelte';

  import IconButton from './IconButton.svelte';
  import { escapeHandler, focusReturn } from './focus.ts';
  import { layers } from './layers.ts';

  interface Props {
    /** The question or the subject, as a heading. Resolved text (ADR-0011). */
    title: string;
    isOpen?: boolean;
    /**
     * The name of the close control. Present means the dialog can be abandoned without answering -
     * so it also gets a backdrop that closes on a click. Absent means the actions are the way out.
     */
    dismissLabel?: string;
    /** Ran when it closes, however it closed. */
    onClose?: () => void;
    /** The buttons that answer it. The primary one is the caller's to name and to place last. */
    actions?: Snippet;
    children: Snippet;
  }

  let { title, isOpen = $bindable(false), dismissLabel, onClose, actions, children }: Props = $props();

  const titleId = `dialog-${Math.random().toString(36).slice(2, 9)}`;

  let node = $state<HTMLDialogElement | null>(null);
  let opener: Element | null = null;

  function close() {
    if (!isOpen) return;
    isOpen = false;
  }

  $effect(() => {
    const dialog = node;
    if (!dialog) return;

    if (!isOpen) {
      if (dialog.open) dialog.close();
      return;
    }

    opener = document.activeElement;
    if (!dialog.open) dialog.showModal();

    const handle = layers.open('dialog', close);
    const onKeydown = escapeHandler();
    document.addEventListener('keydown', onKeydown);

    return () => {
      document.removeEventListener('keydown', onKeydown);
      handle.release();
      if (dialog.open) dialog.close();
      // The browser restores focus on close as well, and does it correctly. This asks anyway,
      // because the one case it cannot answer is the ordinary one here: the trigger removed by the
      // very action the dialog confirmed. `false` means focus is on the body, which is where the
      // browser would have left it too.
      focusReturn(opener);
      onClose?.();
    };
  });
</script>

<!-- `oncancel` is refused rather than allowed: the platform would close this dialog, and the rule
     is that `Escape` closes the topmost layer, which may be a popover opened from inside it. The
     keydown handler above has already asked the register by the time this fires. -->
<dialog
  class="dialog"
  bind:this={node}
  aria-labelledby={titleId}
  oncancel={(event) => event.preventDefault()}
  onclose={() => close()}
  onclick={(event) => {
    if (dismissLabel !== undefined && event.target === node) close();
  }}
>
  <div class="panel">
    <header class="head">
      <h2 id={titleId} class="title">{title}</h2>
      {#if dismissLabel}
        <IconButton icon="x" label={dismissLabel} size="sm" onclick={() => close()} />
      {/if}
    </header>
    <div class="body">{@render children()}</div>
    {#if actions}
      <footer class="actions">{@render actions()}</footer>
    {/if}
  </div>
</dialog>

<style>
  /* No `z-index`: a modal `<dialog>` is in the top layer, which is above every stacking context
     there is. The `dialog` rank in tokens.json still decides what `Escape` reaches (layers.ts) -
     what paints over what and what a key reaches are not the same question. */
  .dialog {
    padding: 0;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-xl);
    background: var(--bg-surface);
    color: var(--text-primary);
    /* Rule 4: it grows with its content and with the text inside it, and never past the viewport. */
    width: min(60ch, 100% - var(--sp-400));
    max-height: 85vh;
    overflow: auto;
    /* Rule 1: raised off the page it covers. */
    box-shadow: var(--shadow-overlay);
    animation: open var(--motion-entrance-duration) var(--motion-entrance-easing) both;
  }

  .dialog::backdrop {
    background: var(--bg-scrim);
  }

  .panel {
    display: flex;
    flex-direction: column;
    gap: var(--sp-200);
    padding: var(--sp-300);
    text-align: start;
  }

  .head {
    display: flex;
    align-items: start;
    justify-content: space-between;
    gap: var(--sp-200);
  }

  .title {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
  }

  .body { font-size: var(--fs-100); }

  /* The buttons wrap rather than being squeezed: rule 4, and the case is German. */
  .actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--sp-100);
  }

  @keyframes open {
    from { opacity: 0; transform: scale(0.98); }
    to { opacity: 1; transform: none; }
  }

  @media (prefers-reduced-motion: reduce) {
    .dialog { animation: none; }
  }

  :global([data-motion='reduced']) .dialog { animation: none; }
</style>
