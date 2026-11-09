<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What this workspace may use, and how close it is (`multi-tenancy.md` §4).
  //
  // **A list page** (page-system §7.1): the limits are objects of one kind, read and compared, never
  // configured here - nothing on the screen is a value this workspace sets, so it is not a settings
  // page, and nothing happened at a time, so it is not a log. Search and the count above the table
  // (PS-7); no head primary, because nothing here is created.
  //
  // **There is no control to raise a limit, and the screen says who to ask.**
  // `/admin/tenants/{id}/quotas` is the installation operator's (multi-tenancy.md §4). A button the
  // server would refuse is worse than no button — it teaches somebody that the product is broken
  // rather than that the decision is somebody else's.
  //
  // **`limit: 0` is unlimited, not zero.** Rendering it as a number would say the opposite of what
  // it means, and "0 of 0 used" is the kind of screen somebody opens a support thread about.
  //
  // **`configured: false` is the mode's default rather than this workspace's own ceiling**, and
  // the difference matters to whoever is deciding whether to ask for more: one is a number
  // somebody chose for this workspace, the other is what every workspace on this installation
  // gets.
  //
  // **A quota this build has never heard of still renders.** Tolerance towards unknown fields is
  // binding, and a limit the client dropped would be a limit nobody can see they are near. Its
  // name is shown as the server's own token, because a made-up sentence would be worse.

  import { untrack } from 'svelte';

  import { Badge, Banner, Callout, DataTable, EmptyState, FilterBar, ListPage, ProgressBar, ReadOnlyNotice, Spinner, Stack, type DataColumn } from '@hubtask/design-system/components';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { quotas, type Standing } from '../lib/data/quotas.svelte.ts';
  import { signInPolicy } from '../lib/data/signinpolicy.svelte.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { sorting } from '../lib/sorting.svelte.ts';
  import AccountLink from '../lib/workspace/AccountLink.svelte';
  import LimitSource from '../lib/workspace/LimitSource.svelte';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  $effect(() => untrack(() => quotas.open()));
  // The plan's name rides on the workspace read the sign-in rule already answers (UC-INS-10
  // check 1): read here for the note beside the heading, never for a limit.
  $effect(() => untrack(() => signInPolicy.open()));
  const plan = $derived(signInPolicy.policy?.plan ?? undefined);

  const reading = $derived(quotas.state);
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  // `approach` is the column this screen exists for: "which limit am I closest to" is the
  // question, and until now the answer was read off five progress bars in the catalogue's order.
  const columns: readonly DataColumn[] = $derived([
    { id: 'quota', label: t('app.quotas.limit_name'), isSortable: true, priority: 1 },
    { id: 'limit', label: t('app.quotas.ceiling'), priority: 2 },
    { id: 'used', label: t('app.quotas.used'), isSortable: true, priority: 2 },
    { id: 'ratio', label: t('app.quotas.approach'), isSortable: true, isStatus: true },
    { id: 'source', label: t('app.quotas.source'), priority: 3 },
  ]);

  // `used` and `ratio` are absent where no live count exists, and absent sorts last in both
  // directions - which is right here: a limit with no number is not the one closest to being
  // reached, whichever way round the column is turned.
  const order = sorting({
    rows: () => quotas.standings,
    read: (standing, columnId) => {
      if (columnId === 'used') return standing.used;
      if (columnId === 'ratio') return standing.ratio;
      return nameOf(standing.quota);
    },
  });

  /** The wording for a limit this build knows. One it does not is shown as the server's token. */
  const KNOWN = new Set([
    'api_requests_per_minute',
    'items',
    'media_bytes',
    'automation_runs_per_hour',
    'webhook_targets',
    'export_jobs',
    'ai_tokens_per_day',
    'ai_own_tokens_per_day',
    'ai_arrival_proposals_per_day',
    'accounts',
    'mailboxes',
  ]);

  /** The people limit, where one is set and reached: nobody new comes in, nobody here goes. */
  const people = $derived(quotas.standings.find((standing) => standing.quota === 'accounts'));
  const peopleReached = $derived(
    people !== undefined && people.limit > 0 && (people.used ?? 0) >= people.limit,
  );

  const nameOf = (quota: string) => (KNOWN.has(quota) ? t(`app.quotas.name.${quota}`) : quota);

  let search = $state('');
  const shown = $derived(order.rows.filter((standing) => matchesSearch(search, [nameOf(standing.quota), standing.quota])));


  /**
   * Who set a limit, in words, for a server that answers no source (UC-INS-10 check 1): a lock says who locked it unless the operator's
   * exception decides anyway; otherwise the level that set the value in force. A server that
   * answers no source is read as before.
   */
  function setBy(standing: Standing): string {
    if (standing.source === undefined) {
      if (standing.configured) return t('app.quotas.workspace');
      return standing.locked ? t('app.quotas.locked_installation') : t('app.quotas.installation');
    }
    // A single installation names no operator (deployments.md D1): its exception is the workspace's.
    if (standing.source === 'EXCEPTION') {
      return manifest.isSingleTenant ? t('app.quotas.workspace') : t('app.quotas.exception');
    }
    if (standing.source === 'WORKSPACE') return t('app.quotas.workspace');
    if (standing.lock === 'INSTANCE') return t('app.quotas.locked_installation');
    if (standing.lock === 'PLAN') return t('app.quotas.locked_plan');
    if (standing.source === 'PLAN') return t('app.quotas.plan');
    return standing.source === 'INSTANCE' ? t('app.quotas.installation') : t('app.quotas.default');
  }

  /**
   * The limits whose use has reached them: a ceiling (zero is "no ceiling") and a live count at or
   * above it - above where a lowered limit left more than it allows. Each is said once above the
   * table, with what still works and, for owners and administrators, the provider's account page
   * (UC-INS-13 check 11).
   */
  const reached = $derived(
    quotas.standings.filter(
      // People have their own sentence above, which says who is affected.
      (standing) => standing.quota !== 'accounts' && standing.limit > 0 && standing.used !== undefined &&
        standing.used !== null && standing.used >= standing.limit,
    ),
  );

  /** A number as the limit counts it: attachment storage in bytes is said as a size. */
  const amount = (quota: string, value: number) =>
    quota === 'media_bytes' ? formatBytes(value, messages.locale) : String(value);

  /** The same number `hubtask_tenant_quota_usage_ratio` reports, as a percentage a person reads. */
  const percentOf = (ratio: number) => Math.round(ratio * 100);
  const title = $derived(t('app.quotas.title'));
  const head = $derived({ title, breadcrumb: adminTrail({ id: 'quotas', label: title }), figure: plan ? planFigure : undefined });
  $effect(() => page.entitle(title));
</script>

{#snippet planFigure()}
  <!-- The plan the workspace is on, so "Set by your plan" refers to something a person can name to
       their provider. A note, not an offer. -->
  <span class="plan" data-testid="limits-plan"><Badge tone="info">{t('app.signin_settings.plan', { name: plan ?? '' })}</Badge></span>
{/snippet}

<ListPage {head}>
  {#snippet intro()}
    <Stack gap="150">
      <!-- A single installation says neither "installation" nor "operator" (deployments.md D1); its
           sentence also serves an administrator who cannot open the section it names. -->
      <p class="quiet">{t(manifest.isSingleTenant ? 'app.quotas.intro_single' : 'app.quotas.intro')}</p>
      {#if reading.status === 'ready'}
        <!-- The whole page is read here and changed elsewhere: said once, at the top, with whom to
             ask (UC-WRK-32 check 4). -->
        <ReadOnlyNotice text={t(manifest.isSingleTenant ? 'app.quotas.raising_single' : 'app.quotas.raising')} />
        {#if peopleReached}
          <Banner tone="warning" title={t('app.quotas.people_reached')}>{t('app.quotas.people_reached_body')} <AccountLink /></Banner>
        {/if}
        {#each reached as standing (standing.quota)}
          <Callout tone="warning" title={t('app.quotas.reached_title', { name: nameOf(standing.quota) })}>
            {t('app.quotas.reached_body')} <AccountLink />
          </Callout>
        {/each}
      {/if}
    </Stack>
  {/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.quotas.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      clearLabel={t('app.page.clear')}
      onclear={() => (search = '')}
      countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(quotas.standings.length) }) : undefined}
    />
  {/snippet}

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting">
      <Spinner label={t('app.quotas.reading')} />
      <span>{t('app.quotas.reading')}</span>
    </p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else}
    <DataTable
      label={title}
      {columns}
      rows={shown}
      key={(standing: Standing) => standing.quota}
      sort={order.sort}
      onSort={(next) => order.by(next)}
      shownLabel={shown.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(standing: Standing, column: DataColumn)}
        {#if column.id === 'quota'}
          <span class="name">{nameOf(standing.quota)}</span>
        {:else if column.id === 'limit'}
          {#if standing.limit === 0}
            <!-- Zero is the contract's word for "no ceiling". Printing the digit would say the
                 opposite of what it means. -->
            {t('app.quotas.unlimited')}
          {:else}
            {amount(standing.quota, standing.limit)}
          {/if}
        {:else if column.id === 'used'}
          {#if standing.used === undefined || standing.used === null}
            <!-- No live count: the request rate is enforced by the limiter and reported in its own
                 headers, so this screen would be inventing a number. -->
            <span class="quiet">{t('app.quotas.no_count')}</span>
          {:else}
            {amount(standing.quota, standing.used)}
          {/if}
        {:else if column.id === 'ratio'}
          {#if standing.ratio === undefined || standing.ratio === null}
            <span class="quiet">—</span>
          {:else}
            <span class="approach">
              <ProgressBar
                size="sm"
                label={nameOf(standing.quota)}
                value={Math.min(100, percentOf(standing.ratio))}
                valueLabel={t('app.quotas.percent', { percent: String(percentOf(standing.ratio)) })}
              />
            </span>
          {/if}
        {:else if standing.source === undefined}
          {setBy(standing)}
        {:else}
          <LimitSource {standing} />
        {/if}
      {/snippet}
      {#snippet empty()}<EmptyState kind="filtered" title={t('app.quotas.none_found')} />{/snippet}
    </DataTable>
    {#if people}<p class="quiet">{t('app.quotas.people_counted')}</p>{/if}
  {/if}
</ListPage>

<style>
  .plan { display: inline-flex; }

  .quiet { margin: 0; color: var(--text-secondary); }

  .waiting {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); white-space: nowrap; }

  /* Wide enough for the bar to mean something, and it is the only column that wants width. */
  .approach { display: block; min-inline-size: 12ch; }
</style>
