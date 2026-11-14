<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Upkeep* (work-surface §6.10, IN-4, scene invcare): what keeps the inventory true, in one place.
  // (1) Six figures - complete, correct, stale, orphaned, duplicates, without responsibility -, each
  // opening what it counts; (2) *To do*, one row per problem with one act; (3) the attestation round;
  // (4) the duplicates; (5) the sources with their runs, *By hand* and the acts to connect and import.
  // The deviations a source's run held are rows of *To do*, so they have no card of their own.
  //
  // **The former sub-pages stay whole** behind the cards' *All …*: an address naming one -
  // `/inventory/runs`, `/duplicates`, `/connectors`, `/campaigns`, `/import` - draws that part alone
  // under *Back to upkeep*, `#care-{part}` its anchor, so a bookmark and the head's *New connector*
  // still land on the part and its dialog (page-system §7: a deeper page, not a dialog in a dialog).
  //
  // **The source definitions are not here.** Their trust order is the workspace's and decides
  // reconciliation in every hub; they stay in the administration beside the object types, which a
  // STRUCTURE holder reaches from *Manage sources* (user-experience.md §7.3).
  //
  // Only what the reader may do is drawn (P-05): *Upkeep* itself only for whoever may run something
  // in it; writing acts - deciding, closing a deviation, starting a round, importing - for a writer;
  // connecting a source for a STRUCTURE holder.
  import { tick, untrack } from 'svelte';
  import { Button, DataTable, Dialog, Icon, Spinner, Stack, type DataColumn } from '@hubtask/design-system/components';
  import type {
    CiCampaign, CiCampaignList, CiClass, CiConnector, CiConnectorList, CiDuplicate, CiDuplicateList, CiHold, CiHoldList, CiRun, CiRunList,
    CmdbHealth, ConfigurationItem, ConfigurationItemList,
  } from '@hubtask/sync-engine';

  import {
    campaignsPath, cisPath, classWord, cmdb, connectorsPath, duplicatesPath, healthPath, holdsPath, inventoryPath, runsPath, type CarePart, type Words,
  } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { live } from '../data/live.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { formatDateTime, formatPlainDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import Campaigns from './Campaigns.svelte';
  import Connectors from './Connectors.svelte';
  import Decisions from './Decisions.svelte';
  import Duplicates from './Duplicates.svelte';
  import ImportFile from './ImportFile.svelte';
  import { TODOS_SHOWN, careTodos, healthFigures, isByHand, sourceRows, type CareTodo, type FigureId, type SourceRow } from './inventoryscenes.ts';
  import Runs from './Runs.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
    /** Whether the reader holds STRUCTURE: connecting a source and managing the sources. */
    mayShape: boolean;
    /** The parts the reader may run, in their order. */
    parts: readonly CarePart[];
    /** The part the address names: drawn alone. */
    part?: CarePart;
    /** The head asked for a connector's or a round's dialog; `onasked` gives the ask back. */
    asked?: 'connector' | 'campaign';
    onasked?: () => void;
    /** A card asks for the same dialogs the head offers. */
    onask: (what: 'connector' | 'campaign') => void;
    /** An import finished: where its objects are. */
    onimported?: () => void;
  }

  const { hubId, classes, words, mayWrite, mayShape, parts, part, asked, onasked, onask, onimported }: Props = $props();
  // The overview's own acts write online and off the phone only (UC-AST-01 check 10); there it reads.
  const isWriting = $derived(!viewport.isCompact && live.state !== 'offline');
  const mayChange = $derived(mayWrite && isWriting);
  const mayConnect = $derived(mayShape && isWriting);

  $effect(() => {
    const at = part;
    if (!at) return;
    untrack(() => {
      void tick().then(() => document.getElementById(`care-${at}`)?.scrollIntoView({ block: 'start' }));
    });
  });

  // The reads of the overview; a part drawn alone reads its own.
  let health = $state<CmdbHealth | undefined>(undefined);
  let holds = $state<readonly CiHold[]>([]);
  let duplicates = $state<readonly CiDuplicate[]>([]);
  let campaigns = $state<readonly CiCampaign[] | undefined>(undefined);
  let connectors = $state<readonly CiConnector[]>([]);
  let runs = $state<readonly CiRun[]>([]);
  let cis = $state<readonly ConfigurationItem[] | undefined>(undefined);
  let names = $state<Record<string, string>>({});
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    if (part) return;
    return untrack(() => {
      const stops = [
        engine.subscribe<CmdbHealth>({ path: healthPath(at) }, (state) => {
          if (state.status === 'ready') health = state.data;
          if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
        }),
        engine.subscribe<CiHoldList>({ path: holdsPath(at) }, (state) => {
          if (state.status === 'ready') holds = state.data.data;
        }),
        engine.subscribe<CiDuplicateList>({ path: duplicatesPath(at) }, (state) => {
          if (state.status === 'ready') duplicates = state.data.data;
        }),
        engine.subscribe<CiCampaignList>({ path: campaignsPath(at) }, (state) => {
          if (state.status === 'ready') campaigns = state.data.data;
        }),
        engine.subscribe<CiConnectorList>({ path: connectorsPath(at) }, (state) => {
          if (state.status === 'ready') connectors = state.data.data;
        }),
        engine.subscribe<CiRunList>({ path: runsPath(at) }, (state) => {
          if (state.status === 'ready') runs = state.data.data;
        }),
        engine.subscribe<ConfigurationItemList>({ path: cisPath(at, { limit: 500 }) }, (state) => {
          if (state.status === 'ready') {
            cis = state.data.data;
            names = { ...names, ...Object.fromEntries(state.data.data.map((ci) => [ci.id, ci.name])) };
          }
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  const todos = $derived(careTodos(health, holds, duplicates));
  // The names a to-do needs beyond the objects read with the page, one by one and at most a few.
  $effect(() => {
    if (!cis) return;
    const unknown = [...new Set(todos.flatMap((each) => [each.ciId, ...(each.duplicate?.ci_ids ?? [])]))]
      .filter((id): id is string => Boolean(id) && !(id! in names))
      .slice(0, 50);
    untrack(() => {
      for (const id of unknown) {
        void cmdb.read(id).then((ci) => {
          if (ci) names = { ...names, [id]: ci.name };
        });
      }
    });
  });
  const nameOf = (id: string | undefined) => (id ? (names[id] ?? t('app.assets.unknown_ci')) : t('app.assets.unknown_ci'));

  // (1) The six figures, each a link to what it counts.
  const figures = $derived(health ? healthFigures(health, holds.length) : []);
  const staleDays = $derived(classes.find((each) => each.key === 'ci')?.staleness_days || 60);
  const figureHref = (id: FigureId) =>
    id === 'without_owner' ? `${inventoryPath(hubId, 'inventory')}?line=${encodeURIComponent('asset.owner: none')}`
    : id === 'duplicates' ? inventoryPath(hubId, 'duplicates')
    : id === 'correct' ? inventoryPath(hubId, 'runs')
    : inventoryPath(hubId, 'health');
  const figureLine = (id: FigureId, detail: number) =>
    id === 'complete' || id === 'correct' ? t(`app.care.line.${id}`, { count: detail }) : id === 'stale' ? t('app.care.line.stale', { days: staleDays }) : t(`app.care.line.${id}`);
  const figureValue = (value: number, isShare: boolean) => (isShare ? t('app.assets.percent', { value }) : String(value));

  // (2) To do.
  let isAllShown = $state(false);
  const shownTodos = $derived(isAllShown ? todos : todos.slice(0, TODOS_SHOWN));
  const ICON = { owner: 'user', relate: 'link', review: 'clock', compare: 'arrow-right-left', decide: 'copy' } as const;
  function sentence(todo: CareTodo): string {
    if (todo.kind === 'HOLD' && todo.hold) return t('app.care.todo.HOLD', { source: todo.hold.source, name: todo.hold.item.name || todo.hold.item.native_key || todo.hold.item.ref || '—' });
    if (todo.kind === 'DUPLICATE' && todo.duplicate) return t('app.care.todo.DUPLICATE', { first: nameOf(todo.duplicate.ci_ids[0]), second: nameOf(todo.duplicate.ci_ids[1]) });
    if (todo.kind === 'STALE') return t('app.care.todo.STALE', { name: nameOf(todo.ciId), days: staleDays });
    if (todo.kind === 'INCOMPLETE') return t('app.care.todo.INCOMPLETE', { name: nameOf(todo.ciId), count: Math.max(1, todo.missing?.length ?? 0) });
    return t(`app.care.todo.${todo.kind}`, { name: nameOf(todo.ciId) });
  }
  const objectHref = (todo: CareTodo) => `/cis/${todo.ciId}${todo.act === 'relate' ? '#ci-relations' : ''}`;

  // Compare: a deviation beside its run's decision on the same item, closed once settled.
  let comparing = $state<CiHold | undefined>(undefined);
  let compared = $state<CiRun | undefined>(undefined);
  let isComparing = $state(false);
  let isClosing = $state(false);
  function compare(hold: CiHold) {
    comparing = hold;
    compared = undefined;
    isComparing = true;
    cmdb.run(hold.run_id).then((run) => (compared = run), (error) => toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' }));
  }
  async function settle() {
    if (!comparing) return;
    isClosing = true;
    try {
      await cmdb.dismissHold(hubId, comparing.id);
      toast.show({ text: t('app.assets.dismissed'), tone: 'success' });
      isComparing = false;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isClosing = false;
    }
  }

  // Decide: the pair side by side, today's duplicates part narrowed to it.
  let deciding = $state<string | undefined>(undefined);
  let isDeciding = $state(false);
  function decide(duplicate: CiDuplicate) {
    deciding = duplicate.id;
    isDeciding = true;
  }

  // (3) The round.
  const openRound = $derived(campaigns?.find((each) => !each.closed_at));
  const pastRounds = $derived((campaigns ?? []).filter((each) => each.closed_at).length);

  // (5) Sources and runs.
  const sources = $derived(sourceRows(connectors, runs));
  const byHand = $derived(cis ? cis.filter((each) => !each.merged_into && isByHand(each)).length : undefined);
  type Source = SourceRow | { readonly key: 'by-hand'; readonly name: string };
  const sourceTable = $derived<Source[]>([...sources, ...(byHand !== undefined ? [{ key: 'by-hand' as const, name: t('app.care.by_hand') }] : [])]);
  const sourceColumns = $derived<DataColumn[]>([
    { id: 'source', label: t('app.care.col.source'), priority: 1 },
    { id: 'last', label: t('app.care.col.last'), priority: 2 },
    { id: 'result', label: t('app.care.col.result'), priority: 2 },
    { id: 'next', label: t('app.care.col.next'), priority: 3 },
  ]);
  const when = (at: string) => formatDateTime(at, messages.locale);
  let detail = $state<CiRun | undefined>(undefined);
  let isDetailOpen = $state(false);
  function openRun(row: Source) {
    if (!('run' in row) || !row.run?.id) return;
    const id = row.run.id;
    detail = undefined;
    isDetailOpen = true;
    cmdb.run(id).then((run) => (detail = run), (error) => toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' }));
  }
  const mayImport = $derived(parts.includes('import'));
</script>

{#if part}
  <Stack gap="200">
    <a class="back" href={inventoryPath(hubId, 'care')} data-care-back><Icon name="arrow-left" size="sm" /> {t('app.care.back')}</a>
    <section id={`care-${part}`} class="part" aria-labelledby={`care-${part}-heading`} data-care-part={part}>
      <h2 id={`care-${part}-heading`} class="heading">{t(`app.assets.section.${part}`)}</h2>
      {#if part === 'runs'}
        <Runs {hubId} {classes} {words} {mayWrite} />
      {:else if part === 'duplicates'}
        <Duplicates {hubId} {classes} {words} {mayWrite} />
      {:else if part === 'connectors'}
        <Connectors {hubId} {classes} {words} {mayWrite} isAsked={asked === 'connector'} {onasked} />
      {:else if part === 'campaigns'}
        <Campaigns {hubId} {classes} {words} {mayWrite} isAsked={asked === 'campaign'} {onasked} />
      {:else if part === 'import'}
        <ImportFile {hubId} {classes} {words} onopen={onimported} />
      {/if}
    </section>
  </Stack>
{:else if refusal}
  <p class="refusal" role="alert">{refusal}</p>
{:else}
  <div class="care" data-care>
    <!-- (1) The six figures. -->
    {#if !health}
      <Spinner label={t('app.assets.loading')} />
    {:else}
      <ul class="figures" aria-label={t('app.care.figures')} data-care-figures>
        {#each figures as figure (figure.id)}
          <li>
            <a class="figure" href={figureHref(figure.id)} data-care-figure={figure.id} data-wanting={figure.isWanting ? '' : undefined}>
              <span class="figure-title">{t(`app.care.figure.${figure.id}`)}</span>
              <span class="figure-value">{figureValue(figure.value, figure.isShare)}</span>
              <span class="figure-line">{figureLine(figure.id, figure.detail)}</span>
            </a>
          </li>
        {/each}
      </ul>
    {/if}

    <div class="middle">
      <!-- (2) To do. -->
      <section class="card todo" aria-labelledby="care-todo" data-care-todos>
        <h2 id="care-todo" class="card-title">{t('app.care.todo_title')}</h2>
        {#if todos.length === 0}
          <p class="meta">{t('app.care.todo_none')}</p>
        {:else}
          <ul class="rows">
            {#each shownTodos as todo (todo.key)}
              {@const said = sentence(todo)}
              {@const act = t(`app.care.act.${todo.act}`)}
              <li class="todo-row" data-care-todo={todo.kind}>
                <span class="glyph" aria-hidden="true"><Icon name={ICON[todo.act]} size="sm" /></span>
                <span class="said">{said}</span>
                {#if todo.act === 'compare' && todo.hold}
                  {@const hold = todo.hold}
                  <button type="button" class="act" aria-label={t('app.care.act_on', { act, what: said })} onclick={() => compare(hold)}>{act}</button>
                {:else if todo.act === 'decide' && todo.duplicate}
                  {@const pair = todo.duplicate}
                  <button type="button" class="act" aria-label={t('app.care.act_on', { act, what: said })} onclick={() => decide(pair)}>{act}</button>
                {:else}
                  <a class="act" href={objectHref(todo)} aria-label={t('app.care.act_on', { act, what: said })}>{act}</a>
                {/if}
              </li>
            {/each}
          </ul>
          {#if todos.length > TODOS_SHOWN}
            <div><Button size="sm" tone="subtle" onclick={() => (isAllShown = !isAllShown)}>{isAllShown ? t('app.care.todo_fewer') : t('app.care.todo_all', { count: todos.length })}</Button></div>
          {/if}
        {/if}
      </section>

      <div class="side">
        <!-- (3) The attestation round. -->
        <section id="care-campaigns" class="card" aria-labelledby="care-round" data-care-part="campaigns">
          <h2 id="care-round" class="card-title">{openRound?.name || t('app.care.round_title')}</h2>
          {#if !campaigns}
            <Spinner label={t('app.assets.loading')} />
          {:else if openRound}
            <p class="line">{t('app.care.round_due', { day: formatPlainDay(openRound.due_on, messages.locale) })}</p>
            <p class="line" data-care-round-score>{t('app.care.round_score', { confirmed: openRound.score.confirmed, total: openRound.score.total, disputed: openRound.score.disputed })}</p>
            <div class="acts"><a class="act" href={inventoryPath(hubId, 'campaigns')}>{t('app.care.round_open')}</a></div>
          {:else}
            <p class="meta">{t('app.care.round_none')}</p>
            {#if mayChange}<div class="acts"><Button size="sm" tone="secondary" icon="check" onclick={() => onask('campaign')}>{t('app.care.round_start')}</Button></div>{/if}
          {/if}
          {#if pastRounds > 0}<a class="more" href={inventoryPath(hubId, 'campaigns')}>{t('app.care.rounds_past', { count: pastRounds })}</a>{/if}
        </section>

        <!-- (4) The duplicates. -->
        <section id="care-duplicates" class="card" aria-labelledby="care-pairs" data-care-part="duplicates">
          <h2 id="care-pairs" class="card-title">{t('app.care.duplicates_title')}</h2>
          {#if duplicates.length === 0}
            <p class="meta">{t('app.care.duplicates_none')}</p>
          {:else}
            <ul class="rows">
              {#each duplicates.slice(0, 3) as pair (pair.id)}
                {@const said = pair.ci_ids.map(nameOf).join(' · ')}
                <li class="todo-row" data-care-pair={pair.id}>
                  <span class="said">{said}</span>
                  <button type="button" class="act" aria-label={t('app.care.act_on', { act: t('app.care.act.decide'), what: said })} onclick={() => decide(pair)}>{t('app.care.act.decide')}</button>
                </li>
              {/each}
            </ul>
            <a class="more" href={inventoryPath(hubId, 'duplicates')}>{t('app.care.duplicates_all')}</a>
          {/if}
        </section>
      </div>
    </div>

    <!-- (5) Sources and runs. -->
    <section id="care-connectors" class="card" aria-labelledby="care-sources" data-care-part="connectors">
      <h2 id="care-sources" class="card-title">{t('app.care.sources_title')}</h2>
      <div id="care-runs" data-care-part="runs">
        <DataTable label={t('app.care.sources_title')} columns={sourceColumns} rows={sourceTable} key={(row: Source) => row.key}
          onopen={(row: Source) => openRun(row)}>
          {#snippet cell(row: Source, column: DataColumn)}
            {#if row.key === 'by-hand'}
              {#if column.id === 'source'}<span data-care-source="by-hand">{row.name}</span>
              {:else if column.id === 'result'}{t('app.care.by_hand_count', { count: byHand ?? 0 })}
              {:else}—{/if}
            {:else if 'run' in row || 'connector' in row}
              {#if column.id === 'source'}<span data-care-source={row.name} data-connector={row.connector?.key}>{row.name}</span>
              {:else if column.id === 'last'}{row.lastAt ? when(row.lastAt) : '—'}
              {:else if column.id === 'result'}{row.read !== undefined ? t('app.care.result', { read: row.read, changed: row.changed ?? 0 }) : '—'}
              {:else}{row.nextAt ? when(row.nextAt) : row.connector && !row.connector.enabled ? t('app.care.paused') : '—'}{/if}
            {/if}
          {/snippet}
        </DataTable>
      </div>
      <div class="acts" id={mayImport ? 'care-import' : undefined} data-care-part={mayImport ? 'import' : undefined}>
        {#if mayConnect}<Button size="sm" tone="subtle" icon="plus" onclick={() => onask('connector')}>{t('app.care.connect')}</Button>{/if}
        {#if mayImport && mayChange}<a class="act" href={inventoryPath(hubId, 'import')}><Icon name="cloud-upload" size="sm" /> {t('app.inventory.import')}</a>{/if}
        <span class="links">
          <a class="more" href={inventoryPath(hubId, 'runs')}>{t('app.care.runs_all')}</a>
          {#if parts.includes('connectors')}<a class="more" href={inventoryPath(hubId, 'connectors')}>{t('app.care.connectors_all')}</a>{/if}
          {#if mayShape}<a class="more" href="/assets/sources">{t('app.care.sources_manage')}</a>{/if}
        </span>
      </div>
    </section>
  </div>
{/if}

{#if isComparing && comparing}
  <Dialog size="l" bind:isOpen={isComparing} title={t('app.care.compare_title', { source: comparing.source })} dismissLabel={t('app.assets.cancel')}
    primary={mayChange ? { label: t('app.care.compare_settled'), isBusy: isClosing, onclick: () => void settle() } : undefined}>
    <Stack gap="150">
      <p class="meta">{t('app.care.compare_hint')}</p>
      <dl class="pairs">
        <dt>{t('app.assets.name_label')}</dt><dd>{comparing.item.name || comparing.item.native_key || comparing.item.ref || '—'}</dd>
        <dt>{t('app.assets.class_label')}</dt><dd>{classWord(comparing.item.class, classes, words)}</dd>
        <dt>{t('app.care.col.source')}</dt><dd>{t('app.assets.hold_from', { source: comparing.source, when: when(comparing.created_at) })}</dd>
        <dt>{t('app.care.col.result')}</dt><dd>{messages.has(comparing.reason) ? t(comparing.reason) : comparing.reason}</dd>
      </dl>
      {#if !compared}
        <Spinner label={t('app.assets.loading')} />
      {:else}
        <Decisions decisions={(compared.decisions ?? []).filter((each) => each.ref === comparing?.item.ref)} {classes} {words} />
      {/if}
    </Stack>
  </Dialog>
{/if}

{#if isDeciding && deciding}
  <Dialog size="l" bind:isOpen={isDeciding} title={t('app.care.decide_title')} dismissLabel={t('app.sheet_close')}>
    <Duplicates {hubId} {classes} {words} mayWrite={mayChange} only={deciding} />
  </Dialog>
{/if}

{#if isDetailOpen}
  <Dialog size="l" bind:isOpen={isDetailOpen} title={detail ? t('app.care.run_title', { source: detail.source, when: when(detail.started_at) }) : t('app.assets.loading')} dismissLabel={t('app.sheet_close')}>
    {#if !detail}
      <Spinner label={t('app.assets.loading')} />
    {:else}
      <Stack gap="150">
        <p class="meta">{t('app.assets.run_counts', {
          created: detail.counts.created, updated: detail.counts.updated, unchanged: detail.counts.unchanged,
          held: detail.counts.held, duplicates: detail.counts.duplicates, refused: detail.counts.refused,
        })}</p>
        <Decisions decisions={detail.decisions ?? []} {classes} {words} />
      </Stack>
    {/if}
  </Dialog>
{/if}

<style>
  .care { display: grid; gap: var(--sp-200); min-inline-size: 0; }
  .part { display: grid; gap: var(--sp-100); scroll-margin-block-start: var(--sp-400); }
  .heading { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .back { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-090); text-decoration: none; justify-self: start; }
  .back:hover { color: var(--text-primary); text-decoration: underline; }
  .figures {
    display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, var(--layout-field-xs)), 1fr)); gap: var(--sp-150);
    margin: 0; padding: 0; list-style: none;
  }
  .figure {
    display: grid; gap: var(--sp-025); padding: var(--sp-150) var(--sp-200); block-size: 100%; box-sizing: border-box;
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface);
    color: var(--text-primary); text-decoration: none;
  }
  .figure:hover { background: var(--bg-surface-hover); }
  .figure-title { color: var(--text-secondary); font-size: var(--fs-090); }
  .figure-value { font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  /* A figure found wanting is a status: on its surface, not in a colour alone (design-system.md §6 rule 3). */
  .figure[data-wanting] { border-color: var(--status-warning-border); background: var(--status-warning-surface); }
  .figure[data-wanting] .figure-value { color: var(--status-warning-text); }
  .figure-line { color: var(--text-secondary); font-size: var(--fs-075); }
  .middle { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); align-items: start; }
  /* design-system-lint-ignore: `primitive.breakpoint.large` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) { .middle { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); } }
  .side { display: grid; gap: var(--sp-200); }
  .card {
    display: grid; align-content: start; gap: var(--sp-100); padding: var(--sp-200); min-inline-size: 0;
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface);
    scroll-margin-block-start: var(--sp-400);
  }
  .card-title { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .todo-row { display: flex; align-items: center; gap: var(--sp-100); padding-block: var(--sp-050); min-block-size: var(--layout-target); }
  .glyph { display: inline-flex; color: var(--text-subtle); }
  .said { flex: 1 1 auto; min-inline-size: 0; overflow-wrap: anywhere; }
  /* Every to-do's one act looks the same, whether it opens the object or a dialog. */
  .act {
    display: inline-flex; align-items: center; gap: var(--sp-050); padding: 0; border: 0; background: none; cursor: pointer;
    font: inherit; color: var(--text-primary); font-weight: var(--fw-semibold); text-decoration: none;
  }
  .act:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-xs); }
  .act:hover, .more:hover { text-decoration: underline; }
  .more { color: var(--text-secondary); font-size: var(--fs-090); text-decoration: none; }
  .acts { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100) var(--sp-200); scroll-margin-block-start: var(--sp-400); }
  .links { display: inline-flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-200); margin-inline-start: auto; }
  .line { margin: 0; color: var(--text-secondary); }
  .meta { color: var(--text-subtle); font-size: var(--fs-090); margin: 0; }
  .refusal { margin: 0; color: var(--text-danger); }
  .pairs { display: grid; grid-template-columns: minmax(0, max-content) minmax(0, 1fr); gap: var(--sp-050) var(--sp-150); margin: 0; }
  .pairs dt { color: var(--text-secondary); }
  .pairs dd { margin: 0; overflow-wrap: anywhere; }
</style>
