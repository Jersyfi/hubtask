<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where a place stands among its siblings, and the one-press way to move it (design-system.md
  // §11.3): the pointer drags a row in the navigation, the keyboard moves it with Alt and an arrow,
  // and this is what both are without a drag - the list in its order, the place marked, Up and
  // Down (WCAG 2.5.7). Each press is the same write the drag sends; the list follows at once.

  import { Button, Dialog } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    isOpen: boolean;
    /** The place being moved. */
    id: string;
    name: string;
    /** Its level in order, itself included: each by its id and name. */
    siblings: readonly { readonly id: string; readonly name: string }[];
    onmove: (offset: number) => void;
  }

  let { isOpen = $bindable(), id, name, siblings, onmove }: Props = $props();

  const position = $derived(siblings.findIndex((each) => each.id === id));
</script>

<Dialog bind:isOpen title={t('app.place.position_title', { name })} dismissLabel={t('app.workspace.cancel')}>
  <ol class="order" aria-label={t('app.place.position_order')}>
    {#each siblings as each (each.id)}
      <li aria-current={each.id === id ? 'true' : undefined}>{each.name}</li>
    {/each}
  </ol>
  {#snippet actions()}
    <Button tone="secondary" icon="chevron-up" disabledReason={position <= 0 ? t('app.rank.already_first') : undefined} onclick={() => onmove(-1)}>
      {t('app.rank.up')}
    </Button>
    <Button tone="secondary" icon="chevron-down" disabledReason={position < 0 || position >= siblings.length - 1 ? t('app.rank.already_last') : undefined} onclick={() => onmove(1)}>
      {t('app.rank.down')}
    </Button>
    <Button onclick={() => (isOpen = false)}>{t('app.place.position_done')}</Button>
  {/snippet}
</Dialog>

<style>
  .order { display: flex; flex-direction: column; gap: var(--sp-025); margin: 0; padding: 0; list-style: none; counter-reset: place; }
  .order li {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150);
    border-radius: var(--r-md);
    color: var(--text-secondary);
    counter-increment: place;
  }
  .order li::before { content: counter(place); min-inline-size: var(--sp-200); color: var(--text-subtle); font-variant-numeric: tabular-nums; }
  /* Rule 3: the place moved is marked by weight and a bar, not by its tint alone. */
  .order li[aria-current] {
    background: var(--bg-surface-pressed);
    color: var(--text-primary);
    font-weight: var(--fw-semibold);
    box-shadow: inset var(--bw-thick) 0 0 0 var(--accent-primary);
  }
</style>
