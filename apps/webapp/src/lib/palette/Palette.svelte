<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One palette for everything (UC-WRK-27, design-system.md §11.4): what is typed is answered with
  // entries, hubs and collections, places and settings pages, and the actions the reader may
  // perform here - and the bar search's narrowings and ways on stay as they were (ADR-0066).
  //
  // **Two drawings, one palette.** `host="bar"` is the bar's field from `medium` up, the list
  // opening under it while focus stays in the field; `host="dialog"` is the same field and list
  // in a dialog, below `medium`, where the bar has no field. The frame mounts one of them.
  //
  // **It lists, it does not decide.** Which places exist for this reader is the navigation's,
  // which actions the screen's head offers is the screen's (`commands.svelte.ts`), and what a
  // collection matched by name offers is `sources.ts`'s, asking what its screen asks; the frame's
  // own actions and the settings rows are `registry.ts`'s, over the questions the frame's controls
  // ask. Nothing here decides a role's meaning.
  //
  // **The words still travel in memory, not in the address.** `/search` is a `POST` with no `GET`
  // so that a term never reaches an access log (`security.md` §9); the words are handed to the
  // store, and the address carries only the narrowing.
  //
  // **Not a `Popover`** under the bar's field: that one moves focus into its surface, which is
  // wrong for a list under a field somebody is still typing in. Focus stays in the field; the
  // arrows move a highlight a screen reader is told about through `aria-activedescendant`.

  import { untrack } from 'svelte';

  import { Dialog, Icon, Kbd, SearchField, Spinner, VisuallyHidden, type IconName } from '@hubtask/design-system/components';
  import type { Container, WorkItem } from '@hubtask/sync-engine';

  import { palette } from './palette.svelte.ts';
  import { commands } from './commands.svelte.ts';
  import { groupOf, indexOfKey, keyOf, rank, rowId, rowsOf, step, type Candidate, type Group, type Row } from './rows.ts';
  import { collectionOffers, containerOffers, placeOffers, type Offer, type Placed, type Reach, type Target } from './sources.ts';
  import { frameActionOffers, recentOffers, settingOffers } from './registry.ts';
  import { land } from './landing.ts';
  import CreateContainerDialog from '../workspace/CreateContainerDialog.svelte';
  import { capture } from '../capture/capture.svelte.ts';
  import { mayCaptureHere } from '../capture/places.svelte.ts';
  import { recents } from '../recents.svelte.ts';
  import { ariaKeysOf, drawnKeysOf } from '../shortcuts.ts';
  import { announcer } from '../announce.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { holds, holdsHere, hubCreationHere, reachesAdministrationArea, roleOrder } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { itemPath } from '../data/item.svelte.ts';
  import { categoriesOf } from '../data/preferences.ts';
  import { levelWords } from '../data/levels.ts';
  import { people } from '../data/people.svelte.ts';
  import { privateHubs } from '../data/privatehubs.svelte.ts';
  import { queryFields } from '../data/query.ts';
  import { search } from '../data/search.svelte.ts';
  import { QUICK, compile, parse } from '../data/searchquery.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { DRAWABLE } from '../entries/layouts.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    host: 'bar' | 'dialog';
    /** Where a row sends the reader. The frame owns the router; this does not. */
    onnavigate: (path: string) => void;
    /** The account's verbs, which are the frame's. */
    onaccount: (action: 'help' | 'sign-out') => void;
    /** Where the reader stands, as the route names it: what the frame's actions are offered for. */
    here?: { readonly hub?: string; readonly collection?: string; readonly item?: string };
  }

  const { host, onnavigate, onaccount, here = {} }: Props = $props();

  let term = $state('');
  /** The highlighted row, held by identity (`rows.ts`). */
  let activeKey = $state<string | undefined>(undefined);
  let hits = $state<readonly WorkItem[]>([]);
  let searching = $state(false);
  let form = $state<HTMLFormElement | null>(null);

  const words = $derived(term.trim());
  const isOpen = $derived(palette.isOpen);

  // ------------------------------------------------------------------------------------------
  // The narrowings, as the bar search had them
  // ------------------------------------------------------------------------------------------

  /** Only the narrowings this installation can answer - the rule every filter surface keeps. */
  const fields = $derived(new Set(queryFields(manifest.value).map((field) => field.field)));
  const context = $derived({ fields, collection: () => undefined, label: () => undefined, hasCollection: false });
  const quick = $derived(
    QUICK.filter((each) => fields.has(each.field) && compile(parse(each.line), context).problems.length === 0),
  );

  // ------------------------------------------------------------------------------------------
  // What may be listed
  // ------------------------------------------------------------------------------------------

  /** Every hub and the collections of every hub read so far. */
  const placed = $derived<readonly Placed[]>(
    containers.hubs.flatMap((hub) => [{ container: hub }, ...containers.collectionsOf(hub.id).map((collection) => ({ container: collection, hub }))]),
  );

  const reach = $derived<Reach>({
    hasJumble: holdsHere('READ'),
    isAdministrationReachable: reachesAdministrationArea() === true,
    isInstanceReachable: manifest.isInstanceReachable,
    isSingleWorkspace: manifest.isSingleTenant,
    isPrivateHubsReadable: privateHubs.isReadable,
    hasAnyPrivateHub: privateHubs.hasAny,
    hasSeveralPeople: workspace.workspace?.several_people === true,
  });

  const pathOf = (container: Container) => ({ hubId: container.parent_id ?? undefined, collectionId: container.id });

  /** The collections whose names answer the words best: the three whose actions are offered. */
  const matched = $derived(
    words === ''
      ? []
      : rank(
          placed.filter((each) => each.container.type === 'COLLECTION').map((each) => ({ kind: 'container' as const, id: each.container.id, label: each.container.name })),
          words,
          messages.locale,
        )
          .slice(0, 3)
          .flatMap((candidate) => placed.filter((each) => each.container.id === candidate.id)),
  );

  // ------------------------------------------------------------------------------------------
  // Where the reader stands
  // ------------------------------------------------------------------------------------------

  /** The collection of the entry the reader is on, read through the entry page's own read. */
  let itemCollectionId = $state<string | undefined>(undefined);
  $effect(() => {
    const id = here.item;
    if (!isOpen || !id) {
      itemCollectionId = undefined;
      return;
    }
    return untrack(() =>
      engine.subscribe<WorkItem>({ path: itemPath(id) }, (next) => {
        if (next.status === 'ready') itemCollectionId = next.data.collection_id;
      }),
    );
  });

  const hereCollection = $derived.by(() => {
    const id = here.collection ?? itemCollectionId;
    return id ? containers.find(id) : undefined;
  });
  const hereHub = $derived.by(() => {
    const id = here.hub ?? hereCollection?.parent_id ?? undefined;
    return id ? containers.find(id) : undefined;
  });
  // The role on the hub, which decides *Create collection in …* as the tree's *New* asks it.
  $effect(() => {
    const hubId = isOpen ? hereHub?.id : undefined;
    if (!hubId) return;
    return untrack(() => people.open({ hubId }));
  });

  /**
   * The collection whose acts are offered without being named: the one of the entry on view. On
   * the collection's own page its head offers them already, and twice would be noise.
   */
  const standing = $derived<readonly Placed[]>(
    here.item && hereCollection?.type === 'COLLECTION' ? [{ container: hereCollection, hub: hereHub }] : [],
  );

  /** The frame's actions: capture, a collection in this hub, a hub. */
  const frameActs = $derived(
    frameActionOffers(
      {
        hubCreation: hubCreationHere(),
        hub: hereHub,
        mayShapeHub:
          hereHub !== undefined &&
          holds(people.roleOf({ hubId: hereHub.id }, actor.account?.id, roleOrder()) ?? undefined, 'STRUCTURE').status === 'permitted',
        isOnHubPage: here.hub !== undefined,
        mayCapture: mayCaptureHere(),
      },
      t,
    ),
  );

  /** The settings rows; no notification rows for an account mail cannot reach, whose screen draws none. */
  const settings = $derived(
    settingOffers(
      { categories: actor.account?.kind === 'USER' && !actor.account.email ? [] : categoriesOf(manifest.value) },
      t,
    ),
  );

  /** What may be offered, each with what pressing it does. */
  const offers = $derived<readonly (Offer | (Candidate & { readonly run: () => void }))[]>([
    ...containerOffers(placed),
    // The common actions first: with nothing typed they are what the list opens with.
    ...frameActs,
    ...commands.all.map((act) => ({
      kind: 'action' as const,
      id: act.id,
      label: act.label,
      // A head's plus makes something: found by the words that mean making, as the frame's own
      // *Create …* rows are, so "anlegen" finds *Eintrag hinzufügen*.
      keywords: act.icon === 'plus' ? t('app.palette.words_create') : undefined,
      shortcut: act.shortcut,
      run: act.run,
    })),
    ...[...standing, ...matched.filter((each) => !standing.some((stood) => stood.container.id === each.container.id))].flatMap((each) =>
      collectionOffers(each, placed, {
        role: people.roleOf(pathOf(each.container), actor.account?.id, roleOrder()),
        manifest: manifest.value,
        drawable: DRAWABLE,
        t,
      }).map((offer) => (offer.id.startsWith('new-in:') ? { ...offer, keywords: t('app.palette.words_create') } : offer)),
    ),
    ...placeOffers(reach, t),
    ...settings,
    ...hits.map((hit) => ({ kind: 'hit' as const, id: hit.id, label: hit.title ?? '', target: { kind: 'route' as const, path: `/items/${hit.id}` } })),
    ...recentOffers(recents.rows),
  ]);

  const byKey = $derived(new Map(offers.map((offer) => [`${offer.kind}:${offer.id}`, offer])));
  const hitById = $derived(new Map(hits.map((hit) => [hit.id, hit])));

  const sourced = $derived(words === '' ? [] : palette.sourced(words));
  const sourcedByKey = $derived(new Map(sourced.map((row) => [row.key, row])));

  const ranked = $derived(rank(offers, words, messages.locale));
  const rows = $derived(rowsOf(term, ranked, quick, sourced.map((row) => row.key)));
  const active = $derived(indexOfKey(rows, activeKey));

  const matchRows = $derived(rows.filter((row) => row.kind !== 'quick' && row.kind !== 'all' && row.kind !== 'refine'));
  const quickRows = $derived(rows.filter((row) => row.kind === 'quick'));
  const allRow = $derived(rows.find((row) => row.kind === 'all'));
  const refineRow = $derived(rows.find((row) => row.kind === 'refine'));

  // ------------------------------------------------------------------------------------------
  // Reads the answer needs, while it is open
  // ------------------------------------------------------------------------------------------

  // Every hub's collections, so a collection is found by name wherever it is - the reads the tree
  // makes when a hub is opened, shared with it; closed with the palette.
  const hubIds = $derived(isOpen && words !== '' ? containers.hubs.map((hub) => hub.id).join(',') : '');
  $effect(() => {
    const ids = hubIds === '' ? [] : hubIds.split(',');
    const opened = untrack(() => ids.map((id) => containers.openLevel(id)));
    return () => opened.forEach((close) => close());
  });

  // The role along each matched collection's path: what decides *New … in X*. Kept open for the
  // session, as every `people.open` is (the search screen opens a named collection's the same way).
  $effect(() => {
    for (const each of [...standing, ...matched]) untrack(() => people.open(pathOf(each.container)));
  });

  /**
   * The first few hits, a beat after the typing stops - a peek that never touches the search
   * screen's state. A generation, because the answer to "mil" can arrive after the answer to
   * "milk", and a list that showed it would ignore what was typed last.
   */
  let generation = 0;
  $effect(() => {
    const asked = words;
    if (!isOpen || asked === '') {
      hits = [];
      searching = false;
      return;
    }
    searching = true;
    const mine = (generation += 1);
    const timer = setTimeout(() => {
      void search
        .peek({ q: asked }, 5)
        .then((found) => {
          if (mine !== generation) return;
          hits = found;
          searching = false;
        })
        .catch(() => {
          // A list is an offer, not an answer: a failed peek shows no hits and the way on still
          // works. The screen it leads to is where a failure is reported, with its reference.
          if (mine !== generation) return;
          hits = [];
          searching = false;
        });
    }, 200);
    return () => clearTimeout(timer);
  });

  // How many rows answer, said once the entries are in - never a region mounted with text.
  $effect(() => {
    if (!isOpen || words === '' || searching) return;
    const count = matchRows.length;
    untrack(() => announcer.say(t('app.palette.count', { count: String(count) })));
  });

  // A new question has no highlighted row: Enter then means the words.
  $effect(() => {
    void term;
    activeKey = undefined;
  });

  // Opened by a shortcut, the bar's field takes focus.
  $effect(() => {
    if (host !== 'bar' || !isOpen) return;
    const node = form;
    if (node && !node.contains(document.activeElement)) node.querySelector('input')?.focus();
  });

  // ------------------------------------------------------------------------------------------
  // Pressing a row
  // ------------------------------------------------------------------------------------------

  /** Hands the words over and goes to the search screen, optionally narrowed or with the filter open. */
  function goSearch(narrowing: string, options: { handOver: boolean; openEditor?: boolean }) {
    if (options.handOver && words !== '') search.handOver(words);
    const carried = new URLSearchParams({
      ...(narrowing === '' ? {} : { f: narrowing }),
      ...(options.openEditor ? { edit: '1' } : {}),
    }).toString();
    finish(() => onnavigate(carried === '' ? '/search' : `/search?${carried}`));
  }

  /**
   * Empties the field and closes, then does what was chosen. Emptied because the screen it leads
   * to has its own field, and two fields showing one question are two places to edit it.
   */
  function finish(run: () => void) {
    term = '';
    activeKey = undefined;
    palette.perform(run);
  }

  /** The create dialog a row opened: held while it is open, so that moving on does not change it. */
  let creating = $state<Extract<Target, { kind: 'create' }> | undefined>(undefined);
  let isCreating = $state(false);

  function follow(target: Target) {
    if (target.kind === 'route') return finish(() => onnavigate(target.path));
    if (target.kind === 'account') return finish(() => onaccount(target.action));
    if (target.kind === 'search') return goSearch(target.line, { handOver: false });
    if (target.kind === 'capture') return finish(() => capture.summon());
    if (target.kind === 'create') {
      return finish(() => {
        creating = target;
        isCreating = true;
      });
    }
    if (target.kind === 'setting') {
      return finish(() => {
        onnavigate(target.path);
        land(target.row);
      });
    }
    const container = containers.find(target.containerId);
    if (target.layout || target.add) commands.intend({ containerId: target.containerId, layout: target.layout, add: target.add });
    finish(() => onnavigate(container?.type === 'HUB' ? `/hubs/${target.containerId}` : `/collections/${target.containerId}`));
  }

  function pick(row: Row | undefined) {
    if (!row) return;
    if (row.kind === 'quick') return goSearch(row.id, { handOver: true });
    if (row.kind === 'all') return goSearch('', { handOver: true });
    if (row.kind === 'refine') return goSearch('', { handOver: true, openEditor: true });
    if (row.kind === 'source') {
      const sourcedRow = sourcedByKey.get(row.id);
      const asked = words;
      if (sourcedRow) finish(() => sourcedRow.run(asked));
      return;
    }
    const offer = byKey.get(keyOf(row));
    if (!offer) return;
    if ('run' in offer) return finish(offer.run);
    follow(offer.target);
  }

  function submit(event: SubmitEvent) {
    event.preventDefault();
    // A highlighted row is what Enter presses; nothing highlighted means the words themselves -
    // "All results", or "Open search" with nothing typed - the row the key is drawn on.
    pick(active >= 0 ? rows[active] : allRow);
  }

  /**
   * The keys, caught on the way **down** to the field. Escape closes the list before it reaches
   * the field: **the browser empties a search input on Escape by itself**, and only
   * `preventDefault` stops that. With the list closed it travels on and empties the field, which
   * is the field's own behaviour.
   */
  function keydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      if (!isOpen) palette.enter();
      event.preventDefault();
      const next = step(active, rows.length, event.key === 'ArrowDown' ? 1 : -1);
      activeKey = next < 0 ? undefined : keyOf(rows[next]!);
      return;
    }
    if (event.key !== 'Escape' || !isOpen) return;
    event.preventDefault();
    event.stopPropagation();
    activeKey = undefined;
    palette.close();
  }

  /** Closing when focus leaves the whole bar palette, not on the field's own blur: a press on a row is a blur. */
  function focusout(event: FocusEvent) {
    if (host !== 'bar') return;
    const next = event.relatedTarget;
    if (next instanceof Node && event.currentTarget instanceof Node && event.currentTarget.contains(next)) return;
    activeKey = undefined;
    if (palette.isOpen) palette.dismiss();
  }

  // ------------------------------------------------------------------------------------------
  // Drawing a row
  // ------------------------------------------------------------------------------------------

  function iconOf(row: Row): IconName {
    if (row.kind === 'hit') return hitById.get(row.id)?.completion?.is_completed ? 'square-check' : 'square';
    if (row.kind === 'container') return containers.find(row.id)?.type === 'HUB' ? 'hub' : 'collection';
    if (row.kind === 'source') return 'sparkles';
    if (row.kind === 'place') return 'arrow-right';
    if (row.kind === 'setting') return 'settings';
    if (row.kind === 'recent') return 'clock';
    return 'zap';
  }

  function kindOf(row: Row): string {
    if (row.kind === 'hit') {
      const hit = hitById.get(row.id);
      return levelWords(hit ? containers.find(hit.collection_id)?.structure?.settings : undefined, hit?.type ?? '', t).label;
    }
    if (row.kind === 'container') return t('app.palette.kind', { kind: containers.find(row.id)?.type ?? '' });
    // A place opened last says what it was: its id starts with its kind (`registry.ts`).
    if (row.kind === 'recent') return t('app.palette.kind', { kind: row.id.slice(0, row.id.indexOf(':')).toUpperCase() });
    return t('app.palette.kind', { kind: row.kind });
  }

  /** The match rows under their headings, in the order `rank` put them - which is the arrows' order. */
  const groups = $derived.by(() => {
    const drawn: { id: Group | 'source'; rows: Row[] }[] = [];
    for (const row of matchRows) {
      const id = row.kind === 'source' ? 'source' : groupOf(row.kind as Candidate['kind']);
      const last = drawn.at(-1);
      if (last?.id === id) last.rows.push(row);
      else drawn.push({ id, rows: [row] });
    }
    return drawn;
  });

  function labelOf(row: Row): string {
    if (row.kind === 'source') return sourcedByKey.get(row.id)?.label ?? '';
    return byKey.get(keyOf(row))?.label ?? '';
  }

  function shortcutOf(row: Row): string | undefined {
    const id = byKey.get(keyOf(row))?.shortcut;
    return id && drawnKeysOf(id).length > 0 ? id : undefined;
  }

  const enterHintId = $derived(`${host}-palette-enter`);
</script>

{#snippet field()}
  <!-- Its own name, not the search screen's: two controls with one accessible name on a screen
       are two a screen reader cannot tell apart. -->
  <SearchField
    label={t('app.searchmenu.field')}
    isLabelHidden
    clearLabel={t('app.search.clear')}
    placeholder={t('app.palette.placeholder')}
    size={host === 'bar' ? 'sm' : 'md'}
    bind:value={term}
    onfocus={() => palette.enter()}
    onclick={() => palette.enter()}
    oninput={() => palette.enter()}
    role="combobox"
    aria-expanded={isOpen}
    aria-controls={isOpen ? 'search-menu' : undefined}
    aria-activedescendant={isOpen && active >= 0 ? rowId(rows[active]!) : undefined}
    autocomplete="off"
  />
{/snippet}

{#snippet option(row: Row)}
  {@const shortcut = shortcutOf(row)}
  {@const context = byKey.get(keyOf(row))?.context}
  <button
    type="button"
    role="option"
    id={rowId(row)}
    aria-selected={keyOf(row) === activeKey}
    aria-keyshortcuts={shortcut ? ariaKeysOf(shortcut).join(' ') : undefined}
    class="row"
    data-active={keyOf(row) === activeKey ? '' : undefined}
    onmouseenter={() => (activeKey = keyOf(row))}
    onclick={() => pick(row)}
  >
    <Icon name={iconOf(row)} />
    <span class="title">{labelOf(row)}</span>
    {#if context}<span class="context">{context}</span>{/if}
    <!-- What a row is, said to a reader; the group's heading already says it to the eye. -->
    <VisuallyHidden>{kindOf(row)}</VisuallyHidden>
    {#if shortcut}<span class="key" aria-hidden="true"><Kbd size="sm">{drawnKeysOf(shortcut)[0]}</Kbd></span>{/if}
  </button>
{/snippet}

{#snippet list()}
  <div class="menu" data-host={host} role="listbox" aria-label={t('app.searchmenu.label')} id="search-menu">
    {#each groups as group (group.id)}
      <div role="group" aria-labelledby={`${host}-palette-${group.id}`}>
        <p class="head" id={`${host}-palette-${group.id}`}>{t('app.palette.group', { group: group.id })}</p>
        {#each group.rows as row (keyOf(row))}
          {@render option(row)}
        {/each}
      </div>
    {/each}
    {#if words !== '' && searching}
      <p class="head"><Spinner size="sm" /> {t('app.searchmenu.searching')}</p>
    {:else if words !== '' && matchRows.length === 0}
      <p class="head">{t('app.searchmenu.none')}</p>
    {/if}

    {#if quickRows.length > 0}
      <!-- With words these narrow *them*; without, each is a list of its own. The heading says
           which, because a control that means two things without saying so surprises. -->
      <div role="group" aria-labelledby={`${host}-palette-quick`}>
        <p class="head" id={`${host}-palette-quick`}>
          {t(words === '' ? 'app.searchmenu.quick' : 'app.searchmenu.quick_with_words')}
        </p>
        <div class="quick">
          {#each quickRows as row (row.id)}
            <button
              type="button"
              role="option"
              id={rowId(row)}
              aria-selected={keyOf(row) === activeKey}
              class="pill"
              data-active={keyOf(row) === activeKey ? '' : undefined}
              onmouseenter={() => (activeKey = keyOf(row))}
              onclick={() => pick(row)}
            >
              {t(QUICK.find((each) => each.line === row.id)?.code ?? '')}
            </button>
          {/each}
        </div>
      </div>
    {/if}

    <div class="feet" role="group" aria-labelledby={`${host}-palette-on`}>
      <VisuallyHidden><span id={`${host}-palette-on`}>{t('app.palette.ways_on')}</span></VisuallyHidden>
      {#if allRow}
        <button
          type="button"
          role="option"
          id={rowId(allRow)}
          aria-selected={keyOf(allRow) === activeKey}
          aria-describedby={enterHintId}
          class="row foot"
          data-active={keyOf(allRow) === activeKey ? '' : undefined}
          onmouseenter={() => (activeKey = keyOf(allRow))}
          onclick={() => pick(allRow)}
        >
          <Icon name="search" />
          <span class="title">{t(words === '' ? 'app.searchmenu.open' : 'app.searchmenu.all')}</span>
          <span class="key" aria-hidden="true"><Kbd size="sm">⏎</Kbd></span>
          <VisuallyHidden><span id={enterHintId}>{t('app.palette.enter_hint')}</span></VisuallyHidden>
        </button>
      {/if}
      {#if refineRow}
        <button
          type="button"
          role="option"
          id={rowId(refineRow)}
          aria-selected={keyOf(refineRow) === activeKey}
          class="row foot"
          data-active={keyOf(refineRow) === activeKey ? '' : undefined}
          onmouseenter={() => (activeKey = keyOf(refineRow))}
          onclick={() => pick(refineRow)}
        >
          <Icon name="funnel" />
          <span class="title">{t('app.searchmenu.refine')}</span>
        </button>
      {/if}
    </div>
  </div>
{/snippet}

{#if host === 'bar'}
  <!-- A form, so that Enter submits the way every search field does, and the landmark a screen
       reader looks for when asked to find the search. -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <form
    class="entry"
    data-open={isOpen ? '' : undefined}
    role="search"
    aria-label={t('app.search.title')}
    bind:this={form}
    onsubmit={submit}
    onfocusout={focusout}
    onkeydowncapture={keydown}
    data-tour="palette"
  >
    <!-- The field and its list float as one: in use they are drawn over the bar, centred on where
         the field stands and as wide as the list needs, so nothing beside it moves - the field
         neither jumps nor pushes *Capture*, which it may cover while open. -->
    <div class="float">
      {@render field()}
      {#if !isOpen && term === ''}
        <!-- The way here from anywhere, as the concept draws it: the key at the field's end. -->
        <span class="keys" aria-hidden="true"><Kbd size="sm">{drawnKeysOf('palette')[0]}</Kbd></span>
      {/if}
      {#if isOpen}
        {@render list()}
      {/if}
    </div>
  </form>
{:else}
  <Dialog
    title={t('app.palette.title')}
    dismissLabel={t('app.palette.close')}
    isOpen={palette.isOpen}
    onClose={() => {
      if (palette.isOpen) palette.close();
    }}
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form class="sheet" role="search" aria-label={t('app.search.title')} bind:this={form} onsubmit={submit} onkeydowncapture={keydown}>
      {@render field()}
      {@render list()}
    </form>
  </Dialog>
{/if}

<!-- The one create dialog, as the tree's *New* opens it, for a row that creates a hub or a
     collection. Outside both drawings: the palette has closed by the time it opens. -->
{#if creating}
  <CreateContainerDialog
    bind:isOpen={isCreating}
    type={creating.type}
    parentId={creating.parentId}
    isPrivateOnly={creating.isPrivateOnly}
    oncreated={(id) => onnavigate(creating?.type === 'HUB' ? `/hubs/${id}` : `/collections/${id}`)}
  />
{/if}

<style>
  /* The field takes the slot whole; the slot is what is bounded, in `AppBar`. The list is
     positioned against this, which is why it is the containing block. */
  .entry { position: relative; display: block; min-inline-size: 0; min-block-size: var(--density-control-sm-min); }

  .float { position: relative; }

  /* In use: centred on the field's own middle, the open width, never into the logo's or the
     account's side of the bar. */
  .entry:focus-within > .float,
  .entry[data-open] > .float {
    position: absolute;
    inset-block-start: 0;
    inset-inline-start: 50%;
    z-index: var(--z-popover);
    inline-size: min(var(--layout-barsearch-open), 100vw - 2 * var(--layout-barsearch-side));
    min-inline-size: 100%;
    translate: -50% 0;
  }

  :global([dir='rtl']) .entry:focus-within > .float,
  :global([dir='rtl']) .entry[data-open] > .float { translate: 50% 0; }

  .keys { display: inline-flex; position: absolute; inset-block-start: 50%; inset-inline-end: var(--sp-100); transform: translateY(-50%); pointer-events: none; }
  /* The words make room for the key rather than run under it - a German placeholder is long. */
  .float:has(> .keys) :global(input) { padding-inline-end: calc(var(--sp-100) + var(--sp-500)); text-overflow: ellipsis; }
  .sheet { display: flex; flex-direction: column; gap: var(--sp-100); }

  .menu[data-host='bar'] {
    position: absolute;
    inset-inline: 0;
    inset-block-start: calc(100% + var(--sp-050));
    z-index: var(--z-popover);
    /* A narrow bar field must not squeeze the list into a column of ellipses. */
    min-inline-size: var(--layout-barsearch-width);
    max-block-size: 70vh;
    overflow-y: auto;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
  }

  .head {
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    margin: var(--sp-050) 0 var(--sp-025);
    padding-inline: var(--sp-100);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    inline-size: 100%;
    min-block-size: var(--density-control-md-min);
    padding-inline: var(--sp-100);
    border: 0;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .row[data-active] { background: var(--bg-surface-hover); }

  .title {
    flex: 1 1 auto;
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .context { flex: none; color: var(--text-secondary); font-size: var(--fs-075); white-space: nowrap; }


  .quick { display: flex; flex-wrap: wrap; gap: var(--sp-050); padding-inline: var(--sp-100); }

  .pill {
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-raised);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .pill[data-active], .pill:hover { border-color: var(--accent-primary); color: var(--text-primary); }

  .feet {
    margin-block-start: var(--sp-050);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    padding-block-start: var(--sp-050);
  }

  .foot { color: var(--text-secondary); font-size: var(--fs-075); }

  .key { display: inline-flex; flex: none; }
</style>
