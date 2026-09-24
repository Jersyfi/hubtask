<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The narrowing, as controls (ADR-0066 decisions 2 and 3).
  //
  // **This is the same state as the field below it.** A chip does not hold a value of its own: it
  // reads the line and writes the line, through `has`, `toggle`, `values` and `clear`. That is the
  // whole of the concept's answer to the thing every product with two filter surfaces gets wrong —
  // Jira's basic mode cannot show a JQL query it did not produce, and says so with a warning — and
  // it is not cleverness, it is having one representation instead of two. A chip press and the
  // equivalent typing produce the same string, so neither surface can be the stale one.
  //
  // **Six questions are drawn and the rest are behind one more.** The six are what the walks saw
  // pressed; "more" holds the ones the grammar can answer and few people ask. That split is Jira's
  // and Linear's alike, and the reason both arrived at it is that a filter bar wide enough for
  // seventeen operators is a filter bar nobody reads.
  //
  // Nothing here names a field the installation has not reported: a chip whose field is missing
  // from `/meta/capabilities` is not drawn — a control that can only ever be refused is worse than
  // one that was never there.

  import { Button, Checkbox, Popover, Stack } from '@hubtask/design-system/components';

  import { accounts } from '../data/accounts.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { KEYS, clear, has, parse, toggle, type Key } from '../data/searchquery.ts';
  import { humanise } from '../i18n/messages.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The line — words and tokens — which is the only state a search screen keeps. */
    value: string;
    /** The fields `/meta/capabilities` reports, so nothing unanswerable is offered. */
    fields: ReadonlySet<string>;
    /** The collection a label chip needs, where the line names exactly one. */
    collectionId?: string;
    onchange: (line: string) => void;
  }

  const { value, fields, collectionId, onchange }: Props = $props();

  const parsed = $derived(parse(value));

  /** One answer of one question: what is written into the line, and the word shown for it. */
  interface Answer {
    readonly key: Key;
    readonly value: string;
    readonly label: string;
  }

  /** One question: the key it writes, the word on the chip, and its answers. */
  interface Question {
    readonly id: string;
    readonly key: Key;
    readonly answers: readonly Answer[];
  }

  const answer = (key: Key, value: string, label: string): Answer => ({ key, value, label });

  const stateAnswers = $derived([
    answer('is', 'open', t('app.searchline.value.open')),
    answer('is', 'done', t('app.searchline.value.done')),
  ]);

  const typeAnswers = $derived(
    ['task', 'work_package', 'activity'].map((kind) => answer('type', kind, humanise(kind))),
  );

  const whenAnswers = (key: Key) =>
    ['overdue', 'today', 'week', 'month', 'none'].map((when) =>
      answer(key, when, t(`app.searchline.value.${when}`)),
    );

  /**
   * Whose work. `me` and nobody are the grammar's own words; the rest are this collection's.
   *
   * The reader is left out of the list, because `me` is already them — and `me` is not the same
   * answer as their name: it compiles to the server's `@me`, so a link carrying it means "whoever
   * is reading" rather than "Jérôme", which is the whole reason the placeholder exists.
   */
  const whoAnswers = $derived([
    answer('who', 'me', t('app.searchline.value.me')),
    answer('who', 'none', t('app.searchline.value.nobody')),
    ...(collectionId
      ? people
          .candidates({ collectionId })
          .filter((id) => id !== actor.account?.id)
          .map((id) => answer('who', accounts.nameOf(id) ?? id, accounts.nameOf(id) ?? id))
      : []),
  ]);

  const whereAnswers = $derived(
    containers.hubs.flatMap((hub) =>
      containers.collectionsOf(hub.id).map((collection) => answer('in', collection.name, collection.name)),
    ),
  );

  const labelAnswers = $derived(
    collectionId ? labels.of(collectionId).map((label) => answer('label', label.name, label.name)) : [],
  );

  /** The six drawn in the bar, and then only where the installation answers them. */
  const primary = $derived<readonly Question[]>(
    (
      [
        { id: 'state', key: 'is' as Key, answers: stateAnswers },
        { id: 'type', key: 'type' as Key, answers: typeAnswers },
        { id: 'due', key: 'due' as Key, answers: whenAnswers('due') },
        { id: 'who', key: 'who' as Key, answers: whoAnswers },
        { id: 'where', key: 'in' as Key, answers: whereAnswers },
        { id: 'label', key: 'label' as Key, answers: labelAnswers },
      ] satisfies Question[]
    ).filter((question) => fields.has(KEYS[question.key]) && question.answers.length > 0),
  );

  /** And the rest, behind one control: the questions the grammar answers and few people ask. */
  const secondary = $derived<readonly Question[]>(
    (
      [
        { id: 'updated', key: 'updated' as Key, answers: whenAnswers('updated') },
        { id: 'created', key: 'created' as Key, answers: whenAnswers('created') },
        { id: 'start', key: 'start' as Key, answers: whenAnswers('start') },
        { id: 'with', key: 'with' as Key, answers: [answer('with', 'me', t('app.searchline.value.me'))] },
        { id: 'by', key: 'by' as Key, answers: [answer('by', 'me', t('app.searchline.value.me'))] },
      ] satisfies Question[]
    ).filter((question) => fields.has(KEYS[question.key]) && question.answers.length > 0),
  );

  /** What widens the search rather than narrowing it: the archive and the trash (ADR-0064). */
  const widenings: readonly Answer[] = [
    { key: 'is', value: 'archived', label: t('app.searchline.value.archived') },
    { key: 'is', value: 'trashed', label: t('app.searchline.value.trashed') },
  ];

  /**
   * What a chip says it holds — **the answers, not how many of them**.
   *
   * A count in a pill inside a chip is a pill inside a pill, and it reads wrong for a reason that
   * survives any amount of tuning: the number carries the badge's own inset *and* the chip's, so
   * there is nine pixels more room after it than before the word. Fixing the spacing would have
   * left the other half of the problem standing — a reader who wants to know what `Kind 2` means
   * has to open it.
   *
   * So the chip names the first answer and says how many more there are: `Kind: Task +1`. One
   * text run, one type treatment, nothing nested — and the bar can be read without being opened,
   * which is what a filter bar is for.
   */
  function chosenIn(question: Question): readonly string[] {
    // By the line, not by the answer list: a value the list does not have yet — a collection whose
    // hub nobody has opened, somebody who is not a candidate here — is still narrowing this search,
    // and a chip that left it out would under-count what it holds. The answer's own word where
    // there is one, and what was written where there is not.
    const words: string[] = [];
    for (const token of parsed.tokens) {
      const answers = question.answers.filter((each) => each.key === token.key);
      if (answers.length === 0) continue;
      // `is:` is two chips: open and done here, the archive and the trash under "More".
      const known = answers.find((each) => each.value === token.value);
      if (!known && question.key === 'is') continue;
      words.push(known?.label ?? token.value);
    }
    return words;
  }

  /** The same, for the one control that stands for several questions. */
  const hidden = $derived([
    ...secondary.flatMap((question) => chosenIn(question)),
    ...widenings.filter((each) => has(parsed, each.key, each.value)).map((each) => each.label),
  ]);

  /** `Open`, or `Task +1`. Empty where nothing is chosen, which is what draws a plain chip. */
  function summarise(words: readonly string[]): string {
    if (words.length === 0) return '';
    if (words.length === 1) return words[0] as string;
    return t('app.searchline.and_more', { first: words[0] as string, more: String(words.length - 1) });
  }
</script>

<div class="bar" role="group" aria-label={t('app.searchline.narrow')}>
  {#each primary as question (question.id)}
    {@const chosen = chosenIn(question)}
    {@const summary = summarise(chosen)}
    <Popover label={t(`app.searchline.chip.${question.id}`)}>
      {#snippet trigger(props)}
        <button type="button" class="chip" data-chosen={summary === '' ? undefined : ''} {...props}>
          <span class="what">{t(`app.searchline.chip.${question.id}`)}</span>
          {#if summary !== ''}<span class="chosen">{summary}</span>{/if}
        </button>
      {/snippet}
      <Stack gap="100">
        {#each question.answers as option (option.value)}
          <Checkbox
            label={option.label}
            checked={has(parsed, option.key, option.value)}
            onchange={() => onchange(toggle(value, option.key, option.value))}
          />
        {/each}
        {#if chosen.length > 0}
          <div>
            <Button size="sm" tone="subtle" onclick={() => onchange(clear(value, question.key))}>
              {t('app.searchline.chip_clear')}
            </Button>
          </div>
        {/if}
      </Stack>
    </Popover>
  {/each}

  {#if secondary.length > 0}
    <Popover label={t('app.searchline.chip.more')}>
      {#snippet trigger(props)}
        {@const summary = summarise(hidden)}
        <button type="button" class="chip" data-chosen={summary === '' ? undefined : ''} {...props}>
          <span class="what">{t('app.searchline.chip.more')}</span>
          {#if summary !== ''}<span class="chosen">{summary}</span>{/if}
        </button>
      {/snippet}
      <Stack gap="150">
        {#each secondary as question (question.id)}
          <fieldset class="group">
            <legend>{t(`app.searchline.chip.${question.id}`)}</legend>
            <Stack gap="050">
              {#each question.answers as option (option.value)}
                <Checkbox
                  label={option.label}
                  checked={has(parsed, option.key, option.value)}
                  onchange={() => onchange(toggle(value, option.key, option.value))}
                />
              {/each}
            </Stack>
          </fieldset>
        {/each}
        <fieldset class="group">
          <legend>{t('app.searchline.chip.also')}</legend>
          <Stack gap="050">
            {#each widenings as option (option.value)}
              <Checkbox
                label={option.label}
                checked={has(parsed, option.key, option.value)}
                onchange={() => onchange(toggle(value, option.key, option.value))}
              />
            {/each}
          </Stack>
        </fieldset>
      </Stack>
    </Popover>
  {/if}
</div>

<!-- Where a label chip would be empty, the reason is said rather than left as an absence: a label
     belongs to a collection (I-W3), so there is nothing to offer until one is named. Below the
     chips rather than among them, because it is a sentence about one of them and not a control. -->
{#if !collectionId && fields.has(KEYS.label)}
  <p class="note">{t('app.searchline.label_needs_collection')}</p>
{/if}

<style>
  .bar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .chip {
    display: inline-flex;
    align-items: baseline;
    gap: var(--sp-050);
    max-inline-size: 28ch;
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

  .chip:hover { border-color: var(--border-default); color: var(--text-primary); }

  .chip[data-chosen] { border-color: var(--accent-primary); }

  /* The question stays quiet and the answer is what carries the weight: a bar somebody is scanning
     is scanned for what is *chosen*, and the words for the questions are the same on every screen. */
  .chip[data-chosen] .what::after { content: ':'; }

  .chosen {
    overflow: hidden;
    color: var(--text-primary);
    font-weight: var(--fw-medium);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .chip:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .note { margin: 0; max-width: 64ch; color: var(--text-secondary); font-size: var(--fs-075); }

  .group { margin: 0; padding: 0; border: 0; }

  .group legend {
    padding: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }
</style>
