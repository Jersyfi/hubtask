<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What AI proposed an arrival should become, in the card's slot (F5-03).
  //
  // The half F4-12 left absent on purpose. `:suggest` answers `202` with no body, like every AI
  // ask, so the proposal is followed through the listing (`suggestions.askJumble`); once it
  // stands, this renders it as an `AISuggestion` - a title, notes, a date, and the subtasks the
  // material implied (J-06, K-01). **Accepting is `:accept` with the collection the person
  // chooses**: a model does not choose a destination (`Producing.go`'s filter, restated here by
  // the picker being required), and the acceptance is what converts the entry with the proposed
  // fields and grows the subtasks under what the conversion made - `:convert` alone would create
  // the entry and lose the rest. Dismissing the proposal leaves the arrival `NEW`.

  import { untrack } from 'svelte';

  import { AISuggestion, Button, Input, Select, Skeleton } from '@hubtask/design-system/components';
  import type { Suggestion, SuggestionPage } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import type { JumbleEntry } from '../data/jumble.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { suggestions, suggestionsPath } from '../data/suggestions.svelte.ts';
  import { shapeOf } from '../data/suggestions.ts';
  import { formatDateTime, formatDue } from '../i18n/datetime.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    entry: JumbleEntry;
    /** Every collection the arrival may become work in, as the view lists them. */
    destinations: readonly { value: string; label: string }[];
    /** The collection the view's own conversion form last chose, so this asks no second time (3.3.7). */
    chosen?: string;
    /** Called once the acceptance converted the entry. The view goes to what was made. */
    onaccepted?: (entryId: string) => void;
  }

  const { entry, destinations, chosen = '', onaccepted }: Props = $props();

  const listing = resource<SuggestionPage>(untrack(() => suggestionsPath(entry.id, 'JUMBLE_ENTRY')));
  $effect(() => () => suggestions.forget(entry.id));

  const open = $derived(
    listing.state.status === 'ready'
      ? (listing.state.data.items ?? []).filter((suggestion) => suggestion.status === 'PROPOSED')
      : [],
  );
  const asking = $derived(suggestions.askingOf(entry.id));

  // A proposal arriving is a job finishing that nobody focused (4.1.3): said once, at the moment
  // the strip shows it - and a wait that ended empty is said the same way, since the note it
  // leaves is as silent as the proposal would have been.
  let announcedFor = $state<string | undefined>(undefined);
  $effect(() => {
    const now = asking;
    if (!now || now.outcome === 'following' || announcedFor === now.askedAt) return;
    announcedFor = now.askedAt;
    announcer.say(
      now.outcome === 'arrived'
        ? t('app.suggestions.arrived_announced')
        : now.outcome === 'gave_up'
          ? t('app.suggestions.gave_up')
          : t('app.suggestions.nothing_near'),
    );
  });
  const listingFailure = $derived(
    listing.state.status === 'failed' ? renderProblem(listing.state.error, messages) : undefined,
  );

  let destination = $state('');
  let title = $state('');
  let accepting = $state<string | undefined>(undefined);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  function provenanceOf(suggestion: Suggestion): string {
    return t('app.suggestions.provenance_line', {
      model: suggestion.model,
      when: formatDateTime(suggestion.produced_at, messages.locale),
      prompt: suggestion.prompt_id,
      version: suggestion.prompt_version,
    });
  }

  function fieldCode(field: 'title' | 'notes' | 'due_date'): string {
    return field === 'title' ? 'app.suggestions.field_title' : field === 'notes' ? 'app.suggestions.field_notes' : 'app.suggestions.field_due_date';
  }

  function startAccepting(suggestion: Suggestion, proposedTitle: string | undefined) {
    accepting = suggestion.id;
    destination = chosen;
    title = proposedTitle ?? entry.raw_subject?.trim() ?? '';
    failure = undefined;
  }

  async function accept(suggestion: Suggestion) {
    if (!destination || isWorking) return;
    isWorking = true;
    failure = undefined;
    try {
      // The destination is the person's, never the model's; an edited title is theirs too. Both
      // are laid over the proposal by the server and checked with their own rights.
      await suggestions.accept(suggestion, {
        collection_id: destination,
        ...(title.trim() ? { title: title.trim() } : {}),
      });
      accepting = undefined;
      onaccepted?.(entry.id);
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isWorking = false;
    }
  }

  async function dismiss(suggestion: Suggestion) {
    if (isWorking) return;
    isWorking = true;
    failure = undefined;
    try {
      await suggestions.dismiss(suggestion);
      announcer.say(t('app.suggestions.dismissed_announced'));
      if (accepting === suggestion.id) accepting = undefined;
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<div class="proposal" data-ai>
  {#if asking?.outcome === 'following'}
    <AISuggestion heading={t('app.jumble.suggested')} state="pending" pendingLabel={t('app.suggestions.pending')} />
  {:else if asking?.outcome === 'gave_up'}
    <p class="note">{t('app.suggestions.gave_up')}</p>
  {/if}

  {#if listingFailure}
    <p class="failure" role="alert">{listingFailure.message}</p>
  {:else if listing.state.status === 'loading' || listing.state.status === 'idle'}
    <div aria-busy="true"><Skeleton lines={1} /></div>
  {/if}

  {#each open as suggestion (suggestion.id)}
    {@const shape = shapeOf(suggestion, undefined)}
    {@const proposedTitle = shape.shape === 'fields' ? shape.proposals.find((p) => p.field === 'title')?.proposed : undefined}
    <AISuggestion
      heading={t('app.jumble.suggested')}
      provenance={provenanceOf(suggestion)}
      provenanceLabel={t('app.suggestions.provenance')}
    >
      {#if shape.shape === 'fields'}
        <dl class="fields">
          {#each shape.proposals as proposal (proposal.field)}
            <dt>{t(fieldCode(proposal.field))}</dt>
            <dd>{proposal.field === 'due_date' ? formatDue(proposal.proposed, messages.locale, actor.zone, { allDay: true }) : proposal.proposed}</dd>
          {/each}
        </dl>
        {#if shape.subtasks.length > 0}
          <p class="label">{t('app.jumble.suggested_subtasks')}</p>
          <ul class="subtasks">
            {#each shape.subtasks as subtask, index (index)}
              <li>{subtask}</li>
            {/each}
          </ul>
        {/if}
      {:else}
        <p class="note">{t('app.suggestions.unknown_hint')}</p>
      {/if}

      {#if accepting === suggestion.id}
        <div class="form">
          <Select
            label={t('app.jumble.destination')}
            hint={t('app.jumble.destination_hint')}
            bind:value={destination}
            placeholder={t('app.jumble.choose_destination')}
            options={destinations}
          />
          <Input label={t('app.jumble.entry_title')} hint={t('app.jumble.entry_title_hint')} bind:value={title} />
        </div>
      {/if}
      {#if failure}
        <p class="failure" role="alert">{failure.message}</p>
      {/if}

      {#snippet actions()}
        {#if accepting === suggestion.id}
          <Button
            tone="primary"
            isBusy={isWorking}
            busyLabel={t('app.jumble.converting')}
            disabledReason={destination ? undefined : t('app.jumble.choose_destination')}
            onclick={() => void accept(suggestion)}
          >
            {t('app.jumble.make_it')}
          </Button>
          <Button tone="subtle" onclick={() => (accepting = undefined)}>{t('app.jumble.cancel')}</Button>
        {:else}
          <Button tone="primary" onclick={() => startAccepting(suggestion, proposedTitle)}>
            {t('app.jumble.accept_suggestion')}
          </Button>
          <Button tone="subtle" isBusy={isWorking} busyLabel={t('app.suggestions.dismissing')} onclick={() => void dismiss(suggestion)}>
            {t('app.suggestions.dismiss')}
          </Button>
        {/if}
      {/snippet}
    </AISuggestion>
  {/each}
</div>

<style>
  .proposal { display: flex; flex-direction: column; gap: var(--sp-100); }
  .note { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
  .fields { display: grid; grid-template-columns: max-content 1fr; gap: var(--sp-050) var(--sp-150); margin: 0; }
  .fields dt { color: var(--text-subtle); font-size: var(--fs-075); }
  .fields dd { margin: 0; min-width: 0; overflow-wrap: anywhere; white-space: pre-wrap; }
  .label { margin: var(--sp-100) 0 0; color: var(--text-subtle); font-size: var(--fs-075); }
  .subtasks { margin: 0; padding-inline-start: var(--sp-300); }
  .subtasks li { overflow-wrap: anywhere; }
  .form { display: flex; flex-direction: column; gap: var(--sp-150); margin-block-start: var(--sp-100); }
</style>
