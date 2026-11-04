<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A service's portal (M5): the kinds of request it offers, each with its own questions
  // (UC-SVC-05 checks 1 and 2), the articles it publishes for the words typed so far (UC-DOC-13
  // check 3), and the reader's own requests there (check 3). A requester reaches it by the address
  // the team gives them; nothing here reads the team's side of a collection. The approvals waiting
  // for the reader's yes are decided here too (UC-SVC-09 check 5), without a link to the entry.
  // Where the reader's organisation shares its requests, its colleagues' are one choice away
  // (UC-SVC-11 check 5); the list is asked for only once chosen. The head is the desk's own - its
  // name, description and mark - and the kinds are named in the reader's language (UC-SVC-05
  // checks 7 and 8); before the catalogue answers, the portal's own words stand in.

  import { tick, untrack } from 'svelte';
  import {
    Badge, Button, EmptyState, ErrorState, Input, ListRow, LoadMore, PageHeader, Radio, Skeleton, Stack, TemplateCard, TemplateGallery, Textarea, VisuallyHidden,
    type IconName,
  } from '@hubtask/design-system/components';
  import { ICON_NAMES } from '@hubtask/design-system/icons';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { ApprovalList, PortalRequestPage, RequestCatalogue } from '@hubtask/sync-engine';

  import { myApprovalsPath } from '../lib/data/approvals.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';

  import { cataloguePath, myRequestsPath, organisationRequestsPath, portal } from '../lib/data/portal.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import ArticleSuggestions from '../lib/portal/ArticleSuggestions.svelte';
  import Question from '../lib/portal/Question.svelte';
  import { answersOf, askable, deflectionText, deskHead, deskLogoOf, isTeamView } from '../lib/portal/portal.ts';
  import { media } from '../lib/data/media.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ApprovalCard from '../lib/servicedesk/ApprovalCard.svelte';
  import PublicFormPanel from '../lib/servicedesk/PublicFormPanel.svelte';
  import DeskLogoPanel from '../lib/servicedesk/DeskLogoPanel.svelte';
  import { offersOrganisation } from '../lib/servicedesk/organisations.ts';
  import ContainerMark from '../lib/workspace/ContainerMark.svelte';

  const { collectionId, onnavigate }: { collectionId: string; onnavigate?: (path: string) => void } = $props();

  const catalogue = untrack(() => resource<RequestCatalogue>(cataloguePath(collectionId)));
  const desk = $derived(catalogue.state.status === 'ready' ? catalogue.state.data.desk : undefined);
  // The desk's own logo, drawn from the download target the server answers (check 9); until it
  // arrives, or where the reader may not have it, the icon and the colour stand.
  const logoUrl = $derived(media.coverUrl(deskLogoOf(desk), Date.now()));
  const head = $derived(deskHead(desk, { title: t('app.portal.title'), intro: t('app.portal.intro') }));
  $effect(() => page.entitle(head.title));
  const mine = untrack(() => resource<PortalRequestPage>(myRequestsPath(collectionId)));
  const asked = untrack(() => resource<ApprovalList>(myApprovalsPath));
  const waiting = $derived(asked.state.status === 'ready' ? asked.state.data.data : []);

  const kinds = $derived(catalogue.state.status === 'ready' ? catalogue.state.data.data : []);
  // The desk's team opening the portal from its collection holds no REQUEST there: told as such,
  // with what it sets up for the portal, rather than shown the refusal (UC-SVC-05).
  const isTeam = $derived(catalogue.state.status === 'failed' && isTeamView(catalogue.state.error));
  const failure = $derived(catalogue.state.status === 'failed' && !isTeam ? renderProblem(catalogue.state.error, messages) : undefined);
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
  // A kind's own mark where the type names one this client draws, else the portal's inbox.
  const markOfKind = (icon: string): IconName => ((ICON_NAMES as readonly string[]).includes(icon) ? (icon as IconName) : 'inbox');
  // The form follows the chosen tile; choosing moves the reader to its first field, once.
  function choose(key: string) {
    kind = key;
    void tick().then(() => document.querySelector<HTMLInputElement>('[data-portal-ask] input')?.focus());
  }
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

{#snippet deskMark()}
  {#if logoUrl}<img class="logo" src={logoUrl} alt={t('app.desk_logo.alt', { name: desk?.name ?? '' })} />
  {:else}<ContainerMark look={{ type: 'COLLECTION', icon: desk?.icon || null, color_token: desk?.color_token || null }} size="md" />{/if}
{/snippet}

<Stack gap="300">
  <PageHeader title={head.title} subtitle={head.subtitle} isTitleInBar={viewport.isCompact}
    mark={desk ? deskMark : undefined} />

  {#if isTeam}
    <EmptyState kind="unused" icon="inbox" title={t('app.portal.team_view')} description={t('app.portal.team_view_body')} />
  {:else if failure}
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
        <!-- The kinds as tiles with a sentence each (user-experience.md §7.4), the form below
             the chosen one; the choice is said by the button's state and its words. -->
        {#if kinds.length > 1}
          <TemplateGallery label={t('app.portal.kind')}>
            {#each kinds as each (each.key)}
              {@const isChosen = each.key === kind}
              <TemplateCard name={each.name || each.key} summary={each.description || undefined} mark={markOfKind(each.icon)} {isChosen} isCompact data-portal-kind={each.key}>
                {#snippet action()}
                  <Button size="sm" tone={isChosen ? 'primary' : 'secondary'} icon={isChosen ? 'check' : undefined} aria-pressed={isChosen} onclick={() => choose(each.key)}>
                    {t(isChosen ? 'app.portal.kind_chosen' : 'app.portal.kind_choose')}<VisuallyHidden>{` ${each.name || each.key}`}</VisuallyHidden>
                  </Button>
                {/snippet}
              </TemplateCard>
            {/each}
          </TemplateGallery>
        {/if}
        {#if chosen}
          <div class="ask" data-portal-ask>
            <Stack gap="150">
              <Input label={t('app.portal.request_title')} hint={t('app.portal.request_title_hint')} isRequired maxlength={500} bind:value={title} />
              <Textarea label={t('app.portal.request_notes')} rows={5} bind:value={notes} />
              {#each questions as question (question.key)}
                <Question {question} help={fieldOf(question.key)?.help} isRequired={fieldOf(question.key)?.required ?? false}
                  bind:value={answers[question.key]} />
              {/each}
            </Stack>
            <!-- Answers from the knowledge base beside the form as the reader types (§7.4). -->
            <aside class="answers">
              {#key typed}
                <ArticleSuggestions {collectionId} text={typed} onopen={(pageId) => onnavigate?.(`/portal/${collectionId}/articles/${pageId}`)} />
              {/key}
            </aside>
          </div>
        {/if}
        {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
        {#if chosen}<div>
          <Button type="submit" tone="primary" icon="send" isBusy={isRaising} busyLabel={t('app.portal.raising')}
            disabledReason={!chosen ? t('app.portal.kind_first') : title.trim() === '' ? t('app.portal.title_first') : undefined}>
            {t('app.portal.raise')}
          </Button>
        </div>{/if}
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

  <!-- Asked only of somebody who manages the workspace's members: a requester's portal would
       otherwise ask, and be refused, at every opening. -->
  {#if holdsHere('MANAGE_MEMBERS') === true}<PublicFormPanel {collectionId} />{/if}
  {#if holdsHere('STRUCTURE') === true && desk}<DeskLogoPanel {collectionId} hasLogo={deskLogoOf(desk) !== undefined} />{/if}

  {#if !isTeam}<section aria-labelledby="portal-mine">
    <Stack gap="100">
      <h2 class="heading" id="portal-mine">{t('app.portal.mine')}</h2>
      {#if offered}
        <Radio label={t('app.portal.whose')} name="portal-whose" bind:value={scope}
          options={[{ value: 'MINE', label: t('app.portal.whose_mine') },
            { value: 'ORGANISATION', label: t('app.portal.whose_organisation', { name: organisation?.name ?? '' }) }]} />
      {/if}
      {#if shown.state.status === 'ready' && requests.length === 0}
        <ProductEmpty kind="unused" title={t('app.portal.mine_none')} />
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
  </section>{/if}
</Stack>

<style>
  .logo { inline-size: var(--sp-400); block-size: var(--sp-400); border-radius: var(--r-md); object-fit: contain; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .refusal { margin: 0; color: var(--text-danger); }
  .raise { margin: 0; container-type: inline-size; }
  /* The form and the articles side by side where there is room, the articles under it on a phone. */
  .ask { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); max-inline-size: 72ch; }
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a container query cannot read a custom property. */
  @container (inline-size >= 905px) {
    .ask { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); max-inline-size: none; }
  }
  .answers { min-inline-size: 0; }
  .meta { display: inline-flex; gap: var(--sp-100); align-items: center; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
