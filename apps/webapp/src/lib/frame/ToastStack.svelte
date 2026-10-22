<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where the frame's passing messages appear: the block end of the window, above the phone's
  // bottom bar and above an open save bar, on the `toast` layer that nothing covers.
  //
  // **Drawn, not announced** (`Toast`'s `isAnnounced`): `toasts.svelte.ts` says each message
  // through the application's one live region, and a second region here would say it twice.
  //
  // **Nobody is timed out of a message they are with** (WCAG 2.2.1): the pointer over it or focus
  // in it holds it. And focus never drops to the document: a message left through its own *Undo*,
  // action or *Dismiss* hands focus back to where it came from, or to what the caller named, or to
  // the main landmark.

  import { Button, Toast } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { toast } from './toasts.svelte.ts';
  import { undoRefusal, type ToastEntry } from './toasts.ts';

  interface Props {
    /** Whether the phone's bottom bar is drawn, which the stack then sits above. */
    isAboveBottomBar?: boolean;
  }

  const { isAboveBottomBar = false }: Props = $props();

  /** Where focus was before it entered each message, by its id. */
  const cameFrom = new Map<number, HTMLElement>();

  function arrived(entry: ToastEntry, event: FocusEvent) {
    if (event.relatedTarget instanceof HTMLElement && !cameFrom.has(entry.id)) cameFrom.set(entry.id, event.relatedTarget);
    toast.hold(entry.id);
  }

  function left(entry: ToastEntry, event: FocusEvent) {
    const into = event.relatedTarget;
    if (into instanceof Node && (event.currentTarget as HTMLElement).contains(into)) return;
    cameFrom.delete(entry.id);
    toast.release(entry.id);
  }

  /** Takes a message away, and focus with it back to where it belongs. */
  function close(entry: ToastEntry, element: HTMLElement | null) {
    const hadFocus = element?.contains(document.activeElement) ?? false;
    const back = cameFrom.get(entry.id);
    cameFrom.delete(entry.id);
    toast.dismiss(entry.id);
    if (!hadFocus) return;
    const target =
      (back?.isConnected ? back : undefined) ??
      (entry.returnTo ? document.querySelector<HTMLElement>(entry.returnTo) : null) ??
      document.getElementById('main');
    target?.focus();
  }

  async function undo(entry: ToastEntry, element: HTMLElement | null) {
    close(entry, element);
    await entry.undo?.();
  }

  async function act(entry: ToastEntry, element: HTMLElement | null) {
    close(entry, element);
    await entry.action?.run();
  }

  const elements = new Map<number, HTMLElement>();
  const keep = (id: number) => (node: HTMLElement) => {
    elements.set(id, node);
    return () => elements.delete(id);
  };
</script>

<div class="stack" data-above-bar={isAboveBottomBar ? '' : undefined}>
  {#each toast.entries as entry (entry.id)}
    <div
      class="slot"
      role="presentation"
      {@attach keep(entry.id)}
      onpointerenter={() => toast.hold(entry.id)}
      onpointerleave={() => {
        if (!elements.get(entry.id)?.contains(document.activeElement)) toast.release(entry.id);
      }}
      onfocusin={(event) => arrived(entry, event)}
      onfocusout={(event) => left(entry, event)}
    >
      <Toast
        tone={entry.tone ?? 'success'}
        isAnnounced={false}
        dismissLabel={t('app.dismiss')}
        onDismiss={() => close(entry, elements.get(entry.id) ?? null)}
      >
        {#snippet action()}
          {#if entry.link}
            <a class="link" href={entry.link.href}>{entry.link.label}</a>
          {/if}
          {#if entry.action}
            <Button size="sm" tone="subtle" onclick={() => void act(entry, elements.get(entry.id) ?? null)}>{entry.action.label}</Button>
          {/if}
          {#if entry.undo}
            <!-- Offline it is shown, not offered: the way back exists and says why it waits. -->
            {@const refusal = undoRefusal(live.state === 'offline')}
            <Button
              size="sm"
              tone="subtle"
              disabledReason={refusal ? t(refusal) : undefined}
              onclick={() => void undo(entry, elements.get(entry.id) ?? null)}
            >{t('app.toast.undo')}</Button>
          {/if}
        {/snippet}
        {entry.text}
      </Toast>
    </div>
  {/each}
</div>

<style>
  /* Out of the flow at every moment: a message arriving moves nothing (design-system.md §6 rule 6). */
  .stack {
    position: fixed;
    inset-block-end: var(--sp-200);
    inset-inline-end: var(--sp-200);
    z-index: var(--z-toast);
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: var(--sp-100);
    max-inline-size: calc(100vw - 2 * var(--sp-200));
    pointer-events: none;
  }

  .slot { pointer-events: auto; }

  /* Above the phone's bottom bar, and above a save bar that is open - never over either. */
  .stack[data-above-bar] { inset-block-end: calc(var(--sp-200) + var(--layout-bottombar-height)); }

  :global(html:has([data-save-bar][data-open])) .stack {
    inset-block-end: calc(var(--sp-200) + var(--layout-bottombar-height));
  }

  :global(html:has([data-save-bar][data-open])) .stack[data-above-bar] {
    inset-block-end: calc(var(--sp-200) + 2 * var(--layout-bottombar-height));
  }

  .link { font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-brand); }
</style>
