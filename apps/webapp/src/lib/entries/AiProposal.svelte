<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One standing proposal, under the subject it is about (UC-AI-04 check 6).
  //
  // Drawn by the shape its payload carries: the fields beside the entry's own, labels as chips and
  // the column by name, the breakdown as a list, the look-alikes by their titles - read from each
  // entry, where the permission is asked again (check 7). A stale one offers the ask and the
  // dismissal, never the apply (voice-and-tone.md §7.3).
  //
  // **Somebody who may only read the entry decides nothing** (P-05): accepting changes it and
  // dismissing clears a proposal from everybody's view, and the server refuses both to a reader. A
  // reader reads it, and *Close* hides it on this page alone - a reading aid that changes nothing
  // (check 8).

  import { AISuggestion, Button, LabelChip } from '@hubtask/design-system/components';
  import type { Suggestion, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { buckets } from '../data/buckets.svelte.ts';
  import { offerIn } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { suggestions } from '../data/suggestions.svelte.ts';
  import { acceptCodeOf, headingCodeOf, isStale, shapeOf } from '../data/suggestions.ts';
  import { formatDateTime, formatDue } from '../i18n/datetime.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import NeighbourLink from './NeighbourLink.svelte';

  interface Props {
    item: WorkItem;
    suggestion: Suggestion;
    /** The reader may change the entry: only then are accept and dismiss offered. */
    mayChange: boolean;
    /** Ask again, for a stale proposal. */
    onaskagain?: () => void;
    /** Hide it on this page: what a reader has instead of a decision. */
    onclose: () => void;
  }

  const { item, suggestion, mayChange, onaskagain, onclose }: Props = $props();

  const shape = $derived(shapeOf(suggestion, item));
  const stale = $derived(isStale(suggestion, item));
  const accept = $derived(acceptCodeOf(shape));

  let deciding = $state<'accept' | 'dismiss' | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);

  async function decide(how: 'accept' | 'dismiss') {
    if (deciding) return;
    deciding = how;
    failure = undefined;
    try {
      if (how === 'accept') await suggestions.accept(suggestion);
      else await suggestions.dismiss(suggestion);
      announcer.say(t(how === 'accept' ? 'app.suggestions.accepted_announced' : 'app.suggestions.dismissed_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      deciding = undefined;
    }
  }

  const provenance = $derived(
    t('app.suggestions.provenance_line', {
      model: suggestion.model,
      when: formatDateTime(suggestion.produced_at, messages.locale),
      prompt: suggestion.prompt_id,
      version: suggestion.prompt_version,
    }),
  );

  function fieldCode(field: 'title' | 'notes' | 'due_date'): string {
    return field === 'title' ? 'app.suggestions.field_title' : field === 'notes' ? 'app.suggestions.field_notes' : 'app.suggestions.field_due_date';
  }

  /** A date as the reader reads one. A proposed date is a day; the entry's own is a moment in its zone. */
  function dateOf(value: string, isProposed: boolean): string {
    return formatDue(value, messages.locale, item.due_time_zone ?? actor.zone, { allDay: isProposed || item.due_date_only });
  }

  const offer = $derived(offerIn(containers.find(item.collection_id)?.structure?.settings));
  const labelNamed = (id: string) => labels.of(item.collection_id).find((label) => label.id === id);
  const bucketNamed = (id: string) => buckets.of(item.collection_id).find((bucket) => bucket.id === id);
  const fieldNamed = (key: string) => customFields.of(item.collection_id).find((definition) => definition.key === key || definition.id === key);
</script>

<AISuggestion
  heading={t(headingCodeOf(shape))}
  state={stale ? 'stale' : 'open'}
  staleNote={t('app.suggestions.stale')}
  {provenance}
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
          <span class="type">{offer.level(node.type).label}</span>
          <span class="proposed">{node.title}</span>
          {#if node.notes}<span class="current">{node.notes}</span>{/if}
        </li>
      {/each}
    </ul>
  {:else if shape.shape === 'duplicates'}
    <ul class="tree">
      {#each shape.neighbours as neighbour (neighbour.itemId)}
        <li>
          <NeighbourLink itemId={neighbour.itemId} />
          <span class="current">{t('app.suggestions.similarity', { percent: Math.round(neighbour.similarity * 100) })}</span>
        </li>
      {/each}
    </ul>
    <p class="line">{t('app.suggestions.duplicates_hint')}</p>
  {:else}
    <p class="line">{t('app.suggestions.unknown_hint')}</p>
  {/if}
  {#if failure}
    <p class="failure" role="alert">{failure}</p>
  {/if}

  {#snippet actions()}
    {#if mayChange}
      {#if stale}
        {#if onaskagain}<Button tone="secondary" onclick={onaskagain}>{t('app.suggestions.ask_again')}</Button>{/if}
      {:else if accept}
        <Button tone="primary" isBusy={deciding === 'accept'} busyLabel={t('app.suggestions.deciding')} onclick={() => decide('accept')}>
          {t(accept)}
        </Button>
      {/if}
      <Button tone="subtle" isBusy={deciding === 'dismiss'} busyLabel={t('app.suggestions.dismissing')} onclick={() => decide('dismiss')}>
        {t('app.suggestions.dismiss')}
      </Button>
    {:else}
      <Button tone="subtle" onclick={onclose}>{t('app.ai.close')}</Button>
    {/if}
  {/snippet}
</AISuggestion>

<style>
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
