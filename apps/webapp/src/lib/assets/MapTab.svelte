<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Map* (work-surface §6.10, scene invmap): the hub's service map in its layers, or one
  // service's (UC-AST-11 check 5). Today's map, until the scene's own is built here.
  import { untrack } from 'svelte';
  import { Select, Stack, type SelectOption } from '@hubtask/design-system/components';
  import type { CiClass, ConfigurationItem, ConfigurationItemList } from '@hubtask/sync-engine';

  import { cisPath, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import ServiceMap from './ServiceMap.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
  }

  const { hubId, classes, words }: Props = $props();

  let hubCis = $state<readonly ConfigurationItem[]>([]);
  $effect(() => {
    const at = hubId;
    return untrack(() =>
      engine.subscribe<ConfigurationItemList>({ path: cisPath(at) }, (state) => {
        if (state.status === 'ready') hubCis = state.data.data;
      }),
    );
  });
  const isService = (ci: ConfigurationItem) => classes.find((each) => each.key === ci.class)?.lineage.includes('service') ?? false;
  const serviceOptions = $derived<SelectOption[]>(hubCis.filter((each) => !each.merged_into && isService(each)).map((each) => ({ value: each.id, label: each.name })));
  let mapService = $state('');
</script>

<Stack gap="150">
  <div class="head">
    <h2 id="map-heading" class="heading">{t('app.service_map.title')}</h2>
    {#if serviceOptions.length > 0}
      <Select label={t('app.service_map.service')} size="sm" options={[{ value: '', label: t('app.service_map.whole_hub') }, ...serviceOptions]} bind:value={mapService} />
    {/if}
  </div>
  {#key mapService}
    <ServiceMap {hubId} serviceId={mapService || undefined} {classes} {words} />
  {/key}
</Stack>

<style>
  .head { display: flex; flex-wrap: wrap; align-items: end; justify-content: space-between; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
</style>
