<!-- SPDX-License-Identifier: BUSL-1.1
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
  }

  const { label, value, id, children }: Props = $props();

  let isOpen = $state(false);
</script>

{#if viewport.isCompact}
  <button type="button" class="detail-row" data-detail={id} aria-haspopup="dialog" aria-expanded={isOpen} onclick={() => (isOpen = true)}>
    {@render face()}
  </button>
  <Drawer bind:isOpen edge="block-end" title={label} dismissLabel={t('app.dismiss')}>
    {@render children()}
  </Drawer>
{:else}
  <Popover label={label} bind:isOpen placement={{ side: 'block-end', align: 'start' }}>
    {#snippet trigger(props)}
      <button type="button" class="detail-row" data-detail={id} {...props}>
        {@render face()}
      </button>
    {/snippet}
    {@render children()}
  </Popover>
{/if}

{#snippet face()}
  <span class="key">{label}</span>
  {#if value === undefined}
    <span class="add"><Icon name="plus" size="sm" />{t('app.detail.add')}</span>
  {:else}
    <span class="value">{value}</span>
  {/if}
{/snippet}

<style>
  .detail-row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--sp-150);
    inline-size: 100%;
    min-block-size: var(--density-control-md-min);
    padding: var(--density-row-block) var(--sp-100);
    border: 0;
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    border-radius: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .detail-row:hover { background: var(--bg-surface-hover); }

  .detail-row:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: calc(var(--sp-025) * -1);
    border-radius: var(--r-sm);
  }

  /* §3's label row: the field's name, small and quiet, so the value reads first. */
  .key {
    flex: none;
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    color: var(--text-subtle);
  }

  .value {
    min-width: 0;
    text-align: end;
    overflow-wrap: anywhere;
  }

  /* An empty field is an offer, in the interaction colour, with the word (rule 3). */
  .add {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    color: var(--text-brand);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }
</style>
