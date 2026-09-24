<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
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
    Button,
    EmptyState,
    ErrorState,
    Inline,
    PageHeader,
    SearchField,
    Skeleton,
    Stack,
    Switch,
    TaskRow,
    canCopy,
  } from '@hubtask/design-system/components';

  import FilterBar from '../lib/search/FilterBar.svelte';
  import SearchLine from '../lib/search/SearchLine.svelte';

  import { announcer } from '../lib/announce.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { labels } from '../lib/data/labels.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { queryFields } from '../lib/data/query.ts';
  import { health } from '../lib/data/health.svelte.ts';
  import { search, type SearchMode } from '../lib/data/search.svelte.ts';
  import {
    QUICK,
    asksSomething,
    compile,
    content,
    isQuick,
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

  /**
   * Whether the narrowing is edited as text rather than as chips.
   *
   * It lives in the address like the narrowing does — the bar's "with the filter open" is what
   * puts it there — so it is adopted on arrival and whenever the address moves under the screen,
   * and written back whenever the reader presses the control. One value, one home.
   */
  let advanced = $state(untrack(() => query.edit) === '1');

  $effect(() => {
    const named = query[HANDLE];
    const carried = query.f ?? '';
    const editing = query.edit === '1';
    untrack(() => {
      // Arriving: a reload or the back button naming a handle this tab wrote, so its words come
      // back and join the narrowing the address carried.
      const known = isHandle(named) ? search.recall(named) : undefined;
      if (isHandle(named) && named !== handle && known !== undefined) {
        handle = named;
        line = joinLine(carried, known);
        advanced = editing;
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
      if (editing !== advanced) advanced = editing;
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
    if (handed !== undefined) line = joinLine(untrack(() => narrowingOf(line)), handed);
  });

  const parsed = $derived(parse(line));

  /** The editing split: the plain words, and every token. What the two basic controls hold. */
  const typed = $derived(wordsOf(line));
  const tokens = $derived(narrowingOf(line));

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

  /**
   * The tokens the chips do not draw, which the reader is **told about** rather than left to
   * discover.
   *
   * This is the one place the concept answers Jira's warning, and it answers it differently. There
   * the basic surface cannot represent the query, so it refuses to open; here there is only one
   * representation, so nothing is lost — the chips simply do not have a control for `title:`,
   * `note:` or `sort:`, and saying so with the way to edit them is the whole of the problem.
   */
  const CHIPPED = new Set(['is', 'type', 'due', 'who', 'in', 'label', 'updated', 'created', 'start', 'with', 'by']);
  const unshown = $derived(parsed.tokens.filter((token) => !CHIPPED.has(token.key)));

  /** The address this search has: the narrowing, and the handle its words are kept under. */
  const address = $derived.by(() => {
    const carried = new URLSearchParams({
      ...(shareable === '' ? {} : { f: shareable }),
      ...(secret === '' ? {} : { [HANDLE]: handle }),
      ...(advanced ? { edit: '1' } : {}),
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

  // Words and meaning, or words only (F5-04). The switch exists only where the manifest says
  // meaning is available here; an installation without it has a search by words that never
  // mentioned the other (ADR-0063 decision 4).
  const hasMeaning = $derived(manifest.value?.features?.semantic_search === true);
  const meaningDegraded = $derived(health.degradations.find((d) => d.feature === 'semantic_search'));
  let byMeaning = $state(true);
  const mode = $derived<SearchMode | undefined>(hasMeaning ? (byMeaning ? 'AUTO' : 'LEXICAL') : undefined);

  /** The quick narrowings, offered on the empty screen as the way in that needs no typing. */
  const quick = $derived(QUICK.filter((each) => reported.has(each.field)));

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
    if (!asksSomething(compiled)) {
      search.reset();
      return;
    }
    const timer = setTimeout(() => void search.run(question), 250);
    return () => clearTimeout(timer);
  });

  // What arrived, said out loud. A list that changes under a reader still in the field is a change
  // a screen reader is told nothing about.
  $effect(() => {
    if (search.status !== 'done') return;
    announcer.say(
      search.hits.length === 0
        ? t('app.search.none')
        : t('app.search.found', { count: search.hits.length }),
    );
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
    const link = `${location.origin}/search${carried === '' ? '' : `?${carried}`}`;
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

  // The bar carries the page's title on a phone (ADR-0061 decision 1); the head then reads its
  // heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.search.title')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.search.title')} isTitleInBar={viewport.isCompact} />

  <!-- `data-tour`: where the tour points for the query language (F6-14). -->
  <Stack gap="150" data-tour="search">
    {#if advanced}
      <!-- The whole question as text. Everything the chips can say, and the four things they
           cannot: `title:`, `note:`, `sort:` and a negation. -->
      <SearchLine value={line} {context} onchange={(next) => (line = next)} />
    {:else}
      <Inline gap="150" align="end">
        <div class="term">
          <SearchField
            label={t('app.search.label')}
            clearLabel={t('app.search.clear')}
            value={typed}
            oninput={(event) => (line = joinLine(tokens, (event.currentTarget as HTMLInputElement).value))}
            onclear={() => (line = tokens)}
          />
        </div>
        {#if hasMeaning}
          <Switch
            label={t('app.search.by_meaning')}
            bind:checked={byMeaning}
            disabledReason={meaningDegraded
              ? t('app.search.meaning_degraded', { reason: t(meaningDegraded.reasonCode) })
              : undefined}
          />
        {/if}
      </Inline>

      <FilterBar
        value={line}
        fields={reported}
        collectionId={oneCollection}
        onchange={(next) => (line = next)}
      />

      {#if unshown.length > 0}
        <!-- Not a warning that something is lost — nothing is. The chips have no control for
             these, and that is worth one sentence and a way through. -->
        <p class="hint">
          {t('app.searchline.not_in_chips', { parts: unshown.map((token) => `${token.key}:${token.value}`).join(' ') })}
        </p>
      {/if}
    {/if}

    <div class="aside">
      <Button
        size="sm"
        tone="subtle"
        icon={advanced ? 'sliders-horizontal' : 'pencil'}
        onclick={() => (advanced = !advanced)}
      >
        {t(advanced ? 'app.searchline.as_controls' : 'app.searchline.as_text')}
      </Button>
      {#if shareable !== '' && canCopy(navigator.clipboard)}
        <!-- The link says what it carries, because what it leaves out is the point: the narrowing
             travels, the words do not. -->
        <Button size="sm" tone="subtle" icon="link" onclick={() => void copyLink()}>
          {t(copied ? 'app.search.link_copied' : 'app.search.copy_link')}
        </Button>
      {/if}
      {#if line !== ''}
        <Button size="sm" tone="subtle" icon="x" onclick={() => (line = '')}>
          {t('app.searchline.clear_all')}
        </Button>
      {/if}
    </div>
  </Stack>

  {#if compiled.problems.length > 0}
    <!-- What the line asks for and cannot have, said by name. A token the installation does not
         answer is refused here rather than sent and answered with an empty page. -->
    <ul class="problems">
      {#each compiled.problems as problem, index (index)}
        <li>{t(problem.code, problem.params ?? {})}</li>
      {/each}
    </ul>
  {/if}

  {#if shareable !== ''}
    <p class="hint">{t('app.search.link_carries')}</p>
  {/if}

  {#if writeFailure}<p class="failure" role="alert">{writeFailure.message}</p>{/if}

  {#if failure}
    <ErrorState
      title={failure.message}
      reference={failure.reference}
      referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')}
      onRetry={() => void search.run(asked)}
    />
  {:else if search.status === 'searching' && search.hits.length === 0}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if search.status === 'idle'}
    <!-- The empty screen is not empty: it is where the narrowings that need no typing live. That
         is Jira's starred filters and Linear's views, at the one moment a reader has arrived with
         no question of their own yet. -->
    <Stack gap="150">
      <EmptyState kind="unused" title={t('app.search.start')} icon="search" />
      {#if quick.length > 0}
        <div class="starts">
          {#each quick as each (each.line)}
            <button
              type="button"
              class="pill"
              data-chosen={isQuick(line, each) ? '' : undefined}
              onclick={() => (line = each.line)}
            >
              {t(each.code)}
            </button>
          {/each}
        </div>
      {/if}
    </Stack>
  {:else if search.hits.length === 0 && typed.trim() === ''}
    <!-- Narrowed and empty is a different sentence from searched and empty: nothing was looked
         *for*, so nothing "does not match the words" — what excluded everything is the narrowing. -->
    <EmptyState kind="filtered" title={t('app.search.narrowed_none')} icon="search" />
  {:else if search.hits.length === 0}
    <EmptyState kind="filtered" title={t('app.search.none')} icon="search" />
  {:else}
    <Stack gap="050">
      {#if hasMeaning}
        <p class="hint">{t(mode === 'LEXICAL' ? 'app.search.ranked_by_words' : 'app.search.ranked_by_meaning')}</p>
      {/if}
      {#each search.hits as hit (hit.id)}
        {@const isCompleted = hit.completion?.is_completed ?? false}
        <TaskRow
          type={hit.type}
          title={hit.title}
          href={`/items/${hit.id}`}
          {isCompleted}
          completeLabel={t(isCompleted ? 'app.entries.reopen' : 'app.entries.complete', { title: hit.title })}
          onToggleComplete={() => toggleComplete(hit.id, hit.title, isCompleted)}
        />
      {/each}
    </Stack>
    {#if search.isPartial}
      <p class="hint">{t('app.search.partial')}</p>
    {/if}
  {/if}
</Stack>

<style>
  .term { flex: 1 1 24ch; min-width: 0; max-width: 48ch; }

  .aside { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .hint { margin: 0; max-width: 64ch; color: var(--text-secondary); font-size: var(--fs-075); }

  .failure { margin: 0; max-width: 64ch; color: var(--text-danger); font-size: var(--fs-075); }

  .problems {
    margin: 0;
    padding-inline-start: var(--sp-200);
    max-width: 64ch;
    color: var(--text-danger);
    font-size: var(--fs-075);
  }

  .starts { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .pill {
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .pill:hover, .pill[data-chosen] { border-color: var(--accent-primary); color: var(--text-primary); }

  .pill:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
