<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What AI has proposed about this entry, and the menu that asks for more (F5-02).
  //
  // One strip for every kind, rendered through one component: each open suggestion is an
  // `AISuggestion` whose payload is drawn by the shape it carries - the fields beside the entry's
  // own, the labels as chips and the column by name, the breakdown as a list, the look-alikes as
  // links with nothing to accept (K-04). Nothing AI-shaped is built twice.
  //
  // **Absent when AI is off.** `ai_suggestions: false` in the manifest means this component is not
  // rendered at all - `ItemView` decides, and `offersFor` answers no operation either way - rather
  // than a gate with a reason: a gate explains a capability to somebody who expected it, and an
  // installation with AI switched off has a product that never mentioned it (decision 4).
  //
  // **A stale proposal is marked before the server refuses it.** `isStale` reads what the client
  // holds; the strip offers the ask and the dismissal, never the apply (voice-and-tone.md §7.3).
  // Where the two disagree, the server's `suggestions.stale` is rendered like any refusal.

  import { untrack } from 'svelte';

  import { AISuggestion, Button, LabelChip, Menu, Skeleton } from '@hubtask/design-system/components';
  import type { CommentPage, Suggestion, SuggestionPage, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { buckets } from '../data/buckets.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { commentsPath } from '../data/comments.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { suggestions, suggestionsPath } from '../data/suggestions.svelte.ts';
  import {
    acceptCodeOf,
    headingCodeOf,
    isStale,
    offersFor,
    operationOf,
    shapeOf,
    type Operation,
  } from '../data/suggestions.ts';
  import { formatDateTime, formatDue } from '../i18n/datetime.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    item: WorkItem;
  }

  const { item }: Props = $props();

  // Read once per entry, the way ItemView reads the entry: the view is keyed on the id.
  const listing = resource<SuggestionPage>(untrack(() => suggestionsPath(item.id)));
  // The thread is read by CommentPanel too, and the engine holds one state per path, so this is
  // the same read - what the menu needs to know is whether there is a discussion at all.
  const thread = resource<CommentPage>(untrack(() => commentsPath(item.id)));

  $effect(() => untrack(() => labels.open(item.collection_id)));
  $effect(() => untrack(() => buckets.open(item.collection_id)));
  $effect(() => () => suggestions.forget(item.id));

  const hasComments = $derived(thread.state.status === 'ready' && (thread.state.data.data ?? []).length > 0);
  const offers = $derived(offersFor(manifest.value?.features, item, hasComments));

  const open = $derived(
    listing.state.status === 'ready'
      ? (listing.state.data.items ?? []).filter((suggestion) => suggestion.status === 'PROPOSED')
      : [],
  );
  const listingFailure = $derived(
    listing.state.status === 'failed' ? renderProblem(listing.state.error, messages) : undefined,
  );

  const asking = $derived(suggestions.askingOf(item.id));

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

  /** The code each operation's menu item reads. */
  const OPERATION_CODE: Record<Operation, string> = {
    'suggest-fields': 'app.suggestions.op_suggest_fields',
    summarize: 'app.suggestions.op_summarize',
    classify: 'app.suggestions.op_classify',
    decompose: 'app.suggestions.op_decompose',
    'summarize-thread': 'app.suggestions.op_summarize_thread',
    duplicates: 'app.suggestions.op_duplicates',
  };

  const menuItems = $derived(
    offers.map((offer) => ({
      id: offer.operation,
      label: t(OPERATION_CODE[offer.operation]),
      disabledReason: offer.disabledCode ? t(offer.disabledCode) : undefined,
    })),
  );

  let askFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** The suggestion being decided, so that the two buttons of one strip say so and the others stay usable. */
  let deciding = $state<{ id: string; how: 'accept' | 'dismiss' } | undefined>(undefined);
  let decideFailure = $state<{ id: string; problem: ReturnType<typeof renderProblem> } | undefined>(undefined);

  async function ask(operation: Operation) {
    askFailure = undefined;
    try {
      await suggestions.ask(item.id, operation);
    } catch (error) {
      // A `503 ai.unavailable` is the one sentence the catalogue gives it, through the same
      // renderer as every other refusal - never a sentence written here.
      askFailure = renderProblem(error as never, messages);
    }
  }

  async function decide(suggestion: Suggestion, how: 'accept' | 'dismiss') {
    if (deciding) return;
    deciding = { id: suggestion.id, how };
    decideFailure = undefined;
    try {
      if (how === 'accept') await suggestions.accept(suggestion);
      else await suggestions.dismiss(suggestion);
      announcer.say(t(how === 'accept' ? 'app.suggestions.accepted_announced' : 'app.suggestions.dismissed_announced'));
    } catch (error) {
      decideFailure = { id: suggestion.id, problem: renderProblem(error as never, messages) };
    } finally {
      deciding = undefined;
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

  function fieldCode(field: 'title' | 'notes' | 'due_date'): string {
    return field === 'title' ? 'app.suggestions.field_title' : field === 'notes' ? 'app.suggestions.field_notes' : 'app.suggestions.field_due_date';
  }

  /** A date as the reader reads one. A proposed date is a day; the entry's own is a moment in its zone. */
  function dateOf(value: string, isProposed: boolean): string {
    if (isProposed) return formatDue(value, messages.locale, item.due_time_zone ?? actor.zone, { allDay: true });
    return formatDue(value, messages.locale, item.due_time_zone ?? actor.zone, { allDay: item.due_date_only });
  }

  const labelNamed = (id: string) => labels.of(item.collection_id).find((label) => label.id === id);
  const bucketNamed = (id: string) => buckets.of(item.collection_id).find((bucket) => bucket.id === id);
  const fieldNamed = (key: string) => customFields.of(item.collection_id).find((definition) => definition.key === key || definition.id === key);
</script>

<div class="strip" data-ai>
  <div class="ask">
    <Menu label={t('app.suggestions.ask_menu', { title: item.title })} items={menuItems} onselect={(id) => ask(id as Operation)}>
      {#snippet trigger(props)}
        <Button size="sm" tone="secondary" icon="sparkles" {...props}>{t('app.suggestions.ask')}</Button>
      {/snippet}
    </Menu>
    {#if asking?.outcome === 'gave_up'}
      <p class="note">{t('app.suggestions.gave_up')}</p>
    {:else if asking?.outcome === 'nothing_near'}
      <p class="note">{t('app.suggestions.nothing_near')}</p>
    {/if}
    {#if askFailure}
      <p class="failure">{askFailure.message}</p>
    {/if}
  </div>

  {#if asking?.outcome === 'following'}
    <AISuggestion heading={t(OPERATION_CODE[asking.operation as Operation])} state="pending" pendingLabel={t('app.suggestions.pending')} />
  {/if}

  {#if listingFailure}
    <p class="failure">{listingFailure.message}</p>
  {:else if listing.state.status === 'loading' || listing.state.status === 'idle'}
    <div aria-busy="true"><Skeleton lines={2} /></div>
  {:else if open.length === 0 && asking?.outcome !== 'following'}
    <p class="note">{t('app.suggestions.none')}</p>
  {/if}

  {#each open as suggestion (suggestion.id)}
    {@const shape = shapeOf(suggestion, item)}
    {@const stale = isStale(suggestion, item)}
    {@const accept = acceptCodeOf(shape)}
    {@const busy = deciding?.id === suggestion.id ? deciding.how : undefined}
    <AISuggestion
      heading={t(headingCodeOf(shape))}
      state={stale ? 'stale' : 'open'}
      staleNote={t('app.suggestions.stale')}
      provenance={provenanceOf(suggestion)}
      provenanceLabel={t('app.suggestions.provenance')}
    >
      {#if shape.shape === 'fields'}
        <dl class="fields">
          {#each shape.proposals as proposal (proposal.field)}
            <dt>{t(fieldCode(proposal.field))}</dt>
            <dd>
              <span class="proposed">{proposal.field === 'due_date' ? dateOf(proposal.proposed, true) : proposal.proposed}</span>
              <span class="current">
                {proposal.current === undefined
                  ? t('app.suggestions.now_empty')
                  : t('app.suggestions.now', { value: proposal.field === 'due_date' ? dateOf(proposal.current, false) : proposal.current })}
              </span>
            </dd>
          {/each}
        </dl>
      {:else if shape.shape === 'classification'}
        <div class="chips">
          {#each shape.labelIds as id (id)}
            {@const label = labelNamed(id)}
            {#if label}
              <LabelChip name={label.name} colorToken={label.color_token} description={label.description} />
            {:else}
              <span class="current">{t('app.suggestions.label_unknown')}</span>
            {/if}
          {/each}
        </div>
        {#if shape.bucketId}
          {@const bucket = bucketNamed(shape.bucketId)}
          <p class="line">{bucket ? t('app.suggestions.bucket', { name: bucket.name }) : t('app.suggestions.bucket_unknown')}</p>
        {/if}
        {#each Object.entries(shape.customFields) as [key, value] (key)}
          <p class="line">{t('app.suggestions.custom_field', { name: fieldNamed(key)?.key ?? key, value: String(value) })}</p>
        {/each}
      {:else if shape.shape === 'breakdown'}
        <ul class="tree">
          {#each shape.nodes as node, index (index)}
            <li data-depth={Math.min(node.depth, 4)}>
              <span class="type">{node.type}</span>
              <span class="proposed">{node.title}</span>
              {#if node.notes}<span class="current">{node.notes}</span>{/if}
            </li>
          {/each}
        </ul>
      {:else if shape.shape === 'duplicates'}
        <ul class="tree">
          {#each shape.neighbours as neighbour (neighbour.itemId)}
            <li>
              <a href={`/items/${neighbour.itemId}`}>{neighbour.itemId}</a>
              <span class="current">{t('app.suggestions.similarity', { percent: Math.round(neighbour.similarity * 100) })}</span>
            </li>
          {/each}
        </ul>
        <p class="line">{t('app.suggestions.duplicates_hint')}</p>
      {:else}
        <p class="line">{t('app.suggestions.unknown_hint')}</p>
      {/if}
      {#if decideFailure?.id === suggestion.id}
        <p class="failure">{decideFailure.problem.message}</p>
      {/if}

      {#snippet actions()}
        {#if stale}
          <Button tone="secondary" onclick={() => ask(operationOf(suggestion, shape))}>{t('app.suggestions.ask_again')}</Button>
        {:else if accept}
          <Button
            tone="primary"
            isBusy={busy === 'accept'}
            busyLabel={t('app.suggestions.deciding')}
            onclick={() => decide(suggestion, 'accept')}
          >
            {t(accept)}
          </Button>
        {/if}
        <Button
          tone="subtle"
          isBusy={busy === 'dismiss'}
          busyLabel={t('app.suggestions.dismissing')}
          onclick={() => decide(suggestion, 'dismiss')}
        >
          {t('app.suggestions.dismiss')}
        </Button>
      {/snippet}
    </AISuggestion>
  {/each}
</div>

<style>
  .strip { display: flex; flex-direction: column; gap: var(--sp-150); }
  .ask { display: flex; flex-direction: column; gap: var(--sp-050); align-items: start; }
  .note { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
  .line { margin: 0; overflow-wrap: anywhere; }

  .fields { display: grid; grid-template-columns: max-content 1fr; gap: var(--sp-050) var(--sp-150); margin: 0; }
  .fields dt { color: var(--text-subtle); font-size: var(--fs-075); }
  .fields dd { margin: 0; display: flex; flex-direction: column; gap: var(--sp-025); min-width: 0; }
  .proposed { overflow-wrap: anywhere; white-space: pre-wrap; }
  .current { color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }

  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); }

  .tree { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-050); }
  .tree li { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: baseline; overflow-wrap: anywhere; }
  .tree li[data-depth='1'] { padding-inline-start: var(--sp-300); }
  .tree li[data-depth='2'] { padding-inline-start: var(--sp-600); }
  .tree li[data-depth='3'] { padding-inline-start: var(--sp-800); }
  .tree li[data-depth='4'] { padding-inline-start: var(--sp-1000); }
  .type { flex: none; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
