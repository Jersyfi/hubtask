<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How the collection stands, as AI summarised it (K-05, F5-04): what is open, what moved, what
  // is overdue. Rendered as an `AISuggestion` with no accept - a collection has nowhere to put a
  // status summary, so it is read and dismissed. Asked through `:summarize`, which answers `202`
  // with no body like every AI ask, and followed through the listing.
  //
  // The same listing carries a template drafted for the collection (P-11, F6-10): asked from the
  // templates dialog, it lands here because the collection is its target. That one is drawn as a
  // tree, read-only, the way a breakdown is, and accepting it is `CreateTemplate` - the verb says
  // so - after which the dialog's list shows it.

  import { untrack } from 'svelte';

  import { AISuggestion, Button } from '@hubtask/design-system/components';
  import type { Suggestion, SuggestionPage } from '@hubtask/sync-engine';

  import { resource } from '../data/resource.svelte.ts';
  import { suggestions, suggestionsPath } from '../data/suggestions.svelte.ts';
  import { acceptCodeOf, headingCodeOf, shapeOf } from '../data/suggestions.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    containerId: string;
  }

  const { containerId }: Props = $props();

  const listing = resource<SuggestionPage>(untrack(() => suggestionsPath(containerId, 'CONTAINER')));
  $effect(() => () => suggestions.forget(containerId));

  const open = $derived(
    listing.state.status === 'ready'
      ? (listing.state.data.items ?? []).filter((suggestion) => suggestion.status === 'PROPOSED')
      : [],
  );
  const asking = $derived(suggestions.askingOf(containerId));

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

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let dismissing = $state<string | undefined>(undefined);
  let accepting = $state<string | undefined>(undefined);

  async function ask() {
    failure = undefined;
    try {
      await suggestions.askContainer(containerId);
    } catch (error) {
      failure = renderProblem(error as never, messages);
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

<div class="summary" data-ai>
  <div>
    <Button
      size="sm"
      tone="secondary"
      icon="sparkles"
      disabledReason={asking?.outcome === 'following' ? t('app.suggestions.pending') : undefined}
      onclick={ask}
    >
      {t('app.workspace.summarise')}
    </Button>
  </div>
  {#if failure}
    <p class="failure" role="alert">{failure.message}</p>
  {/if}
  {#if asking?.outcome === 'following'}
    <AISuggestion
      heading={asking.operation === 'template' ? t('app.suggestions.kind_template') : t('app.workspace.summary')}
      state="pending"
      pendingLabel={t('app.suggestions.pending')}
    />
  {:else if asking?.outcome === 'gave_up'}
    <p class="note">{t('app.suggestions.gave_up')}</p>
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
          <!-- The draft is not the whole answer (issue 767): a step this collection's profile
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
          <!-- No accept: nothing accepts a collection's summary (K-05). Read, then dismissed. -->
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
