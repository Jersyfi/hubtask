<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script module lang="ts">
  import type { IconName } from './icons/index.ts';
  import type { ProductId } from './ProductTile.svelte';

  /** The object's key with its product's tile: `SA-12` on the desk's, `TAMA-18` on the brand's. */
  export interface ObjectKey {
    readonly label: string;
    readonly icon: IconName;
    /** The product the object belongs to; absent, the tile takes `color` (a label token or `brand`). */
    readonly product?: ProductId;
    readonly color?: string;
  }

  /** The one chip of the action bar that says what is breached or due next (ET-6). */
  export interface DeadlineChip {
    readonly label: string;
    readonly tone: 'neutral' | 'warning' | 'danger';
  }
</script>

<script lang="ts">
  // One object, one shape (WB-1): entry, ticket, configuration item, goal, release, agent and
  // portfolio share this head - the key with its product tile, the type, the title edited in place,
  // and under it the action bar: status, next step, person, due date, one deadline chip, ⋯. It
  // spans the full width; only under it does a page split into main column and rail (§6.9).
  import type { Snippet } from 'svelte';

  import Avatar from './Avatar.svelte';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import Menu from './Menu.svelte';
  import ProductTile from './ProductTile.svelte';
  import StateLozenge from './StateLozenge.svelte';
  import type { MenuItem } from './overlay.ts';

  interface Props {
    objectKey: ObjectKey;
    /** The entry type or kind beside the key: *Story*, *Incident*, *Application service*. */
    type?: { label: string; icon?: IconName };
    /** One quiet line after the type: *· Backlog · target 4.3.0 · created 3 Nov by Clara*. */
    meta?: string;
    title: string;
    /** The title field's name while it is edited. */
    titleLabel: string;
    /** *Edit title*: given with `onrename`, the title is edited in place. */
    editTitleLabel?: string;
    onrename?: (title: string) => void;
    /** The `StatusButton`, or a `StateLozenge` where the state is not the reader's to change. */
    status?: Snippet;
    /** The next step, the head's one primary act. */
    next?: { label: string; icon?: IconName; onclick: () => void };
    /** A second step the workflow offers beside it: *Wait for Hannah*. */
    secondary?: { label: string; onclick: () => void };
    /** Who has it; `emptyLabel` when nobody does, said, never left blank (P-05). */
    person?: { name: string; src?: string } | { emptyLabel: string };
    due?: { label: string; tone?: 'neutral' | 'warning' | 'danger' };
    deadline?: DeadlineChip;
    /** Quiet facts at the end of the bar: priority, points, *1 of 3 done*. */
    facts?: Snippet;
    /** The breadcrumb, above the key (hidden on a phone, where the bar carries the way back). */
    trail?: Snippet;
    /** Tools at the end of the top row: pane or page, ⋯. */
    tools?: Snippet;
    /** Tools at the end of the action bar: star, copy link. */
    end?: Snippet;
    menu?: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void; opener?: string };
    /** The head as a card's - in a pane, a preview, a list of objects: the smaller title, no divider. */
    isCompact?: boolean;
    /** Completed: the title is struck through, as on its row (UC-WRK-04 check 2). */
    isDone?: boolean;
    /** An object without a status sheet (goal, release, agent, CI): its next step and ⋯ stay on a phone. */
    hasActsOnPhone?: boolean;
  }

  const {
    objectKey,
    type,
    meta,
    title,
    titleLabel,
    editTitleLabel,
    onrename,
    status,
    next,
    secondary,
    person,
    due,
    deadline,
    facts,
    trail,
    tools,
    end,
    menu,
    isCompact = false,
    hasActsOnPhone = false,
    isDone = false,
  }: Props = $props();

  let isEditing = $state(false);
  let draft = $state('');
  let field = $state<HTMLInputElement | null>(null);
  let editButton = $state<HTMLElement | null>(null);

  const canEdit = $derived(onrename !== undefined && editTitleLabel !== undefined);

  function startEditing() {
    if (!canEdit) return;
    draft = title;
    isEditing = true;
  }

  function finish(isSaved: boolean) {
    if (!isEditing) return;
    isEditing = false;
    const next = draft.trim();
    if (isSaved && next !== '' && next !== title) onrename?.(next);
    queueMicrotask(() => editButton?.querySelector('button')?.focus());
  }

  // The divider joins the steps to the facts; when the bar wraps between them it would end or
  // start a line alone, so it is hidden there - kept in the layout, so hiding it moves nothing.
  let bar = $state<HTMLElement | null>(null);
  let divider = $state<HTMLElement | null>(null);
  let isDividerAlone = $state(false);
  $effect(() => {
    if (!bar || !divider || typeof ResizeObserver === 'undefined') return;
    const measure = () => {
      const mark = divider;
      if (!mark) return;
      const before = mark.previousElementSibling as HTMLElement | null;
      const after = mark.nextElementSibling as HTMLElement | null;
      const middle = (each: HTMLElement) => each.offsetTop + each.offsetHeight / 2;
      const line = middle(mark);
      const isOff = (each: HTMLElement | null) => each !== null && Math.abs(middle(each) - line) > mark.offsetHeight;
      isDividerAlone = before === null || isOff(before) || isOff(after);
    };
    const observer = new ResizeObserver(measure);
    observer.observe(bar);
    measure();
    return () => observer.disconnect();
  });

  $effect(() => {
    if (isEditing && field) {
      field.focus();
      field.select();
    }
  });
</script>

{#snippet ends()}
  {#if tools}<div class="tools">{@render tools()}</div>{/if}
  {#if menu}
    <Menu label={menu.label} items={menu.items} placement={{ side: 'block-end', align: 'end' }} onselect={menu.onselect}>
      {#snippet trigger(props)}
        <IconButton icon="ellipsis" label={menu.label} tone="secondary" size="sm" data-opener={menu.opener} {...props} />
      {/snippet}
    </Menu>
  {/if}
{/snippet}

<header class="object-head" data-compact={isCompact ? '' : undefined} data-phone-acts={hasActsOnPhone ? '' : undefined}>
  <!-- A compact head without a trail spends no row on its tools: they end the title's line. -->
  {#if trail || ((tools || menu) && !isCompact)}
    <div class="top">
      <div class="trail">{#if trail}{@render trail()}{/if}</div>
      {@render ends()}
    </div>
  {/if}

  <div class="key-row">
    <span class="key"><ProductTile icon={objectKey.icon} product={objectKey.product} color={objectKey.color ?? 'brand'} size="sm" />{objectKey.label}</span>
    {#if type}<StateLozenge label={type.label} icon={type.icon} />{/if}
    {#if meta}<span class="meta">{meta}</span>{/if}
  </div>

  <div class="title-row">
    {#if isEditing}
      <input
        class="title-field"
        aria-label={titleLabel}
        bind:this={field}
        bind:value={draft}
        onkeydown={(event) => {
          if (event.key === 'Enter') finish(true);
          else if (event.key === 'Escape') {
            event.stopPropagation();
            finish(false);
          } else return;
          event.preventDefault();
        }}
        onblur={() => finish(true)}
      />
    {:else}
      <!-- A compact head stands inside a page that has its own h1 (a pane, a preview). -->
      <svelte:element this={isCompact ? 'h2' : 'h1'} class="title">
        {#if canEdit}
          <!-- The words take a pointer's press too; the keyboard's way in is the pencil beside them. -->
          <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
          <span class="title-text" data-editable="" data-done={isDone ? '' : undefined} onclick={startEditing}>{title}</span>
        {:else}
          <span class="title-text" data-done={isDone ? '' : undefined}>{title}</span>
        {/if}
      </svelte:element>
      {#if canEdit && editTitleLabel}
        <span class="edit" bind:this={editButton}><IconButton icon="pencil" label={editTitleLabel} tone="subtle" size="sm" onclick={startEditing} /></span>
      {/if}
    {/if}
    {#if isCompact && !trail && (tools || menu)}<span class="title-ends">{@render ends()}</span>{/if}
  </div>

  <div class="action-bar" bind:this={bar}>
    {#if status}{@render status()}{/if}
    {#if next}<span class="step"><Button tone="primary" size="sm" icon={next.icon} onclick={next.onclick}>{next.label}</Button></span>{/if}
    {#if secondary}<span class="step"><Button tone="secondary" size="sm" onclick={secondary.onclick}>{secondary.label}</Button></span>{/if}
    {#if person || due || deadline || facts}<span class="divider" aria-hidden="true" bind:this={divider} data-alone={isDividerAlone ? '' : undefined}></span>{/if}
    {#if person}
      {#if 'name' in person}
        <span class="person"><Avatar name={person.name} src={person.src} size="sm" /><span class="person-name">{person.name}</span></span>
      {:else}
        <span class="person" data-empty=""><Icon name="user" size="sm" />{person.emptyLabel}</span>
      {/if}
    {/if}
    {#if due}<span class="chip" data-tone={due.tone ?? 'neutral'}><Icon name="calendar" size="sm" />{due.label}</span>{/if}
    {#if deadline}
      <span class="chip" data-tone={deadline.tone}><Icon name={deadline.tone === 'danger' ? 'triangle-alert' : 'clock'} size="sm" />{deadline.label}</span>
    {/if}
    {#if facts}<span class="facts">{@render facts()}</span>{/if}
    {#if end}<span class="end">{@render end()}</span>{/if}
  </div>
</header>

<style>
  .object-head { display: flex; flex-direction: column; gap: var(--sp-100); min-inline-size: 0; }

  .top { display: flex; align-items: center; gap: var(--sp-100); margin-block-end: var(--sp-050); }
  .trail { flex: 1; min-inline-size: 0; }
  .tools { display: inline-flex; align-items: center; gap: var(--sp-100); }

  .key-row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); min-inline-size: 0; }

  .key {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    white-space: nowrap;
  }

  .meta { color: var(--text-subtle); font-size: var(--fs-075); }
  /* The dot that joins the meta to the key is drawn here, so a meta that wraps on a phone starts
     its own line with words, not with the dot. */
  .meta::before { content: '·'; margin-inline-end: var(--sp-050); }

  .title-row { display: flex; align-items: flex-start; gap: var(--sp-050); }
  .title-ends { display: inline-flex; align-self: center; align-items: center; gap: var(--sp-100); margin-inline-start: auto; }

  /* Set as a page head's title (H-5): one face, size and weight for every head. */
  .title {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-450);
    font-weight: var(--fw-bold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
  }

  .object-head[data-compact] .title,
  .object-head[data-compact] .title-field { font-size: var(--fs-200); }
  .object-head[data-compact] .divider { display: none; }

  .title-text[data-done] { text-decoration: line-through; color: var(--text-secondary); }
  .title-text[data-editable] { cursor: text; border-radius: var(--r-sm); }
  .title-text[data-editable]:hover { background: var(--bg-surface-hover); }

  /* The pencil comes forward with the pointer or the focus; on touch, where there is no hover, it stays. */
  .edit { opacity: 0; transition: opacity var(--motion-state-duration) var(--motion-state-easing); }
  .title-row:hover .edit,
  .edit:focus-within { opacity: 1; }

  @media (hover: none) {
    .edit { opacity: 1; }
  }

  .title-field {
    flex: 1;
    min-inline-size: 0;
    padding: 0 var(--sp-050);
    border: var(--bw-hairline) solid var(--border-strong);
    border-radius: var(--r-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--fs-450);
    font-weight: var(--fw-bold);
    line-height: var(--lh-tight);
  }

  .title-field:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .action-bar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  /* One type size in the bar: the steps at the status button's 13 px semibold, the next step
     lifted by the primary's glow (the grammar mockup). */
  .step :global(.button[data-tone='primary']) { box-shadow: var(--shadow-glow); }

  .divider {
    inline-size: var(--bw-hairline);
    block-size: var(--sp-250);
    margin-inline: var(--sp-050);
    background: var(--border-default);
  }
  .divider[data-alone] { visibility: hidden; }

  .person { display: inline-flex; align-items: center; gap: calc(var(--sp-050) + var(--sp-025)); font-size: var(--fs-075); }
  .person[data-empty] { color: var(--text-subtle); }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--bw-hairline) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-sm);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    line-height: var(--lh-normal);
    white-space: nowrap;
  }

  .chip[data-tone='warning'] {
    border-color: var(--status-warning-border);
    background: var(--status-warning-surface);
    color: var(--status-warning-text);
  }

  .chip[data-tone='danger'] {
    border-color: var(--status-danger-border);
    background: var(--status-danger-surface);
    color: var(--status-danger-text);
  }

  .facts { display: inline-flex; align-items: center; gap: var(--sp-150); color: var(--text-secondary); font-size: var(--fs-075); }
  .end { display: inline-flex; align-items: center; gap: var(--sp-100); margin-inline-start: auto; }

  /* A phone: the bar carries the way back and the tools; the steps are in the status's sheet (WB-8). */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .meta { flex-basis: 100%; }
    .meta::before { content: none; }
    .top,
    .step,
    .divider,
    .end,
    .person-name { display: none; }
    .title { font-size: var(--fs-450); }
    .object-head[data-phone-acts] .top { display: flex; }
    .object-head[data-phone-acts] .step { display: inline-flex; }
  }

  @media (prefers-reduced-motion: reduce) {
    .edit { transition: none; }
  }

  :global([data-motion='reduced']) .edit { transition: none; }
</style>
