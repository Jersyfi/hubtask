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
    SearchField,
    Select,
    Skeleton,
    Stack,
    TaskRow,
  } from '@hubtask/design-system/components';

  import { announcer } from '../lib/announce.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { platform } from '../lib/platform/index.ts';
  import { textLanguages } from '../lib/data/query.ts';
  import { search } from '../lib/data/search.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  let term = $state('');
  /** Empty is the caller's own locale, which is what the contract does when `language` is absent. */
  let language = $state('');

  const languages = $derived(textLanguages(manifest.value));

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
    language: language || undefined,
    readerLocale: actor.locale ?? messages.locale,
    textLanguages: languages,
    // What the browser says this reader reads. It goes through the platform seam for the reason
    // every platform difference does — a shell has its own answer — and it only ever orders the
    // widening, never decides whether one happens.
    preferredLanguages: platform.preferredLanguages(),
  });

  $effect(() => {
    const question = asked;
    if (question.q.trim() === '') {
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

  async function toggleComplete(id: string, isCompleted: boolean) {
    writeFailure = undefined;
    try {
      await items.setCompleted(id, !isCompleted, crypto.randomUUID());
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }
</script>

<Stack gap="300">
  <h1 class="name">{t('app.search.title')}</h1>

  <Inline gap="150" align="end">
    <SearchField
      label={t('app.search.label')}
      clearLabel={t('app.search.clear')}
      bind:value={term}
      onclear={() => search.reset()}
    />
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
  </Inline>

  <p class="hint">{t('app.search.hint')}</p>

  {#if writeFailure}<p class="failure">{writeFailure.message}</p>{/if}

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
          onToggleComplete={() => toggleComplete(hit.id, isCompleted)}
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
  .name {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .hint { margin: 0; max-width: 64ch; color: var(--text-secondary); font-size: var(--fs-075); }

  .failure { margin: 0; max-width: 64ch; color: var(--text-danger); font-size: var(--fs-075); }
</style>
