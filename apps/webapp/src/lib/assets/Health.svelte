<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's inventory judged (UC-AST-06 check 6): completeness and correctness, the stale, orphaned
  // and duplicate CIs, those without an owner and the reviews overdue, in all and per class, and
  // the CIs found wanting, each opened from its row.

  import { untrack } from 'svelte';
  import { Banner, Spinner, Stack } from '@hubtask/design-system/components';
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
</script>

{#if refusal}
  <Banner tone="danger">{refusal}</Banner>
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
    </dl>

    {#if health.classes.length > 0}
      <div class="scroll">
        <table class="classes">
          <caption>{t('app.assets.section.health')}</caption>
          <thead>
            <tr>
              <th scope="col">{t('app.assets.class_label')}</th>
              <th scope="col">{t('app.assets.health_total')}</th>
              <th scope="col">{t('app.assets.completeness')}</th>
              <th scope="col">{t('app.assets.stale')}</th>
              <th scope="col">{t('app.assets.orphans')}</th>
              <th scope="col">{t('app.assets.duplicates')}</th>
            </tr>
          </thead>
          <tbody>
            {#each health.classes as row (row.class)}
              <tr>
                <th scope="row">{classWord(row.class, classes, words)}</th>
                <td>{row.total}</td>
                <td>{share(row.completeness)}</td>
                <td>{row.stale}</td>
                <td>{row.orphans}</td>
                <td>{row.duplicates}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}

    <section class="part" aria-labelledby="health-problems">
      <h2 id="health-problems" class="heading">{t('app.assets.problems')}</h2>
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
  .scroll { overflow-x: auto; }
  .classes { border-collapse: collapse; inline-size: 100%; font-size: var(--fs-075); }
  .classes caption { text-align: start; color: var(--text-secondary); padding-block-end: var(--sp-050); }
  .classes th, .classes td { text-align: start; padding: var(--sp-050) var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .part { display: grid; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .row a { color: var(--text-primary); font-weight: var(--fw-medium); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
