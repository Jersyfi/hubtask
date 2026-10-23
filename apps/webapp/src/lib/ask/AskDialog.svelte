<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Ask* (UC-AI-08): one question, one proposed answer with the entries it rests on, the filter it
  // implies as chips, and *Save as view*.
  //
  // Each question stands alone: there is no history here and no follow-up turn, and asking again
  // starts from nothing (check 5). The answer performs no action - *Dismiss* is the only decision
  // about it, and saving a view is the person's own act through the ordinary view use case.
  import { untrack } from 'svelte';

  import { AISuggestion, Button, Dialog, IconButton, Inline, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';
  import type { Suggestion, SuggestionPage } from '@hubtask/sync-engine';

  import { answerOf, answerSince, DUE_SPANS, hasChips, viewQueryOf, without, conditionsOf, type Chips, type DueSpan } from './answer.ts';
  import { workspaceAsks } from './asks.svelte.ts';
  import AskName from './AskName.svelte';
  import AskSource from './AskSource.svelte';
  import { isBudgetNearlySpent, type BudgetState } from '../data/aiplaces.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { filterOf } from '../data/query.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { suggestions, suggestionsPath } from '../data/suggestions.svelte.ts';
  import { isPending, noteOf } from '../data/suggestions.ts';
  import { views } from '../data/views.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem, renderRefusal } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    /** The person asking: the answer is about their own account, and only they read it. */
    accountId: string;
    /** The words typed into the palette, as the question to start from. */
    words: string;
  }

  let { isOpen = $bindable(), accountId, words }: Props = $props();

  const listing = resource<SuggestionPage>(untrack(() => suggestionsPath(accountId, 'ACCOUNT')));
  const budget = resource<BudgetState>('/ai-budget-state');
  const nearlySpent = $derived(isBudgetNearlySpent(budget.state.status === 'ready' ? budget.state.data : undefined));

  const question = $derived(workspaceAsks.question);
  const note = $derived(noteOf(question));
  const answered = $derived<Suggestion | undefined>(
    question?.outcome === 'arrived' && listing.state.status === 'ready'
      ? answerSince(listing.state.data.items ?? [], question.askedAt)
      : undefined,
  );
  const answer = $derived(answered ? answerOf(answered) : undefined);

  let asked = $state(untrack(() => words));
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let chips = $state<Chips>({});
  let chipsOf = $state<string | undefined>(undefined);
  let viewName = $state('');
  let isPlaceMissing = $state(false);
  let isNameMissing = $state(false);
  let isSaving = $state(false);
  let isDismissing = $state(false);

  // Choosing *Ask* in the palette is asking: the words typed there are the question, and nothing of
  // an earlier one is carried into it.
  $effect(() => {
    untrack(() => {
      workspaceAsks.forget();
      if (asked.trim() !== '') void ask();
    });
  });

  // The chips are the person's from the moment the answer arrives: taken from it once.
  $effect(() => {
    if (!answered || !answer || chipsOf === answered.id) return;
    chipsOf = answered.id;
    chips = answer.chips;
    viewName = '';
    isPlaceMissing = false;
    isNameMissing = false;
    announcer.say(t('app.ask.arrived_announced'));
  });

  async function ask(): Promise<void> {
    failure = undefined;
    const words = asked.trim();
    if (words === '') return;
    chipsOf = undefined;
    chips = {};
    await workspaceAsks.ask(accountId, words);
  }

  async function dismiss(): Promise<void> {
    if (!answered) return;
    isDismissing = true;
    try {
      await suggestions.dismiss(answered);
      workspaceAsks.forget();
      announcer.say(t('app.suggestions.dismissed_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isDismissing = false;
    }
  }

  async function saveView(): Promise<void> {
    // Never switched off for what is missing (design-system.md §4, case 3): said on submit.
    isPlaceMissing = !chips.place;
    isNameMissing = viewName.trim() === '';
    if (isPlaceMissing || isNameMissing) return;
    const query = viewQueryOf(chips, filterOf(manifest.value, conditionsOf(chips)));
    if (!query) return;
    isSaving = true;
    failure = undefined;
    try {
      await views.create({ scope_type: 'ACCOUNT', name: viewName.trim(), layout: 'LIST_COLLAPSED', query } as never, crypto.randomUUID());
      announcer.say(t('app.ask.view_saved'));
      viewName = '';
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  function provenanceOf(suggestion: Suggestion): string {
    return t('app.suggestions.provenance_line', {
      model: suggestion.model,
      when: formatDateTime(suggestion.produced_at, messages.locale),
      prompt: suggestion.prompt_id,
      version: suggestion.prompt_version,
    });
  }

  const dueOptions = $derived(DUE_SPANS.map((span) => ({ value: span, label: t(`app.ask.due_${span}`) })));
  const doneOptions = $derived([
    { value: 'false', label: t('app.ask.chip_completed', { done: 'false' }) },
    { value: 'true', label: t('app.ask.chip_completed', { done: 'true' }) },
  ]);
</script>

<Dialog bind:isOpen title={t('app.ask.title')} dismissLabel={t('app.workspace.cancel')}>
  <Stack gap="150">
    <div class="ask" data-ai>
      <Textarea label={t('app.ask.label')} hint={t('app.ask.hint')} bind:value={asked} rows={2} error={failure?.fields.get('/question')} />
      {#if nearlySpent}<p class="note">{t('app.ai.budget_nearly_spent')}</p>{/if}
      <Inline gap="100">
        <Button tone="primary" icon="sparkles" disabledReason={isPending(question) ? t('app.suggestions.pending') : undefined} onclick={() => void ask()}>
          {t('app.ask.submit')}
        </Button>
      </Inline>
    </div>

    {#if question && isPending(question)}
      <AISuggestion heading={t('app.ask.heading')} state="pending" pendingLabel={t('app.suggestions.pending')}>
        {#if question.outcome === 'slow'}<p class="note">{t('app.suggestions.slow')}</p>{/if}
        {#snippet actions()}
          <Button tone="subtle" size="sm" onclick={() => void workspaceAsks.stop(accountId)}>{t('app.ai.stop')}</Button>
        {/snippet}
      </AISuggestion>
    {:else if note && !answer}
      <p class={note.tone === 'refusal' ? 'failure' : 'note'} role={note.tone === 'refusal' ? 'alert' : undefined} data-note={note.code}>
        {renderRefusal(note, messages)}
      </p>
    {/if}

    {#if answered && answer}
      <AISuggestion heading={t('app.ask.heading')} provenance={provenanceOf(answered)} provenanceLabel={t('app.suggestions.provenance')}>
        <p class="answer" data-answer>{answer.text}</p>
        {#if answer.sourceIds.length > 0}
          <p class="label">{t('app.ask.sources')}</p>
          <ul class="sources">
            {#each answer.sourceIds as id (id)}<AskSource itemId={id} />{/each}
          </ul>
        {/if}
        {#if hasChips(chips) || hasChips(answer.chips)}
          <p class="label">{t('app.ask.filter')}</p>
          <ul class="chips" aria-label={t('app.ask.filter')}>
            {#if chips.place}
              <li class="chip" class:missing={isPlaceMissing} data-chip="place">
                <AskName path={`/containers/${chips.place.id}`} field="name" fallback={t('app.ask.chip_place_fallback')} />
                <IconButton icon="x" size="sm" tone="subtle" label={t('app.ask.remove_chip', { name: t('app.ask.chip_place_fallback') })} onclick={() => (chips = without(chips, 'place'))} />
              </li>
            {/if}
            {#if chips.isCompleted !== undefined}
              <li class="chip" data-chip="completed">
                <Select size="sm" label={t('app.ask.chip_state')} options={doneOptions} value={String(chips.isCompleted)}
                  onchange={(event: Event) => (chips = { ...chips, isCompleted: (event.currentTarget as HTMLSelectElement).value === 'true' })} />
                <IconButton icon="x" size="sm" tone="subtle" label={t('app.ask.remove_chip', { name: t('app.ask.chip_state') })} onclick={() => (chips = without(chips, 'isCompleted'))} />
              </li>
            {/if}
            {#if chips.due}
              <li class="chip" data-chip="due">
                <Select size="sm" label={t('app.ask.chip_due')} options={dueOptions} value={chips.due}
                  onchange={(event: Event) => (chips = { ...chips, due: (event.currentTarget as HTMLSelectElement).value as DueSpan })} />
                <IconButton icon="x" size="sm" tone="subtle" label={t('app.ask.remove_chip', { name: t('app.ask.chip_due') })} onclick={() => (chips = without(chips, 'due'))} />
              </li>
            {/if}
            {#if chips.assigneeId}
              <li class="chip" data-chip="assignee">
                <AskName path={`/accounts/${chips.assigneeId}`} field="display_name" fallback={t('app.ask.chip_person_fallback')} />
                <IconButton icon="x" size="sm" tone="subtle" label={t('app.ask.remove_chip', { name: t('app.ask.chip_person_fallback') })} onclick={() => (chips = without(chips, 'assigneeId'))} />
              </li>
            {/if}
            {#if chips.labelIds}
              <li class="chip" data-chip="labels">
                {t('app.ask.chip_labels', { count: chips.labelIds.length })}
                <IconButton icon="x" size="sm" tone="subtle" label={t('app.ask.remove_chip', { name: t('app.ask.chip_labels', { count: chips.labelIds.length }) })} onclick={() => (chips = without(chips, 'labelIds'))} />
              </li>
            {/if}
          </ul>
          {#if isPlaceMissing}<p class="failure" role="alert" data-missing="place">{t('app.ask.place_missing')}</p>{/if}
          <div class="save">
            <Input label={t('app.ask.view_name')} bind:value={viewName} error={isNameMissing ? t('app.ask.view_name_missing') : undefined} />
            <Button size="sm" tone="secondary" icon="plus" isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={() => void saveView()}>
              {t('app.ask.save_view')}
            </Button>
          </div>
        {/if}
        {#if failure && !failure.fields.get('/question')}<p class="failure" role="alert">{failure.message}</p>{/if}
        {#snippet actions()}
          <Button size="sm" tone="subtle" isBusy={isDismissing} busyLabel={t('app.suggestions.dismissing')} onclick={() => void dismiss()}>
            {t('app.suggestions.dismiss')}
          </Button>
        {/snippet}
      </AISuggestion>
    {/if}
  </Stack>
</Dialog>

<style>
  .ask { display: flex; flex-direction: column; gap: var(--sp-100); }

  .answer { margin: 0; max-width: 64ch; white-space: pre-line; }

  .label { margin: var(--sp-100) 0 0; font-weight: var(--fw-semibold); }

  .sources { margin: var(--sp-050) 0 0; padding-inline-start: var(--sp-200); }

  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: var(--sp-050) 0 0; padding: 0; list-style: none; }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding-block: var(--sp-025);
    padding-inline: var(--sp-100) var(--sp-025);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
  }

  .chip.missing { border-color: var(--status-danger-border); background: var(--status-danger-surface); }

  .save { display: flex; flex-wrap: wrap; align-items: flex-end; gap: var(--sp-100); margin-block-start: var(--sp-100); }

  .note { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
