<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Pages* in a hub (M3, UC-DOC-01 … UC-DOC-12), in three regions (user-experience.md §7.1): the
  // space's tree with its search, the open page with a one-row head, and a side sheet the head
  // toggles - comments (on the page or at a block), history with two versions compared, what links
  // to it, who reads it and its details. A space opens on its home page; an empty one offers its
  // templates in one press. A page is verified until a date, moved, exported and put in the trash
  // with the pages under it, from the page's menu.
  //
  // **What the reader may never do is not drawn** (P-05): a reader without WRITE_ITEMS on the hub
  // reads the tree and the page and is offered no write; the page's own `writable` says the same
  // for the page.

  import { untrack } from 'svelte';
  import {
    AvatarGroup, Badge, Banner, Breadcrumb, Button, Dialog, EmptyState, IconButton, Inline, Input, Menu, SearchField, Select, Spinner, Stack, Switch,
    TemplateCard, TemplateGallery, Textarea, VisuallyHidden,
  } from '@hubtask/design-system/components';
  import { compareBodies, type Passage, type SelectedBlock } from '@hubtask/editor';
  import type {
    Page, PageBacklinkList, PageComment, PageCommentList, PageHitList, PageLabelCounts, PageList, PageRead, PageVersionList,
    PageVersionRead,
  } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import {
    CHANGED_WITHIN, EXPORT_TYPES, SHIPPED_SPACES, SHIPPED_TEMPLATES, draftStateOf, hasSubpages, homePageOf, shippedSpace, VERIFY_DAYS, ancestorsOf, changedFrom, spaceLabelsPath, exportBytes, backlinksPath, exportFileName, pageCommentsPath, pagePath, pageTree, pages,
    pagesPath, shippedTemplate, subtreeOf, templatesPath, trashedPagesPath, verifiedUntil, versionPath, versionsPath, visibleRows,
    type ChangedWithin, type ShippedSpace, type ShippedTemplate,
  } from '../lib/data/pages.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { views } from '../lib/data/views.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import PageAccess from '../lib/pages/PageAccess.svelte';
  import PageLabels from '../lib/pages/PageLabels.svelte';
  import PageEditor from '../lib/pages/PageEditor.svelte';
  import { levelThreads } from '../lib/pages/margin.ts';
  import { platform } from '../lib/platform/index.ts';
  import { renderProblem } from '../lib/problem.ts';
  import PlaceStrip from '../lib/workspace/PlaceStrip.svelte';

  interface Props {
    /** The hub whose space is drawn; absent when a page's address names only the page. */
    hubId?: string;
    pageId?: string;
    onnavigate: (path: string) => void;
    /** Goes on without a step back: a space's address sent on to its home page. */
    onreplace?: (path: string) => void;
  }

  const { hubId: hubFromRoute, pageId, onnavigate, onreplace }: Props = $props();

  // --- What is read ------------------------------------------------------------------------------

  const opened = untrack(() => (pageId ? resource<PageRead>(pagePath(pageId)) : undefined));
  const current = $derived(opened?.state.status === 'ready' ? opened.state.data : undefined);
  const hubId = $derived(hubFromRoute ?? current?.hub_id ?? '');
  const hub = $derived(hubId ? containers.find(hubId) : undefined);


  let treeState = $state<PageList | undefined>(undefined);
  let templateRows = $state<readonly Page[]>([]);
  let trashRows = $state<readonly Page[]>([]);
  let treeRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    if (!at) return;
    return untrack(() => {
      const stops = [
        engine.subscribe<PageList>({ path: pagesPath(at) }, (next) => {
          if (next.status === 'ready') treeState = next.data;
          if (next.status === 'failed') treeRefusal = renderProblem(next.error, messages).message;
        }),
        engine.subscribe<PageList>({ path: templatesPath(at) }, (next) => {
          if (next.status === 'ready') templateRows = next.data.data;
        }),
        views.open(at),
      ];
      people.open({ hubId: at });
      return () => stops.forEach((stop) => stop());
    });
  });

  const rows = $derived(treeState?.data ?? []);
  const tree = $derived(pageTree(rows));
  let openedBranches = $state<Set<string>>(new Set());
  $effect(() => {
    if (!pageId || rows.length === 0) return;
    const above = ancestorsOf(rows, pageId);
    untrack(() => {
      if (above.every((id) => openedBranches.has(id))) return;
      openedBranches = new Set([...openedBranches, ...above]);
    });
  });
  const drawn = $derived(visibleRows(tree, openedBranches));

  // A space opens on its home page, never on "open a page" (user-experience.md §7.1).
  $effect(() => {
    if (pageId || !hubFromRoute || !treeState) return;
    const home = homePageOf(treeState.data);
    if (home) untrack(() => (onreplace ?? onnavigate)(`/pages/${home}`));
  });
  const hasChildren = $derived(new Set(rows.map((row) => row.parent_id).filter((id): id is string => id !== null)));

  const mayWrite = $derived(
    hubId ? holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted' : false,
  );

  function toggle(id: string) {
    const next = new Set(openedBranches);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    openedBranches = next;
  }

  // --- Writing a page ----------------------------------------------------------------------------

  let busy = $state(false);
  let refusal = $state<string | undefined>(undefined);

  async function act<T>(run: () => Promise<T>, done?: string): Promise<T | undefined> {
    busy = true;
    refusal = undefined;
    try {
      const answer = await run();
      if (done) toast.show({ text: done, tone: 'success' });
      return answer;
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
      return undefined;
    } finally {
      busy = false;
    }
  }

  let isCreating = $state(false);
  let draftTitle = $state('');
  let draftParent = $state<string | undefined>(undefined);
  let draftTemplate = $state('');
  let draftWithSubpages = $state(true);
  const draftOwnTemplate = $derived(draftTemplate.startsWith('own:') ? draftTemplate.slice(4) : undefined);
  const draftOffersSubpages = $derived(draftOwnTemplate !== undefined && hasSubpages(rows, draftOwnTemplate));

  function startCreate(parent?: string) {
    draftTitle = '';
    draftParent = parent;
    draftTemplate = '';
    draftWithSubpages = true;
    isCreating = true;
  }

  /** An empty space started from one of Hubtask's space templates, in one press. */
  async function startSpace(key: ShippedSpace) {
    const space = shippedSpace(key, t);
    const made = await act(() => pages.create(hubId, { title: space.title, body: space.body, subpages: space.subpages }));
    if (made) onnavigate(`/pages/${made.id}`);
  }

  async function create() {
    const title = draftTitle.trim();
    const space = draftTemplate.startsWith('space:') ? shippedSpace(draftTemplate.slice(6) as ShippedSpace, t) : undefined;
    const shipped = draftTemplate.startsWith('shipped:') ? shippedTemplate(draftTemplate.slice(8) as ShippedTemplate, t) : space;
    const own = draftOwnTemplate;
    const subpages = space?.subpages;
    const made = await act(() =>
      pages.create(hubId, {
        title: title || shipped?.title || t('app.pages.untitled'),
        ...(draftParent ? { parent_id: draftParent } : {}),
        ...(shipped ? { body: shipped.body } : {}),
        ...(subpages ? { subpages } : {}),
        ...(own ? { template_id: own } : {}),
        ...(own && draftOffersSubpages && draftWithSubpages ? { with_subpages: true } : {}),
      }),
    );
    if (!made) return;
    isCreating = false;
    onnavigate(`/pages/${made.id}`);
  }

  let titleDraft = $state<string | undefined>(undefined);
  async function rename() {
    if (!current || titleDraft === undefined) return;
    const title = titleDraft.trim();
    titleDraft = undefined;
    if (title === '' || title === current.title) return;
    await act(() => pages.change(current, { title }));
  }

  async function trash() {
    if (!current) return;
    const answer = await act(() => pages.trash(current), undefined);
    if (!answer) return;
    toast.show({ text: t('app.pages.trashed', { count: String(answer.trashed) }), tone: 'success' });
    onnavigate(`/hubs/${current.hub_id}/pages`);
  }

  async function copy() {
    if (!current) return;
    const made = await act(() => pages.copy(current), t('app.pages.copied'));
    if (made) onnavigate(`/pages/${made.id}`);
  }

  async function restore(id: string) {
    const answer = await act(() => pages.restore(hubId, id), t('app.pages.restored'));
    if (answer) onnavigate(`/pages/${id}`);
  }

  // Moving: under another page of the space, or to its top; never under itself or its own.
  let isMoving = $state(false);
  let moveParent = $state('');
  const moveOptions = $derived.by(() => {
    if (!current) return [];
    const own = subtreeOf(rows, current.id);
    return [
      { value: '', label: t('app.pages.move_top') },
      ...rows.filter((row) => !own.has(row.id)).map((row) => ({ value: row.id, label: row.title })),
    ];
  });
  async function move() {
    if (!current) return;
    const done = await act(() => pages.move(current, moveParent ? { parent_id: moveParent } : {}), t('app.pages.moved'));
    if (done) isMoving = false;
  }

  // Verification (UC-DOC-12): until a date, by the reader, or taken away.
  let verifyDays = $state(String(VERIFY_DAYS[1]));
  async function verify(isTakenAway: boolean) {
    if (!current) return;
    await act(
      () => pages.verify(current, isTakenAway ? undefined : verifiedUntil(new Date(), Number(verifyDays)), isTakenAway ? undefined : actor.account?.id),
      isTakenAway ? t('app.pages.verify_removed') : t('app.pages.verified'),
    );
  }

  async function markTemplate(template: boolean) {
    if (!current) return;
    await act(() => pages.change(current, { template }));
  }

  // Export (UC-DOC-09): the page, or it and its pages, as files of the format chosen, saved by the browser.
  let exportsSubtree = $state(false);
  let exportFormat = $state<'MARKDOWN' | 'HTML' | 'DOCX'>('MARKDOWN');
  async function exportPage() {
    if (!current) return;
    const answer = await act(() => pages.export(current.id, exportsSubtree, exportFormat));
    if (!answer) return;
    for (const file of answer.files) {
      platform.saveFile(new Blob([exportBytes(file) as BlobPart], { type: EXPORT_TYPES[answer.format] }), exportFileName(file.path));
    }
  }

  // --- Search over the space --------------------------------------------------------------------

  let words = $state('');
  let hits = $state<PageHitList['data'] | undefined>(undefined);
  let generation = 0;
  // What narrows the search (UC-DOC-01 check 9): one of the space's labels, the reader's own pages,
  // how recently a page changed. A label or the reader's own pages find without words.
  let chosenLabel = $state('');
  let ownOnly = $state(false);
  let changedWithin = $state<ChangedWithin>('any');
  let facet = $state<PageLabelCounts['data']>([]);
  $effect(() => {
    const at = hubId;
    if (!at) return;
    return untrack(() => engine.subscribe<PageLabelCounts>({ path: spaceLabelsPath(at) }, (next) => {
      if (next.status === 'ready') facet = next.data.data;
    }));
  });
  const filters = $derived((chosenLabel ? 1 : 0) + (ownOnly ? 1 : 0) + (changedWithin === 'any' ? 0 : 1));
  const changedOptions = $derived(
    (Object.keys(CHANGED_WITHIN) as ChangedWithin[]).map((value) => ({ value, label: t(`app.pages.changed_${value}`) })),
  );
  $effect(() => {
    const asked = words.trim();
    const at = hubId;
    const narrow = {
      ...(chosenLabel ? { label: chosenLabel } : {}),
      ...(ownOnly && actor.account ? { author_id: actor.account.id } : {}),
    };
    const from = changedFrom(changedWithin, new Date());
    if ((asked === '' && !narrow.label && !narrow.author_id) || !at) {
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

  // --- Beside the page: comments, history, backlinks ---------------------------------------------

  // The side sheet the head toggles; closed, the page has the width (user-experience.md §7.1).
  let side = $state<'comments' | 'history' | 'backlinks' | 'access' | 'details' | undefined>(undefined);
  const toggleSide = (which: NonNullable<typeof side>) => (side = side === which ? undefined : which);
  let present = $state<string[]>([]);
  let isFilterOpen = $state(false);
  // On a narrow screen an open page comes first: the tree folds above it, one press away.
  let isTreeOpen = $state(false);
  const treeFolds = $derived(viewport.isBelowExpanded && Boolean(pageId));
  let block = $state<SelectedBlock | undefined>(undefined);
  let reveal = $state<string | undefined>(undefined);

  let commentState = $state<PageComment[]>([]);
  let versionState = $state<PageVersionList['data']>([]);
  let backlinkState = $state<PageBacklinkList['data']>([]);
  $effect(() => {
    const id = current?.id;
    if (!id) return;
    return untrack(() => {
      const stops = [
        engine.subscribe<PageCommentList>({ path: pageCommentsPath(id) }, (next) => {
          if (next.status === 'ready') {
            commentState = next.data.data;
            accounts.resolve(next.data.data.map((each) => each.author_id));
          }
        }),
        engine.subscribe<PageVersionList>({ path: versionsPath(id) }, (next) => {
          if (next.status === 'ready') {
            versionState = next.data.data;
            accounts.resolve(next.data.data.map((each) => each.author_id));
          }
        }),
        engine.subscribe<PageBacklinkList>({ path: backlinksPath(id) }, (next) => {
          if (next.status === 'ready') backlinkState = next.data.data;
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  const threads = $derived(
    commentState
      .filter((each) => each.parent_id === null)
      .map((first) => ({ first, replies: commentState.filter((each) => each.parent_id === first.id) })),
  );
  let showResolved = $state(false);
  const shownThreads = $derived(threads.filter((each) => showResolved || !each.first.resolved));

  // The margin (UC-DOC-11 check 4): a thread about a passage is drawn level with it while the
  // comments stand beside the page; under it, on a narrow screen, they are a list.
  const passages = $derived<Passage[]>(
    shownThreads
      .filter((each) => each.first.quote && !each.first.deleted)
      .map((each) => ({ id: each.first.id, anchor: each.first.anchor, quote: each.first.quote! })),
  );
  let bodyElement = $state<HTMLElement | undefined>(undefined);
  let marginElement = $state<HTMLElement | undefined>(undefined);
  let passageAt = $state<Map<string, number>>(new Map());
  let marginTop = $state(0);
  let isLevel = $state(false);
  const heights = $state<Record<string, number>>({});
  function passagesMoved(tops: Map<string, number>) {
    passageAt = tops;
    if (!marginElement || !bodyElement) return;
    marginTop = marginElement.getBoundingClientRect().top;
    isLevel = marginElement.getBoundingClientRect().left >= bodyElement.getBoundingClientRect().right;
  }
  const passageThreads = $derived(shownThreads.filter((each) => each.first.quote));
  const pageThreads = $derived(shownThreads.filter((each) => !each.first.quote));
  const margin = $derived.by(() => {
    const gap = marginElement ? Number.parseFloat(getComputedStyle(marginElement).rowGap) || 0 : 0;
    return levelThreads(passageThreads.map((each) => {
      const at = passageAt.get(each.first.id);
      return { id: each.first.id, top: at === undefined ? undefined : at - marginTop, height: heights[each.first.id] ?? 0 };
    }), gap);
  });
  const passageOrder = $derived(
    [...passageThreads].sort((a, b) => (margin.tops.get(a.first.id) ?? 0) - (margin.tops.get(b.first.id) ?? 0)),
  );

  let commentDraft = $state('');
  let isAtBlock = $state(false);
  let replyTo = $state<string | undefined>(undefined);
  let replyDraft = $state('');

  async function addComment() {
    if (!current || commentDraft.trim() === '') return;
    const anchored = isAtBlock && block;
    const made = await act(() =>
      pages.addComment(current.id, {
        body: commentDraft.trim(),
        // Exactly the words selected, else the block's.
        ...(anchored ? { anchor: block!.id, ...((block!.selected ?? block!.text) ? { quote: (block!.selected ?? block!.text).slice(0, 500) } : {}) } : {}),
      }),
    );
    if (made) commentDraft = '';
  }

  async function reply(first: PageComment) {
    if (!current || replyDraft.trim() === '') return;
    const made = await act(() => pages.addComment(current.id, { body: replyDraft.trim(), parent_id: first.id }));
    if (made) {
      replyDraft = '';
      replyTo = undefined;
    }
  }

  const nameOf = (id: string | null) => (id ? accounts.nameOf(id) : undefined) ?? t('app.people.unnamed');
  const when = (iso: string) => formatDateTime(iso, messages.locale);

  // History (UC-DOC-06): a version opened is compared with the page as it stands.
  let comparing = $state<PageVersionRead | undefined>(undefined);
  async function openVersion(seq: number) {
    if (!current) return;
    const state = await engine.refresh<PageVersionRead>({ path: versionPath(current.id, seq) });
    if (state.status === 'ready') comparing = state.data;
    else if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
  }
  const compared = $derived(comparing && current ? compareBodies(comparing.body, current.body ?? '') : []);

  async function restoreVersion(seq: number) {
    if (!current) return;
    const done = await act(() => pages.restoreVersion(current, seq), t('app.pages.version_restored', { seq: String(seq) }));
    if (done) {
      comparing = undefined;
      await opened?.refresh();
    }
  }

  let namingSeq = $state<number | undefined>(undefined);
  let nameDraft = $state('');
  async function nameVersion() {
    if (!current || namingSeq === undefined) return;
    const seq = namingSeq;
    const done = await act(() => pages.nameVersion(current, seq, nameDraft.trim()));
    if (done) namingSeq = undefined;
  }

  const openThreads = $derived(threads.filter((each) => !each.first.resolved).length);
  const sideTitle = $derived(
    side === 'comments' ? t('app.pages.side_comments', { count: String(openThreads) })
      : side === 'history' ? t('app.pages.side_history')
        : side === 'backlinks' ? t('app.pages.side_backlinks', { count: String(backlinkState.length) })
          : side === 'access' ? t('app.pages.access')
            : t('app.pages.side_details'),
  );

  // The page's menu: what is not the head's one row, the destructive item last (§5.9).
  const pageMenu = $derived(current ? [
    ...(current.writable ? [{ id: 'child', label: t('app.pages.new_child'), icon: 'plus' as const }] : []),
    ...(current.writable ? [{ id: 'copy', label: t('app.pages.copy'), icon: 'copy' as const }] : []),
    ...(current.writable ? [{ id: 'move', label: t('app.pages.move'), icon: 'arrow-right' as const }] : []),
    { id: 'history', label: t('app.pages.side_history'), icon: 'clock' as const, hasSeparatorBefore: current.writable },
    { id: 'backlinks', label: t('app.pages.side_backlinks', { count: String(backlinkState.length) }), icon: 'link' as const },
    { id: 'details', label: t('app.pages.side_details'), icon: 'info' as const },
    ...(current.writable ? [{ id: 'trash', label: t('app.pages.to_trash'), icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: true }] : []),
  ] : []);
  function choose(id: string) {
    if (!current) return;
    if (id === 'child') startCreate(current.id);
    else if (id === 'copy') void copy();
    else if (id === 'move') { moveParent = current.parent_id ?? ''; isMoving = true; }
    else if (id === 'trash') void trash();
    else if (id === 'history' || id === 'backlinks' || id === 'details') side = id;
  }

  const spacePages = $derived(rows.map((row) => ({ id: row.id, title: row.title, parent_id: row.parent_id })));
  const hubViews = $derived(views.of(hubId).map((view) => ({ id: view.id, name: view.name })));

  const templateOptions = $derived([
    { value: '', label: t('app.pages.template_none') },
    ...templateRows.map((row) => ({ value: `own:${row.id}`, label: row.title })),
    ...SHIPPED_TEMPLATES.map((key) => ({ value: `shipped:${key}`, label: t(`app.pages.template.${key}.title`) })),
  ]);
  const templateGroups = $derived([
    { label: t('app.pages.start_space'), options: SHIPPED_SPACES.map((key) => ({ value: `space:${key}`, label: t(`app.pages.space.${key}.title`) })) },
  ]);

  // --- A space with drafts (UC-DOC-15) -----------------------------------------------------------

  const pageDraft = $derived(current ? draftStateOf(current) : 'live');
  let discarded = $state<{ body: string } | undefined>(undefined);

  async function release() {
    if (!current) return;
    await act(() => pages.release(current), t('app.pages.released'));
  }

  async function discard() {
    if (!current) return;
    const published = current.body ?? '';
    // A thrown-away draft answers nothing; a refusal is what says it failed.
    await act(() => pages.discardDraft(current), t('app.pages.draft_discarded'));
    if (refusal === undefined) discarded = { body: published };
  }

  let showsTrash = $state(false);
  $effect(() => {
    if (!showsTrash || !hubId) return;
    const at = hubId;
    return untrack(() =>
      engine.subscribe<PageList>({ path: trashedPagesPath(at) }, (next) => {
        if (next.status === 'ready') trashRows = next.data.data;
      }),
    );
  });
</script>

{#if hubId}<PlaceStrip placeId={hubId} current="pages" {onnavigate} />{/if}
<div class="space">
  <nav class="tree" aria-label={t('app.pages.tree', { hub: hub?.name ?? '' })}>
    <Stack gap="100">
      <Inline justify="between" align="center">
        <!-- The screen's one h1 until a page is open; then the page's title is. -->
        <svelte:element this={current ? 'h2' : 'h1'} class="heading">{t('app.pages.title')}</svelte:element>
        <Inline gap="050" align="center">
          {#if treeFolds}
            <Button size="sm" tone="subtle" icon={isTreeOpen ? 'chevron-up' : 'chevron-down'} aria-expanded={isTreeOpen} data-tree-toggle onclick={() => (isTreeOpen = !isTreeOpen)}>
              {t('app.pages.tree_toggle', { count: rows.length })}
            </Button>
          {/if}
          {#if mayWrite}
            <IconButton icon="plus" label={t('app.pages.new')} onclick={() => startCreate()} />
          {/if}
        </Inline>
      </Inline>
      {#if !treeFolds || isTreeOpen}
      <Inline gap="050" align="end">
        <div class="search"><SearchField label={t('app.pages.search')} clearLabel={t('app.pages.search_clear')} bind:value={words} size="sm" /></div>
        <Button
          size="sm"
          tone={filters > 0 ? 'secondary' : 'subtle'}
          icon="funnel"
          aria-expanded={isFilterOpen}
          aria-controls="page-search-narrow"
          onclick={() => (isFilterOpen = !isFilterOpen)}
        >{filters > 0 ? t('app.pages.filter_count', { count: String(filters) }) : t('app.pages.filter')}</Button>
      </Inline>
      {#if isFilterOpen}
        <div class="narrow" id="page-search-narrow" data-page-search-narrow>
          {#if facet.length > 0}
            <Inline gap="050" isWrapping>
              {#each facet.slice(0, 12) as each (each.label)}
                <Button
                  size="sm"
                  tone={chosenLabel === each.label ? 'secondary' : 'subtle'}
                  aria-pressed={chosenLabel === each.label}
                  onclick={() => (chosenLabel = chosenLabel === each.label ? '' : each.label)}
                >{t('app.pages.label_facet', { label: each.label, count: String(each.pages) })}</Button>
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
      {:else if treeRefusal}
        <p class="refusal">{treeRefusal}</p>
      {:else if !treeState}
        <Spinner label={t('app.pages.loading')} />
      {:else if drawn.length === 0}
        <p class="quiet">{t('app.pages.empty_tree')}</p>
      {:else}
        <ul class="rows" role="tree">
          {#each drawn as node (node.page.id)}
            <li
              role="treeitem"
              aria-selected={node.page.id === pageId}
              aria-expanded={hasChildren.has(node.page.id) ? openedBranches.has(node.page.id) : undefined}
              aria-level={node.depth + 1}
              class="row"
              class:current={node.page.id === pageId}
              style:--depth={node.depth}
            >
              {#if hasChildren.has(node.page.id)}
                <button type="button" class="twisty" aria-label={t(openedBranches.has(node.page.id) ? 'app.pages.collapse' : 'app.pages.expand', { title: node.page.title })} onclick={() => toggle(node.page.id)}>
                  {openedBranches.has(node.page.id) ? '▾' : '▸'}
                </button>
              {:else}
                <span class="twisty" aria-hidden="true"></span>
              {/if}
              <a href={`/pages/${node.page.id}`} aria-current={node.page.id === pageId ? 'page' : undefined}>
                {#if node.page.icon}<span aria-hidden="true">{node.page.icon}</span>{/if}
                {node.page.title}
              </a>
              {#if node.page.restricted}<Badge size="sm" icon="lock">{t('app.pages.access_restricted_badge')}</Badge>{/if}
              {#if node.page.due_for_review}<Badge tone="warning" size="sm">{t('app.pages.due_for_review')}</Badge>{/if}
            </li>
          {/each}
        </ul>
      {/if}

      {#if mayWrite && rows.length > 0}
        <Button size="sm" tone="subtle" icon="trash" aria-expanded={showsTrash} onclick={() => (showsTrash = !showsTrash)}>{t('app.pages.trash')}</Button>
        {#if showsTrash}
          <ul class="hits" aria-label={t('app.pages.trash')}>
            {#each trashRows as row (row.id)}
              <li>
                <span>{row.title}</span>
                <Button size="sm" tone="subtle" onclick={() => restore(row.id)}>{t('app.pages.restore')}</Button>
              </li>
            {:else}
              <li class="quiet">{t('app.pages.trash_empty')}</li>
            {/each}
          </ul>
        {/if}
      {/if}
      {/if}
    </Stack>
  </nav>

  <section class="page" aria-label={current?.title ?? t('app.pages.title')}>
    {#if refusal}
      <Banner tone="danger" dismissLabel={t('app.pages.dismiss')} onDismiss={() => (refusal = undefined)}>{refusal}</Banner>
    {/if}

    {#if !pageId}
      {#if !treeState && !treeRefusal}
        <Spinner label={t('app.pages.loading')} />
      {:else if treeState && rows.length === 0 && mayWrite}
        <!-- An empty space starts from a template in one press (user-experience.md §7.1, §5.5). -->
        <div class="start" data-space-start>
          <EmptyState kind="unused" title={t('app.pages.space_empty')} description={t('app.pages.space_empty_hint')} icon="book-open" />
          <TemplateGallery label={t('app.pages.space_starts')} isLabelHidden>
            {#each SHIPPED_SPACES as key (key)}
              <TemplateCard
                name={t(`app.pages.space.${key}.title`)}
                summary={t(`app.pages.space.${key}.lead`)}
                note={t('app.pages.space_pages', { count: shippedSpace(key, t).subpages.length })}
                mark="layout-template"
                data-space-template={key}
              >
                {#snippet action()}
                  <Button size="sm" tone="primary" busyLabel={busy ? t('app.pages.creating') : undefined} onclick={() => void startSpace(key)}>{t('app.pages.space_use')}</Button>
                {/snippet}
              </TemplateCard>
            {/each}
            <TemplateCard name={t('app.pages.blank')} summary={t('app.pages.blank_hint')} mark="file-text" data-space-template="blank">
              {#snippet action()}
                <Button size="sm" tone="secondary" onclick={() => startCreate()}>{t('app.pages.new')}</Button>
              {/snippet}
            </TemplateCard>
          </TemplateGallery>
        </div>
      {:else if treeState && rows.length === 0}
        <EmptyState kind="unused" title={t('app.pages.space_empty_reader')} description={t('app.pages.space_empty_reader_hint')} icon="book-open" />
      {:else if treeRefusal}
        <p class="refusal">{treeRefusal}</p>
      {:else}
        <Spinner label={t('app.pages.loading')} />
      {/if}
    {:else if opened?.state.status === 'failed'}
      <p class="refusal">{renderProblem(opened.state.error, messages).message}</p>
    {:else if !current}
      <Spinner label={t('app.pages.loading')} />
    {:else}
      {#key current.id}
        <Stack gap="150">
          <Breadcrumb
            label={t('app.pages.breadcrumb')}
            expandLabel={t('app.pages.breadcrumb_more')}
            trail={[
              ...(hub ? [{ id: `hub:${hub.id}`, label: hub.name, href: `/hubs/${hub.id}/pages` }] : []),
              ...current.ancestors.map((above) => ({ id: above.id, label: above.title, href: `/pages/${above.id}` })),
            ]}
          />
          <!-- The head's one row (user-experience.md §7.1): the title once, who else is here, the
               comments' toggle, sharing, and the rest in the page's menu. -->
          <div class="head" data-page-head>
            {#if current.writable}
              <!-- A textarea, as an entry's title is, so a long title wraps; Enter saves it. The
                   heading is read, the field is what is seen: the title appears once. -->
              <VisuallyHidden as="h1">{current.title}</VisuallyHidden>
              <textarea
                class="title"
                rows="1"
                aria-label={t('app.pages.page_title')}
                value={titleDraft ?? current.title}
                oninput={(event) => (titleDraft = (event.currentTarget as HTMLTextAreaElement).value.replace(/\n/gu, ' '))}
                onblur={rename}
                onkeydown={(event) => { if (event.key === 'Enter') { event.preventDefault(); (event.currentTarget as HTMLTextAreaElement).blur(); } }}
              ></textarea>
            {:else}
              <h1 class="title">{current.title}</h1>
            {/if}
            <Inline gap="100" align="center">
              {#if present.length > 0}
                <span class="here" data-page-here>
                  <AvatarGroup
                    people={present.map((name) => ({ name }))}
                    max={3}
                    size="sm"
                    overflowLabel={t('app.pages.here_more', { count: Math.max(0, present.length - 3) })}
                  />
                  <VisuallyHidden>{t('app.pages.here')}: {present.join(', ')}</VisuallyHidden>
                </span>
              {/if}
              <Button
                size="sm"
                tone={side === 'comments' ? 'secondary' : 'subtle'}
                icon="message-square"
                aria-pressed={side === 'comments'}
                data-page-comments
                onclick={() => toggleSide('comments')}
              >{t('app.pages.side_comments', { count: String(openThreads) })}</Button>
              {#if !current.shared && hubId}
                <Button size="sm" tone={side === 'access' ? 'secondary' : 'subtle'} icon="users" aria-pressed={side === 'access'} data-page-share onclick={() => toggleSide('access')}>{t('app.pages.share')}</Button>
              {/if}
              <Menu label={t('app.pages.page_menu')} items={pageMenu} onselect={choose}>
                {#snippet trigger(props)}
                  <IconButton icon="ellipsis" label={t('app.pages.page_menu')} data-page-menu {...props} />
                {/snippet}
              </Menu>
            </Inline>
          </div>
          {#if current.template || current.restricted || current.shared || current.due_for_review || current.verified_until}
            <Inline gap="050" isWrapping>
              {#if current.template}<Badge size="sm">{t('app.pages.is_template')}</Badge>{/if}
              {#if current.restricted}<Badge size="sm" icon="lock">{t('app.pages.access_restricted_badge')}</Badge>{/if}
              {#if current.shared}<Badge size="sm" tone="info">{t('app.pages.shared_with_you')}</Badge>{/if}
              {#if current.due_for_review}
                <Badge tone="warning" size="sm">{t('app.pages.due_for_review')}</Badge>
              {:else if current.verified_until}
                <Badge tone="success" size="sm">{t('app.pages.verified_until', { date: when(current.verified_until) })}</Badge>
              {/if}
            </Inline>
          {/if}
          {#key current.id}<PageLabels page={current} onchoose={(label) => { chosenLabel = label; isFilterOpen = true; }} />{/key}
          {#if current.writable && (pageDraft === 'unpublished' || pageDraft === 'draft')}
            <Banner tone="info">
              {t(pageDraft === 'unpublished' ? 'app.pages.unpublished_hint' : 'app.pages.draft_hint')}
              {#snippet action()}
                <Inline gap="050">
                  <Button size="sm" tone="primary" onclick={release}>{t('app.pages.release')}</Button>
                  {#if pageDraft === 'draft'}<Button size="sm" tone="subtle" onclick={discard}>{t('app.pages.discard_draft')}</Button>{/if}
                </Inline>
              {/snippet}
            </Banner>
          {:else if current.writable && pageDraft === 'published'}
            <p class="quiet">{t('app.pages.published_hint')}</p>
          {/if}

          <div class="body-and-side" class:with-side={side !== undefined}>
            <div class="body" bind:this={bodyElement}>
              <PageEditor
                page={current}
                {spacePages}
                views={hubViews}
                collections={hubId ? containers.collectionsOf(hubId).map((each) => ({ id: each.id, name: each.name })) : []}
                {onnavigate}
                {reveal}
                replaceWith={discarded}
                passages={side === 'comments' ? passages : []}
                onpassages={passagesMoved}
                onpresent={(names) => (present = names)}
                onblock={(seen) => {
                  block = seen;
                  // Words selected are a comment's passage, unless the person chooses the page.
                  if (seen?.selected) isAtBlock = true;
                }}
              />
            </div>

            {#if side}
            <aside class="side" aria-label={sideTitle} data-page-side={side}>
              <Inline justify="between" align="center">
                <h2 class="heading">{sideTitle}</h2>
                <IconButton size="sm" icon="x" label={t('app.pages.side_close')} onclick={() => (side = undefined)} />
              </Inline>

              {#snippet threadCard(thread: (typeof threads)[number])}
                <article class="thread" class:resolved={thread.first.resolved}>
                  {#if thread.first.quote}
                    <button type="button" class="quote" onclick={() => (reveal = thread.first.anchor ?? undefined)}>{thread.first.quote}</button>
                  {/if}
                  {#each [thread.first, ...thread.replies] as comment (comment.id)}
                    <div class="comment">
                      <p class="meta">{nameOf(comment.author_id)} · {when(comment.created_at)}</p>
                      <p class="text">{comment.deleted ? t('app.pages.comment_removed') : comment.body}</p>
                      {#if comment.author_id === actor.account?.id && !comment.deleted}
                        <Button size="sm" tone="subtle" onclick={() => act(() => pages.deleteComment(comment))}>{t('app.pages.comment_delete')}</Button>
                      {/if}
                    </div>
                  {/each}
                  <Inline gap="050">
                    <Button size="sm" tone="subtle" onclick={() => act(() => pages.changeComment(thread.first, { resolved: !thread.first.resolved }))}>
                      {thread.first.resolved ? t('app.pages.comment_reopen') : t('app.pages.comment_resolve')}
                    </Button>
                    {#if !thread.first.resolved}
                      <Button size="sm" tone="subtle" onclick={() => { replyTo = thread.first.id; replyDraft = ''; }}>{t('app.pages.comment_reply')}</Button>
                    {/if}
                  </Inline>
                  {#if replyTo === thread.first.id}
                    <Textarea label={t('app.pages.comment_reply')} isLabelHidden rows={2} bind:value={replyDraft} />
                    <Button size="sm" disabledReason={replyDraft.trim() === '' ? t('app.pages.comment_needs_words') : undefined} onclick={() => reply(thread.first)}>{t('app.pages.comment_send')}</Button>
                  {/if}
                </article>
              {/snippet}
              {#if side === 'comments'}
                <Stack gap="150">
                  <Switch label={t('app.pages.show_resolved')} checked={showResolved} onchange={(event: Event) => (showResolved = (event.currentTarget as HTMLInputElement).checked)} />
                  {#if shownThreads.length === 0}
                    <p class="quiet">{t('app.pages.no_comments')}</p>
                  {/if}
                  <div class="margin" class:level={isLevel} bind:this={marginElement} style:--margin-height={isLevel ? `${margin.height}px` : undefined}>
                    {#each passageOrder as thread (thread.first.id)}
                      <div
                        class="placed"
                        bind:offsetHeight={heights[thread.first.id]}
                        style:--thread-top={isLevel ? `${margin.tops.get(thread.first.id) ?? 0}px` : undefined}
                      >
                        {@render threadCard(thread)}
                      </div>
                    {/each}
                  </div>
                  {#each pageThreads as thread (thread.first.id)}
                    {@render threadCard(thread)}
                  {/each}
                  <Textarea label={t('app.pages.comment_new')} rows={3} bind:value={commentDraft} />
                  {#if block}
                    <Switch label={t('app.pages.comment_at_block', { words: (block.selected ?? block.text).slice(0, 60) })} checked={isAtBlock} onchange={(event: Event) => (isAtBlock = (event.currentTarget as HTMLInputElement).checked)} />
                  {/if}
                  <Button size="sm" disabledReason={commentDraft.trim() === '' ? t('app.pages.comment_needs_words') : undefined} onclick={addComment}>{t('app.pages.comment_send')}</Button>
                </Stack>
              {:else if side === 'history'}
                <Stack gap="100">
                  <ol class="versions">
                    {#each versionState as version (version.seq)}
                      <li>
                        <button type="button" class="version" class:chosen={comparing?.seq === version.seq} onclick={() => openVersion(version.seq)}>
                          <span class="seq">{version.name ?? t('app.pages.version', { seq: String(version.seq) })}</span>
                          <span class="meta">{nameOf(version.author_id)} · {when(version.created_at)}{version.restored_from ? ` · ${t('app.pages.version_restored_from', { seq: String(version.restored_from) })}` : ''}</span>
                        </button>
                        {#if current.writable}
                          <IconButton size="sm" icon="pencil" label={t('app.pages.version_name')} onclick={() => { namingSeq = version.seq; nameDraft = version.name ?? ''; }} />
                        {/if}
                      </li>
                    {:else}
                      <li class="quiet">{t('app.pages.no_versions')}</li>
                    {/each}
                  </ol>
                  {#if comparing}
                    <h3 class="heading">{t('app.pages.compare', { seq: String(comparing.seq) })}</h3>
                    <pre class="compare">{#each compared as line, at (at)}<span class={`line ${line.kind}`}>{line.kind === 'added' ? '+ ' : line.kind === 'removed' ? '- ' : '  '}{line.text}
</span>{/each}</pre>
                    {#if current.writable}
                      <Button size="sm" tone="secondary" icon="rotate-ccw" onclick={() => restoreVersion(comparing!.seq)}>{t('app.pages.version_restore')}</Button>
                    {/if}
                  {/if}
                </Stack>
              {:else if side === 'backlinks'}
                <ul class="hits">
                  {#each backlinkState as link (link.source_kind + link.source_id)}
                    <li>
                      <a href={link.page_id ? `/pages/${link.page_id}` : `/items/${link.entry_id}`}>{link.title}</a>
                      <span class="excerpt">{t(`app.pages.backlink.${link.source_kind}`)} · {t(`app.pages.strength.${link.strength}`)}</span>
                    </li>
                  {:else}
                    <li class="quiet">{t('app.pages.no_backlinks')}</li>
                  {/each}
                </ul>
              {:else if side === 'access'}
                {#if !current.shared && hubId}<PageAccess page={current} {hubId} />{/if}
              {:else}
                <Stack gap="150">
                  <section>
                    <h3 class="heading">{t('app.pages.verification')}</h3>
                    {#if current.verified_until}
                      <p class="meta">{t('app.pages.verified_by', { name: nameOf(current.verified_by), date: when(current.verified_until) })}</p>
                    {:else}
                      <p class="quiet">{t('app.pages.not_verified')}</p>
                    {/if}
                    {#if current.writable}
                      <Inline gap="100" align="end" isWrapping>
                        <Select label={t('app.pages.verify_for')} size="sm" bind:value={verifyDays}
                          options={VERIFY_DAYS.map((days) => ({ value: String(days), label: t('app.pages.verify_days', { days: String(days) }) }))} />
                        <Button size="sm" onclick={() => verify(false)}>{t('app.pages.verify')}</Button>
                        {#if current.verified_until}
                          <Button size="sm" tone="subtle" onclick={() => verify(true)}>{t('app.pages.verify_remove')}</Button>
                        {/if}
                      </Inline>
                    {/if}
                  </section>
                  {#if current.writable}
                    <Switch label={t('app.pages.offer_as_template')} checked={current.template} onchange={(event: Event) => markTemplate((event.currentTarget as HTMLInputElement).checked)} />
                  {/if}
                  <section>
                    <h3 class="heading">{t('app.pages.export')}</h3>
                    <Switch label={t('app.pages.export_subtree')} checked={exportsSubtree} onchange={(event: Event) => (exportsSubtree = (event.currentTarget as HTMLInputElement).checked)} />
                    <Select label={t('app.pages.export_format')} size="sm" bind:value={exportFormat}
                      options={(['MARKDOWN', 'HTML', 'DOCX'] as const).map((format) => ({ value: format, label: t(`app.pages.format.${format}`) }))} />
                    <Button size="sm" tone="secondary" onclick={exportPage}>{t('app.pages.export_download')}</Button>
                  </section>
                  <p class="meta">{t('app.pages.changed_by', { name: nameOf(current.updated_by), date: when(current.updated_at) })}</p>
                </Stack>
              {/if}
            </aside>
            {/if}
          </div>
        </Stack>
      {/key}
    {/if}
  </section>
</div>

{#if isCreating}
  <Dialog bind:isOpen={isCreating} title={draftParent ? t('app.pages.new_child') : t('app.pages.new')} dismissLabel={t('app.pages.cancel')}>
    <Stack gap="150">
      <Input label={t('app.pages.page_title')} bind:value={draftTitle} />
      <Select label={t('app.pages.start_from')} options={templateOptions} groups={templateGroups} bind:value={draftTemplate} />
      {#if draftOffersSubpages}
        <Switch label={t('app.pages.with_subpages')} bind:checked={draftWithSubpages} />
      {/if}
    </Stack>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => (isCreating = false)}>{t('app.pages.cancel')}</Button>
      <Button tone="primary" busyLabel={busy ? t('app.pages.creating') : undefined} onclick={create}>{t('app.pages.create')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isMoving && current}
  <Dialog bind:isOpen={isMoving} title={t('app.pages.move_title', { title: current.title })} dismissLabel={t('app.pages.cancel')}>
    <Select label={t('app.pages.move_under')} options={moveOptions} bind:value={moveParent} />
    {#snippet actions()}
      <Button tone="subtle" onclick={() => (isMoving = false)}>{t('app.pages.cancel')}</Button>
      <Button tone="primary" onclick={move}>{t('app.pages.move')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if namingSeq !== undefined}
  <Dialog isOpen title={t('app.pages.version_name')} dismissLabel={t('app.pages.cancel')} onClose={() => (namingSeq = undefined)}>
    <Input label={t('app.pages.version_name')} bind:value={nameDraft} />
    {#snippet actions()}
      <Button tone="subtle" onclick={() => (namingSeq = undefined)}>{t('app.pages.cancel')}</Button>
      <Button tone="primary" onclick={nameVersion}>{t('app.pages.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .space {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--sp-200);
  }
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) {
    .space { grid-template-columns: minmax(22ch, 30ch) minmax(0, 1fr); }
    /* A long tree scrolls in its own column, so the page beside it stays where it is. */
    .tree { position: sticky; inset-block-start: var(--sp-200); max-block-size: calc(100dvh - var(--sp-1000)); overflow-y: auto; }
  }
  .tree {
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    align-self: start;
  }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .search { flex: 1 1 auto; min-inline-size: 0; }
  .start { display: grid; gap: var(--sp-200); }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-100);
  }
  .here { display: inline-flex; }
  .narrow { display: grid; gap: var(--sp-050); }
  .rows, .hits, .versions { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .row {
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    padding-inline-start: calc(var(--depth) * var(--sp-150));
    border-radius: var(--r-sm);
  }
  .row.current { background: var(--accent-primary-subtle); }
  .row a, .hits a { color: var(--text-primary); text-decoration: none; overflow-wrap: anywhere; }
  .row a:hover, .hits a:hover { text-decoration: underline; }
  .twisty {
    inline-size: var(--sp-200);
    flex: none;
    border: none;
    background: none;
    color: var(--text-secondary);
    cursor: pointer;
    font: inherit;
  }
  .hits li { display: grid; gap: var(--sp-025); padding-block: var(--sp-050); }
  .excerpt, .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .page { min-inline-size: 0; }
  .title {
    margin: 0;
    flex: 1 1 24ch;
    min-inline-size: 0;
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-primary);
    padding: var(--sp-025) var(--sp-050);
  }
  textarea.title { resize: none; overflow: hidden; field-sizing: content; font-family: var(--font-display); }
  textarea.title:hover { border-color: var(--border-subtle); }
  textarea.title:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .body-and-side { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); }
  /* design-system-lint-ignore: `primitive.breakpoint.large` (1240px); a media query cannot read a custom property. */
  @media (width >= 1240px) {
    .body-and-side.with-side { grid-template-columns: minmax(0, 1fr) minmax(28ch, 40ch); }
  }
  .body { display: grid; gap: var(--sp-100); min-inline-size: 0; }
  .side {
    display: grid;
    gap: var(--sp-150);
    align-content: start;
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .thread {
    display: grid;
    gap: var(--sp-050);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .thread.resolved { color: var(--text-secondary); }
  .margin { position: relative; display: grid; row-gap: var(--sp-100); }
  .margin.level { display: block; min-block-size: var(--margin-height); }
  .margin.level > .placed { position: absolute; inset-inline: 0; inset-block-start: var(--thread-top); }
  .quote {
    text-align: start;
    padding-inline-start: var(--sp-100);
    border: none;
    border-inline-start: var(--bw-thick) solid var(--border-default);
    background: none;
    color: var(--text-secondary);
    font: inherit;
    cursor: pointer;
  }
  .comment .text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
  .versions li { display: flex; align-items: center; gap: var(--sp-050); }
  .version {
    flex: 1 1 auto;
    display: grid;
    text-align: start;
    padding: var(--sp-050) var(--sp-100);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-primary);
    font: inherit;
    cursor: pointer;
  }
  .version:hover { background: var(--bg-surface-hover); }
  .version.chosen { border-color: var(--border-default); background: var(--bg-surface-sunken); }
  .seq { font-weight: var(--fw-medium); }
  .compare {
    margin: 0;
    max-block-size: 60vh;
    overflow: auto;
    padding: var(--sp-100);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    white-space: pre-wrap;
  }
  .line.added { color: var(--text-success); }
  .line.removed { color: var(--text-danger); text-decoration: line-through; }
</style>
