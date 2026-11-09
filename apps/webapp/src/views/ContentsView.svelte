<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *All contents* of a hub (the navigation concept §2.4 part 4), a list page (page-system §7.1):
  // its pages and whiteboards as one table, the filter bar above it - the space's search, the kind,
  // the order, a label, how recently a page changed and the reader's own pages as chips, and the
  // count (UC-DOC-01 check 9).
  // The tree itself is the sidebar's *Contents*. A whiteboard is made here under a page or at the
  // top (UC-CNV-10 check 6), as ＋ in *Contents* asks; a page is made on the pages' own screen.
  import { untrack } from 'svelte';
  import {
    Banner, Button, DataTable, Disclosure, EmptyState, FilterBar, ListPage, Radio, Skeleton, Switch, UploadField,
    type DataColumn, type Filter, type PageHead,
  } from '@hubtask/design-system/components';
  import type { PageHitList, PageLabelCounts, PageList, WhiteboardList } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { contentsAsk } from '../lib/data/contentsask.ts';
  import { engine } from '../lib/data/engine.ts';
  import { CHANGED_WITHIN, changedFrom, pages, pagesPath, spaceLabelsPath, type ChangedWithin } from '../lib/data/pages.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { boardsPath, placeCarried, readNative, whiteboards } from '../lib/data/whiteboards.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { formatPlainDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { elementId } from '@hubtask/whiteboard';
  import BoardTemplates, { STARTS, startElements, templateOf, type BoardStart, type RetroFormat } from '../lib/whiteboards/BoardTemplates.svelte';
  import CreateDialog from '../lib/product/CreateDialog.svelte';

  interface Props {
    hubId: string;
    /** `whiteboards` or `pages` narrows the table to that kind (`?kind=`). */
    kind?: string;
    /** `whiteboard` (or `1` with the boards' kind) asks for a new whiteboard (`?new=`). */
    create?: string;
    /** The page a new whiteboard goes under (`?parent=`). */
    parentId?: string;
    /** The template card a new whiteboard starts from (`?start=`), as the boards' home picked it. */
    startAsked?: string;
    onnavigate: (path: string) => void;
    onreplace?: (path: string) => void;
  }
  const { hubId, kind, create, parentId, startAsked, onnavigate, onreplace }: Props = $props();

  const hub = $derived(containers.find(hubId));
  $effect(() => frame.entitle(hub ? t('app.nav.contents_of', { hub: hub.name }) : t('app.nav.all_contents')));
  $effect(() => {
    const at = hubId;
    return untrack(() => people.open({ hubId: at }));
  });
  const mayWrite = $derived(holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted');

  let pageList = $state<PageList['data'] | undefined>(undefined);
  let boardList = $state<WhiteboardList['data'] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    return untrack(() => {
      const stops = [
        engine.subscribe<PageList>({ path: pagesPath(at) }, (next) => {
          if (next.status === 'ready') pageList = next.data.data;
          if (next.status === 'failed') { pageList = []; refusal = renderProblem(next.error, messages).message; }
        }),
        engine.subscribe<WhiteboardList>({ path: `${boardsPath(at)}?order=TREE` }, (next) => {
          if (next.status === 'ready') boardList = next.data.data;
          if (next.status === 'failed') boardList = [];
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  type Kind = 'all' | 'pages' | 'whiteboards';
  let shownKind = $state<Kind>(untrack(() => (kind === 'pages' || kind === 'whiteboards' ? kind : 'all')));
  let order = $state<'name' | 'changed'>('name');
  const rows = $derived.by(() => {
    const all = [
      ...(shownKind === 'whiteboards' ? [] : (pageList ?? []).filter((each) => !each.template).map((each) => ({ id: each.id, kind: 'page' as const, title: each.title, at: each.updated_at ?? '' }))),
      ...(shownKind === 'pages' ? [] : (boardList ?? []).map((each) => ({ id: each.id, kind: 'whiteboard' as const, title: each.title, at: each.updated_at ?? '' }))),
    ];
    return order === 'name'
      ? all.sort((a, b) => a.title.localeCompare(b.title, messages.locale))
      : all.sort((a, b) => (a.at < b.at ? 1 : a.at > b.at ? -1 : 0));
  });
  const isReading = $derived(pageList === undefined || boardList === undefined);
  const columns = $derived<DataColumn[]>([
    { id: 'title', label: t('app.whiteboards.board_title'), priority: 1 },
    { id: 'kind', label: t('app.nav.kind_label') },
    { id: 'changed', label: t('app.pages.changed') },
  ]);
  const kinds = $derived([
    { value: 'all', label: t('app.nav.kind_all') },
    { value: 'pages', label: t('app.nav.kind_pages') },
    { value: 'whiteboards', label: t('app.nav.kind_whiteboards') },
  ]);
  const orders = $derived([{ value: 'name', label: t('app.nav.mode_name') }, { value: 'changed', label: t('app.nav.mode_recent') }]);

  // --- The space's search (UC-DOC-01 check 9) ---------------------------------------------------
  let words = $state('');
  let hits = $state<PageHitList['data'] | undefined>(undefined);
  const countLabel = $derived(
    hits !== undefined ? t('app.page.showing', { count: String(hits.length) }) : isReading ? undefined : t('app.page.found', { shown: String(rows.length), count: String((pageList ?? []).filter((each) => !each.template).length + (boardList ?? []).length) }),
  );
  let generation = 0;
  let chosenLabel = $state(untrack(() => contentsAsk.label));
  contentsAsk.label = '';
  let ownOnly = $state(false);
  let changedWithin = $state<ChangedWithin>('any');
  let facet = $state<PageLabelCounts['data']>([]);
  $effect(() => {
    const at = hubId;
    return untrack(() => engine.subscribe<PageLabelCounts>({ path: spaceLabelsPath(at) }, (next) => {
      if (next.status === 'ready') facet = next.data.data;
    }));
  });
  const activeCount = $derived((shownKind === 'all' ? 0 : 1) + (chosenLabel ? 1 : 0) + (ownOnly ? 1 : 0) + (changedWithin === 'any' ? 0 : 1));
  const changedOptions = $derived((Object.keys(CHANGED_WITHIN) as ChangedWithin[]).map((value) => ({ value, label: t(`app.pages.changed_${value}`) })));
  // The chips (PS-7): each names its choice; the space's facets only where it has labels.
  const chips = $derived<Filter[]>([
    { id: 'kind', label: t('app.nav.contents_chip_kind', { kind: kinds.find((each) => each.value === shownKind)?.label ?? '' }), isActive: shownKind !== 'all' },
    { id: 'order', label: t('app.nav.contents_chip_order', { order: orders.find((each) => each.value === order)?.label ?? '' }) },
    ...(facet.length > 0 ? [{ id: 'label', label: t('app.nav.contents_chip_label', { label: chosenLabel || t('app.page.any') }), isActive: chosenLabel !== '' }] : []),
    { id: 'changed', label: t('app.nav.contents_chip_changed', { when: t(`app.pages.changed_${changedWithin}`) }), isActive: changedWithin !== 'any' },
    { id: 'own', label: t('app.pages.own_only'), isActive: ownOnly },
  ]);
  function clearFilters() {
    shownKind = 'all';
    chosenLabel = '';
    changedWithin = 'any';
    ownOnly = false;
  }
  const hitColumns = $derived<DataColumn[]>([
    { id: 'title', label: t('app.whiteboards.board_title'), priority: 1 },
    { id: 'excerpt', label: t('app.nav.contents_excerpt'), priority: 2 },
  ]);
  type Hit = NonNullable<typeof hits>[number];
  $effect(() => {
    const asked = words.trim();
    const at = hubId;
    const narrow = {
      ...(chosenLabel ? { label: chosenLabel } : {}),
      ...(ownOnly && actor.account ? { author_id: actor.account.id } : {}),
    };
    const from = changedFrom(changedWithin, new Date());
    if (asked === '' && !narrow.label && !narrow.author_id) {
      hits = undefined;
      return;
    }
    const mine = (generation += 1);
    const timer = setTimeout(() => {
      pages.search(asked, at, 20, { ...narrow, ...(from ? { updated_from: from } : {}) }).then(
        (found) => { if (mine === generation) hits = found; },
        () => { if (mine === generation) hits = []; },
      );
    }, 250);
    return () => clearTimeout(timer);
  });

  // --- A new whiteboard, under a page or at the top (UC-CNV-10 check 6) -------------------------
  // Made from the template gallery (§5.2: "the board with the template gallery"), as the boards'
  // own home makes one; the name a template suggests is kept until the person types their own.
  let isMaking = $state(false);
  let titleDraft = $state('');
  let start = $state<BoardStart>('BLANK');
  let format = $state<RetroFormat>('RETRO_START_STOP_CONTINUE');
  const suggested = (choice: BoardStart) => (choice === 'BLANK' ? '' : t(`app.whiteboards.start_${choice.toLowerCase()}`));
  let lastSuggested = '';
  $effect(() => {
    const name = suggested(start);
    untrack(() => {
      if (titleDraft.trim() === '' || titleDraft === lastSuggested) titleDraft = name;
      lastSuggested = name;
    });
  });
  function startMaking(choice: BoardStart = 'BLANK') {
    start = choice;
    titleDraft = suggested(choice);
    lastSuggested = titleDraft;
    isMaking = true;
  }
  let makeFailure = $state<string | undefined>(undefined);
  let isBusy = $state(false);
  let wasAsked = false;
  // The page asked for is kept: the address lets go of the ask once it is taken.
  const parentAsked = untrack(() => parentId);
  $effect(() => {
    const asked = create === 'whiteboard' || (create === '1' && kind === 'whiteboards');
    if (!asked || wasAsked || !mayWrite) return;
    wasAsked = true;
    untrack(() => {
      startMaking(STARTS.find((each) => each === startAsked) ?? 'BLANK');
      (onreplace ?? onnavigate)(`/hubs/${hubId}/contents${kind ? `?kind=${kind}` : ''}`);
    });
  });
  const under = $derived(parentAsked ? pageList?.find((each) => each.id === parentAsked) : undefined);

  // A board may start from an exported native document instead of a template (UC-CNV-01).
  let imported = $state<ReturnType<typeof readNative>>(undefined);
  let importFailure = $state<string | undefined>(undefined);
  let isImportOpen = $state(false);
  async function readFile(file: File) {
    const native = readNative(await file.text());
    importFailure = native ? undefined : t('app.whiteboards.import_not_native');
    imported = native ?? undefined;
    if (native) titleDraft = native.title || titleDraft;
  }

  const contentsHead = $derived<PageHead>({
    title: t('app.nav.all_contents'),
    primary: mayWrite ? { label: t('app.whiteboards.new'), icon: 'plus', onclick: () => startMaking() } : undefined,
    breadcrumb: hub
      ? {
          trail: [{ id: hubId, label: hub.name, href: `/hubs/${hubId}` }, { id: 'contents', label: t('app.nav.all_contents') }],
          label: t('app.pages.trail'),
          expandLabel: t('app.admin.expand_trail'),
          onnavigate: () => onnavigate(`/hubs/${hubId}`),
        }
      : undefined,
  });

  async function make() {
    const title = titleDraft.trim();
    if (!title) return;
    isBusy = true;
    makeFailure = undefined;
    try {
      const name = templateOf(start, format);
      const elements = imported?.elements ?? (name ? startElements(name, (key) => t(`app.whiteboards.word_${key}`), elementId) : undefined);
      const made = await whiteboards.create(hubId, {
        title,
        ...(parentAsked ? { parent_page_id: parentAsked } : {}),
        ...(elements && elements.length > 0 ? { elements: elements as never } : {}),
      });
      // An export's pictures and files are placed on the new board, or its readers read none.
      const failed = imported ? await placeCarried(imported.elements, (id) => whiteboards.placeMedia(made.id, id)) : [];
      if (failed.length > 0) toast.show({ text: t('app.whiteboards.carried_unplaced', { count: String(failed.length) }), tone: 'warning' });
      imported = undefined;
      isMaking = false;
      titleDraft = '';
      onnavigate(`/whiteboards/${made.id}`);
    } catch (error) {
      makeFailure = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }
</script>

{#snippet picker(filter: Filter)}
  {#if filter.id === 'kind'}
    <Radio label={t('app.nav.kind_label')} bind:value={() => shownKind, (next) => (shownKind = next as Kind)} options={kinds} />
  {:else if filter.id === 'order'}
    <Radio label={t('app.nav.order_contents')} bind:value={() => order, (next) => (order = next as 'name' | 'changed')} options={orders} />
  {:else if filter.id === 'label'}
    <div data-page-search-narrow>
      <Radio label={t('app.nav.contents_label')} bind:value={chosenLabel}
        options={[{ value: '', label: t('app.page.any') }, ...facet.slice(0, 12).map((each) => ({ value: each.label, label: t('app.pages.label_facet', { label: each.label, count: String(each.pages) }) }))]} />
    </div>
  {:else if filter.id === 'changed'}
    <Radio label={t('app.pages.changed')} bind:value={() => changedWithin, (next) => (changedWithin = next as ChangedWithin)} options={changedOptions} />
  {:else}
    <Switch label={t('app.pages.own_only')} checked={ownOnly} onchange={(event: Event) => (ownOnly = (event.currentTarget as HTMLInputElement).checked)} />
  {/if}
{/snippet}

<ListPage head={contentsHead}>
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.pages.search')}
      clearSearchLabel={t('app.pages.search_clear')}
      bind:search={words}
      filters={chips}
      {picker}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(activeCount) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(hits?.length ?? rows.length) })}
      clearLabel={t('app.page.clear')}
      onclear={clearFilters}
      {countLabel}
    />
  {/snippet}

  {#if hits !== undefined}
    <DataTable label={t('app.pages.search_results')} columns={hitColumns} rows={hits} key={(hit: Hit) => hit.id} href={(hit: Hit) => `/pages/${hit.id}`} rowLabel={(hit: Hit) => hit.title}>
      {#snippet cell(hit: Hit, column: DataColumn)}
        {#if column.id === 'title'}<a href={`/pages/${hit.id}`}>{hit.title}</a>
        {:else}<span class="excerpt">{hit.excerpt.replace(/[«»]/gu, '')}</span>{/if}
      {/snippet}
      {#snippet empty()}<EmptyState kind="filtered" title={t('app.pages.search_none')} />{/snippet}
    </DataTable>
  {:else if refusal}
    <p class="refusal">{refusal}</p>
  {:else if isReading}
    <div aria-busy="true"><Skeleton lines={5} /></div>
  {:else if rows.length === 0 && shownKind === 'whiteboards' && mayWrite}
    <!-- No board yet: one starts from a template in one press (user-experience.md §4, §5.5). -->
    <div class="start" data-boards-start>
      <EmptyState kind="unused" title={t('app.whiteboards.empty')} description={t('app.whiteboards.empty_hint')} icon="layout-template" />
      <BoardTemplates chosen={'BLANK'} {format} onpick={(choice) => startMaking(choice)} />
    </div>
  {:else if rows.length === 0}
    <EmptyState kind="unused" title={t('app.nav.contents_empty')} />
  {:else}
    <DataTable label={t('app.nav.all_contents')} {columns} {rows} key={(row) => row.id} rowLabel={(row) => row.title} href={(row) => (row.kind === 'page' ? `/pages/${row.id}` : `/whiteboards/${row.id}`)}>
      {#snippet cell(row, column: DataColumn)}
        {#if column.id === 'title'}<a data-content-row={row.kind} href={row.kind === 'page' ? `/pages/${row.id}` : `/whiteboards/${row.id}`}>{row.title}</a>
        {:else if column.id === 'kind'}{t(row.kind === 'page' ? 'app.nav.kind_page' : 'app.nav.kind_whiteboard')}
        {:else}<span class="quiet">{row.at ? formatPlainDay(row.at.slice(0, 10), messages.locale) : ''}</span>{/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#if isMaking}
  <CreateDialog
    bind:isOpen={isMaking}
    title={under ? t('app.nav.create_under', { name: under.title }) : t('app.whiteboards.new')}
    cancelLabel={t('app.links.cancel')}
    nameLabel={t('app.whiteboards.board_title')}
    bind:name={titleDraft}
    maxlength={200}
    isWide
    failure={makeFailure}
    primary={{ label: t('app.whiteboards.create'), busyLabel: isBusy ? t('app.whiteboards.making') : undefined, disabledReason: titleDraft.trim() ? undefined : t('app.whiteboards.needs_title'), onclick: () => void make() }}
    onClose={() => { titleDraft = ''; makeFailure = undefined; start = 'BLANK'; imported = undefined; importFailure = undefined; isImportOpen = false; }}
  >
    {#snippet gallery()}
      {#if imported}
        <Banner tone="info">{t('app.whiteboards.import_ready', { count: String(imported.elements.length) })}</Banner>
      {:else}
        <BoardTemplates bind:chosen={start} bind:format />
      {/if}
    {/snippet}
    <Disclosure label={t('app.whiteboards.import_fold')} description={t('app.whiteboards.import_fold_hint')} tone="quiet" bind:isOpen={isImportOpen}>
      <UploadField
        label={t('app.whiteboards.import')}
        chooseLabel={t('app.import.choose')}
        dropLabel={t('app.media.drop')}
        accept="application/json,.json"
        error={importFailure}
        onChoose={(file) => void readFile(file)}
      />
      {#if imported}<Button size="sm" tone="subtle" icon="x" onclick={() => (imported = undefined)}>{t('app.whiteboards.import_drop')}</Button>{/if}
    </Disclosure>
  </CreateDialog>
{/if}

<style>
  .excerpt { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
