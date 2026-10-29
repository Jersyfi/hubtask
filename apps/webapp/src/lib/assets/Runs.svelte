<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's reconciliation runs, the newest first, each opened to its decisions, and the items held
  // for review, which a member who may write closes; a closed hold stays on record (UC-AST-02 check 7).

  import { untrack } from 'svelte';
  import { Banner, Button, EmptyState, Spinner, Stack } from '@hubtask/design-system/components';
  import type { CiClass, CiHold, CiHoldList, CiRun, CiRunList } from '@hubtask/sync-engine';

  import { classWord, cmdb, holdsPath, runsPath, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import Decisions from './Decisions.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
  }

  const { hubId, classes, words, mayWrite }: Props = $props();

  let runs = $state<readonly CiRun[] | undefined>(undefined);
  let holds = $state<readonly CiHold[]>([]);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    return untrack(() => {
      const stops = [
        engine.subscribe<CiRunList>({ path: runsPath(at) }, (state) => {
          if (state.status === 'ready') runs = state.data.data;
          if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
        }),
        engine.subscribe<CiHoldList>({ path: holdsPath(at) }, (state) => {
          if (state.status === 'ready') holds = state.data.data;
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  let opened = $state<Record<string, CiRun>>({});
  async function toggle(run: CiRun) {
    const id = run.id;
    if (!id) return;
    if (opened[id]) {
      const { [id]: _closed, ...rest } = opened;
      opened = rest;
      return;
    }
    try {
      opened = { ...opened, [id]: await cmdb.run(id) };
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  async function dismiss(hold: CiHold) {
    try {
      await cmdb.dismissHold(hubId, hold.id);
      toast.show({ text: t('app.assets.dismissed'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  const when = (at: string) => formatDateTime(at, messages.locale);
  const counts = (run: CiRun) => t('app.assets.run_counts', {
    created: run.counts.created, updated: run.counts.updated, unchanged: run.counts.unchanged,
    held: run.counts.held, duplicates: run.counts.duplicates, refused: run.counts.refused,
  });
</script>

<Stack gap="200">
  <section class="part" aria-labelledby="runs-holds">
    <h2 id="runs-holds" class="heading">{t('app.assets.holds')}</h2>
    {#if holds.length === 0}
      <p class="meta">{t('app.assets.holds_empty')}</p>
    {:else}
      <ul class="rows">
        {#each holds as hold (hold.id)}
          <li class="row">
            <span class="what">
              <strong>{hold.item.name || hold.item.native_key || hold.item.ref || '—'}</strong>
              <span class="meta">{classWord(hold.item.class, classes, words)} · {t('app.assets.hold_from', { source: hold.source, when: when(hold.created_at) })}</span>
              <span>{messages.has(hold.reason) ? t(hold.reason) : hold.reason}</span>
            </span>
            {#if mayWrite}<Button size="sm" tone="subtle" icon="check" onclick={() => void dismiss(hold)}>{t('app.assets.dismiss')}</Button>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="part" aria-labelledby="runs-list">
    <h2 id="runs-list" class="heading">{t('app.assets.section.runs')}</h2>
    {#if refusal}
      <Banner tone="danger">{refusal}</Banner>
    {:else if !runs}
      <Spinner label={t('app.assets.loading')} />
    {:else if runs.length === 0}
      <EmptyState kind="unused" title={t('app.assets.runs_empty')} icon="rotate-ccw" />
    {:else}
      <ul class="rows">
        {#each runs as run (run.id ?? run.started_at)}
          <li class="run">
            <div class="row-line">
              <span class="what">
                <strong>{t('app.assets.run_title', { source: run.source, when: when(run.started_at) })}</strong>
                <span class="meta">{counts(run)}</span>
              </span>
              {#if run.id}
                <Button size="sm" tone="subtle" onclick={() => void toggle(run)}>{opened[run.id] ? t('app.assets.run_close') : t('app.assets.run_open')}</Button>
              {/if}
            </div>
            {#if run.id && opened[run.id]}
              <Decisions decisions={opened[run.id]!.decisions ?? []} {classes} {words} />
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</Stack>

<style>
  .part { display: grid; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row, .row-line { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100); }
  .row, .run {
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .run { display: grid; gap: var(--sp-100); }
  .what { display: grid; gap: var(--sp-025); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
