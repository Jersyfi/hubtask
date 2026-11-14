<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A service's portal, its start (scenes `portal`, `portalext`): a greeting with one large search
  // over the kinds of request and the knowledge - answers before the form -, the kinds as tiles,
  // each asked in a dialog (UC-SVC-05 checks 1 and 2), what waits for the reader's yes on top
  // (UC-SVC-09 check 5, PO-3), the reader's requests in the desk's own words with what comes next
  // (check 3, P-12) and the knowledge the desk shares (UC-DOC-13, PO-4). The head is the desk's
  // own (checks 7 to 9). Inside, every other department is one link away (PO-1); the desk's team
  // sees what it sets up for the portal instead of a refusal.

  import { untrack } from 'svelte';
  import { Button, EmptyState, Icon, SearchField, Skeleton, Stack, WorkPage, type IconName } from '@hubtask/design-system/components';
  import FailureState from '../lib/frame/FailureState.svelte';
  import { ICON_NAMES } from '@hubtask/design-system/icons';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { ApprovalList, DeskLogo, PortalKnowledge, PortalRequestPage, RequestCatalogue } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { myApprovalsPath } from '../lib/data/approvals.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { cataloguePath, deskLogoPath, knowledgePath, myRequestsPath } from '../lib/data/portal.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { resource, resourceWhen } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import ArticleSuggestions from '../lib/portal/ArticleSuggestions.svelte';
  import RequestDialog from '../lib/portal/RequestDialog.svelte';
  import RequestRows from '../lib/portal/RequestRows.svelte';
  import { deskHead, deskLogoOf, isOutsider, isTeamView, teamDeskOf, wordsFind } from '../lib/portal/portal.ts';
  import { media } from '../lib/data/media.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ApprovalCard from '../lib/servicedesk/ApprovalCard.svelte';
  import PublicFormPanel from '../lib/servicedesk/PublicFormPanel.svelte';
  import DeskLogoPanel from '../lib/servicedesk/DeskLogoPanel.svelte';
  import ContainerMark from '../lib/workspace/ContainerMark.svelte';

  const { collectionId, onnavigate }: { collectionId: string; onnavigate?: (path: string) => void } = $props();

  const catalogue = untrack(() => resource<RequestCatalogue>(cataloguePath(collectionId)));
  // The desk's team holds no REQUEST at its own portal: the catalogue answers it by READ with
  // `may_raise` false, and nothing that only a requester reads is asked for it. A server that
  // still refuses the team is answered from the collection's own read and the logo's (check 9).
  const isRefusedTeam = $derived(catalogue.state.status === 'failed' && isTeamView(catalogue.state.error));
  const mayRaise = $derived(catalogue.state.status === 'ready' && catalogue.state.data.may_raise !== false);
  const isTeam = $derived(isRefusedTeam || (catalogue.state.status === 'ready' && !mayRaise));
  let teamLogo = $state<ReturnType<typeof resource<DeskLogo>> | undefined>(undefined);
  $effect(() => {
    if (!isRefusedTeam) return;
    return untrack(() => {
      teamLogo ??= resource<DeskLogo>(deskLogoPath(collectionId));
      return containers.openSingle(collectionId);
    });
  });
  const desk = $derived(
    catalogue.state.status === 'ready'
      ? catalogue.state.data.desk
      : isRefusedTeam ? teamDeskOf(containers.find(collectionId), teamLogo?.state.status === 'ready' ? teamLogo.state.data.logo_media_id : null) : undefined,
  );
  const logoUrl = $derived(media.coverUrl(deskLogoOf(desk), Date.now()));
  const head = $derived(deskHead(desk, { title: t('app.portal.title'), intro: t('app.portal.intro') }));
  $effect(() => page.entitle(head.title));
  const outside = $derived(isOutsider(actor.hasNoAccess, actor.portals));
  const firstName = $derived((actor.account?.display_name ?? '').trim().split(/\s+/)[0] ?? '');

  const mine = resourceWhen<PortalRequestPage>(() => (mayRaise ? myRequestsPath(collectionId) : undefined));
  const knowledge = resourceWhen<PortalKnowledge>(() => (mayRaise ? knowledgePath(collectionId) : undefined));
  const asked = untrack(() => resource<ApprovalList>(myApprovalsPath));
  const waiting = $derived(asked.state.status === 'ready' ? asked.state.data.data : []);
  const waitingHasMore = $derived(asked.state.status === 'ready' && asked.state.data.page?.has_more === true);

  const kinds = $derived(catalogue.state.status === 'ready' ? catalogue.state.data.data : []);
  const failure = $derived(catalogue.state.status === 'failed' && !isTeam ? renderProblem(catalogue.state.error, messages) : undefined);
  const requests = $derived(mine.state.status === 'ready' ? mine.state.data.data.slice(0, 3) : []);
  const spaces = $derived(knowledge.state.status === 'ready' ? knowledge.state.data.data : []);

  // A kind's own mark where the type names one this client draws, else the portal's inbox.
  const markOfKind = (icon: string): IconName => ((ICON_NAMES as readonly string[]).includes(icon) ? (icon as IconName) : 'inbox');

  // The one search: the kinds it finds at once, the articles once typing rests (answers first).
  let query = $state('');
  let settled = $state('');
  $effect(() => {
    const words = query.trim();
    const timer = setTimeout(() => (settled = words), 300);
    return () => clearTimeout(timer);
  });
  const foundKinds = $derived(query.trim() === '' ? [] : kinds.filter((each) => wordsFind(query, each.name, each.key, each.description)));

  let asking = $state<(typeof kinds)[number] | undefined>(undefined);
  let askingWords = $state('');
  function ask(kind: (typeof kinds)[number], words = '') {
    askingWords = words;
    asking = kind;
  }
  const go = (path: string) => (event: MouseEvent) => {
    if (!onnavigate) return;
    event.preventDefault();
    onnavigate(path);
  };
</script>

{#snippet deskMark()}
  {#if logoUrl}<img class="logo" src={logoUrl} alt={t('app.desk_logo.alt', { name: desk?.name ?? '' })} />
  {:else}<ContainerMark look={{ type: 'COLLECTION', icon: desk?.icon || null, color_token: desk?.color_token || null }} size="md" />{/if}
{/snippet}

{#snippet hero()}
  <section class="hero" data-band={outside ? '' : undefined} aria-labelledby="portal-greeting">
    {#if !outside}<p class="eyebrow">{#if desk}<span class="mark">{@render deskMark()}</span>{/if}{head.title}</p>{/if}
    <h1 class="greeting" id="portal-greeting">
      {outside && firstName !== '' ? t('app.portal.greeting_named', { name: firstName }) : t('app.portal.greeting')}
    </h1>
    {#if !isTeam}
      <div class="search" role="search">
        <SearchField label={t('app.portal.search')} isLabelHidden clearLabel={t('app.portal.search_clear')} size="md"
          placeholder={outside ? t('app.portal.search_placeholder_outside') : t('app.portal.search_placeholder')} bind:value={query} data-portal-search />
        {#if query.trim() !== ''}
          <div class="found" data-portal-found>
            {#if foundKinds.length > 0}
              <p class="group">{t('app.portal.found_kinds')}</p>
              {#each foundKinds as each (each.key)}
                <button type="button" class="hit" data-portal-hit={each.key} onclick={() => ask(each, query.trim())}>
                  <Icon name={markOfKind(each.icon)} size="sm" />{each.name || each.key}
                </button>
              {/each}
            {/if}
            {#key settled}
              <ArticleSuggestions {collectionId} text={settled} onopen={(pageId) => onnavigate?.(`/portal/${collectionId}/articles/${pageId}`)} />
            {/key}
            {#if foundKinds.length === 0 && kinds.length > 0}
              <p class="quiet">{t('app.portal.found_no_kind')}</p>
            {/if}
          </div>
        {/if}
      </div>
      <p class="hint">{outside ? t('app.portal.search_hint_outside') : t('app.portal.search_hint')}</p>
      {#if !outside}
        <a class="all" href="/portals" data-portal-all-departments onclick={go('/portals')}>{t('app.portal.all_departments')}</a>
      {/if}
    {:else}
      <p class="hint">{head.subtitle}</p>
    {/if}
  </section>
{/snippet}

{#snippet waitingCard()}
  {#if waiting.length > 0}
    <!-- On top of the start page (PO-3): decided here, the team's entry never opened. -->
    <section class="card waiting" aria-labelledby="portal-yes" data-portal-waiting>
      <Stack gap="100">
        <h2 class="heading" id="portal-yes">{outside ? t('app.portal.waiting_outside') : t('app.portal.waiting_for_yes')}</h2>
        {#each waiting.slice(0, 2) as approval (approval.id)}
          <ApprovalCard {approval} isBrief />
        {/each}
        {#if waiting.length > 2 || waitingHasMore}
          <a href={`/portal/${collectionId}/approvals`} data-portal-more-approvals onclick={go(`/portal/${collectionId}/approvals`)}>{t('app.portal.more_waiting')}</a>
        {/if}
      </Stack>
    </section>
  {/if}
{/snippet}

{#snippet askSection()}
  {#if kinds.length === 0}
    <EmptyState kind="unused" icon="inbox" title={t('app.portal.none')} description={t('app.portal.none_body')} />
  {:else}
    <section class="ask" aria-labelledby="portal-ask">
      <h2 class="label" id="portal-ask">{outside ? t('app.portal.ask_outside') : t('app.portal.ask')}</h2>
      <ul class="tiles" data-rows={outside ? '' : undefined} aria-label={t('app.portal.kind')}>
        {#each kinds as each (each.key)}
          <li>
            <button type="button" class="tile" data-portal-kind={each.key} onclick={() => ask(each)}>
              <span class="tile-mark"><Icon name={markOfKind(each.icon)} size="sm" /></span>
              <span class="words">
                <span class="tile-name">{each.name || each.key}</span>
                {#if each.description}<span class="tile-sub">{each.description}</span>{/if}
              </span>
            </button>
          </li>
        {/each}
      </ul>
    </section>
  {/if}
{/snippet}

{#snippet mineCard()}
  <section class="card mine" aria-labelledby="portal-mine" data-portal-mine>
    <div class="card-head">
      <h2 class="heading" id="portal-mine">{outside ? t('app.portal.mine_outside') : t('app.portal.mine')}</h2>
      <!-- Outside the frame's tab leads to them all. -->
      {#if !outside}<a href={`/portal/${collectionId}/requests`} onclick={go(`/portal/${collectionId}/requests`)}>{t('app.portal.mine_all')}</a>{/if}
    </div>
    {#if mine.state.status === 'ready' && requests.length === 0}
      <ProductEmpty product="SERVICE_DESK" kind="unused" title={t('app.portal.mine_none')} />
    {:else if mine.state.status === 'ready'}
      <RequestRows {requests} {onnavigate} isStacked={outside} />
    {:else}
      <div aria-busy="true"><Skeleton lines={3} /></div>
    {/if}
  </section>
{/snippet}

{#snippet spaceList()}
  <ul class="spaces" data-rows={outside ? '' : undefined}>
    {#each spaces.slice(0, 4) as space (space.page_id)}
      <li>
        <a class="space" href={`/portal/${collectionId}/articles/${space.page_id}`} data-portal-space={space.page_id}
          onclick={go(`/portal/${collectionId}/articles/${space.page_id}`)}>
          <span class="space-icon" aria-hidden="true">{#if space.icon}{space.icon}{:else}<Icon name="book-open" size="sm" />{/if}</span>
          <span class="words">
            <span class="tile-name">{space.title}</span>
            <span class="tile-sub">{space.with_subpages ? t('app.portal.space_count', { count: String(space.pages.length + 1) }) : t('app.portal.article_single')}</span>
          </span>
        </a>
      </li>
    {/each}
  </ul>
{/snippet}

{#snippet knowledgeCard()}
  <section class="card know" aria-labelledby="portal-knowledge" data-portal-knowledge>
    <div class="card-head">
      <h2 class="heading" id="portal-knowledge">{spaces.length > 0 ? t('app.portal.knowledge_for', { name: head.title }) : t('app.portal.often_read')}</h2>
      {#if spaces.length > 0}<a href={`/portal/${collectionId}/knowledge`} onclick={go(`/portal/${collectionId}/knowledge`)}>{t('app.portal.knowledge_all')}</a>{/if}
    </div>
    {#if spaces.length > 0}
      {@render spaceList()}
    {:else}
      <ArticleSuggestions {collectionId} text="" isPlain onopen={(pageId) => onnavigate?.(`/portal/${collectionId}/articles/${pageId}`)} />
    {/if}
  </section>
{/snippet}

{#snippet knowledgeSection()}
  <!-- Outside the shared spaces stand as tiles under their own label (scene `portalext`, PO-4). -->
  {#if spaces.length > 0}
    <section class="know" aria-labelledby="portal-knowledge" data-portal-knowledge>
      <div class="label-head">
        <h2 class="label" id="portal-knowledge">{t('app.portal.knowledge_for', { name: head.title })}</h2>
        <a href={`/portal/${collectionId}/knowledge`} onclick={go(`/portal/${collectionId}/knowledge`)}>{t('app.portal.knowledge_all')}</a>
      </div>
      {@render spaceList()}
    </section>
  {:else}
    {@render knowledgeCard()}
  {/if}
{/snippet}

<WorkPage>
<div class="portal" data-portal-start data-outside={outside ? '' : undefined}>
  {@render hero()}

  {#if isTeam}
    <EmptyState kind="unused" icon="inbox" title={t('app.portal.team_view')} description={t('app.portal.team_view_body')}>
      {#snippet action()}
        {#if holdsHere('MANAGE_MEMBERS') === true}
          <Button tone="primary" icon="user-plus" onclick={() => onnavigate?.('/administration/people')}>{t('app.portal.invite_requesters')}</Button>
        {/if}
      {/snippet}
    </EmptyState>
  {:else if failure}
    <FailureState problem={failure} onRetry={() => void catalogue.refresh()} />
  {:else if catalogue.state.status !== 'ready'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {/if}

  {#if outside && mayRaise}
    <!-- The outsiders' start (scene `portalext`): what to ask and the shared knowledge on the
         left, what waits for them and their requests beside it; one column on a phone, what
         waits first. -->
    <div class="split" data-portal-split>
      <div class="main">
        {@render askSection()}
        {@render knowledgeSection()}
      </div>
      <div class="side">
        {@render waitingCard()}
        {@render mineCard()}
      </div>
    </div>
  {:else}
    {@render waitingCard()}
    {#if mayRaise}
      {@render askSection()}
      <div class="pair">
        {@render mineCard()}
        {@render knowledgeCard()}
      </div>
    {/if}
  {/if}

  <!-- Asked only of somebody who manages the workspace's members: a requester's portal would
       otherwise ask, and be refused, at every opening. -->
  {#if holdsHere('MANAGE_MEMBERS') === true && catalogue.state.status !== 'idle' && catalogue.state.status !== 'loading'}
    <PublicFormPanel {collectionId} hasForm={catalogue.state.status === 'ready' ? catalogue.state.data.has_public_form : undefined} />
  {/if}
  {#if holdsHere('STRUCTURE') === true && desk}<DeskLogoPanel {collectionId} hasLogo={deskLogoOf(desk) !== undefined} />{/if}
</div>
</WorkPage>

{#if asking}
  {#key asking.key}
    <RequestDialog {collectionId} kind={asking} words={askingWords} onclose={() => (asking = undefined)} {onnavigate} />
  {/key}
{/if}

<style>
  .portal { display: flex; flex-direction: column; gap: var(--sp-300); padding-block-end: var(--sp-400); container-type: inline-size; }
  .hero {
    display: flex; flex-direction: column; align-items: center; gap: var(--sp-150); text-align: center;
    padding: var(--sp-500) var(--sp-300); border-radius: var(--r-lg);
    background: linear-gradient(135deg, var(--product-desk-surface), var(--accent-primary-subtle));
  }
  /* Outside the hero is the frame's band from edge to edge (scene `portalext`): it takes back the
     main region's padding. */
  .hero[data-band] {
    margin: calc(-1 * var(--sp-300)) calc(-1 * var(--sp-300)) 0; border-radius: 0; gap: var(--sp-200);
    padding: var(--sp-600) var(--sp-400) var(--sp-500);
    background: linear-gradient(160deg, var(--label-teal-bg), var(--bg-canvas) 80%);
  }
  .hero[data-band] .greeting { font-size: var(--fs-600); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px), the main region's own padding step; a media query cannot read a custom property. */
  @media (width < 600px) {
    .hero[data-band] { margin: calc(-1 * var(--sp-200)) calc(-1 * var(--sp-200)) 0; padding: var(--sp-400) var(--sp-200) var(--sp-300); }
    .hero[data-band] .greeting { font-size: var(--fs-400); }
  }
  .eyebrow { margin: 0; display: inline-flex; align-items: center; gap: var(--sp-100); color: var(--product-desk-ink); font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .mark { display: inline-flex; }
  .logo { inline-size: var(--sp-300); block-size: var(--sp-300); border-radius: var(--r-md); object-fit: contain; }
  .greeting { margin: 0; font-size: var(--fs-400); font-weight: var(--fw-bold); }
  .search { inline-size: min(100%, var(--layout-dialog-m)); position: relative; text-align: start; }
  .found {
    margin-block-start: var(--sp-100); padding: var(--sp-150); display: flex; flex-direction: column; gap: var(--sp-050);
    background: var(--bg-raised); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); box-shadow: var(--shadow-overlay);
  }
  .group { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-semibold); color: var(--text-secondary); text-transform: uppercase; }
  .hit {
    display: flex; align-items: center; gap: var(--sp-100); padding: var(--sp-100); border: 0; border-radius: var(--r-sm);
    background: transparent; color: inherit; font: inherit; text-align: start; cursor: pointer;
  }
  .hit:hover, .hit:focus-visible { background: var(--accent-primary-subtle); }
  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .all { font-size: var(--fs-075); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .label { margin: 0 0 var(--sp-100); font-size: var(--fs-075); font-weight: var(--fw-semibold); color: var(--text-secondary); text-transform: uppercase; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .tiles { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--sp-200); grid-template-columns: repeat(auto-fill, minmax(min(calc(50% - var(--sp-100)), var(--layout-gallery-card)), 1fr)); }
  .tile, .space {
    inline-size: 100%; block-size: 100%; display: flex; flex-direction: column; align-items: flex-start; gap: var(--sp-050);
    padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg);
    background: var(--bg-surface); color: inherit; font: inherit; text-align: start; text-decoration: none; cursor: pointer;
  }
  .space { flex-direction: row; align-items: center; padding: var(--sp-150); box-sizing: border-box; min-inline-size: 0; }
  .tile:hover, .space:hover { background: var(--bg-surface-hover); }
  .tile:focus-visible, .space:focus-visible { outline: var(--focus-ring); }
  .tile-mark {
    display: inline-flex; padding: var(--sp-050); border-radius: var(--r-md);
    background: var(--product-desk-surface); color: var(--product-desk-ink);
  }
  /* Outside a kind and a space read as a row: the mark beside its words, two to a line. */
  .tiles[data-rows], .spaces[data-rows] { gap: var(--sp-150); grid-template-columns: minmax(0, 1fr); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a container query cannot read a custom property. */
  @container (inline-size >= 600px) {
    .tiles[data-rows], .spaces[data-rows] { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }
  .tiles[data-rows] .tile { flex-direction: row; align-items: flex-start; gap: var(--sp-150); padding: var(--sp-200); }
  .spaces[data-rows] .space { padding: var(--sp-200); gap: var(--sp-150); align-items: flex-start; }
  .tile-name { font-weight: var(--fw-semibold); }
  .tile-sub { color: var(--text-secondary); font-size: var(--fs-075); }
  .split { display: flex; flex-direction: column; gap: var(--sp-200); }
  .main, .side { display: contents; }
  .split .waiting { order: 0; }
  .split .ask { order: 1; }
  .split .mine { order: 2; }
  .split .know { order: 3; }
  .label-head { display: flex; justify-content: space-between; align-items: baseline; gap: var(--sp-100); }
  .pair { display: grid; gap: var(--sp-300); grid-template-columns: minmax(0, 1fr); }
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a container query cannot read a custom property. */
  @container (inline-size >= 905px) {
    .pair { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); }
    .split { display: grid; grid-template-columns: minmax(0, 1.5fr) minmax(0, 1fr); gap: var(--sp-400); align-items: start; padding-inline: var(--sp-200); }
    .main, .side { display: flex; flex-direction: column; gap: var(--sp-300); min-inline-size: 0; }
    .side { gap: var(--sp-200); }
  }
  .card { padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .card-head { display: flex; justify-content: space-between; align-items: baseline; gap: var(--sp-100); margin-block-end: var(--sp-100); }
  .spaces { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--sp-100); grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-field-sm)), 1fr)); }
  .space-icon { font-size: var(--fs-200); }
  /* Inside, the card lists the spaces as rows, as scene `portal` lists what is often read. */
  .spaces:not([data-rows]) { grid-template-columns: minmax(0, 1fr); gap: 0; }
  .spaces:not([data-rows]) .space { border: 0; border-radius: var(--r-md); padding: var(--sp-100); }
  .words { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }
</style>
