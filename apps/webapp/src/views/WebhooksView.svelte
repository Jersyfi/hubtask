<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What an external system is told (`automation.md` §5), as a list page (page-system §7.1, §9.3):
  // the subscriptions in one table, narrowed by their address, state and format; *New webhook* the
  // head's primary, a Dialog M; a webhook opening its record page with its settings and deliveries;
  // pause, resume and unsubscribing in its row's menu.
  //
  // **The secret appears once, and this screen keeps no copy.** It is held in one `$state` while
  // its Dialog S is open and dropped when it closes; nothing writes it to storage, to the address,
  // or to a message.
  //
  // **The three states say different things.** `PAUSED` is somebody's decision, `DISABLED` is what
  // sustained unreachability concluded - the record page says why beside its *Switch back on*.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, DataTable, EmptyState, FilterBar, ListPage, Radio, Spinner, Stack, type DataColumn, type DataRowMenu, type Filter } from '@hubtask/design-system/components';
  import type { MenuItem } from '@hubtask/design-system/components';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { keepsSecrets } from '../lib/administration/gaps.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { includedHere, mayHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { webhooks, type Subscription } from '../lib/data/webhooks.svelte.ts';
  import { WEBHOOK_FORMATS, isChat, webhookFigure } from '../lib/data/webhooks.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { Continued } from '../lib/page/continued.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import NewWebhookDialog from '../lib/webhooks/NewWebhookDialog.svelte';
  import SecretDialog from '../lib/webhooks/SecretDialog.svelte';
  import UnsubscribeDialog from '../lib/webhooks/UnsubscribeDialog.svelte';
  import { eventWord, listensAt, stateTone, stateWord, webhookScopes } from '../lib/webhooks/words.svelte.ts';
  import NotInPlan from '../lib/workspace/NotInPlan.svelte';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  const { onnavigate }: { onnavigate: (path: string) => void } = $props();

  $effect(() => page.entitle(t('app.webhooks.title')));

  $effect(() => untrack(() => webhooks.open()));
  $effect(() => untrack(() => containers.start()));
  $effect(() => {
    const stops = containers.hubs.map((hub) => untrack(() => containers.openLevel(hub.id)));
    return () => stops.forEach((stop) => stop());
  });
  const scopes = $derived(webhookScopes());

  const reading = $derived(webhooks.state);
  const listed = $derived(webhooks.all);
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  /** Whether the reader may change the workspace's automation (`AUTOMATION`), as the server asks. */
  const mayAutomate = $derived(mayHere('AUTOMATION').status === 'permitted');
  /**
   * Where the plan leaves webhooks out they rest: nothing is delivered, added, resumed or replayed -
   * pausing, a new secret and unsubscribing stay (multi-tenancy.md §4.2, P-05).
   */
  const webhooksIncluded = $derived(includedHere('WEBHOOKS') === true);
  const resting = $derived(includedHere('WEBHOOKS') === false);
  const mayAdd = $derived(mayAutomate && !resting && keepsSecrets(manifest.value?.features));

  let isAdding = $state(false);
  // The one place a secret exists in this client, for as long as its dialog is open.
  let secret = $state<string | undefined>(undefined);
  let removing = $state<Subscription | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  let search = $state('');
  let stateFilter = $state('');
  let formatFilter = $state('');
  const shown = $derived(
    listed.filter((each) =>
      matchesSearch(search, [each.target_url, each.room ?? '', ...each.event_types.map(eventWord)])
      && (stateFilter === '' || each.state === stateFilter)
      && (formatFilter === '' || (each.format ?? 'CLOUDEVENTS') === formatFilter)),
  );
  const continued = new Continued(() => shown);
  $effect(() => {
    void search;
    void stateFilter;
    void formatFilter;
    untrack(() => continued.restart());
  });

  const formatWord = (format: string) => t(`app.webhooks.format_${format.toLowerCase()}`);
  const chips = $derived<Filter[]>([
    { id: 'state', label: t('app.webhooks.chip_state', { state: stateFilter ? t(`app.webhooks.state_${stateFilter.toLowerCase()}`) : t('app.page.any') }), isActive: stateFilter !== '' },
    { id: 'format', label: t('app.webhooks.chip_format', { format: formatFilter ? formatWord(formatFilter) : t('app.page.any') }), isActive: formatFilter !== '' },
  ]);
  const activeFilters = $derived(chips.filter((chip) => chip.isActive).length);

  const columns = $derived<DataColumn[]>([
    { id: 'target', label: t('app.webhooks.target_column'), priority: 1 },
    { id: 'events', label: t('app.webhooks.events_column'), priority: 2 },
    { id: 'scope', label: t('app.webhooks.scope'), priority: 3 },
    { id: 'state', label: t('app.webhooks.state_column'), priority: 2, isStatus: true },
  ]);

  const eventsOf = (subscription: Subscription) =>
    subscription.event_types.length <= 2
      ? subscription.event_types.map(eventWord).join(', ')
      : t('app.webhooks.events_count', { first: eventWord(subscription.event_types[0] ?? ''), count: String(subscription.event_types.length - 1) });

  function rowMenu(subscription: Subscription): DataRowMenu | undefined {
    if (!mayAutomate) return undefined;
    const items: MenuItem[] = [
      { id: 'open', label: t('app.page.open') },
      ...(subscription.state === 'ACTIVE'
        ? [{ id: 'pause', label: t('app.webhooks.pause') }]
        : webhooksIncluded ? [{ id: 'resume', label: t('app.webhooks.resume') }] : []),
      { id: 'unsubscribe', label: t('app.webhooks.unsubscribe_menu'), isDestructive: true, hasSeparatorBefore: true },
    ];
    return { label: t('app.page.more_for', { name: subscription.target_url }), items };
  }

  async function onRowMenu(subscription: Subscription, id: string): Promise<void> {
    if (id === 'open') {
      onnavigate(`/administration/webhooks/${subscription.id}`);
      return;
    }
    if (id === 'unsubscribe') {
      removing = subscription;
      return;
    }
    failure = undefined;
    try {
      await webhooks.update(subscription.id, { state: id === 'pause' ? 'PAUSED' : 'ACTIVE' }, subscription.version);
      announcer.say(t(id === 'pause' ? 'app.webhooks.paused_announced' : 'app.webhooks.resumed_announced'));
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    }
  }

  const figureText = $derived.by(() => {
    if (listed.length === 0) return '';
    const counts = webhookFigure(listed);
    return [
      t('app.webhooks.figure_active', { count: String(counts.active) }),
      ...(counts.paused > 0 ? [t('app.webhooks.figure_paused', { count: String(counts.paused) })] : []),
      ...(counts.disabled > 0 ? [t('app.webhooks.figure_disabled', { count: String(counts.disabled) })] : []),
    ].join(' · ');
  });

  const head = $derived({
    title: t('app.webhooks.title'),
    breadcrumb: adminTrail({ id: 'screen', label: t('app.webhooks.title') }),
    primary: mayAdd ? { label: t('app.webhooks.add'), icon: 'plus' as const, opener: 'new-webhook', onclick: () => (isAdding = true) } : undefined,
    figure: figureText ? figure : undefined,
  });
</script>

{#snippet figure()}<span class="figure" data-webhooks-figure>{figureText}</span>{/snippet}

<ListPage {head}>
  {#snippet intro()}
    <Stack gap="150">
      <p class="quiet">{t('app.webhooks.intro')}</p>
      {#if resting}<NotInPlan body={t('app.plan.webhooks_body', { count: webhooks.all.length })} />{/if}
      {#if !resting && !keepsSecrets(manifest.value?.features)}
        <!-- Each subscription's secret is sealed under the installation's key; without one the
             server refuses at the end, so nothing is offered and the reason is said first. -->
        <Banner tone="warning" title={t('app.webhooks.no_key_title')}>{t('app.webhooks.no_key_body')}</Banner>
      {:else if !resting}
        <!-- A reader without AUTOMATION is told once why nothing here can be added (P-05). -->
        <WorkspaceGate permission="AUTOMATION" />
      {/if}
      {#if failure}<FailureNote problem={failure} />{/if}
    </Stack>
  {/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.webhooks.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={chips}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(activeFilters) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(shown.length) })}
      clearLabel={t('app.page.clear')}
      onclear={() => { search = ''; stateFilter = ''; formatFilter = ''; }}
      countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(listed.length) }) : undefined}
    >
      {#snippet picker(chip: Filter)}
        {#if chip.id === 'state'}
          <Radio label={t('app.webhooks.state_column')} bind:value={stateFilter}
            options={['', 'ACTIVE', 'PAUSED', 'DISABLED'].map((each) => ({ value: each, label: each === '' ? t('app.page.any') : t(`app.webhooks.state_${each.toLowerCase()}`) }))} />
        {:else}
          <Radio label={t('app.webhooks.format')} bind:value={formatFilter}
            options={['', ...WEBHOOK_FORMATS].map((each) => ({ value: each, label: each === '' ? t('app.page.any') : formatWord(each) }))} />
        {/if}
      {/snippet}
    </FilterBar>
  {/snippet}

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet"><Spinner label={t('app.webhooks.reading')} /> <span>{t('app.webhooks.reading')}</span></p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else}
    <DataTable
      label={t('app.webhooks.title')}
      {columns}
      rows={continued.shown}
      key={(each: Subscription) => each.id}
      href={(each: Subscription) => `/administration/webhooks/${each.id}`}
      rowLabel={(each: Subscription) => each.target_url}
      menu={rowMenu}
      onmenu={(each: Subscription, id: string) => void onRowMenu(each, id)}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      shownLabel={listed.length === 0 ? undefined : continued.shownLabel}
      more={continued.more}
      perPage={continued.perPage}
    >
      {#snippet cell(each: Subscription, column: DataColumn)}
        {#if column.id === 'target'}
          <span class="target" data-webhook={each.id}>{each.target_url}</span>
          {#if isChat(each.format)}<span class="quiet small"> · {formatWord(each.format ?? '')}</span>{/if}
        {:else if column.id === 'events'}
          <span class="quiet">{eventsOf(each)}</span>
        {:else if column.id === 'scope'}
          <span class="quiet">{listensAt(each, scopes)}</span>
        {:else}
          <Badge tone={stateTone(each)}>{stateWord(each)}</Badge>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if listed.length === 0}
          <EmptyState kind="unused" icon="send" title={t('app.webhooks.empty_title')} description={t('app.webhooks.empty_body')} action={mayAdd ? addAct : undefined} />
        {:else}
          <EmptyState kind="filtered" title={t('app.webhooks.none_found')} />
        {/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#snippet addAct()}
  <Button tone="primary" icon="plus" onclick={() => (isAdding = true)}>{t('app.webhooks.add')}</Button>
{/snippet}

{#if mayAdd}
  <NewWebhookDialog bind:isOpen={isAdding} {scopes} onCreated={(value) => (secret = value)} />
{/if}
<SecretDialog {secret} onClose={() => (secret = undefined)} />
<UnsubscribeDialog subscription={removing} onClose={() => (removing = undefined)} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .target { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; color: var(--text-primary); }
  .figure { color: var(--text-secondary); }
</style>
