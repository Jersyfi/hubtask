<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { platform } from '../lib/platform/index.ts';
  // Search, at its own address — and the term is never in it.
  //
  // `/search` is a `POST` with no `GET` because what somebody is looking for is their content, and
  // a query string travels through access logs, proxies and browser history. So the address
  // carries the **narrowing** — `?f=is:open who:me`, which is structural and says nothing about
  // anybody — and the words live under a minted handle in `sessionStorage` (`searchhandle.ts`).
  //
  // **One question, two surfaces, one state.** The line in `data/searchquery.ts` is the state: a
  // string holding the words and the narrowing together. The chips read it and write it; the
  // field for the words reads it and writes it; and the text editor *is* it. Nothing here holds a
  // second copy, which is why the two surfaces cannot drift apart — the failure Jira's basic mode
  // announces with a warning whenever a JQL query is one it could not have produced.
  //
  // **No language is chosen.** The picker, the widening and the "found under" badge were all one
  // workaround for a document indexed under one configuration and a query read under another;
  // the proposal fixes that where it is (`concept/search/proof/language.sql`), and a full-text
  // search is then what it says on the label.

  import { untrack } from 'svelte';

  import {
    Badge,
    Button,
    Checkbox,
    EmptyState,
    Icon,
    Kbd,
    PageHeader,
    ProductTile,
    QueryLine,
    Skeleton,
    Stack,
    TaskRow,
    WorkPage,
    canCopy,
    lineText,
    type IconName,
    type QueryToken,
  } from '@hubtask/design-system/components';
  import FailureState from '../lib/frame/FailureState.svelte';

  import BulkBar from '../lib/entries/BulkBar.svelte';
  import EntryActions from '../lib/entries/EntryActions.svelte';
  import EntryMenu from '../lib/entries/EntryMenu.svelte';
  import { attachEntryGestures } from '../lib/entries/entryGestures.ts';
  import { entryMenu } from '../lib/entries/entryMenu.ts';
  import HitRow from '../lib/search/HitRow.svelte';
  import {
    PAGE_KINDS,
    countBy,
    kindsOn,
    ordered,
    personLine,
    requestOf,
    scopesOf,
    sinceOf,
    tileOf,
    wordsForUnknownKeys,
    type Answer,
    type Hit,
    type Kind,
    type Order,
    type Since,
  } from '../lib/search/everything.ts';
  import { compactLine, lineOf } from '../lib/search/queryline.ts';
  import { askQuery, searchAll } from '../lib/data/everything.ts';
  import { views } from '../lib/data/views.svelte.ts';

  import type { BulkOperation, BulkResult, WorkItem } from '@hubtask/sync-engine';

  import { byItem, outcomeOf } from '../lib/data/bulk.ts';
  import { selection } from '../lib/data/selection.svelte.ts';

  import { announcer } from '../lib/announce.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { changeRefusalCode, holds, mayDo, roleOrder, supports } from '../lib/data/capability.svelte.ts';
  import { archivalOfItem } from '../lib/data/lifecycle.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import EntryPlace from '../lib/workspace/EntryPlace.svelte';
  import { labels } from '../lib/data/labels.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { queryFields } from '../lib/data/query.ts';
  import { health } from '../lib/data/health.svelte.ts';
  import { search, type SearchMode } from '../lib/data/search.svelte.ts';
  import {
    asksSomething,
    compile,
    content,
    joinLine,
    narrowingOf,
    parse,
    structural,
    wordsOf,
    type Compiled,
    type Context,
  } from '../lib/data/searchquery.ts';
  import { HANDLE, isHandle, mint } from '../lib/data/searchhandle.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    /** What the address carries: the narrowing, never the words (see the note at the top). */
    query?: Readonly<Record<string, string>>;
    onnavigate?: (path: string) => void;
  }

  const { query = {}, onnavigate }: Props = $props();

  /**
   * The handle this search is known by in the address, and the words kept under it.
   *
   * Three arrivals, one rule. A fresh visit names no handle, so one is minted. A reload or the
   * back button names one this tab knows, so it is adopted and its words come back. A link
   * somebody was *sent* names one this tab has never written: the narrowing is restored, the
   * words are not — that is the promise — and a handle of this tab's own is minted, so that what
   * this reader types is not written under a name a stranger's link chose.
   */
  let handle = $state('');

  /** The whole question, in one string: words and narrowing together. The only state there is. */
  let line = $state('');

  $effect(() => {
    const named = query[HANDLE];
    const carried = query.f ?? '';
    untrack(() => {
      // Arriving: a reload or the back button naming a handle this tab wrote, so its words come
      // back and join the narrowing the address carried.
      const known = isHandle(named) ? search.recall(named) : undefined;
      if (isHandle(named) && named !== handle && known !== undefined) {
        handle = named;
        line = joinLine(carried, known);
        return;
      }

      // Arriving for the first time: a fresh visit, or a link somebody was sent. The narrowing is
      // restored, the words are not - a handle this tab has never written names a drawer in
      // somebody else's tab - and a handle of this tab's own is minted.
      if (handle === '') {
        if (carried !== '') line = carried;
        handle = mint((bytes) => crypto.getRandomValues(bytes));
        return;
      }

      // Already standing here, and the address changed underneath: the bar navigated into this
      // screen with a narrowing of its own. It has to be adopted, and this is the half that was
      // missing - the address moved, the screen did not, and pressing "Mine, open" from the bar
      // while already on a narrowed search changed the URL and nothing else.
      //
      // The narrowing is replaced and the *words are kept*, because the words were never in the
      // address to be replaced by it: `secret` is exactly the complement of `shareable`, so this
      // swaps one half of the line and leaves the other alone. A press of this screen's own is
      // not this branch - by the time the address carries what it wrote, `carried` already equals
      // `shareable` and there is nothing to adopt.
      if (carried !== shareable) line = joinLine(carried, secret);
      if (isHandle(named) && named !== handle) handle = mint((bytes) => crypto.getRandomValues(bytes));
    });
  });

  /**
   * The words the app bar handed over, taken as they arrive.
   *
   * An effect rather than an initial value, because this screen is not remounted when somebody
   * searches again from the bar while already on it — and that press has to do something. Taken
   * rather than read: the store empties on the way out, so the same words handed over twice are
   * two searches.
   */
  $effect(() => {
    if (search.handedOver === undefined) return;
    const handed = search.takeHandover();
    // Joined to the narrowing rather than replacing it: somebody who searches again from the bar
    // while standing on a narrowed search meant those words *here*.
    if (handed !== undefined) line = joinLine(untrack(() => narrowingOf(compact)), handed);
  });

  /**
   * The one line (Q-1, Q-2): QueryLine's chips and the text not yet a chip, fed by @hubtask/query.
   * `line` is the query's own spelling; the entry search reads it written close (`compact`), and a
   * `word:` that names no key is searched as words, said at the line.
   */
  const queryLine = $derived(lineOf(manifest.value?.query_keys as never, t, (code) => messages.has(code)));
  let chips = $state<readonly QueryToken[]>([]);
  let rest = $state('');
  const unknownWords = $derived(wordsForUnknownKeys(line, queryLine.known));
  const compact = $derived(compactLine(unknownWords.line));

  // The line written in from elsewhere (the address, the bar, a question taken over) becomes chips.
  $effect(() => {
    const next = line;
    untrack(() => {
      if (lineText(queryLine.language, chips, rest) === next.trim()) return;
      const cut = queryLine.language.parse(next.trim() === '' ? '' : `${next.trim()} `);
      chips = cut.tokens;
      rest = cut.rest;
    });
  });

  // What is typed in the line is the line.
  $effect(() => {
    const written = lineText(queryLine.language, chips, rest);
    untrack(() => {
      if (written !== line.trim()) line = written;
    });
  });

  const parsed = $derived(parse(compact));

  /** The plain words of the line, which the hits mark. */
  const typed = $derived(wordsOf(compact));
  const hasWords = $derived(typed.trim() !== '');

  /**
   * The privacy split, which is a different one: what may travel in a link, and what may not.
   *
   * A collection, a label, a kind, a state, a date, `me` — none is anybody's text. `title:` and
   * `note:` are, and they stay under the handle with the words.
   */
  const shareable = $derived(structural(parsed));
  const secret = $derived(content(parsed));

  // What was typed, kept under the handle the address carries. Written as it changes rather than
  // when the search runs: somebody who reloads mid-sentence meant that sentence.
  $effect(() => {
    if (handle !== '') search.remember(handle, secret);
  });

  /** The fields this installation lets anything be filtered by. */
  const reported = $derived(new Set(queryFields(manifest.value).map((field) => field.field)));

  /**
   * Every collection the reader may see, so that `in:` can be answered by name.
   *
   * The levels are opened here rather than assumed: a hub's collections are read when the hub is
   * opened in the tree, so a reader who has never expanded one has no names for it.
   */
  const hubIds = $derived(containers.hubs.map((hub) => hub.id).join(','));

  $effect(() => {
    const ids = hubIds === '' ? [] : hubIds.split(',');
    const opened = untrack(() => ids.map((id) => containers.openLevel(id)));
    return () => opened.forEach((close) => close());
  });

  const everyCollection = $derived(containers.hubs.flatMap((hub) => containers.collectionsOf(hub.id)));

  function collectionNamed(name: string): string | undefined {
    const wanted = name.trim().toLowerCase();
    return everyCollection.find(
      (collection) => collection.name.toLowerCase() === wanted || collection.id === name.trim(),
    )?.id;
  }

  /** The collections the line named, which is where a label and a person may be looked up. */
  const named = $derived(
    parsed.tokens
      .filter((token) => token.key === 'in')
      .flatMap((token) => token.value.split(','))
      .map((name) => collectionNamed(name))
      .filter((id): id is string => id !== undefined),
  );

  const namedKey = $derived(named.join(','));
  /** The one collection named, where exactly one is: what a label chip can be built from. */
  const oneCollection = $derived(named.length === 1 ? named[0] : undefined);

  $effect(() => {
    const ids = namedKey === '' ? [] : namedKey.split(',');
    const opened = untrack(() => ids.map((id) => labels.open(id)));
    return () => opened.forEach((close) => close());
  });

  $effect(() => {
    if (!oneCollection) return;
    return people.open({ collectionId: oneCollection });
  });

  const context = $derived<Context>({
    fields: reported,
    collection: collectionNamed,
    label: (name) => {
      const wanted = name.trim().toLowerCase();
      for (const id of named) {
        const found = labels.of(id).find((label) => label.name.toLowerCase() === wanted);
        if (found) return found.id;
      }
      return undefined;
    },
    hasCollection: named.length > 0,
    hub: (name) => {
      const wanted = name.trim().toLowerCase();
      const hub = containers.hubs.find((each) => each.name.toLowerCase() === wanted || each.id === name.trim());
      return hub ? containers.collectionsOf(hub.id).map((collection) => collection.id) : undefined;
    },
    person: (name) => {
      const wanted = name.trim().toLowerCase();
      if (!oneCollection) return undefined;
      return people
        .candidates({ collectionId: oneCollection })
        .find((id) => (accounts.nameOf(id) ?? '').toLowerCase() === wanted || id === name.trim());
    },
  });

  /** The question the line asks, in the shape `POST /search` takes it. */
  const compiled = $derived<Compiled>(compile(parsed, context));

  /** The address this search has: the narrowing, and the handle its words are kept under. */
  const address = $derived.by(() => {
    const carried = new URLSearchParams({
      ...(shareable === '' ? {} : { f: shareable }),
      ...(secret === '' ? {} : { [HANDLE]: handle }),
    });
    const written = carried.toString();
    return written === '' ? '/search' : `/search?${written}`;
  });

  // The handle joins the address as soon as there are words to keep, so a reload finds them.
  // Replaced rather than pushed: a search is one place, not a history entry per keystroke.
  $effect(() => {
    const wanted = address;
    if (wanted !== `${location.pathname}${location.search}`) untrack(() => onnavigate?.(wanted));
  });

  // Words and meaning where the manifest offers meaning, words only otherwise - no switch (§6.11);
  // a degraded meaning search falls back to words and says so at the results (ADR-0063 decision 4).
  const hasMeaning = $derived(manifest.value?.features?.semantic_search === true);
  const meaningDegraded = $derived(health.degradations.find((d) => d.feature === 'semantic_search'));
  const mode = $derived<SearchMode | undefined>(hasMeaning ? (meaningDegraded ? 'LEXICAL' : 'AUTO') : undefined);

  const asked = $derived({
    q: compiled.q ?? '',
    filter: compiled.filter,
    sort: compiled.sort,
    includeArchived: compiled.includeArchived,
    includeTrashed: compiled.includeTrashed,
    mode,
  });

  /**
   * The search runs a beat after the typing stops, not on every keystroke.
   *
   * A request per character is a request the reader has already replaced by the time it lands, and
   * the store drops those answers anyway — the wait is what keeps them from being made at all.
   */
  $effect(() => {
    const question = asked;
    if (shownKind !== 'ENTRY' || !asksSomething(compiled)) {
      search.reset();
      return;
    }
    const timer = setTimeout(() => void search.run(question), 250);
    return () => clearTimeout(timer);
  });

  // What arrived, said out loud. A list that changes under a reader still in the field is a change
  // a screen reader is told nothing about.
  // The other kinds count too: "nothing matches" while pages match would be a wrong answer.
  $effect(() => {
    if (shownKind === 'ENTRY' ? search.status !== 'done' : allStatus !== 'done') return;
    announcer.say(t('app.search.count', { count: shownCount }));
  });

  const failure = $derived(search.error ? renderProblem(search.error, messages) : undefined);

  /**
   * The link to this search: its narrowing, and nothing of what was typed.
   *
   * Built from the tokens rather than from the address, which is the same thing said twice on
   * purpose — the address may carry the handle, and a handle is this tab's, not a link's.
   */
  let copied = $state(false);

  async function copyLink() {
    const carried = new URLSearchParams(shareable === '' ? {} : { f: shareable }).toString();
    const link = `${platform.webOrigin}/search${carried === '' ? '' : `?${carried}`}`;
    try {
      await navigator.clipboard.writeText(link);
      copied = true;
      announcer.say(t('app.search.link_copied'));
      setTimeout(() => (copied = false), 4_000);
    } catch {
      // The clipboard refused. The address is on screen and can be copied by hand.
    }
  }

  /**
   * A hit is a row like any other, so it can be ticked off where it is found.
   *
   * The hit is not re-read here: a search is a walk that was already made, and re-running it
   * because one entry changed would move the results under the reader's hand.
   */
  let writeFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  /**
   * Where a hit sits, for the reader's role there: its collection and that collection's hub. The
   * role is read along the collection as the collection's own list reads it, so a hit and its row
   * answer the same question the same way - and one read per collection, not per hit.
   */
  const hitPath = (hit: WorkItem) => ({
    hubId: containers.find(hit.collection_id)?.parent_id ?? undefined,
    collectionId: hit.collection_id,
  });

  // The roles along every collection the results come from, read with the results.
  $effect(() => {
    const paths = search.hits.map(hitPath);
    untrack(() => {
      for (const path of paths) people.open(path);
    });
  });

  /**
   * Why a hit cannot be ticked off from here, or nothing: what stops its row in the list stops it
   * here too - the entry archived, its collection archived, or a role that may not change it.
   */
  function completeReason(hit: WorkItem): string | undefined {
    if (hit.archived_at) return t('app.entries.archived');
    if (containers.find(hit.collection_id)?.effective_archived) return t('app.entries.read_only');
    const role = people.roleOf(hitPath(hit), actor.account?.id, roleOrder());
    const code = changeRefusalCode(role, hit.assignee_id != null && hit.assignee_id === actor.account?.id);
    return code ? t(code) : undefined;
  }

  /** The reader's role where a hit sits, once the memberships there have been read. */
  const roleAt = (hit: WorkItem) => people.roleOf(hitPath(hit), actor.account?.id, roleOrder());

  /**
   * A hit's menu: the one every surface draws (UC-WRK-28 check 1). Search holds no level, so the
   * moves wait for the one read when the menu opens (D6); the role is read with the hits.
   */
  function menuOf(hit: WorkItem) {
    const role = roleAt(hit);
    const isMine = hit.assignee_id != null && hit.assignee_id === actor.account?.id;
    const change = mayDo(role ?? undefined, 'change', isMine);
    return entryMenu(
      {
        sendsLinks: platform.sendLink !== undefined,
        surface: 'search',
        item: { title: hit.title, isCompleted: hit.completion?.is_completed ?? false, hasDue: Boolean(hit.due_at) },
        archival: archivalOfItem(hit, false),
        isCollectionReadOnly: containers.find(hit.collection_id)?.effective_archived ?? false,
        mayChange: change,
        mayCreate: mayDo(role ?? undefined, 'create', isMine) === true,
        mayShare: holds(role ?? undefined, 'MANAGE_MEMBERS').status === 'permitted',
        carries: (capability) => supports(hit.type, capability).status === 'permitted',
        level: actions?.levelOf(hit),
      },
      t,
    );
  }

  const menus = $state<Record<string, EntryMenu | undefined>>({});
  let actions = $state<EntryActions | undefined>(undefined);
  let results = $state<HTMLElement | null>(null);

  function openMenu(id: string) {
    const hit = search.hits.find((each) => each.id === id);
    if (hit) actions?.readLevel(hit);
    menus[id]?.open(0);
  }

  $effect(() => {
    const node = results;
    if (!node) return;
    return attachEntryGestures(node, {
      idAt: (target) => (target as Element | null)?.closest?.('[data-hit]')?.getAttribute('data-hit') ?? undefined,
      open: openMenu,
    });
  });

  async function toggleComplete(id: string, title: string, isCompleted: boolean) {
    writeFailure = undefined;
    try {
      await items.setCompleted(id, !isCompleted, crypto.randomUUID());
      announcer.say(
        t(isCompleted ? 'app.entries.reopened_announced' : 'app.entries.completed_announced', { title }),
      );
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /**
   * The hits can be picked and acted on together, as a list's rows can (ADR-0063 decision 8): the
   * same mode, the same bar. A search spans collections, so the bar offers what needs no list of
   * one collection; and after it applies, the search is asked again, since a hit that was trashed
   * or completed is no longer what the reader is looking at.
   */
  const hitIds = $derived(search.hits.map((hit) => hit.id));
  $effect(() => {
    const visible = hitIds;
    untrack(() => selection.keepVisible(visible));
  });

  /** What the last bulk did, per hit, until the selection changes; shown at the row it concerns. */
  let lastResults = $state<ReadonlyMap<string, BulkResult>>(new Map());

  function bulkNote(id: string): string | undefined {
    const result = lastResults.get(id);
    if (!result || outcomeOf(result) === 'applied') return undefined;
    return result.problem ? renderProblem(result.problem as never, messages).message : t('app.bulk.rolled_back');
  }

  function bulkDone(operations: readonly BulkOperation[], results: readonly BulkResult[]) {
    lastResults = byItem(operations, results);
    void search.run(asked);
  }

  // `Escape` leaves the mode from anywhere on the screen, unless an overlay takes it first.
  $effect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || !selection.isOn) return;
      if (document.querySelector('dialog[open], [role="dialog"], [role="menu"]')) return;
      selection.stop();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  // A selection is about what is in front of somebody, so it does not survive the screen.
  $effect(() => () => {
    selection.clear();
    lastResults = new Map();
  });

  // A modifier press on a hit picks it, as on a list's row - in the capture phase, because the row
  // is a link and the router listens for the click after it.
  $effect(() => {
    const node = results;
    if (!node) return;
    const onClick = (event: MouseEvent) => {
      if (!(event.metaKey || event.ctrlKey)) return;
      const id = (event.target as Element | null)?.closest?.('[data-hit]')?.getAttribute('data-hit');
      if (!id) return;
      event.preventDefault();
      event.stopPropagation();
      selection.pick(hitIds, id);
    };
    node.addEventListener('click', onClick, true);
    return () => node.removeEventListener('click', onClick, true);
  });

  // ---- One search over every kind (§6.11, UC-WRK-18 check 9): the facets, the hits, the order.

  /** The kinds this installation answers, by its modules (P-17). */
  const kinds = $derived(kindsOn(PAGE_KINDS, manifest.value?.modules_on as readonly string[] | undefined));
  /** The facet chosen; unchosen, a line of words shows every kind and a narrowing alone entries. */
  let chosenKind = $state<'ALL' | Kind | undefined>(undefined);
  const shownKind = $derived<'ALL' | Kind>(chosenKind ?? (hasWords || line.trim() === '' ? 'ALL' : 'ENTRY'));
  let chosenHub = $state<string | undefined>(undefined);
  let chosenSince = $state<Since | undefined>(undefined);
  let chosenAuthor = $state<string | undefined>(undefined);
  let order = $state<Order>('RELEVANCE');

  let answer = $state<Answer>({ data: [], facets: [] });
  let allStatus = $state<'idle' | 'searching' | 'done' | 'failed'>('idle');
  let allFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  const allBody = $derived(requestOf(unknownWords.line, kinds, { size: 25 }));

  // The line travels whole in the body, never in the address (Q-5); a beat after the typing stops.
  $effect(() => {
    const body = allBody;
    if (!body) {
      answer = { data: [], facets: [] };
      allStatus = 'idle';
      return;
    }
    let isCurrent = true;
    const timer = setTimeout(() => {
      allStatus = 'searching';
      searchAll(body).then(
        (found) => {
          if (!isCurrent) return;
          answer = found;
          allStatus = 'done';
          allFailure = undefined;
        },
        (error) => {
          if (!isCurrent) return;
          allFailure = renderProblem(error as never, messages);
          allStatus = 'failed';
        },
      );
    }, 250);
    return () => {
      isCurrent = false;
      clearTimeout(timer);
    };
  });

  const scopes = $derived(scopesOf(answer.facets, kinds, false));
  const hubOfEntry = (hit: WorkItem) => containers.find(hit.collection_id)?.parent_id ?? null;
  const hubOfHit = (hit: Hit) => hit.hub_id ?? (hit.collection_id ? (containers.find(hit.collection_id)?.parent_id ?? null) : null);
  const now = new Date();

  /** The hits of the kind chosen, before the hub and the change narrow them. */
  const kindHits = $derived(shownKind === 'ALL' ? answer.data : answer.data.filter((hit) => hit.kind === shownKind));
  const hubCounts = $derived(
    shownKind === 'ENTRY' ? countBy(search.hits.map((hit) => ({ hub_id: hubOfEntry(hit) }) as Hit), (hit) => hit.hub_id) : countBy(kindHits, hubOfHit),
  );
  const sinceCounts = $derived(
    shownKind === 'ENTRY'
      ? countBy(search.hits.map((hit) => ({ updated_at: hit.updated_at ?? null }) as Hit), (hit) => sinceOf(hit.updated_at, now))
      : countBy(kindHits, (hit) => sinceOf(hit.updated_at, now)),
  );
  /** Who made the hits read (§6.11, *Von*): entries, tickets and pages name their author. */
  const authorCounts = $derived(
    shownKind === 'ENTRY' ? countBy(search.hits.map((hit) => ({ author_id: hit.created_by ?? null }) as Hit), (hit) => hit.author_id) : countBy(kindHits, (hit) => hit.author_id),
  );
  $effect(() => {
    const ids = [...authorCounts.keys()];
    untrack(() => accounts.resolve(ids));
  });
  const shownHits = $derived(
    ordered(
      kindHits.filter(
        (hit) =>
          (chosenHub === undefined || hubOfHit(hit) === chosenHub) &&
          (chosenSince === undefined || sinceOf(hit.updated_at, now) === chosenSince) &&
          (chosenAuthor === undefined || hit.author_id === chosenAuthor),
      ),
      order,
      messages.locale,
    ),
  );
  /** The entries, narrowed by the facets and in the order chosen; relevance is the server's. */
  const shownEntries = $derived.by(() => {
    const narrowed = search.hits.filter(
      (hit) =>
        (chosenHub === undefined || hubOfEntry(hit) === chosenHub) &&
        (chosenSince === undefined || sinceOf(hit.updated_at ?? null, now) === chosenSince) &&
        (chosenAuthor === undefined || hit.created_by === chosenAuthor),
    );
    if (order === 'UPDATED') return [...narrowed].sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? ''));
    if (order === 'CREATED') return [...narrowed].sort((a, b) => (b.created_at ?? '').localeCompare(a.created_at ?? ''));
    if (order === 'TITLE') return [...narrowed].sort((a, b) => new Intl.Collator(messages.locale).compare(a.title, b.title));
    return narrowed;
  });
  const shownCount = $derived(
    shownKind === 'ENTRY'
      ? shownEntries.length
      : chosenHub !== undefined || chosenSince !== undefined || chosenAuthor !== undefined
        ? shownHits.length
        : (scopes.find((scope) => scope.id === shownKind)?.count ?? 0),
  );

  /** A person's hit searches their entries: their identifier, never their name, in the line. */
  function searchPerson(hit: Hit) {
    line = personLine(hit);
    chosenKind = 'ENTRY';
  }

  // ---- Save as a view (UC-WRK-18 check 12): the account's own view holding the line as `q`.
  let saving = $state(false);
  let savedName = $state<string | undefined>(undefined);
  async function saveAsView() {
    if (line.trim() === '' || saving) return;
    saving = true;
    try {
      const name = t('app.search.save_name', { line: line.trim().slice(0, 120) });
      await views.create({ scope_type: 'ACCOUNT', name, layout: 'LIST_COLLAPSED', query: { q: line.trim() } } as never, crypto.randomUUID());
      savedName = name;
      announcer.say(t('app.search.saved_view', { name }));
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    } finally {
      saving = false;
    }
  }

  // ---- Ask in words (Q-6): only where the workspace's AI writes queries (P-14); the answer is a
  // query one sees and takes over, never run by itself.
  const asksInWords = $derived(manifest.value?.features?.query_from_words === true);
  let question = $state('');
  let asking = $state(false);
  let proposal = $state<{ question: string; query: string } | undefined>(undefined);
  let askFailure = $state<string | undefined>(undefined);
  async function ask(event: SubmitEvent) {
    event.preventDefault();
    const asked = question.trim();
    if (asked === '' || asking) return;
    asking = true;
    askFailure = undefined;
    try {
      proposal = { question: asked, query: await askQuery(asked) };
    } catch {
      askFailure = t('app.search.ask_failed');
    } finally {
      asking = false;
    }
  }
  function takeProposal() {
    if (!proposal) return;
    line = proposal.query;
    proposal = undefined;
    question = '';
  }

  // The bar carries the page's title on a phone (ADR-0061 decision 1); the head then reads its
  // heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.search.title')));
</script>

<!-- On a phone the count stands beside the filters' sheet (palette m); wider, in the hits' head. -->
{#snippet phoneCount()}<b>{t('app.search.count', { count: shownCount })}</b>{/snippet}

<!-- Search is worked in - the line, the facets, the hits, their selection - so a work page (§7.8). -->
<WorkPage>
  {#snippet top()}<PageHeader title={t('app.search.title')} isTitleInBar={viewport.isCompact} />{/snippet}

  <!-- `data-tour`: where the tour points for the query language. One line, no switch (Q-1). -->
  <Stack gap="150" data-tour="search" data-head="search">
    <QueryLine
      label={t('app.search.label')}
      language={queryLine.language}
      catalogue={queryLine.catalogue}
      bind:tokens={chips}
      bind:text={rest}
      count={viewport.isCompact ? phoneCount : undefined}
      placeholder={t('app.search.placeholder')}
      suggestionsLabel={t('app.search.suggestions')}
      fieldsHeading={(typed) => t('app.search.fields_heading', { typed })}
      valuesHeading={(field) => t('app.search.values_heading', { field })}
      editLabel={(spelling) => t('app.search.edit_chip', { spelling })}
      removeLabel={(spelling) => t('app.search.remove_chip', { spelling })}
      phone={{
        open: t('app.search.filters_open'),
        title: t('app.search.filters'),
        close: t('app.create.close'),
        reset: t('app.searchline.clear_all'),
        show: t('app.search.show_hits', { count: shownCount }),
        more: (hidden) => `+${hidden}`,
      }}
    >
      {#snippet keys()}<span><Kbd>↑↓</Kbd> {t('app.palette.hint_move')}</span><span><Kbd>↵</Kbd> {t('app.search.take')}</span><span><Kbd>,</Kbd> {t('app.search.or')}</span><span><Kbd>-</Kbd> {t('app.search.not')}</span>{/snippet}
    </QueryLine>
    {#if unknownWords.asWords.length > 0}
      <p class="hint">{t('app.search.as_words', { words: unknownWords.asWords.join(' ') })}</p>
    {/if}

    {#if asksInWords}
      <form class="ask" onsubmit={ask}>
        <span class="spark" aria-hidden="true">✧</span>
        {#if proposal}
          <p class="proposal">
            {t('app.search.ask_gives', { question: proposal.question })}
            <code>{proposal.query}</code>
            <button type="button" class="take" onclick={takeProposal}>{t('app.search.ask_take')}</button>
          </p>
        {:else}
          <input class="question" aria-label={t('app.search.ask_label')} placeholder={t('app.search.ask_placeholder')} bind:value={question} disabled={asking} />
        {/if}
      </form>
      {#if askFailure}<p class="failure" role="status">{askFailure}</p>{/if}
    {/if}
  </Stack>

  {#if compiled.problems.length > 0 && shownKind === 'ENTRY'}
    <ul class="problems">
      {#each compiled.problems as problem, index (index)}
        <li>{t(problem.code, problem.params ?? {})}</li>
      {/each}
    </ul>
  {/if}

  {#if writeFailure}<p class="failure" role="alert">{writeFailure.message}</p>{/if}

  {#if line.trim() === ''}
    <EmptyState kind="unused" title={t('app.search.start')} icon="search" />
  {:else}
    {#if viewport.isCompact}
      <!-- The phone's kinds as a strip of chips with counts (search m); the rest in the line's sheet. -->
      <div class="strip" role="group" aria-label={t('app.search.facet_kind')}>
        {#each scopes as scope (scope.id)}
          <button type="button" class="chip" aria-pressed={shownKind === scope.id} data-facet={scope.id} onclick={() => { chosenKind = scope.id as 'ALL' | Kind; chosenHub = undefined; chosenAuthor = undefined; }}>
            {t('app.palette.scope', { kind: scope.id })} {scope.count}{scope.more ? '+' : ''}
          </button>
        {/each}
      </div>
    {/if}
    <div class="body" data-compact={viewport.isCompact ? '' : undefined}>
      <!-- Facets with counts (§6.11, scene search 3): the kind from the server, the hub and the
           change counted over the hits read. -->
      <nav class="facets" aria-label={t('app.search.facets')}>
        <section>
          <h2>{t('app.search.facet_kind')}</h2>
          {#each scopes as scope (scope.id)}
            <button type="button" class="facet" aria-pressed={shownKind === scope.id} data-facet={scope.id} onclick={() => { chosenKind = scope.id as 'ALL' | Kind; chosenHub = undefined; chosenAuthor = undefined; }}>
              <!-- Each kind with its product's tile, as the scene draws the facet (search 3). -->
              {#if scope.id === 'ALL'}
                <Icon name="search" size="sm" />
              {:else}
                {@const tile = tileOf({ kind: scope.id as Kind, state: null, on: null })}
                <ProductTile icon={tile.icon as IconName} product={tile.product} color={tile.color} size="sm" />
              {/if}
              <span class="name">{t('app.palette.scope', { kind: scope.id })}</span><span class="n">{scope.count}{scope.more ? '+' : ''}</span>
            </button>
          {/each}
        </section>
        {#if hubCounts.size > 0}
          <section>
            <h2>{t('app.search.facet_hub')}</h2>
            {#each [...hubCounts].sort((a, b) => b[1] - a[1]) as [hub, count] (hub)}
              <button type="button" class="facet" aria-pressed={chosenHub === hub} onclick={() => (chosenHub = chosenHub === hub ? undefined : hub)}>
                <span>{containers.find(hub)?.name ?? ''}</span><span class="n">{count}</span>
              </button>
            {/each}
          </section>
        {/if}
        {#if sinceCounts.size > 0}
          <section>
            <h2>{t('app.search.facet_changed')}</h2>
            {#each (['TODAY', 'WEEK', 'OLDER'] as const).filter((since) => sinceCounts.has(since)) as since (since)}
              <button type="button" class="facet" aria-pressed={chosenSince === since} onclick={() => (chosenSince = chosenSince === since ? undefined : since)}>
                <span>{t('app.search.since', { since })}</span><span class="n">{sinceCounts.get(since)}</span>
              </button>
            {/each}
          </section>
        {/if}
        {#if authorCounts.size > 0}
          <section>
            <h2>{t('app.search.facet_by')}</h2>
            {#each [...authorCounts].sort((a, b) => b[1] - a[1]) as [author, count] (author)}
              <button type="button" class="facet" aria-pressed={chosenAuthor === author} onclick={() => (chosenAuthor = chosenAuthor === author ? undefined : author)}>
                <span>{author === actor.account?.id ? t('app.search.by_me') : (accounts.nameOf(author) ?? t('app.search.by_unknown'))}</span><span class="n">{count}</span>
              </button>
            {/each}
          </section>
        {/if}
      </nav>

      <div class="hits">
        <div class="head">
          <!-- A phone says the count once, beside its filters' sheet. -->
          {#if !viewport.isCompact}<p class="count"><b>{t('app.search.count', { count: shownCount })}</b> <span>{t('app.search.count_where')}</span></p>{/if}
          <div class="tools">
            <div class="orders" role="group" aria-label={t('app.search.order')}>
              {#each ['RELEVANCE', 'UPDATED', 'CREATED'] as const as each (each)}
                <button type="button" aria-pressed={order === each} onclick={() => (order = each)}>{t('app.search.order_value', { order: each })}</button>
              {/each}
            </div>
            <Button size="sm" tone="secondary" icon="star" onclick={() => void saveAsView()} isBusy={saving}>{t('app.search.save_view')}</Button>
            {#if shareable !== '' && canCopy(navigator.clipboard)}
              <Button size="sm" tone="subtle" icon="link" onclick={() => void copyLink()}>{t(copied ? 'app.search.link_copied' : 'app.search.copy_link')}</Button>
            {/if}
          </div>
        </div>
        {#if savedName}<p class="hint" role="status">{t('app.search.saved_view', { name: savedName })}</p>{/if}

        {#if shownKind !== 'ENTRY'}
          {#if allFailure}
            <FailureState problem={allFailure} onRetry={() => (answer = { ...answer })} />
          {:else if allStatus === 'searching' && answer.data.length === 0}
            <div aria-busy="true"><Skeleton lines={4} /></div>
          {:else if shownHits.length === 0 && allStatus === 'done'}
            <EmptyState kind="filtered" title={t(hasWords ? 'app.search.none' : 'app.search.narrowed_none')} icon="search" />
          {:else}
            <div class="list" role="list">
              {#each shownHits as hit (`${hit.kind}:${hit.id}`)}
                <div role="listitem"><HitRow {hit} words={typed} onperson={searchPerson} /></div>
              {/each}
            </div>
          {/if}
        {:else if failure}
          <FailureState problem={failure} onRetry={() => void search.run(asked)} />
        {:else if search.status === 'searching' && search.hits.length === 0}
          <div aria-busy="true"><Skeleton lines={4} /></div>
        {:else if search.hits.length === 0 && typed.trim() === ''}
          <EmptyState kind="filtered" title={t('app.search.narrowed_none')} icon="search" />
        {:else if search.hits.length === 0}
          <EmptyState kind="filtered" title={t('app.search.none')} icon="search" />
        {:else}
    <BulkBar onresults={bulkDone} />
    <div class="results" bind:this={results}>
    <Stack gap="050">
      <div class="head">
        {#if hasMeaning}
          <p class="hint">{t(mode === 'LEXICAL' ? 'app.search.ranked_by_words' : 'app.search.ranked_by_meaning')}</p>
        {/if}
        {#if !selection.isOn}
          <Button size="sm" tone="subtle" icon="square-check" onclick={() => selection.start()}>
            {t('app.bulk.select_mode')}
          </Button>
        {/if}
      </div>
      {#each shownEntries as hit (hit.id)}
        {@const isCompleted = hit.completion?.is_completed ?? false}
        <div class="hit" data-hit={hit.id} data-compact={viewport.isCompact ? '' : undefined} data-by-meaning={search.isByMeaning(hit.id) ? '' : undefined}>
        {#if selection.isOn}
          <span class="pick">
            <Checkbox
              label={t('app.bulk.select', { title: hit.title })}
              isLabelHidden={true}
              checked={selection.has(hit.id)}
              onclick={(event: MouseEvent) => selection.pick(hitIds, hit.id, { range: event.shiftKey })}
              onkeydown={(event: KeyboardEvent) => {
                if (event.key !== ' ' && event.key !== 'Enter') return;
                event.preventDefault();
                selection.pick(hitIds, hit.id, { range: event.shiftKey });
              }}
            />
          </span>
        {/if}
        <TaskRow
          type={hit.type}
          title={hit.title}
          href={`/items/${hit.id}`}
          {isCompleted}
          completeLabel={t(isCompleted ? 'app.entries.reopen' : 'app.entries.complete', { title: hit.title })}
          completeDisabledReason={completeReason(hit)}
          hasTwist={false}
          onToggleComplete={() => {
            if (completeReason(hit) === undefined) void toggleComplete(hit.id, hit.title, isCompleted);
          }}
        >
          <!-- Search mixes collections: each hit says where it lives (UC-WRK-33). -->
          {#snippet trailing()}
            {#if search.isByMeaning(hit.id)}
              <!-- None of the reader's words is in this entry: said, so it does not look like a
                   wrong answer. -->
              <Badge icon="sparkles">{t('app.search.found_by_meaning')}</Badge>
            {/if}
            <EntryPlace collectionId={hit.collection_id} />
            <EntryMenu
              bind:this={menus[hit.id]}
              title={hit.title}
              items={menuOf(hit)}
              onselect={(chosen) => void actions?.run(hit, chosen)}
              onopening={() => actions?.readLevel(hit)}
            />
          {/snippet}
        </TaskRow>
        </div>
        {#if bulkNote(hit.id)}<p class="failure">{bulkNote(hit.id)}</p>{/if}
      {/each}
    </Stack>
    </div>
    <EntryActions bind:this={actions} onopen={(id) => onnavigate?.(`/items/${id}`)} {onnavigate} />
          {#if search.isPartial}
            <p class="hint">{t('app.search.partial')}</p>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</WorkPage>

<style>
  .hint { margin: 0; max-width: 64ch; color: var(--text-secondary); font-size: var(--fs-075); }

  .failure { margin: 0; max-width: 64ch; color: var(--text-danger); font-size: var(--fs-075); }

  .problems { margin: 0; padding-inline-start: var(--sp-200); max-width: 64ch; color: var(--text-danger); font-size: var(--fs-075); }

  /* The question in words: a quiet band under the line, the answer a query one takes over. */
  .ask {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
    min-block-size: var(--density-control-md-min);
    padding-inline: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .spark { color: var(--text-subtle); }

  .question { flex: 1; min-inline-size: 0; border: 0; background: none; color: var(--text-primary); font: inherit; font-size: var(--fs-090); }

  .question:focus-visible { outline: none; }

  .proposal { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); margin: 0; color: var(--text-secondary); font-size: var(--fs-090); }

  .proposal code { color: var(--text-primary); font-family: var(--font-mono); }

  .take { border: 0; background: none; color: var(--text-brand); font: inherit; font-weight: var(--fw-semibold); cursor: pointer; }

  .body { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 3fr); gap: var(--sp-300); align-items: start; }

  .body[data-compact] { grid-template-columns: minmax(0, 1fr); }

  .body[data-compact] .facets { display: none; }

  .strip { display: flex; gap: var(--sp-100); overflow-x: auto; scrollbar-width: none; }

  .chip {
    flex: none;
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .chip[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary); color: var(--text-inverse); }

  .chip:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .facets { display: flex; flex-direction: column; gap: var(--sp-300); }

  .facets section { display: flex; flex-direction: column; gap: var(--sp-025); }

  .facets h2 { margin: 0 0 var(--sp-050); color: var(--text-subtle); font-size: var(--fs-065); font-weight: var(--fw-semibold); text-transform: uppercase; }

  .facet {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-050) var(--sp-100);
    border: 0;
    border-radius: var(--r-md);
    background: none;
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-090);
    text-align: start;
    cursor: pointer;
  }

  .facet:hover { background: var(--bg-surface-hover); }
  .facet .name { flex: 1 1 auto; text-align: start; }

  .facet[aria-pressed='true'] { background: var(--bg-surface-pressed); font-weight: var(--fw-semibold); }

  .facet:focus-visible, .orders button:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .n { color: var(--text-subtle); font-variant-numeric: tabular-nums; }

  .hits { display: flex; flex-direction: column; gap: var(--sp-150); min-inline-size: 0; }

  .head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  .count { margin: 0; }

  .count span { color: var(--text-subtle); font-size: var(--fs-075); }

  .tools { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .orders { display: inline-flex; padding: var(--sp-025); border-radius: var(--r-md); background: var(--bg-recess); }

  .orders button { padding: var(--sp-050) var(--sp-150); border: 0; border-radius: var(--r-sm); background: none; color: var(--text-secondary); font: inherit; font-size: var(--fs-075); cursor: pointer; }

  .orders button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); font-weight: var(--fw-semibold); }

  .list { border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); overflow: hidden; }

  /* As a list's row lays out its pick (`EntryList`): the row takes the rest of the line. */
  .hit { display: flex; align-items: center; gap: var(--sp-050); }

  .hit > :global(*:last-child) { flex: 1; min-width: 0; }

  .pick { display: flex; flex: none; align-items: center; }

  .hit[data-compact] { align-items: start; }

  .hit[data-compact] .pick { min-block-size: calc(var(--density-control-md-min) + 2 * var(--density-row-block)); }
</style>
