<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Pages* in a hub (M3, UC-DOC-01 … UC-DOC-12): the open page with its trail and a one-row head,
  // and a side sheet the head - the space's tree is the sidebar's *Contents*, its search *All
  // contents* (the navigation concept §2.4, UC-DOC-01 check 6); the side sheet the head
  // toggles - comments (on the page or at a block), history with two versions compared, what links
  // to it, who reads it and its details. A space opens on its home page; an empty one offers its
  // templates in one press. A page is verified until a date, moved, exported and put in the trash
  // with the pages under it, from the page's menu.
  //
  // **What the reader may never do is not drawn** (P-05): a reader without WRITE_ITEMS on the hub
  // reads the tree and the page and is offered no write; the page's own `writable` says the same
  // for the page.
  import { placeMark } from '../lib/workspace/PlaceMark.svelte';
  import StarToggle from '../lib/workspace/StarToggle.svelte';

  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { untrack, type Snippet } from 'svelte';
  import {
    AvatarGroup, Badge, Banner, Breadcrumb, Button, Dialog, EmptyState, IconButton, Inline, Input, Menu, PageHeader, PageOutline, Select, Sheet, Spinner, Stack, Switch,
    TemplateCard, TemplateGallery, Textarea, TrustLine, VisuallyHidden, WorkPage,
  } from '@hubtask/design-system/components';
  import { type Passage, type SelectedBlock } from '@hubtask/editor';
  import type {
    Page, PageBacklinkList, PageComment, PageCommentList, PageList, PageRead, PageVersionList,
    PageVersionRead,
  } from '@hubtask/sync-engine';

  import { capture } from '../lib/capture/capture.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { notePageRead } from '../lib/pages/reading.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import {
    EXPORT_TYPES, SHIPPED_SPACES, SHIPPED_TEMPLATES, draftStateOf, hasSubpages, homePageOf, writtenBody, shippedSpace, VERIFY_DAYS, exportBytes, backlinksPath, exportFileName, pageCommentsPath, pagePath, pages,
    pagesPath, shippedTemplate, subtreeOf, templatesPath, verifiedUntil, versionPath, versionsPath,
    type ShippedSpace, type ShippedTemplate,
  } from '../lib/data/pages.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { contentsAsk } from '../lib/data/contentsask.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { views } from '../lib/data/views.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime, formatDay, formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, naming, t } from '../lib/i18n/i18n.svelte.ts';
  import PageAccess from '../lib/pages/PageAccess.svelte';
  import PageLabels from '../lib/pages/PageLabels.svelte';
  import PageEditor from '../lib/pages/PageEditor.svelte';
  import { oneTrashAtATime, trashAndLeave } from '../lib/pages/trashing.ts';
  import PageHistory from '../lib/pages/PageHistory.svelte';
  import PageStarts from '../lib/pages/PageStarts.svelte';
  import PageCover from '../lib/pages/PageCover.svelte';
  import type { PageCover as Cover } from '../lib/pages/covers.ts';
  import PageStartPreview from '../lib/pages/PageStartPreview.svelte';
  import { START_ICONS, isBlankBody, startChange, type PageStart } from '../lib/pages/starts.ts';
  import { levelThreads } from '../lib/pages/margin.ts';
  import { platform } from '../lib/platform/index.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    /** The hub whose space is drawn; absent when a page's address names only the page. */
    hubId?: string;
    pageId?: string;
    onnavigate: (path: string) => void;
    /** Goes on without a step back: a space's address sent on to its home page. */
    onreplace?: (path: string) => void;
    /** The home's create act asked for the create dialog (user-experience.md §5.2): opened once. */
    isCreateAsked?: boolean;
    /** The create asked for is under the open page: ＋ on its row in the sidebar's *Contents* (§5.2). */
    isCreateUnder?: boolean;
    /** The create asked for starts from one of Hubtask's templates: the pages' home's *start with a template*. */
    startWith?: string;
    /**
     * A post's page (B-1): the same reader and editor, the post drawing its own head in place of
     * the trust line, its band, its discussion under the words and its writers' panel beside them.
     */
    post?: PostFrame;
  }

  interface PostFrame {
    /** The step between the hub and the post: *Posts*. */
    trail: { label: string; href: string };
    /** Its kind and origin, over the title. */
    kicker?: Snippet;
    head: Snippet;
    band?: Snippet;
    foot: Snippet;
    aside?: Snippet;
    acts?: Snippet;
    /** Words that replace the post's body through the live document: a template's start. */
    replaceWith?: { body: string };
  }

  const { hubId: hubFromRoute, pageId, onnavigate, onreplace, isCreateAsked = false, isCreateUnder = false, startWith, post }: Props = $props();

  // --- What is read ------------------------------------------------------------------------------

  const opened = untrack(() => (pageId ? resource<PageRead>(pagePath(pageId)) : undefined));
  const current = $derived(opened?.state.status === 'ready' ? opened.state.data : undefined);
  const hubId = $derived(hubFromRoute ?? current?.hub_id ?? '');
  // The sidebar shows this hub's level; the route names only the page, board or item (`levelOf`).
  $effect(() => (hubFromRoute ? undefined : frame.locate(hubId || undefined)));
  const hub = $derived(hubId ? containers.find(hubId) : undefined);
  // The pages' home offers what was read here last to continue (scene phome).
  $effect(() => {
    const read = current?.id;
    if (read) untrack(() => notePageRead(actor.account?.id, read, Date.now()));
  });


  let treeState = $state<PageList | undefined>(undefined);
  let templateRows = $state<readonly Page[]>([]);
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
  // A space opens on its home page, never on "open a page" (user-experience.md §7.1).
  $effect(() => {
    if (pageId || !hubFromRoute || !treeState) return;
    const home = homePageOf(treeState.data);
    if (home) untrack(() => (onreplace ?? onnavigate)(`/pages/${home}${isCreateAsked ? '?new=1' : ''}`));
  });

  const mayWrite = $derived(
    hubId ? holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted' : false,
  );

  // Opened once, when the reader's role here has answered that they may write.
  let isCreateOpened = false;
  $effect(() => {
    if (!isCreateAsked || isCreateOpened || !mayWrite) return;
    isCreateOpened = true;
    untrack(() => startCreate(isCreateUnder ? pageId : undefined, SHIPPED_TEMPLATES.find((key) => key === startWith)));
  });
  /** A wiki's or documents' import makes pages in the hub, which takes shaping it, as every import there does. */
  const mayImport = $derived(
    hubId ? holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'STRUCTURE').status === 'permitted' : false,
  );


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

  // A new page opens at once (S-6): no dialog; its templates wait under its empty title.
  let isMaking = false;
  const named = (code: string, params?: Record<string, string>) => naming.t(code, params);
  /** The untitled words a page carries: the stored language's, or the interface's for a page made before. */
  const untitledAs = (title: string) => {
    const interfaceWords = t('app.pages.untitled');
    return title.trim() === interfaceWords ? interfaceWords : named('app.pages.untitled');
  };

  async function startCreate(parent?: string, template?: ShippedTemplate) {
    if (isMaking || !hubId) return;
    isMaking = true;
    // The page's title and a template's words are stored: the account's language, else the
    // workspace's (`naming.ts`), not the interface's.
    await naming.ready;
    const shipped = template ? shippedTemplate(template, named) : undefined;
    const made = await act(() =>
      pages.create(hubId, {
        title: shipped?.title ?? named('app.pages.untitled'),
        ...(parent ? { parent_id: parent } : {}),
        ...(shipped && template ? { body: shipped.body, icon: START_ICONS[template] } : {}),
      }),
    );
    isMaking = false;
    if (made) onnavigate(`/pages/${made.id}`);
  }

  /** An empty space started from one of Hubtask's space templates, in one press. */
  async function startSpace(key: ShippedSpace) {
    // Stored names, in the naming language as a new page's are.
    await naming.ready;
    const space = shippedSpace(key, named);
    const made = await act(() => pages.create(hubId, { title: space.title, body: space.body, subpages: space.subpages }));
    if (made) onnavigate(`/pages/${made.id}`);
  }

  // --- The cover (E-18, scene pcover) ------------------------------------------------------------

  let isPickingCover = $state(false);
  async function changeCover(cover: Cover | null) {
    if (!current) return;
    await act(() => pages.change(current, { cover }));
  }
  async function addCover() {
    isPickingCover = true;
    await changeCover({ source: 'GRADIENT', key: 'indigo-teal', position: 50 });
  }

  // --- A new page's starts (scene pnew) -----------------------------------------------------------

  let chosenStart = $state<PageStart | undefined>(undefined);
  let ownStart = $state<{ id: string; body: string } | undefined>(undefined);
  let isUsingStart = $state(false);
  const offersStarts = $derived(!post && !!current && current.writable && isBlankBody(writtenBody(current)));
  const ownTemplates = $derived(templateRows.filter((row) => row.id !== current?.id).map((row) => ({ id: row.id, title: row.title, icon: row.icon })));
  $effect(() => {
    if (!offersStarts) chosenStart = undefined;
  });

  /** What a start writes and shows: its title, mark, words and the pages a whole space adds. */
  const startShown = $derived.by(() => {
    const start = chosenStart;
    if (!start) return undefined;
    if (start.kind === 'shipped') {
      const made = shippedTemplate(start.key, named);
      return { ...made, icon: START_ICONS[start.key], lead: `${t(`app.pages.template.${start.key}.lead`)} ${t('app.pages.start_shipped')}`, subpages: [] };
    }
    if (start.kind === 'space') {
      const made = shippedSpace(start.key, named);
      return { ...made, icon: null, lead: t(`app.pages.space.${start.key}.lead`), subpages: made.subpages };
    }
    return { title: start.title, icon: start.icon, lead: t('app.pages.template_own_hint'), body: ownStart?.id === start.id ? ownStart.body : '', subpages: [] };
  });

  async function chooseStart(start: PageStart) {
    chosenStart = start;
    if (start.kind !== 'own' || ownStart?.id === start.id) return;
    const read = await engine.refresh<Page>({ path: pagePath(start.id) });
    if (read.status === 'ready') ownStart = { id: start.id, body: read.data.body ?? '' };
  }

  /** The start written into the page and its live document; a whole space's pages made beneath it. */
  async function useStart() {
    const shown = startShown;
    if (!current || !shown || isUsingStart) return;
    isUsingStart = true;
    const start = chosenStart;
    // A template of the space with pages under it starts them too (UC-DOC-05/5): the server copies
    // them, so the new page takes the blank one's place.
    if (start?.kind === 'own' && hasSubpages(rows, start.id)) {
      const blank = current;
      const title = blank.title.trim() === untitledAs(blank.title) ? shown.title : blank.title;
      const made = await act(() => pages.create(blank.hub_id, { title, template_id: start.id, with_subpages: true, ...(blank.parent_id ? { parent_id: blank.parent_id } : {}) }));
      if (made) {
        await act(() => pages.trash(blank), undefined);
        onnavigate(`/pages/${made.id}`);
      }
      isUsingStart = false;
      return;
    }
    const change = startChange(current, untitledAs(current.title), shown);
    const changed = await act(() => pages.change(current, change));
    if (changed) {
      replaced = { body: change.body };
      for (const sub of shown.subpages) {
        await act(() => pages.create(changed.hub_id, { ...sub, parent_id: changed.id }));
      }
      chosenStart = undefined;
    }
    isUsingStart = false;
  }

  let pageEditor = $state<PageEditor | undefined>(undefined);
  let titleDraft = $state<string | undefined>(undefined);
  async function rename() {
    if (!current || titleDraft === undefined) return;
    const title = titleDraft.trim();
    titleDraft = undefined;
    if (title === '' || title === current.title) return;
    await act(() => pages.change(current, { title }));
  }

  const trash = oneTrashAtATime(async () => {
    if (!current) return;
    const page = current;
    await trashAndLeave(page, {
      drop: () => {
        titleDraft = undefined;
        return pageEditor?.dropPending();
      },
      trash: async () => {
        const answer = await act(() => pages.trash(page, { leaving: true, isPost: !!post }), undefined);
        if (answer) toast.show({ text: t('app.pages.trashed', { count: String(answer.trashed) }), tone: 'success' });
        return answer;
      },
      leave: onnavigate,
    }, post ? undefined : 'pages');
  });

  async function copy() {
    if (!current) return;
    const made = await act(() => pages.copy(current), t('app.pages.copied'));
    if (made) onnavigate(`/pages/${made.id}`);
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

  // --- Beside the page: comments, history, backlinks ---------------------------------------------

  // Beside the reading column from 1240 px the outline and the comments stand open (S-3, S-5);
  // narrower, they are a sheet the head (or the phone's foot) opens. History, what links here,
  // access and details are the side sheet the head's menu opens.
  let side = $state<'outline' | 'comments' | 'history' | 'backlinks' | 'access' | 'details' | undefined>(undefined);
  let outline = $state<{ id: string; label: string; level: number }[]>([]);
  const locateHeading = (id: string) => bodyElement?.querySelector<HTMLElement>(`[data-block="${CSS.escape(id)}"]`) ?? null;
  const toggleSide = (which: NonNullable<typeof side>) => (side = side === which ? undefined : which);
  let present = $state<string[]>([]);
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
  const day = (iso: string) => formatDay(iso, messages.locale);

  // History (UC-DOC-06): a version opened is compared with the page as it stands.
  let comparing = $state<PageVersionRead | undefined>(undefined);
  async function openVersion(seq: number) {
    if (!current) return;
    const state = await engine.refresh<PageVersionRead>({ path: versionPath(current.id, seq) });
    if (state.status === 'ready') comparing = state.data;
    else if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
  }

  // The version comes back through the live document too: the editors hold the text before it and
  // would write it back at their next save.
  async function restoreVersion(seq: number) {
    if (!current || !comparing) return;
    const body = comparing.body;
    const done = await act(() => pages.restoreVersion(current, seq), t('app.pages.version_restored', { seq: String(seq) }));
    if (done) {
      replaced = { body };
      comparing = undefined;
      await opened?.refresh();
    }
  }

  let namingSeq = $state<number | undefined>(undefined);
  let nameDraft = $state('');
  function nameAt(seq: number) {
    namingSeq = seq;
    nameDraft = versionState.find((each) => each.seq === seq)?.name ?? '';
  }
  // Opening the history compares the version before the newest with the page as it stands.
  $effect(() => {
    if (side !== 'history' || comparing || versionState.length === 0) return;
    const first = versionState[1] ?? versionState[0];
    if (first) untrack(() => void openVersion(first.seq));
  });
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
  const pageMenu = $derived(current && post ? [
    { id: 'history', label: t('app.pages.side_history'), icon: 'clock' as const },
    ...(current.writable ? [{ id: 'trash', label: t('app.pages.to_trash'), icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: true }] : []),
  ] : current ? [
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

  // --- A space with drafts (UC-DOC-15) -----------------------------------------------------------

  const pageDraft = $derived(current ? draftStateOf(current) : 'live');
  /** A body that replaces the live document's: the published one after a discarded draft, a version brought back. */
  let replaced = $state<{ body: string } | undefined>(undefined);

  async function release() {
    if (!current) return;
    await act(() => pages.release(current), t('app.pages.released'));
  }

  async function discard() {
    if (!current) return;
    const published = current.body ?? '';
    // A thrown-away draft answers nothing; a refusal is what says it failed.
    await act(() => pages.discardDraft(current), t('app.pages.draft_discarded'));
    if (refusal === undefined) replaced = { body: published };
  }

</script>

<!-- The open page is one work surface, at the hub's address and at a page's (page-system §7.8). -->
<WorkPage>
{#snippet top()}
{#if pageId && !current}
  <!-- The screen's one h1 while the page is read; once it is, the page's own title is it. -->
  <VisuallyHidden as="h1">{t('app.pages.title')}</VisuallyHidden>
{/if}
{#if hubId && !pageId}
  <!-- An empty space's head; an open page's head is its own (the navigation concept §2.4: the
       sidebar's *Contents* is the tree, the page draws none). -->
  <PageHeader
    title={t('app.pages.title')}
    isTitleInBar={viewport.isCompact}
    primary={mayWrite ? { label: t('app.pages.new'), icon: 'plus', onclick: () => startCreate() } : undefined}
    breadcrumb={hub
      ? {
          trail: [
            { id: hubId, label: hub.name, href: `/hubs/${hubId}` },
            current ? { id: 'pages', label: t('app.pages.title'), href: `/hubs/${hubId}/pages` } : { id: 'pages', label: t('app.pages.title') },
          ],
          mark: placeMark,
          label: t('app.pages.trail'),
          expandLabel: t('app.admin.expand_trail'),
          onnavigate: (id) => onnavigate(id === hubId ? `/hubs/${hubId}` : `/hubs/${hubId}/pages`),
        }
      : undefined}
  />
{/if}
{/snippet}
<div class="space">
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
            {#if mayImport}
              <TemplateCard name={t('app.pages.import')} summary={t('app.pages.import_hint')} mark="cloud-upload" data-space-import>
                {#snippet action()}
                  <Button size="sm" tone="secondary" onclick={() => capture.open({ hubId, mode: 'import', importFrom: { family: 'documents' } })}>{t('app.pages.import_open')}</Button>
                {/snippet}
              </TemplateCard>
            {/if}
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
        {#if side === 'history'}
          <PageHistory title={current.title} body={current.body ?? ''} changedBy={nameOf(current.updated_by)} changedAt={current.updated_at}
            versions={versionState} chosen={comparing} writable={current.writable} wide={viewport.isLarge} {nameOf} {when}
            onchoose={openVersion} onrestore={restoreVersion} onname={nameAt} onclose={() => (side = undefined)} />
        {/if}
        <!-- Kept while the history stands in its place: its live document is where a version comes back. -->
        <div class="reading-page" data-page-reading hidden={side === 'history'}>
          <!-- One row on top: only the path and the people (pread 1); the title follows once, under it. -->
          <div class="top-row" data-page-head>
            <Breadcrumb
              label={t('app.pages.breadcrumb')}
              expandLabel={t('app.pages.breadcrumb_more')}
              mark={placeMark}
              trail={[
                ...(hub ? [{ id: hub.id, label: hub.name, href: `/hubs/${hub.id}` }] : []),
                ...(post ? [{ id: 'posts', label: post.trail.label, href: post.trail.href }] : []),
                ...current.ancestors.map((above) => ({ id: above.id, label: above.title, href: `/pages/${above.id}` })),
                { id: current.id, label: current.title },
              ]}
            />
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
              {#if !viewport.isLarge && !viewport.isCompact && outline.length > 0}
                <IconButton size="sm" tone="secondary" icon="list" label={t('app.pages.outline')} aria-pressed={side === 'outline'} data-page-outline-open onclick={() => toggleSide('outline')} />
              {/if}
              <Button
                size="sm"
                tone="subtle"
                icon="message-square"
                aria-pressed={!viewport.isLarge && side === 'comments'}
                data-page-comments
                onclick={() => (viewport.isLarge ? document.querySelector<HTMLElement>('[data-page-comment-new] textarea')?.focus() : toggleSide('comments'))}
              >{String(openThreads)}<VisuallyHidden>{` ${t('app.pages.comments_open', { count: String(openThreads) })}`}</VisuallyHidden></Button>
              {#if post?.acts}{@render post.acts()}{/if}
              {#if !post && !current.shared && hubId}
                <Button size="sm" tone="secondary" icon="users" aria-pressed={side === 'access'} data-page-share onclick={() => toggleSide('access')}>{t('app.pages.share')}</Button>
              {/if}
              {#if !post}<StarToggle kind="PAGE" id={current.id} name={current.title} />{/if}
              <Menu label={t('app.pages.page_menu')} items={pageMenu} onselect={choose}>
                {#snippet trigger(props)}
                  <IconButton icon="ellipsis" label={t('app.pages.page_menu')} size="sm" tone="secondary" data-page-menu {...props} />
                {/snippet}
              </Menu>
            </Inline>
          </div>

          {#if current.cover}
            {#key current.id}<PageCover pageId={current.id} cover={current.cover} isWritable={current.writable} onchange={changeCover} bind:isPicking={isPickingCover} />{/key}
          {/if}
          <div class="reader" class:with-beside={viewport.isLarge}>
            <article class="column" class:with-gutter={current.writable && !viewport.isCompact} class:covered={!!current.cover} aria-labelledby="page-title">
              {#if !post && current.writable && (pageDraft === 'unpublished' || pageDraft === 'draft')}
                <!-- The drafts band (scene pnew 1): what only its writer sees, with publishing beside it. -->
                <Banner tone="warning" icon="lock">
                  <span data-page-draft-band><strong>{t('app.pages.draft_word')}</strong> {t(pageDraft === 'unpublished' ? 'app.pages.unpublished_band' : 'app.pages.draft_hint')}</span>
                  {#snippet action()}
                    <Inline gap="050">
                      <Button size="sm" tone="primary" onclick={release}>{t('app.pages.release')}</Button>
                      <Button size="sm" tone="subtle" onclick={pageDraft === 'draft' ? discard : trash}>{t('app.pages.discard')}</Button>
                    </Inline>
                  {/snippet}
                </Banner>
              {/if}
              {#if current.writable && !current.cover}
                <div class="page-adds"><Button size="sm" tone="subtle" icon="image" data-page-cover-add onclick={addCover}>{t('app.pages.cover_add')}</Button></div>
              {/if}
              {#if current.icon}<span class="icon" aria-hidden="true">{current.icon}</span>{/if}
              {#if post?.kicker}{@render post.kicker()}{/if}
              {#if current.writable}
                <!-- A textarea, as an entry's title is, so a long title wraps; Enter saves it. The
                     heading is read, the field is what is seen: the title appears once. -->
                <VisuallyHidden as="h1" id="page-title">{current.title}</VisuallyHidden>
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
                <h1 class="title" id="page-title">{current.title}</h1>
              {/if}
              {#if post}
                {@render post.head()}
                <div class="post-labels">{#key current.id}<PageLabels page={current} onchoose={(label) => { contentsAsk.label = label; onnavigate(`/hubs/${hubId}/contents`); }} />{/key}</div>
                {#if post.band}{@render post.band()}{/if}
              {:else}
              <TrustLine
                owner={current.owner_id ? nameOf(current.owner_id) : undefined}
                ownerRole={t('app.pages.owner_role')}
                ownerless={t('app.pages.ownerless')}
                verification={current.due_for_review
                  ? { state: 'due', label: t('app.pages.due_for_review') }
                  : current.verified_until ? { state: 'verified', label: t('app.pages.verified_until', { date: day(current.verified_until) }) } : undefined}
                changed={t('app.pages.changed_ago', { when: formatRelative(current.updated_at, messages.locale), name: nameOf(current.updated_by) })}
                onverification={current.writable ? () => (side = 'details') : undefined}
              >
                {#snippet labels()}
                  {#if current.template}<Badge size="sm">{t('app.pages.is_template')}</Badge>{/if}
                  {#if current.restricted}<Badge size="sm" icon="lock">{t('app.pages.access_restricted_badge')}</Badge>{/if}
                  {#if current.shared}<Badge size="sm" tone="info">{t('app.pages.shared_with_you')}</Badge>{/if}
                  {#key current.id}<PageLabels page={current} onchoose={(label) => { contentsAsk.label = label; onnavigate(`/hubs/${hubId}/contents`); }} />{/key}
                {/snippet}
              </TrustLine>
              {/if}
              <!-- Published with nothing waiting is said by nothing: no sentence before the page's first (M-2, §2.1). -->
              {#if offersStarts}
                <PageStarts own={ownTemplates} chosen={chosenStart} onchoose={chooseStart} />
                {#if startShown && !viewport.isLarge}
                  {@render startPreview()}
                {/if}
              {/if}

              <div class="body" bind:this={bodyElement}>
                <PageEditor
                  bind:this={pageEditor}
                  page={current}
                  {spacePages}
                  views={hubViews}
                  collections={hubId ? containers.collectionsOf(hubId).map((each) => ({ id: each.id, name: each.name })) : []}
                  {onnavigate}
                  {reveal}
                  replaceWith={post?.replaceWith ?? replaced}
                  passages={viewport.isLarge || side === 'comments' ? passages : []}
                  onpassages={passagesMoved}
                  onpresent={(names) => (present = names)}
                  onheadings={(seen) => (outline = seen)}
                  oncomment={() => {
                    isAtBlock = true;
                    if (viewport.isLarge) document.querySelector<HTMLElement>('[data-page-comment-new] textarea')?.focus();
                    else side = 'comments';
                  }}
                  onblock={(seen) => {
                    block = seen;
                    // Words selected are a comment's passage, unless the person chooses the page.
                    if (seen?.selected) isAtBlock = true;
                  }}
                />
              </div>

              <!-- What links here, at the page's end (§3 knowledge): a quiet list, not a panel. -->
              {#if post}
                {#if post.aside && !viewport.isLarge}<div class="post-aside">{@render post.aside()}</div>{/if}
                {@render post.foot()}
              {:else if backlinkState.length > 0}
                <section class="backlinks" aria-labelledby="page-backlinks" data-page-backlinks>
                  <h2 class="caps" id="page-backlinks">{t('app.pages.links_here', { count: String(backlinkState.length) })}</h2>
                  <ul class="hits">
                    {#each backlinkState as link (link.source_kind + link.source_id)}
                      <li><a href={link.page_id ? `/pages/${link.page_id}` : `/items/${link.entry_id}`}>{link.title}</a></li>
                    {/each}
                  </ul>
                </section>
              {/if}
            </article>

            {#snippet startPreview()}
              {#if startShown}
                <PageStartPreview
                  title={startShown.title}
                  icon={startShown.icon}
                  lead={startShown.lead}
                  body={startShown.body}
                  subpages={startShown.subpages.length}
                  isBusy={isUsingStart}
                  onuse={useStart}
                  onclose={() => (chosenStart = undefined)}
                />
              {/if}
            {/snippet}

            {#snippet beside()}
              {#if post?.aside && viewport.isLarge}{@render post.aside()}{/if}
              {#if startShown && viewport.isLarge}
                {@render startPreview()}
              {/if}
              {#snippet threadCard(thread: (typeof threads)[number])}
                <article class="thread" class:resolved={thread.first.resolved} class:at-passage={!!thread.first.quote}>
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

              {#if outline.length > 0 && viewport.isLarge}
                <PageOutline label={t('app.pages.outline')} headings={outline} locate={locateHeading} />
              {/if}
              <section class="comments" aria-labelledby="page-comments-head" data-page-margin>
                <div class="comments-head">
                  <h2 class="caps" id="page-comments-head">{t('app.pages.comments_count', { count: String(openThreads) })}</h2>
                  <span class="which" role="group" aria-label={t('app.pages.comments_which')}>
                    <button type="button" aria-pressed={!showResolved} onclick={() => (showResolved = false)}>{t('app.pages.comments_open_tab')}</button>
                    <button type="button" aria-pressed={showResolved} data-page-resolved onclick={() => (showResolved = true)}>{t('app.pages.comments_resolved_tab')}</button>
                  </span>
                </div>
                <!-- Whole-page comments on top of the same column (S-5), the passages' beside them. -->
                {#each pageThreads as thread (thread.first.id)}
                  {@render threadCard(thread)}
                {/each}
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
                {#if shownThreads.length === 0}
                  <p class="quiet">{showResolved ? t('app.pages.no_resolved') : t('app.pages.no_comments')}</p>
                {/if}
                <!-- Writing at the column's foot. -->
                <div class="write" data-page-comment-new>
                  <Textarea label={t('app.pages.comment_on_page')} isLabelHidden placeholder={t('app.pages.comment_on_page')} rows={2} bind:value={commentDraft} />
                  {#if block}
                    <Switch label={t('app.pages.comment_at_block', { words: (block.selected ?? block.text).slice(0, 60) })} checked={isAtBlock} onchange={(event: Event) => (isAtBlock = (event.currentTarget as HTMLInputElement).checked)} />
                  {/if}
                  {#if commentDraft.trim() !== ''}
                    <Button size="sm" onclick={addComment}>{t('app.pages.comment_send')}</Button>
                  {/if}
                </div>
              </section>
            {/snippet}

            {#if viewport.isLarge}
              <aside class="beside" aria-label={t('app.pages.beside')}>{@render beside()}</aside>
            {:else}
              <Sheet
                title={side === 'outline' ? t('app.pages.outline') : t('app.pages.comments_count', { count: String(openThreads) })}
                closeLabel={t('app.pages.side_close')}
                isOpen={side === 'outline' || side === 'comments'}
                onClose={() => (side = undefined)}
              >
                {#if side === 'outline'}
                  <PageOutline label={t('app.pages.outline')} headings={outline} locate={locateHeading} onselect={() => (side = undefined)} />
                {:else}
                  {@render beside()}
                {/if}
              </Sheet>
            {/if}
          </div>

          {#if viewport.isCompact}
            <!-- The phone: reading first, the rest at the foot (§7). -->
            <nav class="foot" aria-label={t('app.pages.foot')} data-page-foot>
              <Button size="sm" tone="subtle" icon="list" disabledReason={outline.length === 0 ? t('app.pages.outline_empty') : undefined} onclick={() => toggleSide('outline')}>{t('app.pages.foot_contents')}</Button>
              <Button size="sm" tone="subtle" icon="message-square" onclick={() => toggleSide('comments')}>{t('app.pages.foot_comments', { count: String(openThreads) })}</Button>
              {#if current.writable}
                <Button size="sm" tone="subtle" icon="pencil" onclick={() => bodyElement?.querySelector<HTMLElement>('[contenteditable="true"]')?.focus()}>{t('app.pages.foot_edit')}</Button>
              {/if}
            </nav>
          {/if}

          {#if side === 'backlinks' || side === 'access' || side === 'details'}
            <aside class="side" aria-label={sideTitle} data-page-side={side}>
              <Inline justify="between" align="center">
                <h2 class="heading">{sideTitle}</h2>
                <IconButton size="sm" icon="x" label={t('app.pages.side_close')} onclick={() => (side = undefined)} />
              </Inline>
              {#if side === 'backlinks'}
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
      {/key}
    {/if}
  </section>
</div>
</WorkPage>


{#if isMoving && current}
  <Dialog size="s" bind:isOpen={isMoving} title={t('app.pages.move_title', { title: current.title })} dismissLabel={t('app.pages.cancel')}
    primary={{ label: t('app.pages.move'), onclick: move }}>
    <Select label={t('app.pages.move_under')} options={moveOptions} bind:value={moveParent} />
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isMoving = false)}>{t('app.pages.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if namingSeq !== undefined}
  <Dialog size="s" isOpen title={t('app.pages.version_name')} dismissLabel={t('app.pages.cancel')} onClose={() => (namingSeq = undefined)}>
    <Input label={t('app.pages.version_name')} bind:value={nameDraft} />
    {#snippet actions()}
      <Button tone="subtle" onclick={() => (namingSeq = undefined)}>{t('app.pages.cancel')}</Button>
      <Button tone="primary" onclick={nameVersion}>{t('app.pages.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .post-labels { margin-block: var(--sp-100); }
  .post-aside { margin-block-start: var(--sp-400); }
  .space {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--sp-200);
  }
  .start { display: grid; gap: var(--sp-200); }
  .heading { display: grid; gap: calc(var(--sp-050) + var(--sp-025)); }
  /* A side section's name on the scale, not the browser's h3 step (16.38 px). */
  h3.heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .here { display: inline-flex; }
  .excerpt, .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .page { min-inline-size: 0; }
  .title {
    margin: 0;
    flex: 0 1 auto;
    min-inline-size: 0;
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
    font-family: var(--font-display);
    font-size: var(--fs-450);
    font-weight: var(--fw-bold);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-primary);
    padding: var(--sp-025) var(--sp-050);
    /* The text, not the field's edge, lines up with the trail above it. */
    margin-inline-start: calc(-1 * (var(--sp-050) + var(--bw-hairline)));
  }
  textarea.title { resize: none; overflow: hidden; field-sizing: content; font-family: var(--font-display); }
  textarea.title:hover { border-color: var(--border-subtle); }
  textarea.title:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .reading-page[hidden] { display: none; }
  .reading-page { display: grid; gap: var(--sp-200); }
  .top-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-100);
  }
  /* The reading column and, from 1240 px, the outline and the comments beside it (S-1, S-3, S-5). */
  .reader { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-400); align-items: start; }
  .reader.with-beside { grid-template-columns: minmax(0, 1fr) var(--layout-rail); }
  .column {
    display: grid;
    gap: var(--sp-100);
    inline-size: 100%;
    max-inline-size: var(--layout-reading);
    min-inline-size: 0;
    justify-self: center;
  }
  /* "+" and the handle stand in a gutter of their own before the text, never at the frame's edge (pwrite 3). */
  .column.with-gutter {
    box-sizing: border-box;
    max-inline-size: calc(var(--layout-reading) + var(--sp-1000));
    padding-inline-start: var(--sp-1000);
  }
  .icon { font-size: var(--fs-700); line-height: var(--lh-tight); }
  .column .title { font-size: var(--fs-600); }
  .beside {
    position: sticky;
    inset-block-start: var(--sp-200);
    display: grid;
    gap: var(--sp-300);
    max-block-size: calc(100dvh - var(--sp-400));
    overflow-y: auto;
  }
  .comments { display: grid; gap: var(--sp-150); }
  .comments-head { display: flex; align-items: baseline; justify-content: space-between; gap: var(--sp-100); }
  .caps {
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .which { display: inline-flex; gap: var(--sp-050); font-size: var(--fs-075); }
  .which button {
    padding: 0;
    border: none;
    background: none;
    color: var(--text-secondary);
    font: inherit;
    cursor: pointer;
  }
  .which button[aria-pressed='true'] { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .which button:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .write {
    display: grid;
    gap: var(--sp-050);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-inline-start: var(--bw-thick) solid var(--accent-primary);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }
  .backlinks {
    display: grid;
    gap: var(--sp-100);
    margin-block-start: var(--sp-400);
    padding-block-start: var(--sp-200);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }
  .hits { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .foot {
    position: sticky;
    inset-block-end: 0;
    display: flex;
    justify-content: space-around;
    padding-block: var(--sp-050);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    background: var(--bg-canvas);
  }
  .body { display: grid; gap: var(--sp-100); min-inline-size: 0; }
  .side {
    display: grid;
    max-inline-size: var(--layout-reading);
    gap: var(--sp-150);
    align-content: start;
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  /* A thread's card as the margin draws it (pwrite): a raised card, the passage's mark at its start. */
  .thread {
    display: grid;
    gap: var(--sp-050);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    font-size: var(--fs-100);
  }
  .thread.at-passage { border-inline-start: var(--bw-thick) solid var(--accent-primary); }
  .thread.resolved { color: var(--text-secondary); }
  .margin { position: relative; display: grid; row-gap: var(--sp-100); }
  .margin.level { display: block; min-block-size: var(--margin-height); }
  .margin.level > .placed { position: absolute; inset-inline: 0; inset-block-start: var(--thread-top); }
  .quote {
    text-align: start;
    padding: 0;
    border: none;
    background: none;
    color: var(--text-secondary);
    font: inherit;
    font-style: italic;
    cursor: pointer;
  }
  .quote::before { content: open-quote; }
  .quote::after { content: close-quote; }
  .comment .text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
  /* Icon and title stand under the cover's picture, never on it (E-18); the icon reaches into its edge. */
  .covered .icon { position: relative; margin-block-start: calc(-1 * var(--sp-600)); }
  .page-adds { display: flex; gap: var(--sp-050); margin-block-end: var(--sp-050); }
  /* While reading (scene pread) nothing to make stands above the first words: with a pointer the add
     shows where the head is pointed at or reached by the keyboard; a touch screen always shows it. */
  @media (hover: hover) {
    .column:not(:hover) .page-adds:not(:focus-within) { opacity: 0; }
  }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .which button { display: inline-flex; align-items: center; min-block-size: var(--layout-target); min-inline-size: var(--layout-target); justify-content: center; }
    .which button { position: relative; }
    .which button::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
