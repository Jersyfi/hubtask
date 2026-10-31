<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The satisfaction question for a requester without an account, opened from the resolution
  // mail without signing in (UC-SVC-07 check 6). The score chosen in the mail is preselected but
  // not sent: a mail scanner that opens every link would otherwise rate the request five times.
  // The answer signs nobody in, and words for the team go by answering the mail.

  import { Banner, Button, Radio, Stack } from '@hubtask/design-system/components';

  import { engine } from '../lib/data/engine.ts';
  import { ratingLinkOf, ratingPath } from '../lib/data/satisfaction.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import SignInCard from '../lib/signin/SignInCard.svelte';

  const link = ratingLinkOf(globalThis.location?.hash ?? '');
  let score = $state(link.score === undefined ? '' : String(link.score));
  let isSending = $state(false);
  let isSent = $state(false);
  let refusal = $state<string | undefined>(undefined);

  async function send(event: SubmitEvent) {
    event.preventDefault();
    if (score === '') return;
    isSending = true;
    refusal = undefined;
    try {
      await engine.mutate<unknown>('POST', ratingPath(link.token), { score: Number(score) });
      isSent = true;
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isSending = false;
    }
  }
</script>

<SignInCard title={t('app.portal.rating')}>
  {#if isSent}
    <Banner tone="success" title={t('app.rate.sent')}>{t('app.rate.sent_body')}</Banner>
  {:else if link.token === ''}
    <Banner tone="warning">{t('satisfaction.link_not_found')}</Banner>
  {:else}
    <form class="rate" onsubmit={(event) => void send(event)} aria-label={t('app.portal.rating')}>
      <Stack gap="150">
        <Radio label={t('app.portal.rating_question')} name="link-score" bind:value={score}
          options={[5, 4, 3, 2, 1].map((n) => ({ value: String(n), label: t(`app.portal.score.${n}`) }))} />
        <p class="hint">{t('app.rate.hint')}</p>
        {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
        <Button type="submit" tone="primary" icon="send" isFull isBusy={isSending} busyLabel={t('app.portal.sending')}
          disabledReason={score === '' ? t('app.portal.score_first') : undefined}>
          {t('app.portal.rate')}
        </Button>
      </Stack>
    </form>
  {/if}
</SignInCard>

<style>
  .rate { margin: 0; }
  .hint { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
