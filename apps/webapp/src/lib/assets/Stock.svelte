<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Stock* (work-surface §6.10, scenes invlist and invreg): all objects, the devices and licences
  // of the register, and the software found on them - one switch, each part with its own address
  // (`/inventory`, `/inventory/register`, `/inventory/software`). The register's numbers also feed
  // the line's `asset.tag`.
  import { untrack } from 'svelte';
  import { Stack, ViewSwitcher } from '@hubtask/design-system/components';
  import type { CiAssetList, CiClass, SoftwareProductList } from '@hubtask/sync-engine';

  import { assetsPath, inventoryPath, softwarePath, type StockPart, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import Inventory from './Inventory.svelte';
  import Register from './Register.svelte';
  import Software from './Software.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
    onnavigate: (path: string) => void;
    part: StockPart;
    wanting?: ReadonlySet<string>;
    isSavedShown?: boolean;
    asked?: 'ci' | 'group';
    onasked?: () => void;
    onimport?: () => void;
  }

  const { hubId, classes, words, mayWrite, onnavigate, part, wanting, isSavedShown = false, asked, onasked, onimport }: Props = $props();

  let registered = $state<CiAssetList['data'] | undefined>(undefined);
  let software = $state<number | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    return untrack(() => {
      const stops = [
        engine.subscribe<CiAssetList>({ path: assetsPath(at) }, (state) => {
          if (state.status === 'ready') registered = state.data.data;
        }),
        engine.subscribe<SoftwareProductList>({ path: softwarePath(at) }, (state) => {
          if (state.status === 'ready') software = state.data.data.length;
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  const tags = $derived(new Map((registered ?? []).filter((each) => each.asset_tag).map((each) => [each.ci_id, each.asset_tag])));

  const counted = (label: string, count: number | undefined) => (count === undefined ? label : t('app.inventory.with_count', { label, count: String(count) }));
  const parts = $derived([
    { id: 'all', label: t('app.inventory.part_all') },
    { id: 'register', label: counted(t('app.assets.section.register'), registered?.length) },
    { id: 'software', label: counted(t('app.assets.section.software'), software) },
  ]);
</script>

<Stack gap="150">
  <div class="parts" data-stock-parts>
    <ViewSwitcher label={t('app.inventory.parts')} views={parts} selected={part}
      onselect={(id) => onnavigate(inventoryPath(hubId, id === 'all' ? 'all' : (id as 'register' | 'software')))} />
  </div>
  {#if part === 'register'}
    <Register {hubId} {classes} {words} />
  {:else if part === 'software'}
    <Software {hubId} />
  {:else}
    <Inventory {hubId} {classes} {words} {mayWrite} {onnavigate} {wanting} {tags} {isSavedShown} {asked} {onasked} {onimport} />
  {/if}
</Stack>

<style>
  .parts { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
</style>
