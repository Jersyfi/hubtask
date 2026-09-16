<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entry read in another language (M-11, F5-06; i18n-l10n.md §7 line 7).
  //
  // A read, not a record: `:translate` answers now, stores nothing, and there is nothing to
  // accept - a person reads the answer and it is gone. So this holds the answer in component
  // state alone, renders it beside the original as an `AISuggestion` with only a dismissal, and
  // puts `lang` on the translated part from the language asked for, so that a screen reader
  // switches voice (design-system.md §10, 3.1.2). The list of languages is the installation's
  // `text_languages`, the person's own preselected; a tag outside it is fine - the provider
  // translates into languages the product does not render.

  import { AISuggestion, Button, Select } from '@hubtask/design-system/components';
  import type { AiTranslation, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { suggestions } from '../data/suggestions.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    item: WorkItem;
    /** The languages the installation indexes, from the manifest. */
    languages: readonly string[];
  }

  const { item, languages }: Props = $props();

  // The person's own language first: it is what they read anything in. Where the manifest does
  // not list it, it is still the default - the provider does not need the installation to index a
  // language to translate into it.
  const own = $derived(actor.locale ?? messages.locale);
  let target = $state('');
  $effect(() => {
    if (target === '') target = own;
  });
  const options = $derived(
    [...new Set([own, ...languages])].map((tag) => ({ value: tag, label: tag })),
  );

  let answer = $state<AiTranslation | undefined>(undefined);
  let isAsking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  async function ask() {
    if (isAsking) return;
    isAsking = true;
    failure = undefined;
    try {
      answer = await suggestions.translate(item.id, target);
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isAsking = false;
    }
  }

  const provenance = $derived(
    answer
      ? t('app.suggestions.provenance_line', {
          model: answer.model,
          when: formatDateTime(answer.produced_at, messages.locale),
          prompt: answer.prompt_id,
          version: answer.prompt_version,
        })
      : '',
  );
</script>

<div class="translate">
  <div class="ask">
    <Select label={t('app.translate.language')} size="sm" bind:value={target} {options} />
    <Button size="sm" tone="secondary" icon="sparkles" isBusy={isAsking} busyLabel={t('app.translate.asking')} onclick={ask}>
      {t('app.translate.read_in')}
    </Button>
  </div>
  {#if failure}
    <p class="failure">{failure.message}</p>
  {/if}
  {#if answer}
    <AISuggestion
      heading={t('app.translate.heading', { language: answer.target_locale })}
      {provenance}
      provenanceLabel={t('app.suggestions.provenance')}
    >
      <!-- The translated part carries the language it is in; the original keeps its own. -->
      <div class="answer" lang={answer.target_locale}>
        <p class="title">{answer.title}</p>
        {#if answer.notes}<p class="notes">{answer.notes}</p>{/if}
      </div>
      <p class="note">{t('app.translate.display_only')}</p>
      {#snippet actions()}
        <Button tone="subtle" onclick={() => (answer = undefined)}>{t('app.suggestions.dismiss')}</Button>
      {/snippet}
    </AISuggestion>
  {/if}
</div>

<style>
  .translate { display: flex; flex-direction: column; gap: var(--sp-100); }
  .ask { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
  .answer { display: flex; flex-direction: column; gap: var(--sp-050); }
  .title { margin: 0; font-weight: var(--fw-medium); overflow-wrap: anywhere; }
  .notes { margin: 0; max-width: 64ch; color: var(--text-secondary); white-space: pre-wrap; overflow-wrap: anywhere; }
  .note { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
