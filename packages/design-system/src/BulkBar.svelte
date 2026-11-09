<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What stands in the head's place while rows are selected (page-system §6): how many, up to three
  // verbs for all of them, ⋯ for the rest, and *Done*. Escape ends the selection; it is in the
  // layer register on the overlay rank, so a menu opened from it closes first.
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Menu from './Menu.svelte';
  import { escapeHandler } from './focus.ts';
  import type { IconName } from './icons/index.ts';
  import { layers } from './layers.ts';
  import type { MenuItem } from './overlay.ts';

  export interface BulkAction {
    readonly id: string;
    readonly label: string;
    readonly icon?: IconName;
    readonly disabledReason?: string;
    readonly isDestructive?: boolean;
  }

  interface Props {
    /** *3 selected*. */
    label: string;
    /** At most three; the type says so. */
    actions?: readonly [] | readonly [BulkAction] | readonly [BulkAction, BulkAction] | readonly [BulkAction, BulkAction, BulkAction];
    onaction?: (id: string) => void;
    menu?: { label: string; items: readonly MenuItem[] };
    doneLabel: string;
    onDone: () => void;
  }

  const { label, actions = [], onaction, menu, doneLabel, onDone }: Props = $props();

  $effect(() => {
    const handle = layers.open('overlay', () => onDone());
    const onKeydown = escapeHandler();
    document.addEventListener('keydown', onKeydown);
    return () => {
      document.removeEventListener('keydown', onKeydown);
      handle.release();
    };
  });
</script>

<div class="bulk-bar" role="toolbar" aria-label={label}>
  <span class="count" aria-live="polite">{label}</span>
  <span class="acts">
    {#each actions as action (action.id)}
      <Button size="sm" tone={action.isDestructive ? 'danger' : 'secondary'} icon={action.icon} disabledReason={action.disabledReason} onclick={() => onaction?.(action.id)}>
        {action.label}
      </Button>
    {/each}
    {#if menu && menu.items.length > 0}
      <Menu label={menu.label} items={menu.items} placement={{ side: 'block-end', align: 'end' }} onselect={(id) => onaction?.(id)}>
        {#snippet trigger(props)}
          <IconButton icon="ellipsis" label={menu.label} size="sm" {...props} />
        {/snippet}
      </Menu>
    {/if}
  </span>
  <span class="done"><Button size="sm" tone="primary" onclick={onDone}>{doneLabel}</Button></span>
</div>

<style>
  .bulk-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-150);
    min-block-size: var(--sp-600);
    padding: var(--sp-100) var(--sp-200);
    border: var(--bw-hairline) solid var(--accent-primary);
    border-radius: var(--r-lg);
    background: var(--accent-primary-subtle);
    box-sizing: border-box;
  }

  .count { color: var(--text-brand); font-weight: var(--fw-semibold); font-size: var(--fs-100); }
  .acts { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .done { margin-inline-start: auto; }
</style>
