<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Map* (work-surface §6.10, scene invmap, UC-AST-11 check 5): one service at a time - the
  // worst-standing first - with how far below it to look, *only problems*, the legend, and the map
  // as a picture or as a list (§8: every chart has a table); under it what is open on the service
  // and the changes planned on it, each saying whether it collides with another. *All services*
  // keeps the hub's layered map. Read only, on a phone too: the picture scrolls sideways in its
  // frame, the two cards stack.
  //
  // **Collisions are the server's** (`readEntryCis`): each planned entry's own read says which other
  // entries are planned in an overlapping window on an object it names or reaches - the same answer
  // the entry's page gives, across the hub, not only among this service's changes.
  import { untrack } from 'svelte';
  import { DataTable, Select, StateLozenge, Switch, ViewSwitcher, type DataColumn, type SelectOption } from '@hubtask/design-system/components';
  import type { CiAlert, CiAlertList, CiClass, CiWork, CiWorkEntry, CmdbHealth, EntryCis, ServiceMap as ServiceMapRead } from '@hubtask/sync-engine';

  import { alertsPath, classWord, collisionsInOrder, entryCisPath, entryKey, healthPath, objectWorkPath, serviceMapPath, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { MAP_DEPTHS, serviceGraph, serviceWork, troublePath, workMarks, type MapDepth } from './inventoryscenes.ts';
  import ServiceGraph from './ServiceGraph.svelte';
  import ServiceMap from './ServiceMap.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
  }

  const { hubId, classes, words }: Props = $props();

  const ALL = '';
  let hubMap = $state<ServiceMapRead | undefined>(undefined);
  let health = $state<CmdbHealth | undefined>(undefined);
  let alerts = $state<readonly CiAlert[]>([]);
  $effect(() => {
    const at = hubId;
    return untrack(() => {
      const stops = [
        engine.subscribe<ServiceMapRead>({ path: serviceMapPath(at) }, (state) => {
          if (state.status === 'ready') hubMap = state.data;
        }),
        engine.subscribe<CmdbHealth>({ path: healthPath(at) }, (state) => {
          if (state.status === 'ready') health = state.data;
        }),
        engine.subscribe<CiAlertList>({ path: alertsPath(at, 'FIRING') }, (state) => {
          if (state.status === 'ready') alerts = state.data.data;
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  // The hub's services and offerings, the worst-standing first as the health ranks them.
  const services = $derived.by(() => {
    const ranked = (health?.services ?? []).map((each) => each.ci_id);
    const rank = (id: string) => (ranked.includes(id) ? ranked.indexOf(id) : Number.POSITIVE_INFINITY);
    return (hubMap?.nodes ?? [])
      .filter((node) => node.layer <= 1)
      .sort((a, b) => (rank(a.ci_id) === rank(b.ci_id) ? 0 : rank(a.ci_id) - rank(b.ci_id)) || a.name.localeCompare(b.name));
  });
  let chosen = $state<string | undefined>(undefined);
  const serviceId = $derived(chosen ?? services[0]?.ci_id ?? ALL);
  const service = $derived(services.find((each) => each.ci_id === serviceId));
  const serviceOptions = $derived<SelectOption[]>([
    ...services.map((each) => ({ value: each.ci_id, label: t('app.service_map.service_option', { name: each.name }) })),
    { value: ALL, label: t('app.service_map.all_services') },
  ]);

  let depth = $state<MapDepth>(3);
  let isProblemsOnly = $state(false);
  let isList = $state(false);

  // The chosen service's own map and the work on it.
  let map = $state<ServiceMapRead | undefined>(undefined);
  let work = $state<readonly CiWorkEntry[]>([]);
  $effect(() => {
    const at = hubId;
    const id = serviceId;
    if (!id) return;
    return untrack(() => {
      map = undefined;
      work = [];
      const stops = [
        engine.subscribe<ServiceMapRead>({ path: serviceMapPath(at, id) }, (state) => {
          if (state.status === 'ready') map = state.data;
        }),
        engine.subscribe<CiWork>({ path: objectWorkPath(id) }, (state) => {
          if (state.status === 'ready') work = state.data.data;
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  const marks = $derived(workMarks(work, serviceId));
  const wanting = $derived(new Set([
    ...(health?.problems ?? []).map((each) => each.ci_id),
    ...alerts.filter((each) => each.ci_id).map((each) => each.ci_id as string),
  ]));
  const troubled = $derived(new Set([...marks.keys(), ...wanting]));
  const graph = $derived(map && serviceId ? serviceGraph(map, serviceId, depth, isProblemsOnly ? troubled : undefined) : undefined);
  const dashed = $derived(graph ? troublePath(graph, new Set([...marks].filter(([, mark]) => mark.struck).map(([id]) => id))) : new Set<string>());
  const drawn = $derived(graph ? graph.columns.flat() : []);
  const isQuiet = $derived(isProblemsOnly && drawn.length <= 1);

  const { open, planned } = $derived(serviceWork(work));

  // Each planned entry's collisions, read once per entry.
  let collisions = $state<Record<string, EntryCis['collisions']>>({});
  $effect(() => {
    const wanted = planned.map((each) => each.item_id);
    return untrack(() => {
      const stops = wanted.map((id) =>
        engine.subscribe<EntryCis>({ path: entryCisPath(id) }, (state) => {
          if (state.status === 'ready') collisions = { ...collisions, [id]: collisionsInOrder(state.data).filter((each) => each.item_id !== id) };
        }),
      );
      return () => stops.forEach((stop) => stop());
    });
  });

  const when = (at: string) => formatDateTime(at, messages.locale);
  const windowOf = (entry: CiWorkEntry) =>
    entry.start_at && entry.due_at ? t('app.service_map.window', { from: when(entry.start_at), to: when(entry.due_at) }) : when((entry.start_at ?? entry.due_at)!);
  const keyOf = (entry: CiWorkEntry) => entry.key ?? '';

  // The map as a list: every object drawn with how far below it lies, its stage, what is open on it
  // and what it rests on within the picture.
  const listColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.service_map.col.object'), priority: 1 },
    { id: 'class', label: t('app.service_map.col.type'), priority: 2 },
    { id: 'depth', label: t('app.service_map.col.depth'), priority: 2 },
    { id: 'stage', label: t('app.service_map.col.stage'), priority: 3, isStatus: true },
    { id: 'open', label: t('app.service_map.col.open'), priority: 2 },
    { id: 'below', label: t('app.service_map.col.rests_on'), priority: 3 },
  ]);
  const nameOf = (id: string) => drawn.find((each) => each.ci_id === id)?.name ?? '';
  type Row = (typeof drawn)[number];
  const depthViews = $derived(MAP_DEPTHS.map((each) => ({ id: String(each), label: t(`app.service_map.depth_${each}`) })));
</script>

<div class="map-tab" data-inventory-map>
  <div class="tools" role="group" aria-label={t('app.service_map.tools')}>
    <div class="choose">
      <Select label={t('app.service_map.service')} isLabelHidden size="sm" options={serviceOptions} value={serviceId}
        onchange={(event: Event) => (chosen = (event.currentTarget as HTMLSelectElement).value)} />
    </div>
    {#if serviceId}
      <ViewSwitcher label={t('app.service_map.depth')} views={depthViews} selected={String(depth)} onselect={(id) => (depth = Number(id) as MapDepth)} />
      <Switch label={t('app.service_map.problems_only')} bind:checked={isProblemsOnly} />
      <Switch label={t('app.service_map.as_list')} bind:checked={isList} />
      <p class="legend" data-map-legend><span class="flag" aria-hidden="true">!</span> {t('app.service_map.key_legend')}</p>
    {/if}
  </div>

  {#if !serviceId}
    {#if hubMap && services.length === 0}<p class="meta">{t('app.service_map.empty')}</p>{/if}
    <ServiceMap {hubId} {classes} {words} />
  {:else if !graph}
    <div aria-busy="true" class="meta">{t('app.assets.loading')}</div>
  {:else}
    {#if isQuiet}<p class="meta" role="status" data-map-quiet>{t('app.service_map.no_problems')}</p>{/if}
    {#if isList}
      <div data-service-list>
        <DataTable label={t('app.service_map.list_of', { name: service?.name ?? '' })} columns={listColumns} rows={drawn} key={(row: Row) => row.ci_id}>
          {#snippet cell(row: Row, column: DataColumn)}
            {@const mark = marks.get(row.ci_id)}
            {#if column.id === 'name'}<a href={`/cis/${row.ci_id}`}>{row.name}</a>
            {:else if column.id === 'class'}{classWord(row.class, classes, words)}
            {:else if column.id === 'depth'}{row.depth === 0 ? t('app.service_map.the_service') : row.depth}
            {:else if column.id === 'stage'}<StateLozenge label={t(`app.assets.stage.${row.stage}`)} tone={row.stage === 'IN_USE' ? 'success' : 'neutral'} />
            {:else if column.id === 'open'}
              <span class="keys">
                {#if mark?.struck}<a class="key danger" href={`/items/${mark.struck.item_id}`}>{keyOf(mark.struck) || mark.struck.title}</a>{/if}
                {#if mark?.planned}<a class="key" href={`/items/${mark.planned.item_id}`}>{keyOf(mark.planned) || mark.planned.title}</a>{/if}
                {#if !mark}—{/if}
              </span>
            {:else}{graph.edges.filter((edge) => edge.source === row.ci_id).map((edge) => nameOf(edge.target)).join(', ') || '—'}{/if}
          {/snippet}
        </DataTable>
      </div>
    {:else}
      <ServiceGraph {graph} {marks} {wanting} {dashed} {classes} {words} />
    {/if}

    <div class="cards">
      <section class="card" aria-labelledby="map-open" data-map-open>
        <h2 id="map-open" class="card-title">{t('app.service_map.open_title')}</h2>
        {#if open.length === 0}
          <p class="meta">{t('app.service_map.open_none')}</p>
        {:else}
          <ul class="rows">
            {#each open as entry (entry.item_id)}
              <li class="row" data-map-entry={entry.key ?? entry.item_id}>
                <a class="what" href={`/items/${entry.item_id}`}>{#if entry.key}<span class="key">{entry.key}</span>{/if} <span>{entry.title}</span></a>
                <span class="meta">{entry.reason === 'BENEATH' && entry.via ? t('app.service_map.open_via', { name: entry.via.name }) : t('app.service_map.open_direct')}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
      <section class="card" aria-labelledby="map-planned" data-map-changes>
        <h2 id="map-planned" class="card-title">{t('app.service_map.planned_title')}</h2>
        {#if planned.length === 0}
          <p class="meta">{t('app.service_map.planned_none')}</p>
        {:else}
          <ul class="rows">
            {#each planned as entry (entry.item_id)}
              {@const clashes = collisions[entry.item_id]}
              <li class="planned" data-map-entry={entry.key ?? entry.item_id}>
                <span class="row">
                  <a class="what" href={`/items/${entry.item_id}`}>{#if entry.key}<span class="key">{entry.key}</span>{/if} <span>{entry.title}</span></a>
                  {#if entry.start_at || entry.due_at}<span class="meta">{windowOf(entry)}</span>{/if}
                </span>
                {#if clashes}
                  {#if clashes.length === 0}
                    <span class="said" data-collides="none">{t('app.service_map.collides_none')}</span>
                  {:else}
                    {#each clashes as clash (clash.item_id)}
                      <span class="said clash" data-collides={entryKey(clash)}>{t('app.service_map.collides', { key: entryKey(clash), title: clash.title })}</span>
                    {/each}
                  {/if}
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>
  {/if}
</div>

<style>
  .map-tab { display: grid; gap: var(--sp-200); min-inline-size: 0; }
  .tools { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100) var(--sp-200); }
  .legend { margin: 0; margin-inline-start: auto; color: var(--text-secondary); font-size: var(--fs-075); }
  .flag { color: var(--text-warning); font-weight: var(--fw-semibold); }
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, var(--layout-rail)), 1fr)); gap: var(--sp-200); }
  .card { display: grid; align-content: start; gap: var(--sp-100); padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); min-inline-size: 0; }
  .card-title { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .row { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-050) var(--sp-150); }
  .planned { display: grid; gap: var(--sp-050); }
  .what { color: var(--text-primary); text-decoration: none; min-inline-size: 0; overflow-wrap: anywhere; }
  .what:hover span:last-child { text-decoration: underline; }
  .keys { display: inline-flex; flex-wrap: wrap; gap: var(--sp-050); }
  .key { font-family: var(--font-mono); font-size: var(--fs-075); color: var(--text-secondary); }
  .key.danger { padding-inline: var(--sp-050); border-radius: var(--r-sm); background: var(--status-danger-surface); color: var(--status-danger-text); }
  .said { color: var(--text-secondary); font-size: var(--fs-075); }
  .said.clash { color: var(--text-warning); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
