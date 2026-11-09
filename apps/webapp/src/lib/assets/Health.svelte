<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's inventory judged (UC-AST-06 checks 6 to 9): completeness and correctness, the stale,
  // orphaned and duplicate CIs, those without an owner, the reviews overdue and the relations no
  // program confirms any more, in all and per class; each service's score, worst first; each
  // source's conflict rate; and the CIs found wanting, each opened from its row.

  import { untrack } from 'svelte';
  import { DataTable, Spinner, Stack, type DataColumn } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { CiClass, CmdbHealth, ConfigurationItem, ConfigurationItemList } from '@hubtask/sync-engine';

  import { cisPath, classWord, cmdb, healthPath, percent, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  /** How many names of CIs beyond the inventory's first page are read one by one. */
  const READ_ALONE = 100;

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
  }

  const { hubId, classes, words }: Props = $props();

  let health = $state<CmdbHealth | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let names = $state<Record<string, string>>({});
  $effect(() => {
    const at = hubId;
    return untrack(() => {
      const stops = [
        engine.subscribe<CmdbHealth>({ path: healthPath(at) }, (state) => {
          if (state.status === 'ready') health = state.data;
          if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
        }),
        engine.subscribe<ConfigurationItemList>({ path: cisPath(at) }, (state) => {
          if (state.status === 'ready') names = { ...names, ...Object.fromEntries(state.data.data.map((ci: ConfigurationItem) => [ci.id, ci.name])) };
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  $effect(() => {
    const unknown = (health?.problems ?? []).map((each) => each.ci_id).filter((id) => !(id in names)).slice(0, READ_ALONE);
    untrack(() => {
      for (const id of unknown) {
        void cmdb.read(id).then((ci) => {
          if (ci) names = { ...names, [id]: ci.name };
        });
      }
    });
  });

  const share = (value: number) => t('app.assets.percent', { value: percent(value) });
  const word = (key: string) => (messages.has(`app.assets.attribute.${key}`) ? t(`app.assets.attribute.${key}`) : key);
  const classColumns = $derived<DataColumn[]>([
    { id: 'class', label: t('app.assets.class_label'), priority: 1 },
    { id: 'total', label: t('app.assets.health_total'), priority: 1 },
    { id: 'completeness', label: t('app.assets.completeness'), priority: 2 },
    { id: 'stale', label: t('app.assets.stale'), priority: 2 },
    { id: 'orphans', label: t('app.assets.orphans'), priority: 3 },
    { id: 'duplicates', label: t('app.assets.duplicates'), priority: 3 },
  ]);
  const serviceColumns = $derived<DataColumn[]>([
    { id: 'service', label: t('app.assets.service_label'), priority: 1 },
    { id: 'class', label: t('app.assets.class_label'), priority: 2 },
    { id: 'cis', label: t('app.assets.service_cis'), priority: 2 },
    { id: 'wanting', label: t('app.assets.service_wanting'), priority: 2 },
    { id: 'score', label: t('app.assets.service_score'), priority: 1 },
  ]);
  const sourceColumns = $derived<DataColumn[]>([
    { id: 'source', label: t('app.assets.source_label'), priority: 1 },
    { id: 'reported', label: t('app.assets.source_reported'), priority: 2 },
    { id: 'kept', label: t('app.assets.source_kept'), priority: 2 },
    { id: 'conflicts', label: t('app.assets.source_conflicts'), priority: 1 },
  ]);
  type ClassRow = CmdbHealth['classes'][number];
  type ServiceRow = CmdbHealth['services'][number];
  type SourceRow = CmdbHealth['sources'][number];
</script>

{#if refusal}
  <FailureNote problem={refusal} />
{:else if !health}
  <Spinner label={t('app.assets.loading')} />
{:else}
  <Stack gap="200">
    <dl class="figures">
      <div><dt>{t('app.assets.health_total')}</dt><dd>{health.total}</dd></div>
      <div><dt>{t('app.assets.completeness')}</dt><dd>{share(health.completeness)}</dd></div>
      <div><dt>{t('app.assets.correctness')}</dt><dd>{share(health.correctness)}</dd></div>
      <div><dt>{t('app.assets.stale')}</dt><dd>{health.stale}</dd></div>
      <div><dt>{t('app.assets.orphans')}</dt><dd>{health.orphans}</dd></div>
      <div><dt>{t('app.assets.duplicates')}</dt><dd>{health.duplicates}</dd></div>
      <div><dt>{t('app.assets.without_owner')}</dt><dd>{health.without_owner}</dd></div>
      <div><dt>{t('app.assets.review_overdue')}</dt><dd>{health.review_overdue}</dd></div>
      <div><dt>{t('app.assets.stale_relations')}</dt><dd>{health.stale_relations}</dd></div>
    </dl>

    {#if health.classes.length > 0}
      <DataTable label={t('app.assets.section.health')} columns={classColumns} rows={health.classes} key={(row: ClassRow) => row.class}>
        {#snippet cell(row: ClassRow, column: DataColumn)}
          {#if column.id === 'class'}{classWord(row.class, classes, words)}
          {:else if column.id === 'total'}{row.total}
          {:else if column.id === 'completeness'}{share(row.completeness)}
          {:else if column.id === 'stale'}{row.stale}
          {:else if column.id === 'orphans'}{row.orphans}
          {:else}{row.duplicates}{/if}
        {/snippet}
      </DataTable>
    {/if}

    <section class="part" aria-labelledby="health-services">
      <h2 id="health-services" class="heading">{t('app.assets.services')}</h2>
      {#if health.services.length === 0}
        <p class="meta">{t('app.assets.services_none')}</p>
      {:else}
        <DataTable label={t('app.assets.services')} columns={serviceColumns} rows={health.services} key={(service: ServiceRow) => service.ci_id}>
          {#snippet cell(service: ServiceRow, column: DataColumn)}
            {#if column.id === 'service'}<a href={`/cis/${service.ci_id}`}>{service.name}</a>
            {:else if column.id === 'class'}{classWord(service.class, classes, words)}
            {:else if column.id === 'cis'}{service.cis}
            {:else if column.id === 'wanting'}{service.wanting}
            {:else}{share(service.score)}{/if}
          {/snippet}
        </DataTable>
      {/if}
    </section>

    <section class="part" aria-labelledby="health-sources">
      <h2 id="health-sources" class="heading">{t('app.assets.sources')}</h2>
      {#if health.sources.length === 0}
        <p class="meta">{t('app.assets.sources_none')}</p>
      {:else}
        <DataTable label={t('app.assets.sources')} columns={sourceColumns} rows={health.sources} key={(source: SourceRow) => source.source}>
          {#snippet cell(source: SourceRow, column: DataColumn)}
            {#if column.id === 'source'}{source.source}
            {:else if column.id === 'reported'}{source.reported}
            {:else if column.id === 'kept'}{source.kept}
            {:else}{share(source.conflict_rate)}{/if}
          {/snippet}
        </DataTable>
      {/if}
    </section>

    <section class="part" aria-labelledby="health-problems">
      <h2 id="health-problems" class="heading">{t('app.assets.problems')}</h2>
      <p class="meta">{t('app.assets.attention_why')}</p>
      {#if health.problems.length === 0}
        <p class="meta">{t('app.assets.problems_none')}</p>
      {:else}
        <ul class="rows">
          {#each health.problems as problem (problem.ci_id)}
            <li class="row">
              <a href={`/cis/${problem.ci_id}`}>{names[problem.ci_id] ?? t('app.assets.unknown_ci')}</a>
              <span class="meta">
                {classWord(problem.class, classes, words)} · {problem.kinds.map((kind) => t(`app.assets.kind.${kind}`)).join(', ')}
                {#if problem.missing?.length} · {t('app.assets.missing', { keys: problem.missing.map(word).join(', ') })}{/if}
              </span>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  </Stack>
{/if}

<style>
  .figures { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--sp-1600)), 1fr)); gap: var(--sp-100); margin: 0; }
  .figures div {
    display: grid; gap: var(--sp-025); padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface);
  }
  .figures dt { color: var(--text-secondary); font-size: var(--fs-075); }
  .figures dd { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .part { display: grid; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .row a, .part a { color: var(--text-primary); font-weight: var(--fw-medium); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
