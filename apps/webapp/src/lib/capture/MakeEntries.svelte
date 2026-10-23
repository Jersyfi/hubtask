<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Make entries* in the capture field (UC-AI-09): pasted text of more than one line, sent to the
  // workspace's AI, comes back as a proposal of entries for the collection the field points at -
  // each with only the fields that collection carries, and a mark where an entry of that title
  // may already be there. *Take all*, *Take* on one entry, and *Dismiss* are one press each;
  // nothing is created before. What was taken is undone the way every capture is.
  //
  // **Offered, never pushed.** The field mounts this only where it is offered (`entries.ts`), and
  // everything it draws is marked as AI, so an installation without AI has none of it.

  import { untrack } from 'svelte';

  import { AISuggestion, Button } from '@hubtask/design-system/components';
  import type { Suggestion, SuggestionPage } from '@hubtask/sync-engine';

  import { dayOf, entriesOf, entriesProposalSince, offersMakeEntries } from './entries.ts';
  import { announcer } from '../announce.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { suggestions, suggestionsPath } from '../data/suggestions.svelte.ts';
  import { isPending, noteOf } from '../data/suggestions.ts';
  import Lookalikes from '../entries/Lookalikes.svelte';
  import { offerUndo } from '../entries/undo.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem, renderRefusal } from '../problem.ts';

  interface Props {
    /** The person asking: the proposal is about their own account, and only they read it. */
    accountId: string;
    /** The collection the field points at, where the entries would go. */
    collection: { readonly id: string; readonly name: string };
    /** The words in the field. */
    text: string;
    /** Everything was taken: the field empties and closes, as after a capture. */
    ondone: () => void;
  }

  const { accountId, collection, text, ondone }: Props = $props();

  const listing = resource<SuggestionPage>(untrack(() => suggestionsPath(accountId, 'ACCOUNT')));

  const asking = $derived.by(() => {
    const now = suggestions.askingOf(accountId);
    return now?.operation === 'entries' ? now : undefined;
  });
  const note = $derived(noteOf(asking));
  const offered = $derived(offersMakeEntries(text, { aiOn: manifest.value?.features?.ai_suggestions === true, intoCollection: true }));

  /** The proposal on show: the one the last ask produced, as the listing holds it now. */
  let shownId = $state<string | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let busy = $state<string | undefined>(undefined);

  const items = $derived(listing.state.status === 'ready' ? (listing.state.data.items ?? []) : []);
  const shown = $derived(shownId ? items.find((each) => each.id === shownId && each.status === 'PROPOSED') : undefined);
  const entries = $derived(shown ? entriesOf(shown) : []);

  // The ask arrived: the newest proposal for this collection since it is the one shown.
  $effect(() => {
    if (!asking || asking.outcome !== 'arrived') return;
    const found = entriesProposalSince(items, asking.askedAt, collection.id);
    if (!found || untrack(() => shownId) === found.id) return;
    shownId = found.id;
    announcer.say(t('app.suggestions.arrived_announced'));
  });

  // A note the ask ended with is said once, where nobody had focus.
  let saidFor = $state<string | undefined>(undefined);
  $effect(() => {
    if (!asking || isPending(asking) || asking.outcome === 'arrived' || !note) return;
    const key = `${asking.askedAt} ${asking.outcome}`;
    if (saidFor === key) return;
    saidFor = key;
    announcer.say(renderRefusal(note, messages));
  });

  $effect(() => () => suggestions.forget(accountId));

  async function ask(): Promise<void> {
    failure = undefined;
    shownId = undefined;
    try {
      await suggestions.makeEntries(accountId, collection.id, text);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  async function take(proposal: Suggestion, keys?: readonly string[]): Promise<void> {
    failure = undefined;
    busy = keys?.[0] ?? 'all';
    try {
      const answer = await suggestions.take(proposal, keys);
      const created = answer.created_ids ?? [];
      if (created.length > 0) {
        offerUndo({ kind: 'captured', itemIds: created, place: collection.name }, undefined, {
          label: t('app.capture.open'),
          href: created.length === 1 ? `/items/${created[0]}` : `/collections/${collection.id}`,
        });
      }
      if (answer.status !== 'PROPOSED') {
        shownId = undefined;
        suggestions.forget(accountId);
        ondone();
      }
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      busy = undefined;
    }
  }

  async function dismiss(proposal: Suggestion): Promise<void> {
    failure = undefined;
    busy = 'dismiss';
    try {
      await suggestions.dismiss(proposal);
      shownId = undefined;
      suggestions.forget(accountId);
      announcer.say(t('app.suggestions.dismissed_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      busy = undefined;
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

  const detailsOf = (entry: (typeof entries)[number]): string =>
    [entry.amount, entry.dueDate ? dayOf(entry.dueDate, messages.locale) : undefined, entry.notes].filter((part) => part).join(' · ');
</script>

{#if offered || asking || shown}
  <div class="entries" data-ai>
    {#if !shown && !(asking && isPending(asking))}
      <Button size="sm" tone="secondary" icon="sparkles" onclick={() => void ask()}>{t('app.capture.make_entries')}</Button>
    {/if}

    {#if asking && isPending(asking)}
      <AISuggestion heading={t('app.capture.entries_heading')} state="pending" pendingLabel={t('app.suggestions.pending')}>
        {#snippet actions()}
          <Button tone="subtle" size="sm" onclick={() => void suggestions.stop(accountId, 'ACCOUNT')}>{t('app.ai.stop')}</Button>
        {/snippet}
      </AISuggestion>
    {:else if note && !shown}
      <p class={note.tone === 'refusal' ? 'failure' : 'note'} data-note={note.code}>{renderRefusal(note, messages)}</p>
    {/if}

    {#if shown}
      <AISuggestion heading={t('app.capture.entries_heading')} provenance={provenanceOf(shown)} provenanceLabel={t('app.suggestions.provenance')}>
        <p class="quiet">{t('app.capture.entries_into', { place: collection.name })}</p>
        <ul class="proposed" aria-label={t('app.capture.entries_heading')}>
          {#each entries as entry (entry.key)}
            <li>
              <div class="row">
                <span class="title">{entry.title}</span>
                <Button
                  size="sm"
                  tone="subtle"
                  icon="plus"
                  isBusy={busy === entry.key}
                  busyLabel={t('app.capture.submitting')}
                  aria-label={t('app.capture.take_one', { title: entry.title })}
                  onclick={() => void take(shown, [entry.key])}>{t('app.capture.take')}</Button
                >
              </div>
              {#if detailsOf(entry) !== ''}<p class="details">{detailsOf(entry)}</p>{/if}
              <Lookalikes title={entry.title} />
            </li>
          {/each}
        </ul>
        {#snippet actions()}
          <Button size="sm" tone="primary" isBusy={busy === 'all'} busyLabel={t('app.capture.submitting')} onclick={() => void take(shown)}>
            {t('app.capture.take_all')}
          </Button>
          <Button size="sm" tone="subtle" isBusy={busy === 'dismiss'} busyLabel={t('app.suggestions.dismissing')} onclick={() => void dismiss(shown)}>
            {t('app.suggestions.dismiss')}
          </Button>
        {/snippet}
      </AISuggestion>
    {/if}

    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  </div>
{/if}

<style>
  .entries { display: flex; flex-direction: column; gap: var(--sp-100); align-items: stretch; }
  .entries > :global(button) { align-self: flex-start; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .proposed { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .title { min-width: 0; overflow-wrap: anywhere; }
  .details { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); overflow-wrap: anywhere; }
  .note { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
