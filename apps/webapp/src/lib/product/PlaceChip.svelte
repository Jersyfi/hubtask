<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A create dialog's *In:* chip (user-experience.md §5.2): where the new object is made, one press
  // to change - capture's place chip (design-system.md §11.8) for every product. With one place it
  // names it and offers nothing to choose.
  import { Menu } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import type { CreatePlace } from './home.ts';

  interface Props {
    /** The place chosen now; drawn by its label among `places`, or by `fallback` while not listed. */
    current: string;
    fallback?: string;
    places: readonly CreatePlace[];
    onchoose: (id: string) => void;
  }
  const { current, fallback = '', places, onchoose }: Props = $props();

  const label = $derived(places.find((place) => place.id === current)?.label ?? fallback);
  const others = $derived(places.length > 1 || (places.length === 1 && places[0]?.id !== current));
</script>

{#if label}
  <div class="where" data-create-place>
    {#if others}
      <Menu label={t('app.capture.place_choose')} items={places.map((place) => ({ id: place.id, label: place.label }))} onselect={onchoose}>
        {#snippet trigger(props)}
          <button type="button" class="pill" data-chip="place" {...props}>
            <span class="pill-key">{t('app.capture.place_key')}</span><span class="pill-value">{label}</span><span class="pill-more" aria-hidden="true">▾</span>
          </button>
        {/snippet}
      </Menu>
    {:else}
      <span class="pill" data-chip="place">
        <span class="pill-key">{t('app.capture.place_key')}</span><span class="pill-value">{label}</span>
      </span>
    {/if}
  </div>
{/if}

<style>
  .where { display: flex; flex-wrap: wrap; gap: var(--sp-100); min-inline-size: 0; }

  .pill {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    max-inline-size: 100%;
    min-block-size: var(--sp-400);
    padding-inline: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-canvas);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-075);
  }

  button.pill { cursor: pointer; }
  button.pill:hover { background: var(--bg-surface-hover); }
  button.pill:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .pill-key { color: var(--text-secondary); }
  .pill-value { font-weight: var(--fw-medium); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .pill-more { color: var(--text-subtle); }
</style>
