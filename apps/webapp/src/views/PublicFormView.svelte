<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection's public form, opened by its link without signing in (UC-SVC-13). The token is in
  // the fragment and stays there. The kinds of request the form takes are asked like the portal's;
  // on *Send* the browser first does the small proof of work the server handed out - no
  // third-party CAPTCHA -, then sends once. The answer is the same sentence whoever the address
  // belongs to, and an unknown or dead link says only that there is no form here.

  import { untrack } from 'svelte';
  import { Banner, Button, Input, Radio, Skeleton, Stack, Textarea } from '@hubtask/design-system/components';
  import type { PublicFormView } from '@hubtask/sync-engine';

  import { formTokenOf, submissionOf } from '../lib/data/publicforms.ts';
  import { openFormPath, publicForms } from '../lib/data/publicforms.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { solve } from '../lib/forms/work.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import Question from '../lib/portal/Question.svelte';
  import { answersOf, askable } from '../lib/portal/portal.ts';
  import { renderProblem } from '../lib/problem.ts';
  import SignInCard from '../lib/signin/SignInCard.svelte';

  const token = formTokenOf(globalThis.location?.hash ?? '');
  const form = token === '' ? undefined : untrack(() => resource<PublicFormView>(openFormPath(token)));
  const view = $derived(form?.state.status === 'ready' ? form.state.data : undefined);
  const gone = $derived(token === '' || form?.state.status === 'failed');
  const kinds = $derived(view?.data ?? []);

  let kind = $state('');
  let title = $state('');
  let notes = $state('');
  let name = $state('');
  let email = $state('');
  let answers = $state<Record<string, unknown>>({});
  let isSending = $state(false);
  let isSent = $state(false);
  let refusal = $state<string | undefined>(undefined);

  $effect(() => {
    if (kind === '' && kinds.length === 1) kind = kinds[0]?.key ?? '';
  });
  const chosen = $derived(kinds.find((each) => each.key === kind));
  const questions = $derived(chosen ? askable(chosen.questions) : []);
  const fieldOf = (key: string) => chosen?.form.fields?.find((field) => field.key === `custom_fields.${key}`);

  async function send(event: SubmitEvent) {
    event.preventDefault();
    if (!chosen || !view) return;
    isSending = true;
    refusal = undefined;
    try {
      const solution = await solve(view.challenge.value, view.challenge.bits);
      await publicForms.submit(token, {
        ...submissionOf({ entryType: chosen.key, title, notes, name, email, answers: answersOf(questions, answers) }),
        challenge: view.challenge.value, solution,
      });
      isSent = true;
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isSending = false;
    }
  }
</script>

<SignInCard title={t('app.public_form.page_title')}>
  {#if isSent}
    <Banner tone="success" title={t('app.public_form.sent')}>{t('app.public_form.sent_body')}</Banner>
  {:else if gone}
    <Banner tone="warning">{t('app.public_form.gone')}</Banner>
  {:else if !view}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else}
    <form class="send" onsubmit={(event) => void send(event)} aria-label={t('app.public_form.page_title')}>
      <Stack gap="150">
        {#if kinds.length > 1}
          <Radio label={t('app.portal.kind')} name="public-kind" bind:value={kind}
            options={kinds.map((each) => ({ value: each.key, label: each.name || each.key }))} />
        {/if}
        <Input label={t('app.portal.request_title')} hint={t('app.portal.request_title_hint')} isRequired maxlength={500} bind:value={title} />
        <Textarea label={t('app.portal.request_notes')} rows={5} bind:value={notes} />
        {#each questions as question (question.key)}
          <Question {question} help={fieldOf(question.key)?.help} isRequired={fieldOf(question.key)?.required ?? false}
            bind:value={answers[question.key]} />
        {/each}
        <Input label={t('app.public_form.name')} maxlength={200} autocomplete="name" bind:value={name} />
        <Input label={t('app.public_form.email')} hint={t('app.public_form.email_hint')} type="email" maxlength={320}
          autocomplete="email" isRequired={name.trim() !== ''} bind:value={email} />
        {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
        <Button type="submit" tone="primary" icon="send" isFull isBusy={isSending} busyLabel={t('app.public_form.sending')}
          disabledReason={!chosen ? t('app.portal.kind_first') : title.trim() === '' ? t('app.portal.title_first') : undefined}>
          {t('app.public_form.send')}
        </Button>
      </Stack>
    </form>
  {/if}
</SignInCard>

<style>
  .send { margin: 0; }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
