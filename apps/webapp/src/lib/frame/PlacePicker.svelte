<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  /** A place *Move to…* lists, worded: greyed with its reason where it cannot take the row. */
  export interface PickedPlace {
    readonly key: string;
    readonly label: string;
    readonly depth: number;
    readonly icon?: import('@hubtask/design-system/components').IconName;
    readonly reason?: string;
  }
</script>

<script lang="ts">
  // *Move to…* for a row of the sidebar (the navigation concept's §5.4, UC-WRK-38 check 6): a place
  // picker with search, what may not take the row greyed with its reason rather than hidden, so a
  // reader learns why. A dialog on a pointer, a sheet on the phone (§9: "the place picker as a
  // stack"). The move itself is the caller's.
  import { Dialog, Icon, Input, Sheet } from '@hubtask/design-system/components';

  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { viewport } from './viewport.svelte.ts';

  interface Props {
    title: string;
    places: readonly PickedPlace[];
    isOpen?: boolean;
    onpick: (key: string) => void;
  }

  let { title, places, isOpen = $bindable(false), onpick }: Props = $props();

  let words = $state('');
  const shown = $derived.by(() => {
    const needle = words.trim().toLocaleLowerCase(messages.locale);
    return needle ? places.filter((each) => each.label.toLocaleLowerCase(messages.locale).includes(needle)) : places;
  });

  function pick(place: PickedPlace) {
    if (place.reason) return;
    // Picked before closing: the caller lets go of what moves when the picker closes.
    onpick(place.key);
    isOpen = false;
  }
</script>

{#snippet list()}
  <Input label={t('app.nav.find_place')} isLabelHidden icon="search" bind:value={words} />
  <ul class="places" aria-label={title}>
    {#each shown as place (place.key)}
      <li data-level={String(Math.min(place.depth, 6))}>
        <button type="button" class="place" aria-disabled={place.reason ? 'true' : undefined} onclick={() => pick(place)}>
          {#if place.icon}<Icon name={place.icon} size="sm" />{/if}
          <span class="name">{place.label}</span>
          {#if place.reason}<small class="why">{place.reason}</small>{/if}
        </button>
      </li>
    {/each}
  </ul>
{/snippet}

{#if viewport.isCompact}
  <Sheet {title} closeLabel={t('app.sheet_close')} bind:isOpen>{@render list()}</Sheet>
{:else}
  <Dialog size="m" {title} bind:isOpen dismissLabel={t('app.workspace.cancel')}>{@render list()}</Dialog>
{/if}

<style>
  .places {
    display: flex;
    flex-direction: column;
    gap: var(--sp-025);
    max-block-size: calc(var(--sp-800) * 5);
    margin: var(--sp-150) 0 0;
    padding: 0;
    overflow: auto;
    list-style: none;
  }

  .place {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    inline-size: 100%;
    min-block-size: var(--layout-nav-row);
    padding: var(--sp-050) var(--sp-100);
    border: 0;
    border-radius: var(--layout-nav-radius);
    background: none;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .place:hover:not([aria-disabled]) { background: var(--bg-surface-hover); }
  .place:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--bw-ring)); }
  .place[aria-disabled] { color: var(--text-subtle); cursor: not-allowed; }

  .name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .why { color: var(--text-subtle); }

  li[data-level='1'] .place { padding-inline-start: calc(var(--sp-100) + var(--layout-nav-indent)); }
  li[data-level='2'] .place { padding-inline-start: calc(var(--sp-100) + 2 * var(--layout-nav-indent)); }
  li[data-level='3'] .place { padding-inline-start: calc(var(--sp-100) + 3 * var(--layout-nav-indent)); }
  li[data-level='4'] .place { padding-inline-start: calc(var(--sp-100) + 4 * var(--layout-nav-indent)); }
  li[data-level='5'] .place { padding-inline-start: calc(var(--sp-100) + 5 * var(--layout-nav-indent)); }
  li[data-level='6'] .place { padding-inline-start: calc(var(--sp-100) + 6 * var(--layout-nav-indent)); }
</style>
