<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One kind of request, asked in a dialog of the page system's ladder (UC-SVC-05 check 2): the
  // words, the details and the kind's own questions, with the articles that may answer it already
  // beside them as the reader types (UC-DOC-13 check 2). Sent, the reader lands on the request.

  import { Dialog, Input, Stack, Textarea } from '@hubtask/design-system/components';
  import type { RequestCatalogue } from '@hubtask/sync-engine';

  import { portal } from '../data/portal.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import ArticleSuggestions from './ArticleSuggestions.svelte';
  import Question from './Question.svelte';
  import { answersOf, askable, deflectionText } from './portal.ts';

  type Kind = RequestCatalogue['data'][number];
  interface Props {
    collectionId: string;
    kind: Kind;
    /** What the reader typed into the portal's search before choosing: the request's first words. */
    words?: string;
    onclose: () => void;
    onnavigate?: (path: string) => void;
  }
  const { collectionId, kind, words = '', onclose, onnavigate }: Props = $props();

  let isOpen = $state(true);
  // svelte-ignore state_referenced_locally
  let title = $state(words);
  let notes = $state('');
  let answers = $state<Record<string, unknown>>({});
  let isRaising = $state(false);
  let refusal = $state<string | undefined>(undefined);
  let typed = $state('');

  const questions = $derived(askable(kind.questions));
  const fieldOf = (key: string) => kind.form.fields?.find((field) => field.key === `custom_fields.${key}`);

  // Articles are asked for the words once typing rests, not for every key.
  $effect(() => {
    const said = deflectionText(title, notes);
    const timer = setTimeout(() => (typed = said), 400);
    return () => clearTimeout(timer);
  });

  async function raise() {
    if (title.trim() === '') return;
    isRaising = true;
    refusal = undefined;
    try {
      const raised = await portal.raise(collectionId, {
        entry_type: kind.key, title: title.trim(), notes: notes.trim() === '' ? undefined : notes,
        custom_fields: answersOf(questions, answers),
      });
      isOpen = false;
      onnavigate?.(`/portal/requests/${raised.id}`);
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isRaising = false;
    }
  }
</script>

<Dialog bind:isOpen title={kind.name || kind.key} size="l" dismissLabel={t('app.portal.cancel')} onClose={onclose}
  primary={{ label: t('app.portal.raise'), onclick: () => void raise(), isBusy: isRaising, busyLabel: t('app.portal.raising'),
    disabledReason: title.trim() === '' ? t('app.portal.title_first') : undefined }}>
  <div class="ask" data-portal-ask data-wide={viewport.isCompact ? undefined : ''}>
    <Stack gap="150">
      {#if kind.description}<p class="quiet">{kind.description}</p>{/if}
      <Input label={t('app.portal.request_title')} hint={t('app.portal.request_title_hint')} isRequired maxlength={500} bind:value={title} />
      <Textarea label={t('app.portal.request_notes')} rows={5} bind:value={notes} />
      {#each questions as question (question.key)}
        <Question {question} help={fieldOf(question.key)?.help} isRequired={fieldOf(question.key)?.required ?? false}
          bind:value={answers[question.key]} />
      {/each}
      {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
    </Stack>
    <aside class="answers">
      {#key typed}
        <ArticleSuggestions {collectionId} text={typed} onopen={(pageId) => { isOpen = false; onnavigate?.(`/portal/${collectionId}/articles/${pageId}`); }} />
      {/key}
    </aside>
  </div>
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .ask { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); }
  .ask[data-wide] { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); }
  .answers { min-inline-size: 0; }
</style>
