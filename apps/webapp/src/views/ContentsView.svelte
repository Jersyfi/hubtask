<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *All contents* of a hub (the navigation concept §2.4 part 4): its pages and whiteboards as one
  // table, narrowed to a kind and sorted by name or by the last change, with the space's search -
  // its words, labels, the reader's own pages and how recently a page changed (UC-DOC-01 check 9).
  // The tree itself is the sidebar's *Contents*. A whiteboard is made here under a page or at the
  // top (UC-CNV-10 check 6), as ＋ in *Contents* asks; a page is made on the pages' own screen.
  import { untrack } from 'svelte';
  import {
    Button, Dialog, EmptyState, Inline, Input, PageHeader, SearchField, Select, Skeleton, Stack, Switch, Table, ViewSwitcher,
  } from '@hubtask/design-system/components';
  import type { PageHitList, PageLabelCounts, PageList, WhiteboardList } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { contentsAsk } from '../lib/data/contentsask.ts';
  import { engine } from '../lib/data/engine.ts';
  import { CHANGED_WITHIN, changedFrom, pages, pagesPath, spaceLabelsPath, type ChangedWithin } from '../lib/data/pages.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { boardsPath, whiteboards } from '../lib/data/whiteboards.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatPlainDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    hubId: string;
    /** `whiteboards` or `pages` narrows the table to that kind (`?kind=`). */
    kind?: string;
    /** `whiteboard` (or `1` with the boards' kind) asks for a new whiteboard (`?new=`). */
    create?: string;
    /** The page a new whiteboard goes under (`?parent=`). */
    parentId?: string;
    onnavigate: (path: string) => void;
    onreplace?: (path: string) => void;
  }
  const { hubId, kind, create, parentId, onnavigate, onreplace }: Props = $props();

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
  const columns = $derived([
    { id: 'title', label: t('app.whiteboards.board_title') },
    { id: 'kind', label: t('app.nav.kind_label') },
    { id: 'changed', label: t('app.pages.changed') },
  ]);
  const kinds = $derived([
    { id: 'all', label: t('app.nav.kind_all') },
    { id: 'pages', label: t('app.nav.kind_pages') },
    { id: 'whiteboards', label: t('app.nav.kind_whiteboards') },
  ]);

  // --- The space's search (UC-DOC-01 check 9) ---------------------------------------------------
  let words = $state('');
  let hits = $state<PageHitList['data'] | undefined>(undefined);
  let generation = 0;
  let chosenLabel = $state(untrack(() => contentsAsk.label));
  contentsAsk.label = '';
  let ownOnly = $state(false);
  let changedWithin = $state<ChangedWithin>('any');
  let isFilterOpen = $state(untrack(() => chosenLabel !== ''));
  let facet = $state<PageLabelCounts['data']>([]);
  $effect(() => {
    const at = hubId;
    return untrack(() => engine.subscribe<PageLabelCounts>({ path: spaceLabelsPath(at) }, (next) => {
      if (next.status === 'ready') facet = next.data.data;
    }));
  });
  const filters = $derived((chosenLabel ? 1 : 0) + (ownOnly ? 1 : 0) + (changedWithin === 'any' ? 0 : 1));
  const changedOptions = $derived((Object.keys(CHANGED_WITHIN) as ChangedWithin[]).map((value) => ({ value, label: t(`app.pages.changed_${value}`) })));
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
  let isMaking = $state(false);
  let titleDraft = $state('');
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
      isMaking = true;
      (onreplace ?? onnavigate)(`/hubs/${hubId}/contents${kind ? `?kind=${kind}` : ''}`);
    });
  });
  const under = $derived(parentAsked ? pageList?.find((each) => each.id === parentAsked) : undefined);
  async function make() {
    const title = titleDraft.trim();
    if (!title) return;
    isBusy = true;
    makeFailure = undefined;
    try {
      const made = await whiteboards.create(hubId, { title, ...(parentAsked ? { parent_page_id: parentAsked } : {}) });
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

<Stack gap="300">
  <PageHeader
    title={t('app.nav.all_contents')}
    isTitleInBar={viewport.isCompact}
    primary={mayWrite ? { label: t('app.whiteboards.new'), icon: 'plus', onclick: () => (isMaking = true) } : undefined}
    breadcrumb={hub
      ? {
          trail: [{ id: 'place', label: hub.name, href: `/hubs/${hubId}` }, { id: 'contents', label: t('app.nav.all_contents') }],
          label: t('app.pages.trail'),
          expandLabel: t('app.admin.expand_trail'),
          onnavigate: () => onnavigate(`/hubs/${hubId}`),
        }
      : undefined}
  />

  <Inline gap="100" align="end" isWrapping>
    <div class="search"><SearchField label={t('app.pages.search')} clearLabel={t('app.pages.search_clear')} bind:value={words} size="sm" /></div>
    <Button size="sm" tone={filters > 0 ? 'secondary' : 'subtle'} icon="funnel" aria-expanded={isFilterOpen} onclick={() => (isFilterOpen = !isFilterOpen)}>
      {filters > 0 ? t('app.pages.filter_count', { count: String(filters) }) : t('app.pages.filter')}
    </Button>
  </Inline>
  {#if isFilterOpen}
    <div class="narrow" data-page-search-narrow>
      {#if facet.length > 0}
        <Inline gap="050" isWrapping>
          {#each facet.slice(0, 12) as each (each.label)}
            <Button size="sm" tone={chosenLabel === each.label ? 'secondary' : 'subtle'} aria-pressed={chosenLabel === each.label} onclick={() => (chosenLabel = chosenLabel === each.label ? '' : each.label)}>
              {t('app.pages.label_facet', { label: each.label, count: String(each.pages) })}
            </Button>
          {/each}
        </Inline>
      {/if}
      <Select label={t('app.pages.changed')} size="sm" bind:value={changedWithin} options={changedOptions} />
      <Switch label={t('app.pages.own_only')} checked={ownOnly} onchange={(event: Event) => (ownOnly = (event.currentTarget as HTMLInputElement).checked)} />
    </div>
  {/if}

  {#if hits !== undefined}
    <ul class="hits" aria-label={t('app.pages.search_results')}>
      {#each hits as hit (hit.id)}
        <li>
          <a href={`/pages/${hit.id}`}>{hit.title}</a>
          <span class="excerpt">{hit.excerpt.replace(/[«»]/gu, '')}</span>
        </li>
      {:else}
        <li class="quiet">{t('app.pages.search_none')}</li>
      {/each}
    </ul>
  {:else}
    <Inline gap="100" align="end" justify="between" isWrapping>
      <ViewSwitcher label={t('app.nav.kind_label')} views={kinds} selected={shownKind} onselect={(id) => (shownKind = id as Kind)} />
      <Select label={t('app.nav.order_contents')} size="sm" bind:value={order} options={[{ value: 'name', label: t('app.nav.mode_name') }, { value: 'changed', label: t('app.nav.mode_recent') }]} />
    </Inline>
    {#if refusal}
      <p class="refusal">{refusal}</p>
    {:else if isReading}
      <div aria-busy="true"><Skeleton lines={5} /></div>
    {:else if rows.length === 0}
      <EmptyState kind="unused" title={t('app.nav.contents_empty')} />
    {:else}
      <Table label={t('app.nav.all_contents')} isLabelHidden {columns}>
        {#each rows as row (row.id)}
          <tr data-content-row={row.kind}>
            <th scope="row"><a href={row.kind === 'page' ? `/pages/${row.id}` : `/whiteboards/${row.id}`}>{row.title}</a></th>
            <td>{t(row.kind === 'page' ? 'app.nav.kind_page' : 'app.nav.kind_whiteboard')}</td>
            <td class="quiet">{row.at ? formatPlainDay(row.at.slice(0, 10), messages.locale) : ''}</td>
          </tr>
        {/each}
      </Table>
    {/if}
  {/if}
</Stack>

<Dialog bind:isOpen={isMaking} title={under ? t('app.nav.create_under', { name: under.title }) : t('app.whiteboards.new')} dismissLabel={t('app.links.cancel')}>
  <Stack gap="150">
    <Input label={t('app.whiteboards.board_title')} bind:value={titleDraft} />
    {#if makeFailure}<p class="refusal" role="alert">{makeFailure}</p>{/if}
    <Inline gap="100" justify="end">
      <Button tone="secondary" onclick={() => (isMaking = false)}>{t('app.links.cancel')}</Button>
      <Button tone="primary" busyLabel={isBusy ? t('app.whiteboards.making') : undefined} disabledReason={titleDraft.trim() ? undefined : t('app.whiteboards.needs_title')} onclick={() => void make()}>{t('app.whiteboards.create')}</Button>
    </Inline>
  </Stack>
</Dialog>

<style>
  .search { flex: 1 1 24ch; min-inline-size: 0; }
  .narrow { display: grid; gap: var(--sp-050); }
  .hits { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .hits li { display: grid; gap: var(--sp-025); padding-block: var(--sp-050); }
  .hits a, th a { color: var(--text-primary); text-decoration: none; overflow-wrap: anywhere; }
  .hits a:hover, th a:hover { text-decoration: underline; }
  .excerpt { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
