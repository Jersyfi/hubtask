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

  import { actor } from '../data/account.svelte.ts';
  import { assetsPath, costText, inventoryPath, softwarePath, type StockPart, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { registerSummary } from './inventoryscenes.ts';
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
    /** The line, or the saved group, the stock opens with. */
    line?: string;
    group?: string;
    /** The register's device the address names. */
    asset?: string;
    /** A writer, online, on a screen that writes: the register's moves. */
    mayAct?: boolean;
  }

  const { hubId, classes, words, mayWrite, onnavigate, part, wanting, isSavedShown = false, asked, onasked, onimport, line, group, asset, mayAct = false }: Props = $props();

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

  // The register's summary beside the switch (scene invreg 1): the value issued, the warranties ending.
  const summary = $derived(registered ? registerSummary(registered, todayIn(actor.zone)) : undefined);
  const summaryParts = $derived(summary ? [
    ...(summary.inUse.length > 0 ? [t('app.register.summary_value', { value: summary.inUse.map((each) => costText(each.minor, each.currency, messages.locale)).join(', ') })] : []),
    ...(summary.ending > 0 ? [t('app.register.summary_ending', { count: summary.ending, months: summary.firstInMonths ?? 0 })] : []),
  ] : []);

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
    {#if part === 'register' && summaryParts.length > 0}<p class="summary" data-register-summary>{summaryParts.join(' · ')}</p>{/if}
  </div>
  {#if part === 'register'}
    <Register {hubId} {classes} {words} selected={asset} {mayAct} {onnavigate} />
  {:else if part === 'software'}
    <Software {hubId} />
  {:else}
    <Inventory {hubId} {classes} {words} {mayWrite} {onnavigate} {wanting} {tags} {isSavedShown} {asked} {onasked} {onimport} openLine={line} openGroup={group} />
  {/if}
</Stack>

<style>
  .parts { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100) var(--sp-200); }
  .summary { margin: 0; margin-inline-start: auto; color: var(--text-secondary); font-size: var(--fs-090); }
</style>
