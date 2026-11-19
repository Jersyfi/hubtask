<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One row of an entry's details (ADR-0061 decision 4): the field's name, what it holds or the
  // word "add", and behind it the editor the product already has - `DuePanel`, `ReminderPanel`,
  // the field renderer - opened on request rather than standing open twelve times down a page.
  //
  // The editor arrives in a `Popover` from `medium` up and in a `Drawer` from the bottom on a
  // phone. Width decides, never the platform, and the caller's panel is the same component in
  // both: no editor is built again here, this only says where it opens. The row is a button
  // named by the field, so a reader hears "Due, add" or "Due, Friday" and presses.
  //
  // The label (§3's `label` row: small, semibold, subtle) names the field; the value is the
  // reader's text and is not a message code.

  import type { Snippet } from 'svelte';

  import { Drawer, Icon, Popover } from '@hubtask/design-system/components';

  import { viewport } from '../frame/viewport.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The field's name. Resolved text (ADR-0011). */
    label: string;
    /** What it holds, as words; absent means nothing is set and the row says "add". */
    value?: string;
    /** For tests and the tour: the attribute the row is found by. */
    id: string;
    /** The editor. Rendered only while open. */
    children: Snippet;
    /**
     * The reader may not change this entry (P-05): the row says what is set and opens nothing, and
     * a field with nothing set is not drawn - "add" would be an offer the server refuses.
     */
    isReadOnly?: boolean;
    /** The ✦ of the subject this row shows, beside the row (UC-AI-04 check 6). */
    ask?: Snippet;
    /** What AI has to say about it, under the row. Renders nothing where nothing stands. */
    below?: Snippet;
    /** A quiet line under the value saying where it comes from: *from impact × urgency*. */
    note?: string;
    /** The value drawn otherwise than as its words - a label's chips; `value` still names it. */
    shown?: Snippet;
    /** The value in full where the row shows it short: a tip on the row. */
    title?: string;
  }

  const { label, value, id, children, isReadOnly = false, ask, below, note, shown, title }: Props = $props();

  let isOpen = $state(false);
</script>

<!-- `data-empty`: a field with nothing set, which the details fold away under *More fields* until
     somebody asks for it (concept round 2, §4). -->
<div class="detail" data-empty={value === undefined ? '' : undefined}>
{#if ask || below}
  <div class="with-ai">
    <div class="line">
      {@render row()}
      {#if ask}<span class="ask">{@render ask()}</span>{/if}
    </div>
    {#if below}{@render below()}{/if}
  </div>
{:else}
  {@render row()}
{/if}
</div>

{#snippet row()}
{#if isReadOnly}
  {#if value !== undefined}
    <div class="detail-row" data-detail={id} {title}>{@render face()}</div>
  {/if}
{:else if viewport.isCompact}
  <button type="button" class="detail-row" data-detail={id} {title} aria-haspopup="dialog" aria-expanded={isOpen} onclick={() => (isOpen = true)}>
    {@render face()}
  </button>
  <Drawer bind:isOpen edge="block-end" title={label} dismissLabel={t('app.dismiss')}>
    {@render children()}
  </Drawer>
{:else}
  <Popover label={label} bind:isOpen placement={{ side: 'block-end', align: 'start' }}>
    {#snippet trigger(props)}
      <button type="button" class="detail-row" data-detail={id} {title} {...props}>
        {@render face()}
      </button>
    {/snippet}
    {@render children()}
  </Popover>
{/if}
{/snippet}

{#snippet face()}
  <span class="key">{label}</span>
  {#if value === undefined}
    <span class="add"><Icon name="plus" size="sm" />{t('app.detail.add')}</span>
  {:else}
    <span class="value">{#if shown}{@render shown()}{:else}{value}{/if}{#if note}<span class="note" data-detail-note>{note}</span>{/if}</span>
  {/if}
{/snippet}

<style>
  /* A row of its own in the rows' grid, and gone with nothing in it when the reader may only read. */
  .detail { display: contents; }

  .with-ai { display: flex; flex-direction: column; gap: var(--sp-050); }
  .line { display: flex; align-items: center; gap: var(--sp-050); }
  .line > :global(:first-child) { flex: 1 1 auto; min-width: 0; }
  .ask { flex: none; }

  /* A form's row (task.d): the field's name in a column of its own, the value beside it from
     the same start, no rule between the rows - the section is the group. */
  .detail-row {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
    align-items: baseline;
    gap: var(--sp-100);
    inline-size: 100%;
    box-sizing: border-box;
    min-block-size: var(--sp-400);
    padding: var(--sp-050) var(--sp-100);
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  button.detail-row:hover { background: var(--bg-surface-hover); }

  div.detail-row { cursor: default; }

  .detail-row:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: calc(var(--sp-025) * -1);
    border-radius: var(--r-sm);
  }

  /* §3's label row: the field's name, small and quiet, so the value reads first. */
  .key {
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    color: var(--text-secondary);
  }

  .value {
    min-width: 0;
    text-align: start;
    overflow-wrap: anywhere;
  }

  .note { display: block; color: var(--text-subtle); font-size: var(--fs-075); }

  /* An empty field is an offer, in the interaction colour, with the word (rule 3). */
  .add {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    color: var(--text-brand);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  /* A thumb's target (§7): a row is layout.target tall where the pointer is coarse. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .detail-row { min-block-size: var(--layout-target); align-items: center; }
  }
</style>
