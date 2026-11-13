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

  import { tick, untrack } from 'svelte';

  import { Button, Dialog, Icon, Kbd, ProductTile, SearchField, Sheet, Spinner, VisuallyHidden, type IconName } from '@hubtask/design-system/components';
  import type { Container, SettingDefinition, WorkItem } from '@hubtask/sync-engine';

  import { palette } from './palette.svelte.ts';
  import { commands } from './commands.svelte.ts';
  import { groupOf, indexOfKey, keyOf, rank, rowId, step, type Candidate, type Group, type Row } from './rows.ts';
  import { collectionOffers, containerOffers, placeTabOffers, placeOffers, type Offer, type Placed, type Reach, type Target } from './sources.ts';
  import { frameActionOffers, recentOffers, settingOffers } from './registry.ts';
  import { land } from './landing.ts';
  import { catalogueOffers, type Place } from '../settings/catalogue.ts';
  import { settingsCatalogue } from '../data/settingsCatalogue.svelte.ts';
  import type { AccountAction } from '../navigation.ts';
  import CreateContainerDialog from '../workspace/CreateContainerDialog.svelte';
  import { capture } from '../capture/capture.svelte.ts';
  import { mayCaptureHere } from '../capture/places.svelte.ts';
  import { recents } from '../recents.svelte.ts';
  import { GLANCE } from '../recents.ts';
  import { ariaKeysOf, drawnKeysOf } from '../shortcuts.ts';
  import { announcer } from '../announce.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { holds, holdsHere, hubCreationHere, reachesAdministrationArea, roleOrder } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { modules } from '../data/modules.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { itemPath } from '../data/item.svelte.ts';
  import { categoriesOf } from '../data/preferences.ts';
  import { people } from '../data/people.svelte.ts';
  import { privateHubs } from '../data/privatehubs.svelte.ts';
  import { search } from '../data/search.svelte.ts';
  import { searchAll } from '../data/everything.ts';
  import { content, parse, structural } from '../data/searchquery.ts';
  import { compactLine } from '../search/queryline.ts';
  import { views } from '../data/views.svelte.ts';
  import { BAR_KINDS, arrange, keyIn, kindsOn, markWords, pathOf as hitPath, personLine, requestOf, scopesOf, tileOf, type Answer, type Hit, type Scope } from '../search/everything.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { DRAWABLE } from '../entries/layouts.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    host: 'bar' | 'dialog';
    /** Where a row sends the reader. The frame owns the router; this does not. */
    onnavigate: (path: string) => void;
    /** The account's verbs, which are the frame's. */
    onaccount: (action: AccountAction) => void;
    /** Where the reader stands, as the route names it: what the frame's actions are offered for. */
    here?: { readonly hub?: string; readonly collection?: string; readonly item?: string };
  }

  const { host, onnavigate, onaccount, here = {} }: Props = $props();

  let term = $state('');
  /** The highlighted row, held by identity (`rows.ts`). */
  let activeKey = $state<string | undefined>(undefined);
  /** What `/search:all` answered for the words (§6.11): every kind, with the counts per kind. */
  let answer = $state<Answer>({ data: [], facets: [] });
  /** The scope chip chosen: every kind, one kind, or the hub one stands in. */
  let scope = $state<Scope['id']>('ALL');
  /** The phone's filters as a sheet (palette m 4), and whether it is open. */
  let isFilterOpen = $state(false);
  let searching = $state(false);
  let form = $state<HTMLFormElement | null>(null);


  /** `>` first asks for commands only, as the foot says. */
  const isCommandsOnly = $derived(term.trimStart().startsWith('>'));
  const words = $derived(term.trim().replace(/^>\s*/u, ''));
  const isOpen = $derived(palette.isOpen);

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
    modulesOn: manifest.value?.modules_on,
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

  /**
   * The hubs whose names answer the words best: the three whose views are offered as *Go to*. The
   * hub on screen offers its own through its head (`ContainerView`), and twice would be noise.
   */
  const matchedHubs = $derived(
    words === ''
      ? []
      : rank(
          placed.filter((each) => each.container.type === 'HUB' && each.container.id !== here.hub).map((each) => ({ kind: 'container' as const, id: each.container.id, label: each.container.name })),
          words,
          messages.locale,
        )
          .slice(0, 3)
          .flatMap((candidate) => placed.filter((each) => each.container.id === candidate.id)),
  );
  /** The matched collections whose views are offered: not the one on screen, whose strip draws them. */
  const matchedCollections = $derived(matched.filter((each) => each.container.id !== here.collection));
  /**
   * The modules on at each matched hub and at each matched collection's hub, as the hub answered
   * them; until then its views are not offered.
   */
  let hubModules = $state<Record<string, readonly string[]>>({});
  const askedHubs = $derived(
    isOpen
      ? [...new Set([...matchedHubs.map((each) => each.container.id), ...matchedCollections.flatMap((each) => (each.container.parent_id ? [each.container.parent_id] : []))])].join(',')
      : '',
  );
  $effect(() => {
    const ids = askedHubs === '' ? [] : askedHubs.split(',');
    return untrack(() => {
      const stops = ids.map((hubId) => modules.here(hubId, (next) => {
        if (next.status === 'ready') hubModules = { ...hubModules, [hubId]: next.data.modules_on };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });

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

  /**
   * Every catalogue setting at each level the reader reaches (configuration/ui.md A7), read only
   * while the palette is open for somebody who reaches a settings screen of the catalogue.
   */
  let catalogue = $state<readonly SettingDefinition[]>([]);
  $effect(() => {
    if (!isOpen || (!reach.isAdministrationReachable && !reach.isInstanceReachable && !shapedPlace)) return;
    return untrack(() =>
      settingsCatalogue.read((next) => {
        // An answer without the list (an older server, a proxy's page) offers no setting rather
        // than taking the palette down with it.
        if (next.status === 'ready') catalogue = Array.isArray(next.data.settings) ? next.data.settings : [];
      }),
    );
  });
  /** The hub or collection the reader is at, where their role there may shape it (A7). */
  const shapedPlace = $derived.by((): Place | undefined => {
    const at = here.collection && hereCollection ? hereCollection : here.hub && hereHub ? hereHub : undefined;
    if (!at) return undefined;
    const path = at.type === 'COLLECTION' ? pathOf(at) : { hubId: at.id };
    const role = people.roleOf(path, actor.account?.id, roleOrder()) ?? undefined;
    return holds(role, 'STRUCTURE').status === 'permitted' ? { level: at.type === 'COLLECTION' ? 'COLLECTION' : 'HUB', id: at.id } : undefined;
  });
  const catalogueRows = $derived(catalogueOffers(catalogue, reach, t, shapedPlace));

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
    ...matchedHubs.flatMap((each) => placeTabOffers(each, manifest.value?.modules_on, hubModules[each.container.id], t)),
    ...matchedCollections.flatMap((each) => placeTabOffers(each, manifest.value?.modules_on, hubModules[each.container.parent_id ?? ''], t)),
    ...placeOffers(reach, t),
    ...settings,
    ...catalogueRows,
    ...recentOffers(recents.rows.slice(0, GLANCE)),
  ]);

  const byKey = $derived(new Map(offers.map((offer) => [`${offer.kind}:${offer.id}`, offer])));

  const sourced = $derived(words === '' ? [] : palette.sourced(words));
  const sourcedByKey = $derived(new Map(sourced.map((row) => [row.key, row])));

  const ranked = $derived(rank(offers, words, messages.locale));

  // ------------------------------------------------------------------------------------------
  // The hits of every kind (§6.11): scopes with counts, best hits, the other kinds
  // ------------------------------------------------------------------------------------------

  const kinds = $derived(kindsOn(BAR_KINDS, manifest.value?.modules_on as readonly string[] | undefined));
  const scopes = $derived(scopesOf(answer.facets, kinds, hereHub !== undefined));
  const arranged = $derived(isCommandsOnly ? { best: [], more: [] } : arrange(answer.data, kinds, scope === 'ALL' || scope === 'HERE' ? undefined : scope));
  const foundByKey = $derived(new Map(answer.data.map((hit) => [`${hit.kind}:${hit.id}`, hit])));
  /** A typed key such as `SA-12` that an entry carries: Enter jumps to it. */
  const jumpHit = $derived.by(() => {
    const key = keyIn(words);
    return key ? answer.data.find((hit) => (hit.kind === 'ENTRY' || hit.kind === 'TICKET') && hit.key?.toUpperCase() === key) : undefined;
  });
  /** The account's own saved searches, offered before anything is typed. */
  const saved = $derived(
    views
      .of()
      .filter((view) => view.scope_type === 'ACCOUNT' && typeof (view.query as { q?: unknown } | undefined)?.q === 'string')
      .slice(0, 3),
  );
  const savedLine = (id: string) => String((saved.find((view) => view.id === id)?.query as { q?: string } | undefined)?.q ?? '');

  const rows = $derived.by((): readonly Row[] => {
    if (words === '' && !isCommandsOnly) {
      return [
        ...ranked.filter((each) => each.kind === 'recent').map((each) => ({ kind: each.kind, id: each.id })),
        ...saved.map((view) => ({ kind: 'saved' as const, id: view.id })),
        ...ranked.filter((each) => each.kind !== 'recent').map((each) => ({ kind: each.kind, id: each.id })),
        { kind: 'all' as const, id: 'all' },
      ];
    }
    const found = (hit: Hit) => ({ kind: 'found' as const, id: `${hit.kind}:${hit.id}` });
    return [
      ...(jumpHit ? [{ kind: 'jump' as const, id: jumpHit.id }] : []),
      ...arranged.best.map(found),
      ...arranged.more.map(found),
      ...ranked.map((each) => ({ kind: each.kind, id: each.id })),
      ...sourced.map((row) => ({ kind: 'source' as const, id: row.key })),
      ...(mayCaptureHere() && !isCommandsOnly ? [{ kind: 'make' as const, id: 'make' }] : []),
      ...(isCommandsOnly ? [] : [{ kind: 'all' as const, id: 'all' }]),
    ];
  });
  /** Typed, the first hit is marked before an arrow is pressed: Enter opens it (§6.11). */
  const active = $derived(activeKey !== undefined ? indexOfKey(rows, activeKey) : rows[0]?.kind === 'found' || rows[0]?.kind === 'jump' ? 0 : -1);

  const matchRows = $derived(rows.filter((row) => !['found', 'jump', 'saved', 'make', 'all', 'recent'].includes(row.kind)));
  const recentRows = $derived(rows.filter((row) => row.kind === 'recent'));
  const savedRows = $derived(rows.filter((row) => row.kind === 'saved'));
  const allRow = $derived(rows.find((row) => row.kind === 'all'));
  const makeRow = $derived(rows.find((row) => row.kind === 'make'));
  const jumpRow = $derived(rows.find((row) => row.kind === 'jump'));
  const total = $derived(scopes[0]?.count ?? 0);
  /** A phone draws *Filter* and *Show all* as a foot under the hits, as thumbs reach them (palette m 4). */
  const hasPhoneFoot = $derived(host === 'dialog' && viewport.isCompact && words !== '' && !isCommandsOnly && allRow !== undefined);
  const scopeName = (each: Scope) => (each.id === 'HERE' ? t('app.palette.scope_here', { name: hereHub?.name ?? '' }) : t('app.palette.scope', { kind: each.id }));

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
   * The hits of every kind, a beat after the typing stops - a read that never touches the search
   * screen's state. A generation, because the answer to "pass" can arrive after the answer to
   * "passkey", and a list that showed it would ignore what was typed last.
   */
  let generation = 0;
  $effect(() => {
    const body = isOpen && !isCommandsOnly ? requestOf(words, kinds, { size: 8, hubId: scope === 'HERE' ? hereHub?.id : undefined }) : undefined;
    if (!body) {
      answer = { data: [], facets: [] };
      searching = false;
      return;
    }
    searching = true;
    const mine = (generation += 1);
    const timer = setTimeout(() => {
      void searchAll(body).then(
        (found) => {
          if (mine !== generation) return;
          answer = { data: Array.isArray(found.data) ? found.data : [], facets: Array.isArray(found.facets) ? found.facets : [] };
          searching = false;
        },
        // A list is an offer, not an answer: a failed read shows no hits and the way on still
        // works. The screen it leads to is where a failure is reported, with its reference.
        () => {
          if (mine !== generation) return;
          answer = { data: [], facets: [] };
          searching = false;
        },
      );
    }, 200);
    return () => clearTimeout(timer);
  });

  // A new question starts over every kind.
  $effect(() => {
    if (words === '') scope = 'ALL';
  });

  // The saved searches, read while the palette is open.
  $effect(() => {
    if (!isOpen) return;
    return untrack(() => views.open());
  });

  // How many rows answer, said once the entries are in - never a region mounted with text.
  $effect(() => {
    if (!isOpen || words === '' || searching) return;
    const count = total + matchRows.length;
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

  // Summoned again, the words left from the last question are selected: typing replaces them.
  $effect(() => {
    if (palette.summons === 0 || !isOpen) return;
    void tick().then(() => form?.querySelector('input')?.select());
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
    if (row.kind === 'jump') return finish(() => onnavigate(`/items/${row.id}`));
    if (row.kind === 'make') {
      const typed = words;
      return finish(() => capture.open({ text: typed }));
    }
    if (row.kind === 'saved') {
      // Split as the search page splits it: the narrowing travels in the address, so a saved
      // search picked while standing on another replaces it; the words are handed over.
      const kept = parse(compactLine(savedLine(row.id)));
      if (content(kept) !== '') search.handOver(content(kept));
      return goSearch(structural(kept), { handOver: false });
    }
    if (row.kind === 'found') {
      const hit = foundByKey.get(row.id);
      if (!hit) return;
      if (hit.kind === 'PERSON') {
        search.handOver(personLine(hit));
        return finish(() => onnavigate('/search'));
      }
      const path = hitPath(hit);
      if (path) finish(() => onnavigate(path));
      return;
    }
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
    // ⌘↵ shows every hit on the search page, whatever is marked.
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey) && allRow) {
      event.preventDefault();
      pick(allRow);
      return;
    }
    // Tab moves through the scopes while there are hits; Shift Tab leaves as everywhere.
    if (event.key === 'Tab' && !event.shiftKey && isOpen && words !== '' && scopes.length > 1) {
      event.preventDefault();
      const at = scopes.findIndex((each) => each.id === scope);
      scope = scopes[(at + 1) % scopes.length]!.id;
      return;
    }
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
    if (row.kind === 'container') return containers.find(row.id)?.type === 'HUB' ? 'hub' : 'collection';
    if (row.kind === 'source') return 'sparkles';
    if (row.kind === 'place') return row.id.startsWith('page:') ? 'file-text' : 'arrow-right';
    if (row.kind === 'setting') return 'settings';
    if (row.kind === 'recent') return 'clock';
    return 'zap';
  }

  function kindOf(row: Row): string {
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

  function initialsOf(name: string): string {
    return name.split(/\s+/u).filter(Boolean).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }

  /** What stands beside a hit: its place, its key's collection, its state - never colour alone. */
  function contextOf(hit: Hit): string {
    const place = hit.collection_id ? containers.find(hit.collection_id)?.name : hit.hub_id ? containers.find(hit.hub_id)?.name : undefined;
    const state = hit.state ? t('app.search.state', { state: hit.state }) : undefined;
    return [hit.kind === 'FILE' || hit.kind === 'COMMENT' ? t('app.search.kind_one', { kind: hit.kind }) : undefined, place, state].filter(Boolean).join(' · ');
  }
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

{#snippet found(row: Row)}
  {@const hit = row.kind === 'jump' ? answer.data.find((each) => each.id === row.id) : foundByKey.get(row.id)}
  {#if hit}
    {@const tile = tileOf(hit)}
    {@const key = keyOf(row)}
    <button
      type="button"
      role="option"
      id={rowId(row)}
      aria-selected={rows[active] !== undefined && keyOf(rows[active]!) === key}
      class="row hit"
      data-kind={hit.kind}
      data-active={rows[active] !== undefined && keyOf(rows[active]!) === key ? '' : undefined}
      onmouseenter={() => (activeKey = key)}
      onclick={() => pick(row)}
    >
      {#if hit.kind === 'PERSON'}
        <span class="initials" aria-hidden="true">{initialsOf(hit.title)}</span>
      {:else}
        <ProductTile icon={tile.icon as IconName} product={tile.product} color={tile.color} size="sm" />
      {/if}
      {#if row.kind === 'jump'}<span class="title">{t('app.palette.jump', { key: hit.key ?? '' })}</span>{/if}
      {#if hit.key}<span class="hitkey">{hit.key}</span>{/if}
      <span class="title">
        {#each markWords(hit.title, words) as piece, index (index)}{#if piece.isFound}<mark>{piece.text}</mark>{:else}{piece.text}{/if}{/each}
      </span>
      <span class="context">{contextOf(hit)}</span>
      <VisuallyHidden>{t('app.search.kind_one', { kind: hit.kind })}</VisuallyHidden>
      {#if rows[active] !== undefined && keyOf(rows[active]!) === key}<span class="key" aria-hidden="true"><Kbd size="sm">↵</Kbd></span>{/if}
    </button>
  {/if}
{/snippet}

{#snippet list()}
  <div class="menu" data-host={host} role="listbox" aria-label={t('app.searchmenu.label')} id="search-menu">
    {#if words !== '' && !isCommandsOnly && scopes.length > 1}
      <!-- The scopes with their counts (§6.11): Tab moves through them from the field. -->
      <div class="scopes" role="group" aria-label={t('app.palette.scopes')}>
        {#each scopes as each (each.id)}
          <button
            type="button"
            class="scope"
            tabindex="-1"
            aria-pressed={scope === each.id}
            onmousedown={(event) => event.preventDefault()}
            onclick={() => (scope = each.id)}
          >
            {scopeName(each)}
            {#if each.count !== undefined}<span class="count">{each.count}{each.more ? '+' : ''}</span>{/if}
          </button>
        {/each}
      </div>
    {/if}

    {#if jumpRow}
      <div role="group" aria-label={t('app.palette.jump_group')}>{@render found(jumpRow)}</div>
    {/if}
    {#if arranged.best.length > 0}
      <div role="group" aria-labelledby={`${host}-palette-best`}>
        <p class="head" id={`${host}-palette-best`}>{t('app.palette.best')}</p>
        {#each arranged.best as hit (hit.kind + hit.id)}{@render found({ kind: 'found', id: `${hit.kind}:${hit.id}` })}{/each}
      </div>
    {/if}
    {#if arranged.more.length > 0}
      <div role="group" aria-labelledby={`${host}-palette-more`}>
        <p class="head" id={`${host}-palette-more`}>{t('app.palette.more_kinds')}</p>
        {#each arranged.more as hit (hit.kind + hit.id)}{@render found({ kind: 'found', id: `${hit.kind}:${hit.id}` })}{/each}
      </div>
    {/if}

    {#if recentRows.length > 0}
      <div role="group" aria-labelledby={`${host}-palette-recent`}>
        <p class="head" id={`${host}-palette-recent`}>{t('app.palette.group', { group: 'recent' })}</p>
        {#each recentRows as row (keyOf(row))}{@render option(row)}{/each}
      </div>
    {/if}
    {#if savedRows.length > 0}
      <div role="group" aria-labelledby={`${host}-palette-saved`}>
        <p class="head" id={`${host}-palette-saved`}>{t('app.palette.saved')}</p>
        {#each savedRows as row (keyOf(row))}
          <button type="button" role="option" id={rowId(row)} aria-selected={keyOf(row) === activeKey} class="row" data-active={keyOf(row) === activeKey ? '' : undefined} onmouseenter={() => (activeKey = keyOf(row))} onclick={() => pick(row)}>
            <Icon name="star" />
            <span class="title">{saved.find((view) => view.id === row.id)?.name}</span>
            <code class="context">{savedLine(row.id)}</code>
          </button>
        {/each}
      </div>
    {/if}

    {#each groups as group (group.id)}
      <div role="group" aria-labelledby={`${host}-palette-${group.id}`}>
        <p class="head" id={`${host}-palette-${group.id}`}>{t(words === '' ? 'app.palette.quick_head' : 'app.palette.group', { group: group.id })}</p>
        {#each group.rows as row (keyOf(row))}
          {@render option(row)}
        {/each}
      </div>
    {/each}
    {#if words !== '' && searching && answer.data.length === 0}
      <p class="head"><Spinner size="sm" /> {t('app.searchmenu.searching')}</p>
    {:else if words !== '' && !searching && answer.data.length === 0 && matchRows.length === 0}
      <p class="head">{t('app.searchmenu.none')}</p>
    {/if}

    <div class="feet" role="group" aria-labelledby={`${host}-palette-on`}>
      <VisuallyHidden><span id={`${host}-palette-on`}>{t('app.palette.ways_on')}</span></VisuallyHidden>
      {#if makeRow}
        <button type="button" role="option" id={rowId(makeRow)} aria-selected={keyOf(makeRow) === activeKey} class="row foot" data-active={keyOf(makeRow) === activeKey ? '' : undefined} onmouseenter={() => (activeKey = keyOf(makeRow))} onclick={() => pick(makeRow)}>
          <Icon name="plus" />
          <span class="title">{t('app.palette.make_entry', { words: words.length > 24 ? `${words.slice(0, 24)} …` : `${words} …` })}</span>
        </button>
      {/if}
      {#if allRow && !hasPhoneFoot}
        <button
          type="button"
          role="option"
          id={rowId(allRow)}
          aria-selected={keyOf(allRow) === activeKey}
          aria-describedby={enterHintId}
          aria-keyshortcuts="Meta+Enter Control+Enter"
          class="row foot"
          data-active={keyOf(allRow) === activeKey ? '' : undefined}
          onmouseenter={() => (activeKey = keyOf(allRow))}
          onclick={() => pick(allRow)}
        >
          <Icon name="search" />
          <span class="title">{words === '' ? t('app.searchmenu.open') : t('app.palette.all_hits', { count: String(total) })}</span>
          <span class="key" aria-hidden="true"><Kbd size="sm">{words === '' ? '↵' : '⌘↵'}</Kbd></span>
          <VisuallyHidden><span id={enterHintId}>{t('app.palette.enter_hint')}</span></VisuallyHidden>
        </button>
      {/if}
    </div>
    {#if host === 'bar'}
      <!-- The keys of the list, learnt where they are used. -->
      <p class="hints" aria-hidden="true">
        <span><Kbd size="sm">↑↓</Kbd> {t('app.palette.hint_move')}</span>
        <span><Kbd size="sm">↵</Kbd> {t('app.palette.hint_open')}</span>
        <span><Kbd size="sm">Tab</Kbd> {t('app.palette.hint_scope')}</span>
        <span><Kbd size="sm">&gt;</Kbd> {t('app.palette.hint_commands')}</span>
        <span class="tip">{t('app.palette.hint_type', { key: 'type:' })}</span>
      </p>
    {/if}
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
    size="m"
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
      {#if hasPhoneFoot && allRow}
        <div class="phone-foot">
          {#if scopes.length > 1}<Button tone="secondary" icon="funnel" onclick={() => (isFilterOpen = true)}>{t('app.palette.filter')}</Button>{/if}
          <Button tone="primary" onclick={() => pick(allRow)}>{t('app.palette.all_hits', { count: String(total) })}</Button>
        </div>
      {/if}
    </form>
    <Sheet
      title={t('app.palette.filter')}
      closeLabel={t('app.palette.close')}
      bind:isOpen={isFilterOpen}
      items={scopes.map((each) => ({ id: each.id, label: each.count !== undefined ? t('app.palette.scope_counted', { scope: scopeName(each), count: `${each.count}${each.more ? '+' : ''}` }) : scopeName(each), icon: scope === each.id ? 'check' : undefined }))}
      onselect={(id) => (scope = id as Scope['id'])}
    />
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
    oncreated={(id, landing) => onnavigate(creating?.type === 'HUB' ? `/hubs/${id}` : (landing ?? `/collections/${id}`))}
  />
{/if}

<style>
  /* The field takes the slot whole; the slot is what is bounded, in `AppBar`. The list is
     positioned against this, which is why it is the containing block. */
  .entry { position: relative; display: block; min-inline-size: 0; block-size: var(--density-control-sm-min); }

  /* Always drawn over the slot, centred on it, so using it changes one thing - its width - and that
     change is a movement rather than a jump: the field grows evenly to both sides, never down or up.
     The slot is the window's middle from `expanded` (AppFrame), so the open field is too. */
  .float {
    position: absolute;
    inset-block-start: 50%;
    inset-inline-start: 50%;
    inline-size: 100%;
    /* Centred on both axes: the field is a hair taller than the slot's control height, and pinned
       to the slot's top it stood lower than the bar's middle. */
    translate: -50% -50%;
    transition: inline-size var(--dur-base) var(--ease-standard);
  }

  /* In use: the open width, never into the logo's or the account's side of the bar. */
  .entry:focus-within > .float,
  .entry[data-open] > .float {
    z-index: var(--z-popover);
    inline-size: min(var(--layout-barsearch-open), 100vw - 2 * var(--layout-barsearch-side));
    min-inline-size: 100%;
  }

  :global([dir='rtl']) .float { translate: 50% -50%; }

  @media (prefers-reduced-motion: reduce) {
    .float { transition: none; }
  }

  .keys { display: inline-flex; position: absolute; inset-block-start: 50%; inset-inline-end: var(--sp-100); transform: translateY(-50%); pointer-events: none; }
  /* The words make room for the key rather than run under it - a German placeholder is long. */
  .float:has(> .keys) :global(input) { padding-inline-end: calc(var(--sp-100) + var(--sp-500)); text-overflow: ellipsis; }
  .sheet { display: flex; flex-direction: column; gap: var(--sp-100); }
  /* Under the hits, where a thumb is: *Filter* beside the one primary act. */
  .phone-foot {
    position: sticky;
    inset-block-end: 0;
    display: flex;
    gap: var(--sp-100);
    padding-block: var(--sp-100);
    background: var(--bg-raised);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

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


  .scopes { display: flex; flex-wrap: wrap; gap: var(--sp-050); padding: var(--sp-050) var(--sp-050) var(--sp-100); }

  .scope {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-full);
    background: var(--bg-raised);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .scope:hover { background: var(--bg-surface-hover); }
  .scope[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary); color: var(--text-inverse); }
  .scope .count { font-variant-numeric: tabular-nums; }

  .head { text-transform: uppercase; }

  .hit .title { flex: 0 1 auto; }
  .hit .context { flex: 1 1 auto; overflow: hidden; text-overflow: ellipsis; }
  .hitkey { flex: none; color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--fs-075); }
  mark { border-radius: var(--r-sm); background: var(--status-warning-surface); color: inherit; }

  .initials {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-300);
    block-size: var(--sp-300);
    border-radius: var(--r-full);
    background: var(--accent-signature-subtle);
    color: var(--text-primary);
    font-size: var(--fs-050);
    font-weight: var(--fw-medium);
  }

  .hints {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-150);
    margin: var(--sp-050) 0 0;
    padding: var(--sp-100) var(--sp-100) 0;
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    inline-size: 100%;
    box-sizing: border-box;
  }

  .hints .tip { margin-inline-start: auto; }

  .feet {
    margin-block-start: var(--sp-050);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    padding-block-start: var(--sp-050);
  }

  .foot { color: var(--text-secondary); font-size: var(--fs-075); }

  .key { display: inline-flex; flex: none; }
</style>
