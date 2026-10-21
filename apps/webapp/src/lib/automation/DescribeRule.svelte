<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Describe it* (UC-AUT-10): a sentence in, a rule drafted on the canvas, switched off.
  //
  // This component owns the asking and the proposal around the draft - the dialog that takes the
  // sentence, the pending card while the job runs, and the strip that names the draft as a
  // proposal, says what could not be drafted and where it came from, and offers to dismiss it. The
  // canvas, the probe and the save are the editor's: the draft is handed to it once (`ondraft`),
  // and the editor's *Save* is the acceptance.
  //
  // Only the person who asked reads the draft (the listing is about their own account), so the
  // newest proposed rule made after the ask is the one this ask produced.
  import { untrack } from 'svelte';

  import { AISuggestion, Button, Dialog, Inline, Stack, Textarea } from '@hubtask/design-system/components';
  import type { Suggestion, SuggestionPage } from '@hubtask/sync-engine';

  import { notDraftedOf, type NotDrafted } from './model.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { suggestions, suggestionsPath } from '../data/suggestions.svelte.ts';
  import { isPending, noteOf, ruleDraftSince } from '../data/suggestions.ts';
  import { announcer } from '../announce.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem, renderRefusal } from '../problem.ts';

  interface Props {
    /** The person asking: their own listing is where the draft appears. */
    accountId: string;
    /** Where the rule would apply, as the editor holds it now. */
    scope: { type: string; id?: string };
    /** The draft the editor drew, or nothing. */
    drafted: Suggestion | undefined;
    /** A draft arrived: the editor draws it. */
    ondraft: (draft: Suggestion) => void;
    /** The draft was dismissed: the editor forgets it and keeps what is on the canvas. */
    ondismissed: () => void;
  }

  const { accountId, scope, drafted, ondraft, ondismissed }: Props = $props();

  const listing = resource<SuggestionPage>(untrack(() => suggestionsPath(accountId, 'ACCOUNT')));
  $effect(() => () => suggestions.forget(accountId));

  const asking = $derived(suggestions.askingOf(accountId));
  const note = $derived(noteOf(asking));

  // The draft this ask produced, handed to the editor once.
  let handed = $state<string | undefined>(undefined);
  $effect(() => {
    if (!asking || asking.outcome !== 'arrived' || listing.state.status !== 'ready') return;
    const draft = ruleDraftSince(listing.state.data.items ?? [], asking.askedAt);
    if (!draft || handed === draft.id) return;
    handed = draft.id;
    untrack(() => ondraft(draft));
    announcer.say(t('app.flow.drafted_announced'));
  });

  // Every other end of an ask is said once, where nothing else would say it.
  let announcedFor = $state<string | undefined>(undefined);
  $effect(() => {
    const now = asking;
    if (!now || now.outcome === 'following' || now.outcome === 'arrived') return;
    const key = `${now.askedAt} ${now.outcome}`;
    if (announcedFor === key) return;
    announcedFor = key;
    if (note) announcer.say(renderRefusal(note, messages));
  });

  let isOpen = $state(false);
  let wanted = $state('');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isAsking = $state(false);
  let isDismissing = $state(false);

  async function ask(): Promise<void> {
    failure = undefined;
    isAsking = true;
    try {
      await suggestions.describeRule(accountId, scope, wanted.trim());
      isOpen = false;
      wanted = '';
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isAsking = false;
    }
  }

  async function dismiss(): Promise<void> {
    if (!drafted) return;
    isDismissing = true;
    try {
      await suggestions.dismiss(drafted);
      announcer.say(t('app.suggestions.dismissed_announced'));
      ondismissed();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isDismissing = false;
    }
  }

  const parts = $derived<NotDrafted[]>(drafted ? notDraftedOf(drafted.payload ?? {}) : []);

  function saidOf(part: NotDrafted): string {
    if ('part' in part) return t('app.flow.drafted_quote', { part: part.part });
    return t(part.code, part.params ?? {});
  }

  function provenanceOf(suggestion: Suggestion): string {
    return t('app.suggestions.provenance_line', {
      model: suggestion.model,
      when: formatDateTime(suggestion.produced_at, messages.locale),
      prompt: suggestion.prompt_id,
      version: suggestion.prompt_version,
    });
  }
</script>

<div class="describe" data-ai>
  <Button
    size="sm"
    tone="secondary"
    icon="sparkles"
    disabledReason={isPending(asking) ? t('app.suggestions.pending') : undefined}
    onclick={() => {
      failure = undefined;
      isOpen = true;
    }}
  >
    {t('app.flow.describe')}
  </Button>

  {#if asking && isPending(asking)}
    <AISuggestion heading={t('app.flow.drafted_heading')} state="pending" pendingLabel={t('app.suggestions.pending')} />
  {:else if note && !drafted}
    <p class="note" data-note={note.code}>{renderRefusal(note, messages)}</p>
  {/if}

  {#if drafted}
    <AISuggestion
      heading={t('app.flow.drafted_heading')}
      provenance={provenanceOf(drafted)}
      provenanceLabel={t('app.suggestions.provenance')}
    >
      <p class="quiet">{t('app.flow.drafted_explains')}</p>
      {#if parts.length > 0}
        <p class="label">{t('app.flow.not_drafted')}</p>
        <ul class="parts">
          {#each parts as part, index (index)}<li>{saidOf(part)}</li>{/each}
        </ul>
      {/if}
      {#snippet actions()}
        <Button size="sm" tone="subtle" isBusy={isDismissing} busyLabel={t('app.workspace.saving')} onclick={dismiss}>
          {t('app.suggestions.dismiss')}
        </Button>
      {/snippet}
    </AISuggestion>
  {/if}
</div>

{#if isOpen}
  <Dialog bind:isOpen title={t('app.flow.describe_title')} dismissLabel={t('app.workspace.cancel')}>
    <Stack gap="150">
      <Textarea
        label={t('app.flow.describe_label')}
        hint={t('app.flow.describe_hint')}
        bind:value={wanted}
        rows={3}
        error={failure?.fields.get('/description')}
      />
      <p class="quiet">{t('app.flow.describe_note')}</p>
      {#if failure && !failure.fields.get('/description')}<p class="failure" role="alert">{failure.message}</p>{/if}
      <Inline gap="100">
        <Button tone="primary" icon="sparkles" isBusy={isAsking} busyLabel={t('app.suggestions.pending')} onclick={ask}>
          {t('app.flow.describe_submit')}
        </Button>
        <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .describe { display: flex; flex-direction: column; gap: var(--sp-100); align-items: flex-start; }

  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  .label { margin: var(--sp-100) 0 0; font-weight: var(--fw-semibold); }

  .parts { margin: var(--sp-050) 0 0; padding-inline-start: var(--sp-200); }

  .note { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
