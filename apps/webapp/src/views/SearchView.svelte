<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Search, at its own address — and the term is never in it.
  //
  // `/search` is a `POST` with no `GET` because what somebody is looking for is their content, and
  // a query string travels through access logs, proxies and browser history. A screen that put the
  // term in the address bar would undo that, so this route carries no parameter, nothing is pushed
  // to the history as it is typed, and the field is where the term lives.
  //
  // The **language picker** is a different question from the reader's own language: an entry is
  // indexed under the language it was written in, and this decides how the *query* is read. What
  // it offers is `text_languages` — the installation's answer rather than the product's, because
  // the mapping from a tag to a text search configuration is what its PostgreSQL was built with
  // (ADR-0034).

  import {
    Badge,
    Button,
    EmptyState,
    ErrorState,
    Inline,
    PageHeader,
    SearchField,
    Select,
    Skeleton,
    Stack,
    Switch,
    TaskRow,
  } from '@hubtask/design-system/components';

  import FilterChips from '../lib/search/FilterChips.svelte';

  import { announcer } from '../lib/announce.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { platform } from '../lib/platform/index.ts';
  import { textLanguages } from '../lib/data/query.ts';
  import { health } from '../lib/data/health.svelte.ts';
  import { search, type SearchMode } from '../lib/data/search.svelte.ts';
  import { fromQuery, toFilter, toQuery, type Chosen } from '../lib/data/searchfilters.ts';
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

  let term = $state('');

  /**
   * The words the app bar handed over, taken as they arrive.
   *
   * An effect rather than an initial value, because this screen is not remounted when somebody
   * searches again from the bar while already on it — and that press has to do something. Taken
   * rather than read: the store empties on the way out, so the same words handed over twice are
   * two searches (`search.svelte.ts`).
   */
  $effect(() => {
    if (search.handedOver === undefined) return;
    term = search.takeHandover() ?? term;
  });
  /**
   * The narrowing, read from the address and written back to it.
   *
   * The chips are structural — a kind, a state, a label, a collection — so they belong in the
   * address: a narrowing somebody can link to, bookmark and edit. The **words do not**, and that
   * is the one sentence of ADR-0063 decision 4 this screen does not do: `POST /search` has no
   * `GET` because a term is content and a query string travels through access logs, proxies and
   * browser history, and a screen that reflected the term would undo the reason the operation is
   * a POST (`api-guidelines.md` §2, `search.svelte.ts`).
   */
  const chosen = $derived<Chosen>(fromQuery(query));

  function narrow(next: Chosen) {
    const carried = new URLSearchParams(toQuery(next)).toString();
    onnavigate?.(carried === '' ? '/search' : `/search?${carried}`);
  }
  /** Empty is the caller's own locale, which is what the contract does when `language` is absent. */
  let language = $state('');

  const languages = $derived(textLanguages(manifest.value));

  // Words and meaning, or words only (F5-04). The switch exists only where the manifest says
  // meaning is available here: `semantic_search` is answered for the caller's workspace, and an
  // installation without it has a search by words that never mentioned the other (decision 4).
  // Where the feature is *degraded* - a provider configured and unreachable - the switch stays and
  // is gated with the reason `/meta/health` names, which is decision 4's one exception: a feature
  // the reader was shown and is now refused says why.
  const hasMeaning = $derived(manifest.value?.features?.semantic_search === true);
  const meaningDegraded = $derived(health.degradations.find((d) => d.feature === 'semantic_search'));
  let byMeaning = $state(true);
  const mode = $derived<SearchMode | undefined>(hasMeaning ? (byMeaning ? 'AUTO' : 'LEXICAL') : undefined);

  /**
   * The search is run a beat after the typing stops, not on every keystroke.
   *
   * A request per character is a request the reader has already replaced by the time it lands, and
   * the store drops those answers anyway — the wait is what keeps them from being made at all. It
   * is an effect rather than a handler because the language is part of the question too: changing
   * it re-asks without the reader typing anything.
   */
  /**
   * The question, including what to widen to when the reader's own language finds nothing.
   *
   * The reader's locale and the installation's languages travel with it rather than being read in
   * the store: the manifest is read once, in one place, and a data module that reached for it would
   * be a second answer to what the installation says about itself.
   */
  const asked = $derived({
    q: term,
    filter: toFilter(chosen),
    language: language || undefined,
    mode,
    readerLocale: actor.locale ?? messages.locale,
    textLanguages: languages,
    // What the browser says this reader reads. It goes through the platform seam for the reason
    // every platform difference does — a shell has its own answer — and it only ever orders the
    // widening, never decides whether one happens.
    preferredLanguages: platform.preferredLanguages(),
  });

  $effect(() => {
    const question = asked;
    // Either is a question; neither is the empty screen this starts on (ADR-0064).
    if (question.q.trim() === '' && question.filter === undefined) {
      search.reset();
      return;
    }
    const timer = setTimeout(() => void search.run(question), 250);
    return () => clearTimeout(timer);
  });

  // What arrived, said out loud. A list that changes under a reader who is still in the field is a
  // change a screen reader is told nothing about — the same gap a rank change has.
  $effect(() => {
    if (search.status !== 'done') return;
    announcer.say(
      search.hits.length === 0
        ? t(search.didWiden ? 'app.search.widened_none' : 'app.search.none')
        : t('app.search.found', { count: search.hits.length }),
    );
  });

  const failure = $derived(search.error ? renderProblem(search.error, messages) : undefined);

  /**
   * A hit is a row like any other, so it can be ticked off where it is found.
   *
   * Re-read rather than predicted, for the reason the list records: with `completionPolicy =
   * ROLLUP` a parent completes when its children do (I-W5). The hit is not re-read here, though —
   * a search is a walk that was already made, and re-running it because one entry changed would
   * move the results under the reader's hand.
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
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.search.title')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.search.title')} isTitleInBar={viewport.isCompact} />

  <!-- `data-tour`: where the tour points for the query language (F6-14). -->
  <Inline gap="150" align="end" data-tour="search">
    <!-- The field takes the room the line has - the whole width on a phone, a few words' worth
         beside the language and the switch on a desk - rather than the width a bare input picks. -->
    <div class="term">
      <SearchField
        label={t('app.search.label')}
        clearLabel={t('app.search.clear')}
        bind:value={term}
        onclear={() => search.reset()}
      />
    </div>
    <!-- Offered only where the installation reports more than one, because a picker with a single
         option is a decision nobody has. -->
    {#if languages.length > 1}
      <Select
        label={t('app.search.language')}
        size="sm"
        bind:value={language}
        placeholder={t('app.search.language_mine')}
        options={languages.map((tag) => ({ value: tag, label: tag }))}
      />
    {/if}
    {#if hasMeaning}
      <Switch
        label={t('app.search.by_meaning')}
        bind:checked={byMeaning}
        disabledReason={meaningDegraded ? t('app.search.meaning_degraded', { reason: t(meaningDegraded.reasonCode) }) : undefined}
      />
    {/if}
  </Inline>

  <!-- What it is narrowed to, under the field: each chip a question, each saying how many of its
       answers are chosen (ADR-0063 decision 4). -->
  <FilterChips {chosen} onchange={narrow} />

  <p class="hint">{t('app.search.hint')}</p>

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
    <EmptyState kind="unused" title={t('app.search.start')} icon="search" />
  {:else if search.hits.length === 0 && term.trim() === ''}
    <!-- Narrowed and empty is a different sentence from searched and empty: nothing was looked
         *for*, so nothing "does not match the words" — what excluded everything is the narrowing. -->
    <EmptyState kind="filtered" title={t('app.search.narrowed_none')} icon="search" />
  {:else if search.hits.length === 0}
    <!-- `filtered`, not `unused`: something excluded everything, and voice-and-tone.md §4.2 is
         about exactly that — the emptiness has a cause and the sentence names it. And when the
         other languages were asked too, the sentence says so — otherwise "nothing matches" would
         be hiding how hard this looked. -->
    <Stack gap="150">
      <EmptyState
        kind="filtered"
        title={t(search.didWiden || languages.length <= 1 || language ? 'app.search.widened_none' : 'app.search.none')}
        icon="search"
      />
      {#if search.remainingCount > 0}
        <!-- Offered rather than spent. Thirty languages is thirty round trips, and the list comes
             alphabetically, so a subset of it would be a guess — this asks the whole of it, once
             somebody says the silence was worth the wait. It is not the picker R-08 objected to:
             it makes no choice and needs no knowledge, and it is here exactly when it is useful. -->
        <div>
          <Button tone="secondary" onclick={() => void search.widenToRest()}>
            {t('app.search.look_wider', { count: String(search.remainingCount) })}
          </Button>
          <p class="hint">{t('app.search.look_wider_hint')}</p>
        </div>
      {/if}
    </Stack>
  {:else}
    <Stack gap="050">
      {#if hasMeaning}
        <!-- Which ranking answered. The page carries no `ranking` of its own (the contract answers
             a WorkItemPage), so this says what was asked: words and meaning, or words only. -->
        <p class="hint">{t(mode === 'LEXICAL' ? 'app.search.ranked_by_words' : 'app.search.ranked_by_meaning')}</p>
      {/if}
      {#if search.didWiden}
        <!-- Said once, above the results, rather than inferred from the labels: the reader asked a
             question that found nothing and got an answer to a wider one, and that is worth a
             sentence rather than a badge they have to interpret. -->
        <p class="hint">{t('app.search.widened')}</p>
      {/if}
      {#each search.hits as hit (hit.id)}
        {@const isCompleted = hit.completion?.is_completed ?? false}
        <TaskRow
          type={hit.type}
          title={hit.title}
          href={`/items/${hit.id}`}
          {isCompleted}
          completeLabel={t(isCompleted ? 'app.entries.reopen' : 'app.entries.complete', {
            title: hit.title,
          })}
          onToggleComplete={() => toggleComplete(hit.id, hit.title, isCompleted)}
        >
          {#snippet trailing()}
            {#if search.languageOf(hit.id)}
              <!-- Which language found it. A language tag rather than a name: the installation
                   reports tags, and naming them would be a table this client would be wrong about
                   on the installation that indexes one more. -->
              <Badge>{t('app.search.found_under', { language: search.languageOf(hit.id) ?? '' })}</Badge>
            {/if}
          {/snippet}
        </TaskRow>
      {/each}
    </Stack>
    {#if search.isPartial}
      <p class="hint">{t('app.search.partial')}</p>
    {/if}
  {/if}
</Stack>

<style>
  .term { flex: 1 1 24ch; min-width: 0; max-width: 48ch; }

  .hint { margin: 0; max-width: 64ch; color: var(--text-secondary); font-size: var(--fs-075); }

  .failure { margin: 0; max-width: 64ch; color: var(--text-danger); font-size: var(--fs-075); }
</style>
