<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What AI answered about a collection, in one of two places (ux-inventory.md §E4):
  //
  // * `summary` - how the collection stands: what is open, what moved, what is overdue. Asked by
  //   the ✦ at the collection's figure (`:summarize`, answered `202` and followed through the
  //   listing) and drawn under the head. Nothing on the server accepts it - a description says what
  //   a collection is *for*, not how its week went - so it is kept, where the collection's entries
  //   carry notes, as a new entry whose notes it is (`keepAs`), and then closed.
  // * `template` - a template drafted for the collection, asked and drawn in *Structure →
  //   Templates*: as a tree, read-only, the way a breakdown is; accepting it is `CreateTemplate`,
  //   after which the tab's list shows it.
  //
  // Both read the same listing, the collection's suggestions; each draws its own kind.

  import { untrack } from 'svelte';

  import { AISuggestion, Button } from '@hubtask/design-system/components';
  import type { Suggestion, SuggestionPage } from '@hubtask/sync-engine';

  import { resource } from '../data/resource.svelte.ts';
  import { suggestions, suggestionsPath } from '../data/suggestions.svelte.ts';
  import { acceptCodeOf, headingCodeOf, isPending, noteOf, shapeOf } from '../data/suggestions.ts';
  import { formatDateTime, formatDay } from '../i18n/datetime.ts';
  import { items } from '../data/items.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem, renderRefusal } from '../problem.ts';

  interface Props {
    containerId: string;
    /** Which of the collection's answers this place draws. */
    part: 'summary' | 'template';
    /**
     * The level a kept summary becomes an entry of: a top level of this collection that carries
     * notes, where the reader may add one. Absent, the summary is read and closed only.
     */
    keepAs?: string;
  }

  const { containerId, part, keepAs }: Props = $props();

  const listing = resource<SuggestionPage>(untrack(() => suggestionsPath(containerId, 'CONTAINER')));
  // The page's place forgets the collection's asks when the page goes; the tab closing does not.
  $effect(() => () => {
    if (part === 'summary') suggestions.forget(containerId);
  });

  const open = $derived(
    listing.state.status === 'ready'
      ? (listing.state.data.items ?? []).filter(
          (suggestion) => suggestion.status === 'PROPOSED' && (shapeOf(suggestion, undefined).shape === 'template') === (part === 'template'),
        )
      : [],
  );
  /** The ask running here: a summary's at the page, a template's in the tab. */
  const asking = $derived.by(() => {
    const now = suggestions.askingOf(containerId);
    return now && (now.operation === 'template') === (part === 'template') ? now : undefined;
  });

  const note = $derived(noteOf(asking));

  // A proposal arriving is a job finishing that nobody focused (4.1.3): said once, at the moment
  // the strip shows it - and every other change of an ask (slow, ended with a note, refused) is
  // said the same way, since the note it leaves is as silent as the proposal would have been.
  let announcedFor = $state<string | undefined>(undefined);
  $effect(() => {
    const now = asking;
    if (!now || now.outcome === 'following') return;
    const key = `${now.askedAt} ${now.outcome}`;
    if (announcedFor === key) return;
    announcedFor = key;
    const said = now.outcome === 'arrived' ? t('app.suggestions.arrived_announced') : note ? renderRefusal(note, messages) : undefined;
    if (said) announcer.say(said);
  });

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let dismissing = $state<string | undefined>(undefined);
  let accepting = $state<string | undefined>(undefined);

  let keeping = $state<string | undefined>(undefined);

  /**
   * Keeps a summary as a new entry's notes, then closes the suggestion: the answer now lives in
   * the collection, where it can be read, changed and found again.
   */
  async function keep(suggestion: Suggestion, text: string) {
    if (!keepAs) return;
    keeping = suggestion.id;
    failure = undefined;
    try {
      await items.create(
        {
          type: keepAs,
          collection_id: containerId,
          parent_id: null,
          title: t('app.workspace.summary_kept_title', { day: formatDay(suggestion.produced_at, messages.locale) }),
          notes: text,
        },
        crypto.randomUUID(),
      );
      await suggestions.dismiss(suggestion);
      announcer.say(t('app.workspace.summary_kept_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      keeping = undefined;
    }
  }

  async function accept(suggestion: Suggestion) {
    accepting = suggestion.id;
    failure = undefined;
    try {
      await suggestions.accept(suggestion);
      announcer.say(t('app.templates.kept_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      accepting = undefined;
    }
  }

  async function dismiss(suggestion: Suggestion) {
    dismissing = suggestion.id;
    failure = undefined;
    try {
      await suggestions.dismiss(suggestion);
      announcer.say(t('app.suggestions.dismissed_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      dismissing = undefined;
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

  /** The summary's text: the notes the payload proposes, which is the one thing a status summary is. */
  function summaryOf(suggestion: Suggestion): string | undefined {
    const shape = shapeOf(suggestion, undefined);
    if (shape.shape !== 'fields') return undefined;
    return shape.proposals.find((proposal) => proposal.field === 'notes')?.proposed;
  }
</script>

<div class="summary" data-ai={part}>
  {#if failure}
    <p class="failure" role="alert">{failure.message}</p>
  {/if}
  {#if asking && isPending(asking)}
    <AISuggestion
      heading={asking.operation === 'template' ? t('app.suggestions.kind_template') : t('app.workspace.summary')}
      state="pending"
      pendingLabel={t('app.suggestions.pending')}
    />
  {/if}
  {#if note}
    <p class="note" data-note={note.code}>{renderRefusal(note, messages)}</p>
  {/if}
  {#each open as suggestion (suggestion.id)}
    {@const shape = shapeOf(suggestion, undefined)}
    {#if shape.shape === 'template'}
      {@const accepts = acceptCodeOf(shape)}
      <AISuggestion
        heading={t(headingCodeOf(shape))}
        provenance={provenanceOf(suggestion)}
        provenanceLabel={t('app.suggestions.provenance')}
      >
        <!-- The draft, read-only: its name, what it is for, and its tree the way a breakdown is
             drawn. The root type heads the tree because the template's root takes its title at
             instantiation, so there is no title to show for it here. -->
        <p class="name">{shape.name}</p>
        {#if shape.description}<p class="text">{shape.description}</p>{/if}
        <ul class="tree">
          <li data-depth="0"><span class="type">{shape.rootType}</span><span class="proposed">{t('app.templates.root_of_draft')}</span></li>
          {#each shape.nodes as node, index (index)}
            <li data-depth={Math.min(node.depth + 1, 4)}>
              <span class="type">{node.type}</span>
              <span class="proposed">{node.title}</span>
              {#if node.notes}<span class="current">{node.notes}</span>{/if}
            </li>
          {/each}
        </ul>
        {#if shape.dropped > 0}
          <!-- The draft is not the whole answer: a step this collection's profile
               does not allow was left out, with everything under it, and the count says so. -->
          <p class="text">{t('app.suggestions.template_dropped', { count: shape.dropped })}</p>
        {/if}
        {#snippet actions()}
          {#if accepts}
            <Button
              tone="primary"
              isBusy={accepting === suggestion.id}
              busyLabel={t('app.suggestions.deciding')}
              onclick={() => void accept(suggestion)}
            >
              {t(accepts)}
            </Button>
          {/if}
          <Button
            tone="subtle"
            isBusy={dismissing === suggestion.id}
            busyLabel={t('app.suggestions.dismissing')}
            onclick={() => void dismiss(suggestion)}
          >
            {t('app.suggestions.dismiss')}
          </Button>
        {/snippet}
      </AISuggestion>
    {:else}
      {@const text = summaryOf(suggestion)}
      <AISuggestion
        heading={t('app.workspace.summary')}
        provenance={provenanceOf(suggestion)}
        provenanceLabel={t('app.suggestions.provenance')}
      >
        <p class="text">{text ?? t('app.suggestions.unknown_hint')}</p>
        {#snippet actions()}
          <!-- Nothing on the server accepts a collection's summary; kept, it is an entry's notes. -->
          {#if keepAs && text}
            <Button
              tone="primary"
              isBusy={keeping === suggestion.id}
              busyLabel={t('app.suggestions.deciding')}
              onclick={() => void keep(suggestion, text)}
            >
              {t('app.workspace.summary_keep')}
            </Button>
          {/if}
          <Button
            tone="subtle"
            isBusy={dismissing === suggestion.id}
            busyLabel={t('app.suggestions.dismissing')}
            onclick={() => void dismiss(suggestion)}
          >
            {t('app.suggestions.dismiss')}
          </Button>
        {/snippet}
      </AISuggestion>
    {/if}
  {/each}
</div>

<style>
  .summary { display: flex; flex-direction: column; gap: var(--sp-100); }
  .note { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
  .text { margin: 0; max-width: 72ch; white-space: pre-wrap; overflow-wrap: anywhere; }
  .name { margin: 0; font-weight: var(--fw-semibold); overflow-wrap: anywhere; }

  .tree { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-050); }
  .tree li { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: baseline; overflow-wrap: anywhere; }
  .tree li[data-depth='1'] { padding-inline-start: var(--sp-300); }
  .tree li[data-depth='2'] { padding-inline-start: var(--sp-600); }
  .tree li[data-depth='3'] { padding-inline-start: var(--sp-800); }
  .tree li[data-depth='4'] { padding-inline-start: var(--sp-1000); }
  .type { flex: none; color: var(--text-subtle); font-size: var(--fs-075); }
  .proposed { overflow-wrap: anywhere; white-space: pre-wrap; }
  .current { color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }
</style>
