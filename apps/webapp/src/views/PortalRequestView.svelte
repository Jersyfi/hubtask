<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One of the reader's own requests (UC-SVC-05 checks 4 and 5, UC-SVC-07 check 1): where it
  // stands, how its promised times stand, the team's public replies - never an internal note -,
  // an answer of their own, and once it is resolved how satisfied they were.

  import { untrack } from 'svelte';
  import { Badge, Button, ErrorState, PageHeader, Radio, Skeleton, Stack, Textarea } from '@hubtask/design-system/components';
  import type { PortalRequestDetail } from '@hubtask/sync-engine';

  import { myRequestPath, portal } from '../lib/data/portal.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { goalTone } from '../lib/portal/portal.ts';
  import { renderProblem } from '../lib/problem.ts';

  const { id, onnavigate }: { id: string; onnavigate?: (path: string) => void } = $props();

  const request = untrack(() => resource<PortalRequestDetail>(myRequestPath(id)));
  const detail = $derived(request.state.status === 'ready' ? request.state.data : undefined);
  const failure = $derived(request.state.status === 'failed' ? renderProblem(request.state.error, messages) : undefined);
  $effect(() => page.entitle(detail?.title ?? t('app.portal.request')));

  const at = (iso: string | null) => (iso ? formatDateTime(iso, messages.locale) : '');
  const now = new Date();

  let answer = $state('');
  let isAnswering = $state(false);
  let score = $state('');
  let ratingComment = $state('');
  let isRating = $state(false);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    if (detail?.satisfaction && score === '') score = String(detail.satisfaction.score);
  });

  async function send(event: SubmitEvent) {
    event.preventDefault();
    if (!detail || answer.trim() === '') return;
    isAnswering = true;
    refusal = undefined;
    try {
      await portal.answer(detail, answer.trim());
      answer = '';
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isAnswering = false;
    }
  }

  async function rate(event: SubmitEvent) {
    event.preventDefault();
    if (!detail || score === '') return;
    isRating = true;
    refusal = undefined;
    try {
      await portal.rate(detail, Number(score), ratingComment);
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isRating = false;
    }
  }
</script>

<Stack gap="300">
  <PageHeader title={detail?.title ?? t('app.portal.request')} subtitle={detail ? t('app.portal.raised_at', { at: at(detail.created_at) }) : undefined}
    isTitleInBar={viewport.isCompact} />
  {#if failure}
    <ErrorState title={failure.message} reference={failure.reference} referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')} onRetry={() => void request.refresh()} />
  {:else if !detail}
    <div aria-busy="true"><Skeleton lines={5} /></div>
  {:else}
    <div class="standing">
      <Badge tone={detail.is_completed ? 'success' : 'info'}>
        {detail.is_completed ? t('app.portal.resolved') : (detail.status ?? t('app.portal.open'))}
      </Badge>
      <Button tone="subtle" size="sm" icon="arrow-left" onclick={() => onnavigate?.(`/portal/${detail.collection_id}`)}>{t('app.portal.back')}</Button>
    </div>
    {#if detail.notes}<p class="notes">{detail.notes}</p>{/if}

    {#if detail.sla.length > 0}
      <section aria-labelledby="portal-goals">
        <Stack gap="050">
          <h2 class="heading" id="portal-goals">{t('app.portal.goals')}</h2>
          <ul class="goals">
            {#each detail.sla as goal (goal.goal)}
              {@const tone = goalTone(goal, now)}
              <li>
                <Badge tone={tone} size="sm">{t(`app.portal.goal.${goal.goal}`)}</Badge>
                {#if goal.standing === 'MET'}{t('app.portal.goal_met', { at: at(goal.met_at) })}
                {:else if goal.standing === 'BREACHED'}{t('app.portal.goal_breached', { at: at(goal.breached_at) })}
                {:else if goal.standing === 'PAUSED'}{t('app.portal.goal_paused')}
                {:else}{t('app.portal.goal_due', { at: at(goal.due_at) })}{/if}
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}

    <section aria-labelledby="portal-replies">
      <Stack gap="100">
        <h2 class="heading" id="portal-replies">{t('app.portal.replies')}</h2>
        {#if detail.comments.length === 0}<p class="quiet">{t('app.portal.replies_none')}</p>{/if}
        <ol class="replies">
          {#each detail.comments as comment (comment.id)}
            <li class:mine={comment.mine}>
              <span class="who">{comment.mine ? t('app.portal.you') : t('app.portal.team')} · {at(comment.created_at)}</span>
              <p class="body">{comment.body ?? ''}</p>
            </li>
          {/each}
        </ol>
        <form onsubmit={(event) => void send(event)}>
          <Stack gap="100">
            <Textarea label={t('app.portal.answer')} rows={3} bind:value={answer} />
            <div><Button type="submit" tone="secondary" icon="send" isBusy={isAnswering} busyLabel={t('app.portal.sending')}
              disabledReason={answer.trim() === '' ? t('app.portal.answer_first') : undefined}>{t('app.portal.send')}</Button></div>
          </Stack>
        </form>
      </Stack>
    </section>

    {#if detail.is_completed}
      <section aria-labelledby="portal-rating">
        <form onsubmit={(event) => void rate(event)}>
          <Stack gap="100">
            <h2 class="heading" id="portal-rating">{t('app.portal.rating')}</h2>
            <Radio label={t('app.portal.rating_question')} name="portal-score" bind:value={score}
              options={[1, 2, 3, 4, 5].map((n) => ({ value: String(n), label: t(`app.portal.score.${n}`) }))} />
            <Textarea label={t('app.portal.rating_comment')} rows={2} bind:value={ratingComment} />
            {#if detail.satisfaction}<p class="quiet">{t('app.portal.rated_at', { at: at(detail.satisfaction.rated_at) })}</p>{/if}
            <div><Button type="submit" tone="secondary" icon="check" isBusy={isRating} busyLabel={t('app.portal.sending')}
              disabledReason={score === '' ? t('app.portal.score_first') : undefined}>{t('app.portal.rate')}</Button></div>
          </Stack>
        </form>
      </section>
    {/if}
    {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
  {/if}
</Stack>

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .standing { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: center; }
  .notes, .body { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
  .goals, .replies { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .goals li { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: center; }
  .replies li { padding: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); }
  .replies li.mine { border-color: var(--border-default); }
  .who { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
