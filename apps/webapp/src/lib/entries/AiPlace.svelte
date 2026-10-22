<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What AI has to say at one place of an entry, under its subject (UC-AI-04 checks 6 and 9): the
  // questions asked from here while they run, each with *Stop*; how one ended where it ended
  // without a proposal; and the proposals that stand here.
  //
  // **Nothing at all where nothing is** - no heading, no "nothing suggested". An entry shows no
  // empty suggestion area; the ✦ beside the subject is how one is asked for.

  import { AISuggestion, Button } from '@hubtask/design-system/components';
  import type { Suggestion, WorkItem } from '@hubtask/sync-engine';

  import { ASK_CODE, askOf, asksOf, type Place } from '../data/aiplaces.ts';
  import { entryAsks } from '../data/entryasks.svelte.ts';
  import { isPending, noteOf } from '../data/suggestions.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderRefusal } from '../problem.ts';
  import AiProposal from './AiProposal.svelte';

  interface Props {
    item: WorkItem;
    place: Place;
    /** The proposals that stand at this place. */
    proposals: readonly Suggestion[];
    mayChange: boolean;
    /** Hide one on this page: a reader's *Close*. */
    onclose: (id: string) => void;
  }

  const { item, place, proposals, mayChange, onclose }: Props = $props();

  const questions = $derived(
    asksOf(place).flatMap((ask) => {
      const question = entryAsks.of(item.id, ask);
      return question ? [{ ask, question, note: noteOf(question) }] : [];
    }),
  );

  // A question that ends is a job finishing that nobody focused (4.1.3): said once, at the moment
  // it ends - a proposal arriving, or the note it ends with.
  const announced = new Set<string>();
  $effect(() => {
    for (const { question, note } of questions) {
      if (question.outcome === 'following' || question.outcome === 'slow') continue;
      const key = `${question.ask} ${question.askedAt} ${question.outcome}`;
      if (announced.has(key)) continue;
      announced.add(key);
      const said = question.outcome === 'arrived' ? t('app.suggestions.arrived_announced') : note ? renderRefusal(note, messages) : undefined;
      if (said) announcer.say(said);
    }
  });
</script>

{#each questions as { ask, question, note } (ask)}
  {#if isPending(question)}
    <AISuggestion heading={t(ASK_CODE[ask])} state="pending" pendingLabel={t('app.suggestions.pending')}>
      {#snippet actions()}
        <Button tone="subtle" size="sm" onclick={() => void entryAsks.stop(item.id, ask)}>{t('app.ai.stop')}</Button>
      {/snippet}
    </AISuggestion>
  {/if}
  {#if note}
    <p class={note.tone === 'refusal' ? 'failure' : 'note'} role={note.tone === 'refusal' ? 'alert' : undefined} data-note={note.code}>{renderRefusal(note, messages)}</p>
  {/if}
{/each}

{#each proposals as suggestion (suggestion.id)}
  {@const again = askOf(suggestion)}
  <!-- Where *Show the suggestion* moves focus to. -->
  <div class="anchor" id={`proposal-${suggestion.id}`} tabindex="-1">
    <AiProposal {item} {suggestion} {mayChange} onclose={() => onclose(suggestion.id)} onaskagain={again ? () => void entryAsks.ask(item.id, again) : undefined} />
  </div>
{/each}

<style>
  .note { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
  .anchor:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
