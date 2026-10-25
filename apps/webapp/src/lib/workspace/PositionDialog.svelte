<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where the places of one level stand, and the one-press way to move any of them (design-system.md
  // §11.3): the pointer drags a row in the navigation, the keyboard moves it with Alt and an arrow,
  // and this is what both are without a drag - the level in its order, the place it was opened from
  // marked, and Up and Down on every row (WCAG 2.5.7). Each press is the same write the drag sends;
  // the list follows at once.

  import { Button, Dialog, IconButton } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    isOpen: boolean;
    /** The place it was opened from, marked in the list. */
    id: string;
    name: string;
    /** The level in order, the place itself included: each by its id and name. */
    siblings: readonly { readonly id: string; readonly name: string }[];
    /** Moves one place of the level by `offset` steps. */
    onmove: (placeId: string, offset: number) => void;
  }

  let { isOpen = $bindable(), id, name, siblings, onmove }: Props = $props();
</script>

<Dialog bind:isOpen title={t('app.place.position_title', { name })} dismissLabel={t('app.workspace.cancel')}>
  <ol class="order" aria-label={t('app.place.position_order')} data-position-order>
    {#each siblings as each, index (each.id)}
      <li aria-current={each.id === id ? 'true' : undefined}>
        <span class="name">{each.name}</span>
        <span class="steps">
          <IconButton icon="chevron-up" size="sm" label={t('app.rank.up_named', { name: each.name })} disabledReason={index === 0 ? t('app.rank.already_first') : undefined} onclick={() => onmove(each.id, -1)} />
          <IconButton icon="chevron-down" size="sm" label={t('app.rank.down_named', { name: each.name })} disabledReason={index === siblings.length - 1 ? t('app.rank.already_last') : undefined} onclick={() => onmove(each.id, 1)} />
        </span>
      </li>
    {/each}
  </ol>
  {#snippet actions()}
    <Button onclick={() => (isOpen = false)}>{t('app.place.position_done')}</Button>
  {/snippet}
</Dialog>

<style>
  .order { display: flex; flex-direction: column; gap: var(--sp-025); margin: 0; padding: 0; list-style: none; counter-reset: place; }
  .order li {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding-block: var(--sp-050);
    padding-inline: var(--sp-150) var(--sp-050);
    border-radius: var(--r-md);
    color: var(--text-secondary);
    counter-increment: place;
  }
  .order li::before { content: counter(place); min-inline-size: var(--sp-200); color: var(--text-subtle); font-variant-numeric: tabular-nums; }
  .name { flex: 1 1 auto; min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .steps { display: flex; flex: none; gap: var(--sp-025); }
  /* Rule 3: the place it was opened from is marked by weight and a bar, not by its tint alone. */
  .order li[aria-current] {
    background: var(--bg-surface-pressed);
    color: var(--text-primary);
    font-weight: var(--fw-semibold);
    box-shadow: inset var(--bw-thick) 0 0 0 var(--accent-primary);
  }
</style>
