<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Pages* in a hub (M3, UC-DOC-01 … UC-DOC-12): the hub's space as a tree beside the open page.
  // The page is the block editor (`PageEditor`), and beside it its comments (on the page or at a
  // block), its history with two versions compared, and what links to it. Templates - the space's
  // own and Hubtask's, in the reader's words - start a new page; a page is verified until a date,
  // moved, exported as Markdown and put in the trash with the pages under it.
  //
  // **What the reader may never do is not drawn** (P-05): a reader without WRITE_ITEMS on the hub
  // reads the tree and the page and is offered no write; the page's own `writable` says the same
  // for the page.

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Breadcrumb, Button, Dialog, EmptyState, IconButton, Inline, Input, SearchField, Select, Spinner, Stack, Switch, Tabs,
    Textarea,
  } from '@hubtask/design-system/components';
  import { compareBodies, type Passage, type SelectedBlock } from '@hubtask/editor';
  import type {
    Page, PageBacklinkList, PageComment, PageCommentList, PageHitList, PageList, PageRead, PageVersionList, PageVersionRead,
  } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import {
    EXPORT_TYPES, SHIPPED_TEMPLATES, VERIFY_DAYS, ancestorsOf, exportBytes, backlinksPath, exportFileName, pageCommentsPath, pagePath, pageTree, pages,
    pagesPath, shippedTemplate, subtreeOf, templatesPath, trashedPagesPath, verifiedUntil, versionPath, versionsPath, visibleRows,
    type ShippedTemplate,
  } from '../lib/data/pages.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { views } from '../lib/data/views.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import PageAccess from '../lib/pages/PageAccess.svelte';
  import PageEditor from '../lib/pages/PageEditor.svelte';
  import { levelThreads } from '../lib/pages/margin.ts';
  import { platform } from '../lib/platform/index.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    /** The hub whose space is drawn; absent when a page's address names only the page. */
    hubId?: string;
    pageId?: string;
    onnavigate: (path: string) => void;
  }

  const { hubId: hubFromRoute, pageId, onnavigate }: Props = $props();

  // --- What is read ------------------------------------------------------------------------------

  const opened = untrack(() => (pageId ? resource<PageRead>(pagePath(pageId)) : undefined));
  const current = $derived(opened?.state.status === 'ready' ? opened.state.data : undefined);
  const hubId = $derived(hubFromRoute ?? current?.hub_id ?? '');
  const hub = $derived(hubId ? containers.find(hubId) : undefined);

  $effect(() => frame.entitle(current?.title ?? t('app.pages.title')));

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

  function startCreate(parent?: string) {
    draftTitle = '';
    draftParent = parent;
    draftTemplate = '';
    isCreating = true;
  }

  async function create() {
    const title = draftTitle.trim();
    const shipped = draftTemplate.startsWith('shipped:') ? shippedTemplate(draftTemplate.slice(8) as ShippedTemplate, t) : undefined;
    const own = draftTemplate.startsWith('own:') ? draftTemplate.slice(4) : undefined;
    const made = await act(() =>
      pages.create(hubId, {
        title: title || shipped?.title || t('app.pages.untitled'),
        ...(draftParent ? { parent_id: draftParent } : {}),
        ...(shipped ? { body: shipped.body } : {}),
        ...(own ? { template_id: own } : {}),
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
  $effect(() => {
    const asked = words.trim();
    const at = hubId;
    if (asked === '' || !at) {
      hits = undefined;
      return;
    }
    const mine = (generation += 1);
    const timer = setTimeout(() => {
      pages.search(asked, at).then(
        (found) => { if (mine === generation) hits = found; },
        () => { if (mine === generation) hits = []; },
      );
    }, 250);
    return () => clearTimeout(timer);
  });

  // --- Beside the page: comments, history, backlinks ---------------------------------------------

  let side = $state('comments');
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

  const sideTabs = $derived([
    { id: 'comments', label: t('app.pages.side_comments', { count: String(threads.filter((each) => !each.first.resolved).length) }) },
    { id: 'history', label: t('app.pages.side_history') },
    { id: 'backlinks', label: t('app.pages.side_backlinks', { count: String(backlinkState.length) }) },
    { id: 'details', label: t('app.pages.side_details') },
  ]);

  const spacePages = $derived(rows.map((row) => ({ id: row.id, title: row.title })));
  const hubViews = $derived(views.of(hubId).map((view) => ({ id: view.id, name: view.name })));

  const templateOptions = $derived([
    { value: '', label: t('app.pages.template_none') },
    ...templateRows.map((row) => ({ value: `own:${row.id}`, label: row.title })),
    ...SHIPPED_TEMPLATES.map((key) => ({ value: `shipped:${key}`, label: t(`app.pages.template.${key}.title`) })),
  ]);

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

<div class="space">
  <nav class="tree" aria-label={t('app.pages.tree', { hub: hub?.name ?? '' })}>
    <Stack gap="100">
      <Inline justify="between" align="center">
        <h1 class="heading">{t('app.pages.title')}</h1>
        {#if mayWrite}
          <IconButton icon="plus" label={t('app.pages.new')} onclick={() => startCreate()} />
        {/if}
      </Inline>
      {#if hub}
        <a class="hub" href={`/hubs/${hub.id}`}>{hub.name}</a>
      {/if}
      <SearchField label={t('app.pages.search')} clearLabel={t('app.pages.search_clear')} bind:value={words} size="sm" />

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

      {#if mayWrite}
        <Button size="sm" tone="subtle" icon="trash" onclick={() => (showsTrash = !showsTrash)}>{t('app.pages.trash')}</Button>
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
    </Stack>
  </nav>

  <section class="page" aria-label={current?.title ?? t('app.pages.title')}>
    {#if refusal}
      <Banner tone="danger" dismissLabel={t('app.pages.dismiss')} onDismiss={() => (refusal = undefined)}>{refusal}</Banner>
    {/if}

    {#if !pageId}
      <EmptyState kind="unused" title={t('app.pages.none_open')} description={t('app.pages.none_open_hint')} icon="file-text">
        {#snippet action()}
          {#if mayWrite}<Button icon="plus" onclick={() => startCreate()}>{t('app.pages.new')}</Button>{/if}
        {/snippet}
      </EmptyState>
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
          <Inline gap="100" align="center" isWrapping>
            {#if current.writable}
              <input
                class="title"
                aria-label={t('app.pages.page_title')}
                value={titleDraft ?? current.title}
                oninput={(event) => (titleDraft = (event.currentTarget as HTMLInputElement).value)}
                onblur={rename}
                onkeydown={(event) => { if (event.key === 'Enter') (event.currentTarget as HTMLInputElement).blur(); }}
              />
            {:else}
              <h2 class="title">{current.title}</h2>
            {/if}
            {#if current.template}<Badge size="sm">{t('app.pages.is_template')}</Badge>{/if}
            {#if current.restricted}<Badge size="sm" icon="lock">{t('app.pages.access_restricted_badge')}</Badge>{/if}
            {#if current.shared}<Badge size="sm" tone="info">{t('app.pages.shared_with_you')}</Badge>{/if}
            {#if current.due_for_review}
              <Badge tone="warning" size="sm">{t('app.pages.due_for_review')}</Badge>
            {:else if current.verified_until}
              <Badge tone="success" size="sm">{t('app.pages.verified_until', { date: when(current.verified_until) })}</Badge>
            {/if}
          </Inline>
          {#if current.writable}
            <Inline gap="100" isWrapping>
              <Button size="sm" tone="secondary" icon="plus" onclick={() => startCreate(current.id)}>{t('app.pages.new_child')}</Button>
              <Button size="sm" tone="subtle" onclick={() => { moveParent = current.parent_id ?? ''; isMoving = true; }}>{t('app.pages.move')}</Button>
              <Button size="sm" tone="subtle" data-page-copy onclick={() => void copy()}>{t('app.pages.copy')}</Button>
              <Button size="sm" tone="danger" icon="trash" onclick={trash}>{t('app.pages.to_trash')}</Button>
            </Inline>
          {/if}

          <div class="body-and-side">
            <div class="body" bind:this={bodyElement}>
              <PageEditor
                page={current}
                {spacePages}
                views={hubViews}
                collections={hubId ? containers.collectionsOf(hubId).map((each) => ({ id: each.id, name: each.name })) : []}
                {onnavigate}
                {reveal}
                passages={side === 'comments' ? passages : []}
                onpassages={passagesMoved}
                onblock={(seen) => {
                  block = seen;
                  // Words selected are a comment's passage, unless the person chooses the page.
                  if (seen?.selected) isAtBlock = true;
                }}
              />
            </div>

            <aside class="side" aria-label={t('app.pages.side')}>
              <Tabs label={t('app.pages.side')} tabs={sideTabs} selected={side} onselect={(id) => (side = id)} />

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
              {:else}
                <Stack gap="150">
                  {#if !current.shared && hubId}<PageAccess page={current} {hubId} />{/if}
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
      <Select label={t('app.pages.start_from')} options={templateOptions} bind:value={draftTemplate} />
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
  }
  .tree {
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    align-self: start;
  }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .hub { color: var(--text-secondary); font-size: var(--fs-075); }
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
    flex: 1 1 auto;
    min-inline-size: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-primary);
    padding: var(--sp-025) var(--sp-050);
  }
  input.title:hover { border-color: var(--border-subtle); }
  input.title:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .body-and-side { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); }
  /* design-system-lint-ignore: `primitive.breakpoint.large` (1240px); a media query cannot read a custom property. */
  @media (width >= 1240px) {
    .body-and-side { grid-template-columns: minmax(0, 1fr) minmax(28ch, 40ch); }
  }
  .body { display: grid; gap: var(--sp-100); min-inline-size: 0; }
  .side { display: grid; gap: var(--sp-150); align-content: start; }
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
