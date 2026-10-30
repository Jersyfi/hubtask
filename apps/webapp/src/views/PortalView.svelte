<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A service's portal (M5): the kinds of request it offers, each with its own questions
  // (UC-SVC-05 checks 1 and 2), the articles it publishes for the words typed so far (UC-DOC-13
  // check 3), and the reader's own requests there (check 3). A requester reaches it by the address
  // the team gives them; nothing here reads the team's side of a collection. The approvals waiting
  // for the reader's yes are decided here too (UC-SVC-09 check 5), without a link to the entry.
  // Where the reader's organisation shares its requests, its colleagues' are one choice away
  // (UC-SVC-11 check 5); the list is asked for only once chosen.

  import { untrack } from 'svelte';
  import {
    Badge, Button, EmptyState, ErrorState, Input, ListRow, LoadMore, PageHeader, Radio, Skeleton, Stack, Textarea,
  } from '@hubtask/design-system/components';
  import type { ApprovalList, PortalRequestPage, RequestCatalogue } from '@hubtask/sync-engine';

  import { myApprovalsPath } from '../lib/data/approvals.svelte.ts';

  import { cataloguePath, myRequestsPath, organisationRequestsPath, portal } from '../lib/data/portal.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import ArticleSuggestions from '../lib/portal/ArticleSuggestions.svelte';
  import Question from '../lib/portal/Question.svelte';
  import { answersOf, askable, deflectionText } from '../lib/portal/portal.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ApprovalCard from '../lib/servicedesk/ApprovalCard.svelte';
  import { offersOrganisation } from '../lib/servicedesk/organisations.ts';

  const { collectionId, onnavigate }: { collectionId: string; onnavigate?: (path: string) => void } = $props();

  $effect(() => page.entitle(t('app.portal.title')));

  const catalogue = untrack(() => resource<RequestCatalogue>(cataloguePath(collectionId)));
  const mine = untrack(() => resource<PortalRequestPage>(myRequestsPath(collectionId)));
  const asked = untrack(() => resource<ApprovalList>(myApprovalsPath));
  const waiting = $derived(asked.state.status === 'ready' ? asked.state.data.data : []);

  const kinds = $derived(catalogue.state.status === 'ready' ? catalogue.state.data.data : []);
  const failure = $derived(catalogue.state.status === 'failed' ? renderProblem(catalogue.state.error, messages) : undefined);
  const organisation = $derived(mine.state.status === 'ready' ? mine.state.data.organisation : undefined);
  const offered = $derived(offersOrganisation(organisation));
  let scope = $state<'MINE' | 'ORGANISATION'>('MINE');
  let theirs = $state<ReturnType<typeof resource<PortalRequestPage>> | undefined>(undefined);
  $effect(() => {
    if (scope === 'ORGANISATION' && !theirs) theirs = untrack(() => resource<PortalRequestPage>(organisationRequestsPath(collectionId)));
  });
  const shown = $derived(scope === 'ORGANISATION' && offered && theirs ? theirs : mine);
  const requests = $derived(shown.state.status === 'ready' ? shown.state.data.data : []);
  const hasMore = $derived(shown.state.status === 'ready' && Boolean(shown.state.data.page_info.next_cursor));

  let kind = $state('');
  let title = $state('');
  let notes = $state('');
  let answers = $state<Record<string, unknown>>({});
  let isRaising = $state(false);
  let isLoadingMore = $state(false);
  let refusal = $state<string | undefined>(undefined);
  let typed = $state('');

  $effect(() => {
    if (kind === '' && kinds.length === 1) kind = kinds[0]?.key ?? '';
  });
  const chosen = $derived(kinds.find((each) => each.key === kind));
  const questions = $derived(chosen ? askable(chosen.questions) : []);
  const fieldOf = (key: string) => chosen?.form.fields?.find((field) => field.key === `custom_fields.${key}`);

  // Articles are asked for the words once typing rests, not for every key.
  $effect(() => {
    const words = deflectionText(title, notes);
    const timer = setTimeout(() => (typed = words), 400);
    return () => clearTimeout(timer);
  });

  async function raise(event: SubmitEvent) {
    event.preventDefault();
    if (!chosen) return;
    isRaising = true;
    refusal = undefined;
    try {
      const raised = await portal.raise(collectionId, {
        entry_type: chosen.key, title: title.trim(), notes: notes.trim() === '' ? undefined : notes,
        custom_fields: answersOf(questions, answers),
      });
      onnavigate?.(`/portal/requests/${raised.id}`);
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isRaising = false;
    }
  }

  async function more() {
    isLoadingMore = true;
    try {
      await shown.loadMore();
    } finally {
      isLoadingMore = false;
    }
  }
</script>

<Stack gap="300">
  <PageHeader title={t('app.portal.title')} subtitle={t('app.portal.intro')} isTitleInBar={viewport.isCompact} />

  {#if failure}
    <ErrorState title={failure.message} reference={failure.reference} referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')} onRetry={() => void catalogue.refresh()} />
  {:else if catalogue.state.status !== 'ready'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if kinds.length === 0}
    <EmptyState kind="unused" icon="inbox" title={t('app.portal.none')} description={t('app.portal.none_body')} />
  {:else}
    <form class="raise" onsubmit={(event) => void raise(event)} aria-labelledby="portal-ask">
      <Stack gap="150">
        <h2 class="heading" id="portal-ask">{t('app.portal.ask')}</h2>
        <Radio label={t('app.portal.kind')} name="portal-kind" bind:value={kind}
          options={kinds.map((each) => ({ value: each.key, label: each.name || each.key }))} />
        <Input label={t('app.portal.request_title')} hint={t('app.portal.request_title_hint')} isRequired maxlength={500} bind:value={title} />
        {#key typed}
          <ArticleSuggestions {collectionId} text={typed} onopen={(pageId) => onnavigate?.(`/portal/${collectionId}/articles/${pageId}`)} />
        {/key}
        <Textarea label={t('app.portal.request_notes')} rows={5} bind:value={notes} />
        {#each questions as question (question.key)}
          <Question {question} help={fieldOf(question.key)?.help} isRequired={fieldOf(question.key)?.required ?? false}
            bind:value={answers[question.key]} />
        {/each}
        {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
        <div>
          <Button type="submit" tone="primary" icon="send" isBusy={isRaising} busyLabel={t('app.portal.raising')}
            disabledReason={!chosen ? t('app.portal.kind_first') : title.trim() === '' ? t('app.portal.title_first') : undefined}>
            {t('app.portal.raise')}
          </Button>
        </div>
      </Stack>
    </form>
  {/if}

  {#if waiting.length > 0}
    <section aria-labelledby="portal-yes">
      <Stack gap="100">
        <h2 class="heading" id="portal-yes">{t('app.portal.waiting_for_yes')}</h2>
        {#each waiting as approval (approval.id)}
          <ApprovalCard {approval} />
        {/each}
      </Stack>
    </section>
  {/if}

  <section aria-labelledby="portal-mine">
    <Stack gap="100">
      <h2 class="heading" id="portal-mine">{t('app.portal.mine')}</h2>
      {#if offered}
        <Radio label={t('app.portal.whose')} name="portal-whose" bind:value={scope}
          options={[{ value: 'MINE', label: t('app.portal.whose_mine') },
            { value: 'ORGANISATION', label: t('app.portal.whose_organisation', { name: organisation?.name ?? '' }) }]} />
      {/if}
      {#if shown.state.status === 'ready' && requests.length === 0}
        <p class="quiet">{t('app.portal.mine_none')}</p>
      {:else if shown.state.status === 'ready'}
        {#each requests as request (request.id)}
          <ListRow href={`/portal/requests/${request.id}`} onactivate={() => onnavigate?.(`/portal/requests/${request.id}`)}>
            {request.title}
            <span class="meta">
              <Badge tone={request.is_completed ? 'success' : 'info'} size="sm">
                {request.is_completed ? t('app.portal.resolved') : (request.status ?? t('app.portal.open'))}
              </Badge>
              {formatDateTime(request.created_at, messages.locale)}
              {#if request.is_own === false}<Badge tone="neutral" size="sm">{t('app.portal.colleague')}</Badge>{/if}
            </span>
          </ListRow>
        {/each}
        <LoadMore label={t('app.portal.more')} busyLabel={t('app.portal.loading_more')} {hasMore} isBusy={isLoadingMore}
          endLabel={t('app.portal.end')} onLoadMore={() => void more()} />
      {/if}
    </Stack>
  </section>
</Stack>

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .raise { margin: 0; max-inline-size: 72ch; }
  .meta { display: inline-flex; gap: var(--sp-100); align-items: center; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
