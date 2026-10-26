<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Today*: the page the app opens on (UC-WRK-34), in the overview's place (ADR-0063 decision 1).
  //
  // It answers what is the reader's today across every hub and lets them act on it without going
  // anywhere: what is due on them, ticked off where it stands; each list as one tile with its
  // figure; each project's next due step; what waits in the jumble; what this device opened last.
  // Nobody else's day is shown - every read here is the reader's own or narrowed to what they may
  // see.
  //
  // **Composed of reads that already exist**: one wordless search for what is on the reader, the
  // collections of each hub with their figures (the same reads the tree makes), one entry per
  // project, the jumble's first page; what was opened last is this device's and is sent nowhere.
  // What the reads mean - the day's bands, which collection is a tile - is `lib/today.ts`.
  //
  // **Ticking off is the ordinary completion**: `items.setCompleted`, the moment if it is one, the
  // announcement - and queued like any other while the server cannot be reached. The row keeps its
  // place and shows the tick, so a mistaken one is undone where it was made.
  //
  // **Nothing here says what the person did not do** (check 6): what is past is listed plainly
  // under *Earlier*, with its date and without the overdue word.
  //
  // **Every panel says something when it is empty**, so "nothing here" is never mistaken for a
  // screen that has not finished loading.

  import { untrack } from 'svelte';

  import type { WorkItem } from '@hubtask/sync-engine';
  import { Button, EmptyState, ErrorState, Icon, IconButton, PageHeader, Skeleton, Stack, TaskRow } from '@hubtask/design-system/components';

  import { announcer } from '../lib/announce.svelte.ts';
  import { celebration } from '../lib/celebration.svelte.ts';
  import CelebrationSlot from '../lib/entries/CelebrationSlot.svelte';
  import CollectionFigure from '../lib/entries/CollectionFigure.svelte';
  import DueMark from '../lib/entries/DueMark.svelte';
  import EntryActions from '../lib/entries/EntryActions.svelte';
  import EntryMenu from '../lib/entries/EntryMenu.svelte';
  import { attachEntryGestures } from '../lib/entries/entryGestures.ts';
  import { entryMenu } from '../lib/entries/entryMenu.ts';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, holdsHere, hubCreationHere, mayDo, roleOrder, supports } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { jumble } from '../lib/data/jumble.svelte.ts';
  import { readMailSettings } from '../lib/data/mailintake.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import IntakeAddress from '../lib/mail/IntakeAddress.svelte';
  import { archivalOfItem } from '../lib/data/lifecycle.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { today } from '../lib/data/today.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatRelative, formatToday } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { recents } from '../lib/recents.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { bandsOf, greetingOf, isListFigure, isProjectFigure } from '../lib/today.ts';
  import { capture } from '../lib/capture/capture.svelte.ts';
  import { mayCaptureHere } from '../lib/capture/places.svelte.ts';
  import ContainerMark from '../lib/workspace/ContainerMark.svelte';
  import CreateContainerDialog from '../lib/workspace/CreateContainerDialog.svelte';
  import StarterDialog from '../lib/workspace/StarterDialog.svelte';
  import EntryPlace from '../lib/workspace/EntryPlace.svelte';

  interface Props {
    onnavigate?: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let isCreatingHub = $state(false);
  // The empty workspace's one action: a shared hub with STRUCTURE on the workspace, a private one
  // of one's own where the manifest offers it, nothing otherwise (UC-WRK-01 check 6, UC-ID-16
  // check 1). Not the head's: a workspace with hubs creates them in the navigation (ux-inventory
  // §E9).
  const creation = $derived(hubCreationHere());
  const createLabel = $derived(
    creation === 'shared' ? t('app.workspace.create_hub') : creation === 'private' ? t('app.workspace.create_private_hub') : undefined,
  );

  $effect(() => page.entitle(t('app.nav.today')));

  const zone = $derived(actor.zone);
  const now = $derived.by(() => Date.now());
  const dayLine = $derived(formatToday(messages.locale, zone, now));
  // The day starts with the person (design-system.md §11.7): a greeting by the hour where they are,
  // with the first word of their name where they gave one, beside the date.
  const firstName = $derived(actor.account?.display_name?.trim().split(/\s+/u)[0] || undefined);
  const greeting = $derived.by(() => {
    const code = `app.today.greeting_${greetingOf(zone, now)}`;
    return firstName ? t(`${code}_named`, { name: firstName }) : t(code);
  });

  // `untrack`, for the reason every store records: subscribing delivers the current state at once
  // and the listener writes the store, and writing it reads it - so an effect that both subscribes
  // and tracks that read re-runs on its own first delivery, for ever.
  $effect(() => untrack(() => today.open()));

  // The jumble's only for somebody who reads the workspace itself (UC-JUM-01 check 7): anybody else
  // would be refused the read, and the refusal recorded on every visit here.
  /** What waits: the undecided arrivals only - one sorted in or dismissed is decided. */
  const WAITING = 'NEW';
  const hasJumble = $derived(holdsHere('READ') === true);
  $effect(() => {
    if (!hasJumble) return;
    return untrack(() => jumble.open(WAITING));
  });

  // Every hub's collections, for their figures: the tree's own reads, so a hub the tree has open
  // costs nothing more. Keyed by the ids, so a hub arriving opens its level and nothing re-reads.
  const hubIds = $derived(containers.hubs.map((hub) => hub.id).join(' '));
  $effect(() => {
    const ids = hubIds ? hubIds.split(' ') : [];
    const stops = untrack(() => ids.map((id) => containers.openLevel(id)));
    return () => stops.forEach((stop) => stop());
  });

  const collections = $derived(containers.hubs.flatMap((hub) => containers.collectionsOf(hub.id)));
  /** Whether every hub's collections have answered: until then nobody knows which are lists. */
  const areCollectionsSettled = $derived(
    containers.hubsState.status === 'ready' && containers.hubs.every((hub) => !containers.isLevelLoading(hub.id)),
  );
  const lists = $derived(collections.filter(isListFigure));
  const projects = $derived(collections.filter(isProjectFigure));
  const listIds = $derived(new Set(lists.map((list) => list.id)));

  const projectIds = $derived(projects.map((project) => project.id).join(' '));
  $effect(() => {
    const ids = projectIds ? projectIds.split(' ') : [];
    const stops = untrack(() => ids.map((id) => today.openStep(id)));
    return () => stops.forEach((stop) => stop());
  });

  /** The bands, from the entries as the server answered them: a tick does not move a row. */
  const bands = $derived(bandsOf(today.mine, zone, now, (id) => listIds.has(id)));
  const isMineEmpty = $derived(bands.earlier.length + bands.today.length + bands.later.length === 0);

  const failure = $derived(today.state.status === 'failed' ? renderProblem(today.state.error, messages) : undefined);
  const isLoading = $derived(today.state.status === 'loading' || today.state.status === 'idle' || !areCollectionsSettled);

  const waiting = $derived(jumble.of(WAITING));
  // The workspace's mail intake address, for the card while nothing waits and for the first steps
  // (ADR-0094): read once where the operator's receiving domain makes one and the intake is on.
  // Only its administrators may read the settings; for everybody else nothing is drawn.
  let jumbleAddress = $state<string | undefined>(undefined);
  $effect(() => {
    if (!hasJumble || manifest.value?.features?.mail_domain !== true) return;
    untrack(() => void readMailSettings().then((read) => (jumbleAddress = read?.enabled ? (read.address ?? undefined) : undefined), () => undefined));
  });
  const isJumbleLoading = $derived(jumble.stateOf(WAITING).status === 'loading' || jumble.stateOf(WAITING).status === 'idle');

  // What this page ticked, by entry: the search is a walk already made and is not re-read for one
  // entry (as on the search page), so the row shows the answer the completion brought back.
  let ticked = $state<Record<string, boolean>>({});
  let writeFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const isDone = (item: WorkItem) => ticked[item.id] ?? item.completion?.is_completed ?? false;

  async function toggle(item: WorkItem) {
    writeFailure = undefined;
    const wasDone = isDone(item);
    try {
      // Queued while the server is away, the answer is the copy's prediction - and nothing where
      // the copy does not hold the entry yet, which is still a completion on its way.
      const answered: WorkItem | undefined = await items.setCompleted(item.id, !wasDone, crypto.randomUUID());
      ticked = { ...ticked, [item.id]: answered?.completion?.is_completed ?? !wasDone };
      announcer.say(t(wasDone ? 'app.entries.reopened_announced' : 'app.entries.completed_announced', { title: item.title }));
      if (answered?.completion?.is_completed) void celebration.celebrate(answered);
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /**
   * A row's menu: the one every surface draws (UC-WRK-28 check 1). *Today* mixes collections and
   * holds no level, so, as on the search page, the role is read along each row's collection with
   * the rows and the moves wait for the level read when the menu opens.
   */
  const rowPath = (item: WorkItem) => ({
    hubId: containers.find(item.collection_id)?.parent_id ?? undefined,
    collectionId: item.collection_id,
  });
  const shown = $derived([
    ...bands.earlier,
    ...bands.today,
    ...bands.later,
    ...projects.map((project) => today.stepOf(project.id)).filter((step): step is WorkItem => step !== undefined),
  ]);
  $effect(() => {
    const paths = shown.map(rowPath);
    untrack(() => {
      for (const path of paths) people.open(path);
    });
  });

  function menuOf(item: WorkItem) {
    const role = people.roleOf(rowPath(item), actor.account?.id, roleOrder()) ?? undefined;
    const isMine = item.assignee_id != null && item.assignee_id === actor.account?.id;
    return entryMenu(
      {
        surface: 'today',
        item: { title: item.title, isCompleted: isDone(item), hasDue: Boolean(item.due_at) },
        archival: archivalOfItem(item, false),
        isCollectionReadOnly: containers.find(item.collection_id)?.effective_archived ?? false,
        mayChange: mayDo(role, 'change', isMine),
        mayCreate: mayDo(role, 'create', isMine) === true,
        mayShare: holds(role, 'MANAGE_MEMBERS').status === 'permitted',
        carries: (capability) => supports(item.type, capability).status === 'permitted',
        level: actions?.levelOf(item),
      },
      t,
    );
  }

  const menus = $state<Record<string, EntryMenu | undefined>>({});
  let actions = $state<EntryActions | undefined>(undefined);
  let panels = $state<HTMLElement | null>(null);

  function openMenu(id: string) {
    const item = shown.find((each) => each.id === id);
    if (item) actions?.readLevel(item);
    menus[id]?.open(0);
  }

  // A right click, the context-menu key and a long press open the same menu as its ⋯.
  $effect(() => {
    const node = panels;
    if (!node) return;
    return attachEntryGestures(node, {
      idAt: (target) => (target as Element | null)?.closest?.('[data-entry-row]')?.getAttribute('data-entry-row') ?? undefined,
      open: openMenu,
    });
  });

  /**
   * How many of the reader's rows *On you* draws before it says how many more and leads to the
   * search (concept round 2, §1): the day at a glance, not the whole list.
   */
  const ROWS_SHOWN = 6;
  const shownBands = $derived.by(() => {
    let left = ROWS_SHOWN;
    const take = (rows: readonly WorkItem[]) => {
      const out = rows.slice(0, Math.max(0, left));
      left -= out.length;
      return out;
    };
    return { earlier: take(bands.earlier), today: take(bands.today), later: take(bands.later) };
  });
  const mineTotal = $derived(bands.earlier.length + bands.today.length + bands.later.length);
  const mineHidden = $derived(mineTotal - shownBands.earlier.length - shownBands.today.length - shownBands.later.length);

  /** The jumble's acts at a row: only for who may decide about an arrival. */
  const mayWriteJumble = $derived(holdsHere('WRITE_ITEMS') === true);
  const channelIcon = (channel: string) => (channel === 'EMAIL' ? 'mail' : channel === 'WEBHOOK' ? 'zap' : 'inbox');
  const subjectOf = (arrival: { raw_subject?: string | null }) => arrival.raw_subject?.trim() || t('app.jumble.no_subject');
  async function dismissArrival(id: string) {
    try {
      await jumble.dismiss(id);
      announcer.say(t('app.jumble.dismissed_announced'));
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /**
   * A workspace with nothing in it yet for this reader: no entry on them, no list, no project,
   * nothing opened, nothing waiting. Instead of empty cards, the first steps (concept round 2, §1).
   */
  const isNewWorkspace = $derived(
    !isLoading && !failure && !(hasJumble && isJumbleLoading) && isMineEmpty && lists.length === 0 && projects.length === 0 && recents.rows.length === 0 && (!hasJumble || waiting.length === 0),
  );
  const mayInvite = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const hasCompany = $derived(people.along({}).filter((membership) => membership.scope_type === 'TENANT').length > 1);

  /** Whether anything stands beside *On you*: an empty card is not drawn, so the column may be empty. */
  const hasSide = $derived(lists.length > 0 || projects.length > 0 || (hasJumble && (waiting.length > 0 || jumbleAddress !== undefined)) || recents.rows.length > 0);

  const moment = $derived(celebration.current);

  const recentHref = (kind: string, id: string) => (kind === 'item' ? `/items/${id}` : kind === 'hub' ? `/hubs/${id}` : `/collections/${id}`);
</script>

{#snippet entry(item: WorkItem, hasPlace: boolean)}
  {#if moment && moment.item.id === item.id}
    <CelebrationSlot current={moment} />
  {/if}
  <div data-entry-row={item.id}>
    <TaskRow
      type={item.type}
      title={item.title}
      href={`/items/${item.id}`}
      isCompleted={isDone(item)}
      completeLabel={t('app.entries.complete', { title: item.title })}
      hasTwist={false}
      onToggleComplete={() => void toggle(item)}
    >
      {#snippet trailing()}
        {#if hasPlace}<EntryPlace collectionId={item.collection_id} withHub />{/if}<DueMark {item} />
        <EntryMenu
          bind:this={menus[item.id]}
          title={item.title}
          items={menuOf(item)}
          onselect={(chosen) => void actions?.run(item, chosen)}
          onopening={() => actions?.readLevel(item)}
        />
      {/snippet}
    </TaskRow>
  </div>
{/snippet}

{#snippet band(code: string, rows: readonly WorkItem[])}
  {#if rows.length > 0}
    <h3 class="band">{t(code)}</h3>
    <Stack gap="050">
      {#each rows as item (item.id)}{@render entry(item, true)}{/each}
    </Stack>
  {/if}
{/snippet}

<Stack gap="300">
  <!-- The day starts with the person (§11.7): the greeting is the heading, the date at its end. -->
  <PageHeader title={greeting} isTitleInBar={viewport.isCompact}>
    {#snippet aside()}<span class="date">{dayLine}</span>{/snippet}
  </PageHeader>

  {#if actor.hasNoAccess}
    <!-- Nobody gave this person anything yet: said in a sentence rather than an empty workspace
         without explanation (UC-ID-08 check 2), and without *Create hub*, which the server refuses
         to an account holding no role anywhere - a private hub of one's own included. -->
    <EmptyState kind="unused" title={t('app.workspace.no_access')} description={t('app.workspace.no_access_detail')} icon="hub" />
  {:else if containers.hasNoHubs && containers.hubsState.status === 'ready'}
    <!-- A workspace with nothing in it has one thing to say and one thing to do. -->
    <EmptyState kind="unused" title={t('app.workspace.no_hubs')} icon="hub">
      {#snippet action()}
        {#if createLabel}
          <Button icon="plus" onclick={() => (isCreatingHub = true)}>{createLabel}</Button>
        {/if}
      {/snippet}
    </EmptyState>
  {:else}
    <!-- The day at a glance (§11.7): each figure a way into the search that lists it. -->
    {#if !isLoading && !failure}
      <ul class="glance" aria-label={t('app.today.glance')}>
        <li><a href={`/search?${new URLSearchParams({ f: 'who:me is:open due:today' })}`}><b>{bands.today.length}</b> {t('app.today.glance_today', { count: bands.today.length })}</a></li>
        <!-- What is late, said as late (UC-WRK-34 check 6, as changed): the figure in the danger
             ink with its mark, so it is not the colour alone (rule 3). -->
        <li><a data-late={bands.earlier.length > 0 ? '' : undefined} href={`/search?${new URLSearchParams({ f: 'who:me is:open due:overdue' })}`}><b>{bands.earlier.length}</b> {#if bands.earlier.length > 0}<span class="hot-mark" aria-hidden="true"><Icon name="triangle-alert" size="sm" /></span>{/if}{t('app.today.glance_overdue', { count: bands.earlier.length })}</a></li>
        <li><a href={`/search?${new URLSearchParams({ f: 'who:me is:open due:week' })}`}><b>{bands.later.length}</b> {t('app.today.glance_later', { count: bands.later.length })}</a></li>
        {#if hasJumble}
          <li><a href="/jumble"><b>{waiting.length}</b> {t('app.today.glance_jumble', { count: waiting.length })}</a></li>
        {/if}
        <li><a href={`/search?${new URLSearchParams({ f: 'who:me is:open' })}`}><b>{bands.earlier.length + bands.today.length + bands.later.length}</b> {t('app.today.glance_mine', { count: bands.earlier.length + bands.today.length + bands.later.length })}</a></li>
      </ul>
    {/if}
    <div class="panels" data-alone={hasSide ? undefined : ''} bind:this={panels}>
      <!-- What is on the reader, split where their own day divides it. -->
      <section class="panel mine" aria-labelledby="today-mine">
        <Stack gap="150">
          <div class="head-row">
            <h2 class="head" id="today-mine">{isNewWorkspace ? t('app.today.first_steps') : t('app.today.mine')}</h2>
            {#if !isLoading && !failure && mineTotal > 0}<span class="head-count">{t('app.today.mine_count', { count: mineTotal })}</span>{/if}
          </div>
          {#if writeFailure}
            <p class="failure" role="alert">{writeFailure.message}</p>
          {/if}
          {#if failure}
            <ErrorState
              title={failure.message}
              reference={failure.reference}
              referenceLabel={t('app.reference')}
              retryLabel={t('app.retry')}
              onRetry={() => today.open()()}
            />
          {:else if isLoading}
            <div aria-busy="true"><Skeleton lines={3} /></div>
          {:else if isNewWorkspace}
            <!-- The first steps, each ticked where it is already done, each with its act. -->
            <ul class="steps">
              <li data-done=""><span class="step-mark" aria-hidden="true"><Icon name="check" size="sm" /></span><span class="step-word">{t('app.today.step_hub')}</span></li>
              <li>
                <span class="step-mark" aria-hidden="true"></span><span class="step-word">{t('app.today.step_capture')}</span>
                {#if mayCaptureHere()}<Button size="sm" tone="secondary" icon="plus" onclick={() => capture.summon()}>{t('app.capture.open_field')}</Button>{/if}
              </li>
              {#if mayInvite}
                <li data-done={hasCompany ? '' : undefined}>
                  <span class="step-mark" aria-hidden="true">{#if hasCompany}<Icon name="check" size="sm" />{/if}</span><span class="step-word">{t('app.today.step_invite')}</span>
                  {#if !hasCompany}<Button size="sm" tone="secondary" onclick={() => onnavigate?.('/administration/people')}>{t('app.today.step_invite_act')}</Button>{/if}
                </li>
              {/if}
              {#if jumbleAddress}
                <li data-step-mail>
                  <span class="step-mark" aria-hidden="true"></span><span class="step-word">{t('app.today.step_mail')}</span>
                  <IntakeAddress address={jumbleAddress} />
                </li>
              {/if}
            </ul>
          {:else if isMineEmpty}
            <!-- An empty card offers the next act, centred (§13, G4). -->
            <div class="empty">
              <p class="none">{t('app.today.mine_none')}</p>
              {#if mayCaptureHere()}
                <Button size="sm" icon="plus" onclick={() => capture.summon()}>{t('app.capture.open_field')}</Button>
              {/if}
            </div>
          {:else}
            {@render band('app.today.earlier', shownBands.earlier)}
            {@render band('app.today.today', shownBands.today)}
            {@render band('app.today.later', shownBands.later)}
            {#if mineHidden > 0 || today.hasMore}
              <!-- The rest is the search's: `has_more` as well as the rows held back, because a
                   search answers what the caller may see, so a short page is not the end. -->
              <p class="more">
                {#if mineHidden > 0}<span>{t('app.today.mine_hidden', { count: mineHidden })} · </span>{/if}<a href={`/search?${new URLSearchParams({ f: 'who:me is:open' })}`}>{t('app.today.mine_more')}</a>
              </p>
            {/if}
          {/if}
        </Stack>
      </section>

      <!-- Beside it, what the day is made of besides: the lists, the projects, the jumble, what
           was opened last. -->
      {#if hasSide}
      <div class="side">
        <!-- A list is one tile with its figure, never its entries one by one (check 2). -->
        {#if lists.length > 0}
          <section class="panel" aria-labelledby="today-lists">
            <Stack gap="150">
              <h2 class="head" id="today-lists">{t('app.today.lists')}</h2>
              <ul class="tiles">
                {#each lists as list (list.id)}
                  <li>
                    <a class="tile" href={`/collections/${list.id}`} data-token={list.color_token ?? undefined}>
                      <span class="tile-name"><ContainerMark look={list} size="md" /><span class="name">{list.name}</span></span>
                      {#if list.figure}<span class="tile-figure"><CollectionFigure figure={list.figure} name={list.name} /></span>{/if}
                    </a>
                  </li>
                {/each}
              </ul>
            </Stack>
          </section>
        {/if}

        <!-- A project shows how far it is and the step that is due next (check 3). -->
        {#if projects.length > 0}
          <section class="panel" aria-labelledby="today-projects">
            <Stack gap="200">
              <h2 class="head" id="today-projects">{t('app.today.projects')}</h2>
              {#each projects as project (project.id)}
                {@const step = today.stepOf(project.id)}
                {@const stepState = today.stepStateOf(project.id).status}
                <div class="project">
                  <a class="project-name" href={`/collections/${project.id}`}>
                    <ContainerMark look={project} /><span class="name">{project.name}</span>
                  </a>
                  {#if project.figure}<CollectionFigure figure={project.figure} name={project.name} />{/if}
                  {#if step}
                    <div class="step">
                      <span class="step-word">{t('app.today.next_step')}</span>
                      {@render entry(step, false)}
                    </div>
                  {:else if stepState === 'ready'}
                    <p class="none">{t('app.today.step_none')}</p>
                  {/if}
                </div>
              {/each}
            </Stack>
          </section>
        {/if}

        <!-- What arrived and has not been put anywhere, as a small inbox: where it came from, from
             whom and how long ago, and the two decisions at the row (concept round 2, §1). Not
             drawn while nothing waits: an empty card is not a card. -->
        {#if hasJumble && waiting.length > 0}
          <section class="panel" aria-labelledby="today-jumble">
            <Stack gap="100">
              <div class="head-row">
                <h2 class="head" id="today-jumble">{t('app.today.jumble')}</h2>
                <span class="head-count">{t(jumble.moreAfter(WAITING) ? 'app.today.jumble_waiting_more' : 'app.today.jumble_waiting', { count: waiting.length })}</span>
              </div>
              <ul class="inbox">
                {#each waiting.slice(0, 3) as arrival (arrival.id)}
                  <li class="arrival">
                    <span class="channel" data-channel={arrival.channel} aria-hidden="true"><Icon name={channelIcon(arrival.channel)} size="sm" /></span>
                    <a class="subject" href="/jumble">{subjectOf(arrival)}</a>
                    <span class="from">{[arrival.sender, arrival.received_at ? formatRelative(arrival.received_at, messages.locale) : undefined].filter(Boolean).join(' · ')}</span>
                    {#if mayWriteJumble}
                      <span class="acts">
                        <Button size="sm" tone="secondary" onclick={() => onnavigate?.('/jumble')}>{t('app.today.jumble_sort')}</Button>
                        <IconButton icon="x" size="sm" label={t('app.today.jumble_dismiss', { subject: subjectOf(arrival) })} onclick={() => void dismissArrival(arrival.id)} />
                      </span>
                    {/if}
                  </li>
                {/each}
              </ul>
              <p class="more"><a href="/jumble">{t(jumble.moreAfter(WAITING) ? 'app.today.jumble_more_than' : 'app.today.jumble_more', { count: waiting.length })} →</a></p>
            </Stack>
          </section>
        {:else if hasJumble && jumbleAddress && !isNewWorkspace}
          <!-- Nothing waits: the card says where to send things instead of standing empty. -->
          <section class="panel" aria-labelledby="today-jumble" data-jumble-address>
            <Stack gap="100">
              <h2 class="head" id="today-jumble">{t('app.today.jumble')}</h2>
              <p class="none">{t('app.today.jumble_none')}</p>
              <IntakeAddress address={jumbleAddress} lead={t('app.mail.jumble_lead')} />
            </Stack>
          </section>
        {/if}

        <!-- What this device opened last (check 5), as tiles: its mark and colour, its place and
             when - so the eye finds it again. Kept here and sent nowhere; not drawn while empty. -->
        {#if recents.rows.length > 0}
          <section class="panel" aria-labelledby="today-recent">
            <Stack gap="100">
              <h2 class="head" id="today-recent">{t('app.today.recent')}</h2>
              <ul class="recent">
                {#each recents.rows.slice(0, 4) as row (row.kind + row.id)}
                  {@const self = row.kind === 'item' ? undefined : containers.find(row.id)}
                  {@const place = row.placeId ? containers.find(row.placeId) : undefined}
                  <li>
                    <a class="rtile" href={recentHref(row.kind, row.id)}>
                      <span class="rmark" aria-hidden="true">
                        {#if self}<ContainerMark look={self} size="md" />{:else if place}<ContainerMark look={place} size="md" />{:else}<Icon name="square-check" size="sm" />{/if}
                      </span>
                      <span class="rtitle">{row.title}</span>
                      <span class="rwhere">{[place?.name, row.at ? formatRelative(new Date(row.at).toISOString(), messages.locale) : undefined].filter(Boolean).join(' · ')}</span>
                    </a>
                  </li>
                {/each}
              </ul>
            </Stack>
          </section>
        {/if}
      </div>
      {/if}
    </div>
  {/if}
</Stack>

<EntryActions bind:this={actions} onopen={(id) => onnavigate?.(`/items/${id}`)} {onnavigate} />

<StarterDialog onstarted={(id) => onnavigate?.(`/hubs/${id}`)} />
<CreateContainerDialog bind:isOpen={isCreatingHub} type="HUB" isPrivateOnly={creation === 'private'} oncreated={(id) => onnavigate?.(`/hubs/${id}`)} />

<style>
  /* One column on a phone; from `expanded` the concept's two - what is on the reader the wider,
     because it is the reason they are here, the rest stacked beside it. */
  .panels {
    display: grid;
    gap: var(--sp-200);
    grid-template-columns: minmax(0, 1fr);
    align-items: start;
  }

  .side { display: grid; gap: var(--sp-200); min-width: 0; }

  .head-row { display: flex; align-items: baseline; gap: var(--sp-100); }
  .head-count { margin-inline-start: auto; color: var(--text-subtle); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  /* The first steps of a new workspace: a checklist, each step's act at its end. */
  .steps { display: grid; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .steps li { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); min-block-size: var(--density-control-sm-min); }
  .step-mark {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-250);
    block-size: var(--sp-250);
    border: var(--bw-thick) solid var(--border-strong);
    border-radius: var(--r-sm);
  }
  .steps li[data-done] .step-mark { border-color: var(--accent-primary); background: var(--accent-primary); color: var(--text-inverse); }
  .steps li[data-done] .step-word { color: var(--text-subtle); text-decoration: line-through; }
  .step-word { flex: 1; min-width: 0; }

  /* The jumble as a small inbox: the channel's mark, the subject, who and when below it, the two
     decisions at the end. */
  .inbox { display: grid; margin: 0; padding: 0; list-style: none; }
  .arrival {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    grid-template-areas: 'mark subject acts' 'mark from acts';
    align-items: center;
    gap: 0 var(--sp-100);
    padding-block: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }
  .arrival:first-child { border-block-start: 0; }
  .channel {
    grid-area: mark;
    display: inline-grid;
    place-items: center;
    inline-size: var(--sp-300);
    block-size: var(--sp-300);
    border-radius: var(--r-sm);
    background: var(--label-slate-bg);
    color: var(--label-slate-fg);
  }
  .channel[data-channel='EMAIL'] { background: var(--label-blue-bg); color: var(--label-blue-fg); }
  .channel[data-channel='WEBHOOK'] { background: var(--label-green-bg); color: var(--label-green-fg); }
  .subject { grid-area: subject; overflow: hidden; color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; text-overflow: ellipsis; white-space: nowrap; }
  .subject:hover { text-decoration: underline; }
  .from { grid-area: from; overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
  .acts { grid-area: acts; display: inline-flex; align-items: center; gap: var(--sp-050); }

  /* What was opened last, as tiles two to a row: the mark, the title, the place and when. */
  .recent { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 22ch), 1fr)); gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .rtile {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    grid-template-areas: 'mark title' 'mark where';
    align-items: center;
    gap: 0 var(--sp-100);
    padding: var(--sp-100);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-primary);
    text-decoration: none;
  }
  .rtile:hover { background: var(--bg-surface-hover); }
  .rmark { grid-area: mark; display: inline-flex; color: var(--text-secondary); }
  .rtitle { grid-area: title; overflow: hidden; font-weight: var(--fw-medium); text-overflow: ellipsis; white-space: nowrap; }
  .rwhere { grid-area: where; overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) {
    .panels { grid-template-columns: minmax(0, 1.3fr) minmax(0, 1fr); }
    /* Nothing beside it: *On you* takes the width rather than leaving a column empty. */
    .panels[data-alone] { grid-template-columns: minmax(0, 1fr); }
  }

  .date { color: var(--text-secondary); font-size: var(--fs-100); white-space: nowrap; }

  /* A standalone element, so it takes the surface and the raised shadow rather than a border
     (`design-system.md` rule 1). The frame's canvas is what it sits on. */
  .panel {
    background: var(--bg-surface);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-raised);
    padding: var(--sp-150) var(--sp-200);
    min-width: 0;
  }

  .head {
    margin: 0;
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  /* The band's word is flush left: the same start as the panel's heading and the rows' boxes. */
  .band {
    margin: 0;
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    color: var(--text-secondary);
  }

  .none { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-100); }

  .more { margin: 0; font-size: var(--fs-075); }

  a { color: var(--text-brand); }

  a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }

  /* The tiles: a list in a glance, its name and its figure, as many to a row as fit. Inside a
     panel, so sunk into it rather than raised a second time. */
  .tiles {
    display: grid;
    gap: var(--sp-100);
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 16ch), 1fr));
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .tiles > li { display: flex; }

  .tile {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-primary);
    text-decoration: none;
  }

  .tile:hover { background: var(--bg-surface-hover); }

  /* A list's tile in its own colour where its people chose one (design-system.md §6): the place's
     colour, never a level's; the label's measured pair, so its words keep their contrast. */
  .tile[data-token] { background: var(--tile-bg); color: var(--tile-fg); }
  .tile[data-token]:hover { background: var(--tile-bg); filter: brightness(0.97); }
  .tile[data-token='slate'] { --tile-bg: var(--label-slate-bg); --tile-fg: var(--label-slate-fg); }
  .tile[data-token='blue'] { --tile-bg: var(--label-blue-bg); --tile-fg: var(--label-blue-fg); }
  .tile[data-token='teal'] { --tile-bg: var(--label-teal-bg); --tile-fg: var(--label-teal-fg); }
  .tile[data-token='green'] { --tile-bg: var(--label-green-bg); --tile-fg: var(--label-green-fg); }
  .tile[data-token='lime'] { --tile-bg: var(--label-lime-bg); --tile-fg: var(--label-lime-fg); }
  .tile[data-token='amber'] { --tile-bg: var(--label-amber-bg); --tile-fg: var(--label-amber-fg); }
  .tile[data-token='orange'] { --tile-bg: var(--label-orange-bg); --tile-fg: var(--label-orange-fg); }
  .tile[data-token='red'] { --tile-bg: var(--label-red-bg); --tile-fg: var(--label-red-fg); }
  .tile[data-token='magenta'] { --tile-bg: var(--label-magenta-bg); --tile-fg: var(--label-magenta-fg); }
  .tile[data-token='violet'] { --tile-bg: var(--label-violet-bg); --tile-fg: var(--label-violet-fg); }

  /* The day at a glance: a row of figures, each a link, wrapping where the width ends. */
  .glance {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-100);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .glance a {
    display: inline-flex;
    align-items: baseline;
    gap: var(--sp-050);
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    text-decoration: none;
    white-space: nowrap;
  }

  .glance a:hover { background: var(--bg-surface-hover); }

  /* Something late: the figure on the danger state's surface, with its mark beside the word
     (rule 3) - a status, drawn as one. */
  .glance a[data-late] {
    border-color: var(--status-danger-border);
    background: var(--status-danger-surface);
    color: var(--status-danger-text);
  }
  .hot-mark { display: inline-flex; }

  .glance b {
    font-size: var(--fs-400);
    font-weight: var(--fw-bold);
    line-height: 1;
    font-variant-numeric: tabular-nums;
  }

  /* An empty card's sentence and its act, centred in the card (§13, G4). */
  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--sp-100);
    padding-block: var(--sp-200);
    text-align: center;
  }

  .tile-name, .project-name {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    min-width: 0;
    font-weight: var(--fw-medium);
  }

  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .tile-figure { color: var(--text-secondary); font-size: var(--fs-075); }

  .project {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    min-width: 0;
  }

  .project-name { color: var(--text-primary); text-decoration: none; }

  .project-name:hover .name { text-decoration: underline; }

  .project :global(.figure) { color: var(--text-secondary); font-size: var(--fs-075); }

  .step {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
  }

  .step-word { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
