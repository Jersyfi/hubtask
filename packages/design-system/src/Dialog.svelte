<!-- SPDX-License-Identifier: Apache-2.0
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
  //
  // The one other thing it does not get right is where the focus lands. `showModal` focuses the
  // first focusable descendant, which in this layout is the dismiss control in the header - so a
  // dialog whose whole purpose is to be answered opened with the caret on its close button. Where
  // the body holds a field, the focus goes into the field instead; where it holds none, the
  // browser's own answer stands, because focusing a destructive button for somebody would be
  // worse than focusing nothing.
  //
  // ## The three sizes (page-system §5)
  //
  // `size` is the rung of the dialog ladder: `s` a confirmation or one field, `m` two to six fields,
  // `l` up to twelve in three groups or one list to pick from. A sized dialog keeps its head and
  // foot and scrolls only its body. Below `medium` an `m` or an `l` takes the whole screen - the
  // close control at the start of its bar, the title, and the primary at the end, where a thumb
  // finds it - and its foot goes, since the bar now holds both ways out. Without `size` it keeps the
  // look it had before the sizes, so no caller changes by accident; each names its rung in turn.
  //
  // `onDismiss` is asked before the close control, Escape or the backdrop close it. Answering
  // `false` keeps it open, and the caller then opens its *Discard changes?* Dialog S - the one
  // dialog allowed over another. A close by an action never asks.

  import type { Snippet } from 'svelte';

  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import { escapeHandler, focusReturn } from './focus.ts';
  import { layers } from './layers.ts';

  /** The primary of a sized dialog: drawn last in the foot, and in the bar when it is full screen. */
  export interface DialogAction {
    readonly label: string;
    readonly onclick: () => void;
    readonly tone?: 'primary' | 'danger';
    readonly disabledReason?: string;
    readonly isBusy?: boolean;
    readonly busyLabel?: string;
  }

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
    /** A dialog worked in rather than answered - capture - takes the wide measure. Before `size`. */
    isWide?: boolean;
    /** The rung of the dialog ladder (page-system §5.1). */
    size?: 's' | 'm' | 'l';
    /** The one primary, named with the dialog's verb. With `size`, the component places it. */
    primary?: DialogAction;
    /** Asked before the person closes it without answering; `false` keeps it open. */
    onDismiss?: () => boolean;
    /** The buttons that answer it - *Cancel*; the primary goes into `primary` where there is one. */
    actions?: Snippet;
    children: Snippet;
  }

  let {
    title,
    isOpen = $bindable(false),
    dismissLabel,
    onClose,
    isWide = false,
    size,
    primary,
    onDismiss,
    actions,
    children,
  }: Props = $props();

  // Svelte's own id, not a random one: unique per instance and the same on every render, so a
  // prerendered page is identical between builds and a hydrated one matches its markup.
  const instanceId = $props.id();
  const titleId = `dialog-${instanceId}`;

  let node = $state<HTMLDialogElement | null>(null);
  let body = $state<HTMLDivElement | null>(null);
  let opener: Element | null = null;

  /** The first control in the body somebody would type into, if there is one. */
  function firstField(): HTMLElement | undefined {
    const candidates = body?.querySelectorAll<HTMLElement>(
      'input:not([type="hidden"]), textarea, select, [contenteditable="true"]',
    );
    for (const candidate of candidates ?? []) {
      const disabled = candidate.matches(':disabled') || candidate.getAttribute('aria-disabled') === 'true';
      if (!disabled && candidate.offsetParent !== null) return candidate;
    }
    return undefined;
  }

  function close() {
    if (!isOpen) return;
    isOpen = false;
  }

  /** A close the person asked for without answering: the caller may want to confirm it first. */
  function dismiss(): boolean {
    if (!isOpen) return false;
    if (onDismiss && !onDismiss()) return false;
    isOpen = false;
    return true;
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
    firstField()?.focus();

    // Refused, the dismissal leaves the dialog open, so it goes back into the register - below the
    // confirmation the caller opens next, which is opened later at the same rank.
    let handle = layers.open('dialog', function onEscape() {
      if (!dismiss()) handle = layers.open('dialog', onEscape);
    });
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
      // WebKit, busy, can refuse the opener focus in the very task that closes the modal - its
      // own restore and the call above both leave it on the body. A task later it takes it; a
      // microtask is still too early, and a frame never comes in a background tab.
      const back = opener;
      setTimeout(() => {
        if (document.activeElement === document.body) focusReturn(back);
      }, 0);
    };
  });
</script>

<!-- `oncancel` is refused rather than allowed: the platform would close this dialog, and the rule
     is that `Escape` closes the topmost layer, which may be a popover opened from inside it. The
     keydown handler above has already asked the register by the time this fires.
     `close` is queued as a task after the element closes, so it can arrive after the dialog was
     opened again within that task; only an element still closed means it was closed. -->
<dialog
  class="dialog"
  data-wide={isWide && !size ? '' : undefined}
  data-size={size}
  bind:this={node}
  aria-labelledby={titleId}
  oncancel={(event) => event.preventDefault()}
  onclose={(event) => {
    if (!event.currentTarget.open) close();
  }}
  onclick={(event) => {
    if (dismissLabel !== undefined && event.target === node) dismiss();
  }}
>
  <div class="panel">
    <header class="head" data-dismissible={dismissLabel !== undefined ? '' : undefined}>
      <h2 id={titleId} class="title">{title}</h2>
      {#if primary && size}
        <span class="bar-primary">
          <Button tone={primary.tone ?? 'primary'} size="sm" onclick={primary.onclick} disabledReason={primary.disabledReason} isBusy={primary.isBusy} busyLabel={primary.busyLabel}>
            {primary.label}
          </Button>
        </span>
      {/if}
      {#if dismissLabel}
        <span class="dismiss"><IconButton icon="x" label={dismissLabel} size="sm" onclick={() => dismiss()} /></span>
      {/if}
    </header>
    <div class="body" bind:this={body}>{@render children()}</div>
    {#if actions || primary}
      <footer class="actions">
        {@render actions?.()}
        {#if primary}
          <Button tone={primary.tone ?? 'primary'} onclick={primary.onclick} disabledReason={primary.disabledReason} isBusy={primary.isBusy} busyLabel={primary.busyLabel}>
            {primary.label}
          </Button>
        {/if}
      </footer>
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
    background: var(--bg-raised);
    color: var(--text-primary);
    /* Rule 4: it grows with its content and with the text inside it, and never past the viewport. */
    width: min(60ch, 100% - var(--sp-400));
    max-height: 85vh;
    overflow: auto;
    /* Rule 1: raised off the page it covers. */
    box-shadow: var(--shadow-overlay);
    animation: open var(--motion-entrance-duration) var(--motion-entrance-easing) both;
  }

  .dialog[data-wide] { width: min(var(--layout-dialog-wide), 100% - var(--sp-400)); }

  /* ── The sizes ─────────────────────────────────────────────────────────────

     A sized dialog is a column of three: the head and the foot stay, the body between them
     scrolls. It stands at most the window less two `sp.600` margins high, and is never wider than
     the window less the same. */
  .dialog[data-size] {
    inline-size: min(var(--dialog-size), 100% - var(--sp-600) * 2);
    max-inline-size: 100%;
    max-block-size: calc(100% - var(--sp-600) * 2);
    overflow: hidden;
  }

  .dialog[data-size='s'] { --dialog-size: var(--layout-dialog-s); }
  .dialog[data-size='m'] { --dialog-size: var(--layout-dialog-m); }
  .dialog[data-size='l'] { --dialog-size: var(--layout-dialog-l); }

  .dialog[data-size][open] { display: flex; flex-direction: column; }

  .dialog[data-size] .panel {
    flex: 1 1 auto;
    min-block-size: 0;
    gap: 0;
    padding: 0;
  }

  .dialog[data-size] .head { align-items: center; gap: var(--sp-150); padding: var(--sp-300) var(--sp-300) var(--sp-150); }
  .dialog[data-size] .title { flex: 1; font-size: var(--fs-500); }
  .dialog[data-size] .body {
    flex: 1 1 auto;
    min-block-size: 0;
    overflow: auto;
    display: flex;
    flex-direction: column;
    gap: var(--sp-250);
    padding: var(--sp-100) var(--sp-300) var(--sp-200);
  }
  .dialog[data-size] .actions {
    gap: var(--sp-150);
    padding: var(--sp-200) var(--sp-300);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  .bar-primary { display: none; }
  .dismiss { display: inline-flex; }

  /* Below `medium` an M or an L is the whole screen: the close control first, the title, the
     primary at the end of the bar; the foot goes, the bar holds both ways out. An S stays a
     centred card - a confirmation is answered, not worked in. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .dialog[data-size='m'],
    .dialog[data-size='l'] {
      inline-size: 100%;
      block-size: 100%;
      max-block-size: 100%;
      margin: 0;
      border: 0;
      border-radius: 0;
    }

    .dialog[data-size='m'] .head,
    .dialog[data-size='l'] .head {
      padding: var(--sp-100) var(--sp-150);
      padding-block-start: calc(var(--sp-100) + env(safe-area-inset-top, 0));
      border-block-end: var(--bw-hairline) solid var(--border-subtle);
    }

    .dialog[data-size='m'] .title,
    .dialog[data-size='l'] .title { font-size: var(--fs-300); }

    .dialog[data-size='m'] .dismiss,
    .dialog[data-size='l'] .dismiss { order: -1; }

    .dialog[data-size='m'] .bar-primary,
    .dialog[data-size='l'] .bar-primary { display: inline-flex; }

    .dialog[data-size='m'] .head[data-dismissible] ~ .actions,
    .dialog[data-size='l'] .head[data-dismissible] ~ .actions { display: none; }

    .dialog[data-size] .body { padding: var(--sp-200); }

    .dialog[data-size='s'] { inline-size: calc(100% - var(--sp-300) * 2); }
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
