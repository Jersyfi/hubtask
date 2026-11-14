<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script module lang="ts">
  /** Which kind of post is being written. */
  export type ComposerMode = 'reply' | 'note';
</script>

<script lang="ts">
  // One input for a comment, a reply and an internal note (work-surface concept §5, ET-7). Where a
  // ticket has both, two tabs - *Reply to Hannah Albers | Internal note* - and the note is yellow
  // with a lock, so the two are never confused (UC-SVC-04); the button says what it does, and
  // beside the tabs stands who will see it. Attachments, snippets through "/" and the macros sit in
  // its bar. On a phone it is docked at the foot of the screen (§7). The editor itself is the
  // caller's (`packages/editor`, E-3); without one, a plain text field.
  import type { Snippet } from 'svelte';

  import Button from './Button.svelte';
  import { FoldGate } from './foldgate.ts';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import type { IconName } from './icons/index.ts';
  import Menu from './Menu.svelte';
  import type { MenuItem } from './overlay.ts';

  interface Props {
    /** The field's name: *Comment*, *Reply to Hannah Albers*. */
    label: string;
    placeholder?: string;
    value?: string;
    /** Given, the two tabs; absent, the composer writes one kind (a comment). */
    modes?: { reply: string; note: string; label: string };
    mode?: ComposerMode;
    /** Who sees a reply: *sees: Hannah, Stadtwerke Lüneburg (shares requests)*. */
    audience?: string;
    /** Who sees a note: *only people in the desk*. */
    noteAudience?: string;
    /** *Send reply*, *Comment*. */
    sendLabel: string;
    /** `secondary` where the page's object carries the one primary act (a ticket's *Lösen*). */
    sendTone?: 'primary' | 'secondary';
    /** *Save note*. */
    noteSendLabel?: string;
    /** *Ctrl+↵ sends*. */
    hint?: string;
    onsend?: (value: string, mode: ComposerMode) => void;
    attach?: { label: string; onattach: () => void };
    /** Snippets, also reached by typing "/" at the start of a line. */
    snippets?: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void };
    /** Macros: automations with the trigger *by hand*, each saying what it does. */
    macros?: { label: string; title?: string; items: readonly MenuItem[]; onselect: (id: string) => void };
    /**
     * The formatting bar inside the frame, above the field (ET-5): bold, italic, a list, a link,
     * a mention. The caller's editor applies what is pressed; the plain field gets it as markup.
     */
    format?: { label: string; items: readonly { id: string; label: string; icon: IconName }[]; onformat: (id: string) => void };
    /** The pages' editor, in place of the plain field. */
    editor?: Snippet;
    /** Present: writing is not possible here, and this says why. */
    disabledReason?: string;
    isBusy?: boolean;
    busyLabel?: string;
  }

  let {
    label,
    placeholder,
    value = $bindable(''),
    modes,
    mode = $bindable('reply'),
    audience,
    noteAudience,
    sendLabel,
    sendTone = 'primary',
    noteSendLabel,
    hint,
    onsend,
    attach,
    snippets,
    macros,
    format,
    editor,
    disabledReason,
    isBusy = false,
    busyLabel,
  }: Props = $props();

  const uid = $props.id();
  const isNote = $derived(modes !== undefined && mode === 'note');
  const who = $derived(isNote ? noteAudience : audience);
  const isEmpty = $derived(value.trim() === '');

  let snippetMenu = $state<{ open: (index?: number) => void } | null>(null);
  // A menu of the bar takes the focus out of the composer: while one is open the bar stays, or its
  // trigger would vanish under it and close it.
  let isSnippetsOpen = $state(false);
  let isMacrosOpen = $state(false);

  // Folding waits for the press that took the focus away to click: folding at the blur would move
  // what stands below under the pointer, and the press would be lost.
  const gate = new FoldGate();
  let isOpen = $state(false);
  let root = $state<HTMLElement | undefined>();
  function folding(change: () => void) {
    change();
    isOpen = gate.isOpen;
  }

  /** Opens the insert menu at its first item, as `/` does in the plain field: the caller's editor asks for it. */
  export function openSnippets(): void {
    snippetMenu?.open(0);
  }

  function send() {
    if (isEmpty || isBusy || disabledReason !== undefined) return;
    onsend?.(value, isNote ? 'note' : 'reply');
  }

  function onTabKey(event: KeyboardEvent) {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight' && event.key !== 'Home' && event.key !== 'End') return;
    event.preventDefault();
    mode = mode === 'reply' ? 'note' : 'reply';
    const tabs = (event.currentTarget as HTMLElement).parentElement?.querySelectorAll<HTMLElement>('[role="tab"]');
    tabs?.[mode === 'reply' ? 0 : 1]?.focus();
  }
</script>

<svelte:window
  onpointerdowncapture={() => folding(() => gate.press())}
  onclickcapture={() => setTimeout(() => folding(() => gate.settle()))}
  onpointercancelcapture={() => folding(() => gate.settle())}
  onkeydowncapture={() => folding(() => gate.settle())}
/>

<div
  bind:this={root}
  class="composer"
  onfocusin={() => folding(() => gate.focusIn())}
  onfocusout={(event) => { if (!root?.contains(event.relatedTarget as Node | null)) folding(() => gate.focusOut()); }}
  data-open={isOpen ? '' : undefined}
  data-mode={isNote ? 'note' : 'reply'} data-readonly={disabledReason !== undefined ? '' : undefined} data-empty={isEmpty ? '' : undefined} data-held={isSnippetsOpen || isMacrosOpen ? '' : undefined}>
  {#if modes}
    <div class="modes">
      <div class="tabs" role="tablist" aria-label={modes.label}>
        <button type="button" role="tab" class="tab" id={`composer-${uid}-reply`} aria-selected={!isNote} aria-controls={`composer-${uid}-panel`} tabindex={isNote ? -1 : 0} onclick={() => (mode = 'reply')} onkeydown={onTabKey}>
          <Icon name="send" size="sm" />{modes.reply}
        </button>
        <button type="button" role="tab" class="tab" data-note="" id={`composer-${uid}-note`} aria-selected={isNote} aria-controls={`composer-${uid}-panel`} tabindex={isNote ? 0 : -1} onclick={() => (mode = 'note')} onkeydown={onTabKey}>
          <Icon name="lock" size="sm" />{modes.note}
        </button>
      </div>
      {#if who}<span class="who" id={`composer-${uid}-who`}>{who}</span>{/if}
    </div>
  {/if}
  <div class="panel" id={`composer-${uid}-panel`} role={modes ? 'tabpanel' : undefined} aria-labelledby={modes ? `composer-${uid}-${isNote ? 'note' : 'reply'}` : undefined}>
    {#if disabledReason !== undefined}
      <p class="refused"><Icon name="lock" size="sm" />{disabledReason}</p>
    {:else}
      {#if format}
        <div class="format" role="group" aria-label={format.label}>
          {#each format.items as item (item.id)}
            <IconButton icon={item.icon} label={item.label} tone="subtle" size="sm" onclick={() => format.onformat(item.id)} />
          {/each}
        </div>
      {/if}
      {#if editor}
        <div class="editor">{@render editor()}</div>
      {:else}
        <textarea
          class="field"
          aria-label={label}
          aria-describedby={who ? `composer-${uid}-who` : undefined}
          {placeholder}
          rows="3"
          bind:value
          onkeydown={(event) => {
            if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
              event.preventDefault();
              send();
            } else if (event.key === '/' && snippetMenu && (value === '' || value.endsWith('\n'))) {
              event.preventDefault();
              snippetMenu.open(0);
            }
          }}
        ></textarea>
      {/if}
      <div class="bar">
        {#if attach}<IconButton icon="paperclip" label={attach.label} tone="subtle" size="sm" onclick={attach.onattach} />{/if}
        {#if snippets}
          <Menu bind:this={snippetMenu} bind:isOpen={isSnippetsOpen} label={snippets.label} items={snippets.items} onselect={snippets.onselect}>
            {#snippet trigger(props)}
              <IconButton icon="file-text" label={snippets.label} tone="subtle" size="sm" {...props} />
            {/snippet}
          </Menu>
        {/if}
        {#if macros}
          <Menu bind:isOpen={isMacrosOpen} label={macros.label} items={macros.items} onselect={macros.onselect} head={macros.title ? macroTitle : undefined}>
            {#snippet trigger(props)}
              <IconButton icon="zap" label={macros.label} tone="subtle" size="sm" {...props} />
            {/snippet}
          </Menu>
        {/if}
        <span class="grow"></span>
        {#if hint}<span class="hint">{hint}</span>{/if}
        <Button tone={sendTone} size="sm" icon={isNote ? 'lock' : undefined} {isBusy} {busyLabel} onclick={send}>
          {isNote ? (noteSendLabel ?? sendLabel) : sendLabel}
        </Button>
      </div>
    {/if}
  </div>
</div>

{#snippet macroTitle()}
  <span data-menu-label class="menu-title">{macros?.title}</span>
{/snippet}

<style>
  .composer {
    display: flex;
    flex-direction: column;
    min-inline-size: 0;
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    overflow: hidden;
  }

  /* The note: the warning tint and its accent as the frame, with the lock on its tab and button. */
  .composer[data-mode='note'] { border-color: var(--status-warning-accent); background: var(--status-warning-surface); }
  .composer[data-mode='note'] .panel { background: var(--status-warning-surface); }

  .modes {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100);
    padding-inline: var(--sp-100) var(--sp-150);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .tabs { display: flex; }

  .tab {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--sp-500);
    padding: 0 var(--sp-150);
    border: 0;
    border-block-end: var(--bw-ring) solid transparent;
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-090);
    cursor: pointer;
  }

  .tab[aria-selected='true'] { border-block-end-color: var(--accent-primary); color: var(--text-primary); font-weight: var(--fw-semibold); }
  .composer[data-mode='note'] .tab[data-note][aria-selected='true'] { border-block-end-color: var(--status-warning-accent); }

  .tab:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .who { margin-inline-start: auto; color: var(--text-secondary); font-size: var(--fs-075); }

  .panel { display: flex; flex-direction: column; }

  .field {
    min-block-size: var(--sp-1000);
    padding: var(--sp-150) var(--sp-200);
    border: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-100);
    line-height: var(--lh-normal);
    /* It grows with what is written instead of offering a grip. */
    field-sizing: content;
    resize: none;
  }

  .format {
    display: flex;
    gap: var(--sp-025);
    padding: var(--sp-050) var(--sp-150);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  /* A thumb's target (§7): the bar's and the format's icons and the tabs grow to layout.target. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .bar :global(.icon-button[data-size]),
    .format :global(.icon-button[data-size]) { width: var(--layout-target); height: var(--layout-target); }
    .tab { min-block-size: var(--layout-target); }
  }

  .field::placeholder { color: var(--text-subtle); }

  .field:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: calc(-1 * var(--bw-ring));
  }

  /* The caller's editor sits where the field would: inset as the field is, three lines once in use. */
  .editor { min-inline-size: 0; padding: var(--sp-150) var(--sp-200); }
  .composer:is([data-open], [data-held]) .editor,
  .composer:not([data-empty]) .editor { min-block-size: var(--sp-1000); }

  /* Empty and not in use, it is one line to start writing in (§6.9): the formatting and the send
     come forward with the focus, so a quiet conversation does not open on an empty form. */
  .composer[data-empty]:not([data-open], [data-held]) .format,
  .composer[data-empty]:not([data-open], [data-held]) .bar { display: none; }
  .composer[data-empty]:not([data-open], [data-held]) .field { min-block-size: 0; }

  .bar {
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-100) var(--sp-150);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  .grow { flex: 1; }
  .hint { color: var(--text-subtle); font-size: var(--fs-075); margin-inline-end: var(--sp-100); }

  .refused { display: flex; align-items: center; gap: var(--sp-100); margin: 0; padding: var(--sp-200); color: var(--text-secondary); font-size: var(--fs-090); }

  .menu-title {
    color: var(--text-subtle);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  /* A phone: docked at the foot, above the bottom bar, as the reply is (§7). */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .composer {
      position: sticky;
      inset-block-end: calc(var(--layout-bottombar-height) + env(safe-area-inset-bottom, 0));
      z-index: var(--z-sticky);
      box-shadow: var(--shadow-raised);
    }
    .field { min-block-size: var(--layout-nav-touchrow); resize: none; }
    .hint { display: none; }
    .who { flex-basis: 100%; margin-inline-start: 0; padding-block-end: var(--sp-100); }
  }
</style>
