<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One webhook as its record page (page-system §7.2, §9.3): its address, state and what it is
  // told in the head; its deliveries as a table narrowed by outcome and continued at its foot, each
  // a Dialog M; pause or resume, a new secret (Dialog M, then the secret once in a Dialog S) in the
  // head's ⋯, and unsubscribing in the danger zone.
  //
  // **The three states say different things.** `PAUSED` is somebody's decision, `DISABLED` is what
  // sustained unreachability concluded - so the failure count and the last error stand above the
  // deliveries, beside *Switch back on*, because whoever presses it should know what they re-enable.

  import { untrack } from 'svelte';

  import { Badge, Banner, DataTable, EmptyState, RecordPage, Select, Spinner, Stack, type DataColumn, type RecordList } from '@hubtask/design-system/components';
  import type { MenuItem } from '@hubtask/design-system/components';

  import { announcer } from '../lib/announce.svelte.ts';
  import { includedHere, mayHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { webhooks, type Delivery } from '../lib/data/webhooks.svelte.ts';
  import { isChat } from '../lib/data/webhooks.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import DeliveryDialog from '../lib/webhooks/DeliveryDialog.svelte';
  import RotateDialog from '../lib/webhooks/RotateDialog.svelte';
  import SecretDialog from '../lib/webhooks/SecretDialog.svelte';
  import UnsubscribeDialog from '../lib/webhooks/UnsubscribeDialog.svelte';
  import { eventWord, listensAt, outcomeTone, sentence, stateTone, stateWord, webhookScopes } from '../lib/webhooks/words.svelte.ts';

  interface Props {
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();

  const LIST = '/administration/webhooks';
  /** The outcomes the contract lets a caller narrow to. `DEAD_LETTER` is the one operators want. */
  const OUTCOMES = ['PENDING', 'SUCCEEDED', 'FAILED', 'DEAD_LETTER', 'FILTER_FAILED'];

  $effect(() => untrack(() => webhooks.open()));
  $effect(() => untrack(() => containers.start()));
  $effect(() => {
    const stops = containers.hubs.map((hub) => untrack(() => containers.openLevel(hub.id)));
    return () => stops.forEach((stop) => stop());
  });
  const scopes = $derived(webhookScopes());

  const subscription = $derived(webhooks.all.find((each) => each.id === id));
  const title = $derived(subscription?.target_url ?? t('app.webhooks.kind'));
  $effect(() => page.entitle(title));

  const mayAutomate = $derived(mayHere('AUTOMATION').status === 'permitted');
  const webhooksIncluded = $derived(includedHere('WEBHOOKS') === true);

  let outcome = $state('');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  $effect(() => {
    const status = outcome || undefined;
    untrack(() => void webhooks.readDeliveries(id, status).catch((cause) => (failure = renderProblem(cause as never, messages))));
  });
  const deliveryState = $derived(webhooks.deliveryState(id, outcome || undefined));
  const deliveries = $derived(webhooks.deliveriesOf(id, outcome || undefined));

  /** What the last press of *Show more* brought, said by `LoadMore`'s live region. */
  let arrived = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  let moreFailed = $state<string | undefined>(undefined);
  $effect(() => {
    void outcome;
    arrived = undefined;
    moreFailed = undefined;
  });

  async function more(): Promise<void> {
    const status = outcome || undefined;
    const before = webhooks.deliveriesOf(id, status).length;
    moreFailed = undefined;
    isLoadingMore = true;
    try {
      await webhooks.more(id, status);
      arrived = t('app.webhooks.arrived', { count: webhooks.deliveriesOf(id, status).length - before });
    } catch (error) {
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }

  async function setState(next: 'PAUSED' | 'ACTIVE'): Promise<void> {
    if (!subscription) return;
    failure = undefined;
    try {
      await webhooks.update(subscription.id, { state: next }, subscription.version);
      announcer.say(t(next === 'PAUSED' ? 'app.webhooks.paused_announced' : 'app.webhooks.resumed_announced'));
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    }
  }

  let opened = $state<Delivery | undefined>(undefined);
  let rotating = $state(false);
  let removing = $state(false);
  // The one place a secret exists in this client, for as long as its dialog is open.
  let secret = $state<string | undefined>(undefined);

  const columns = $derived<DataColumn[]>([
    { id: 'when', label: t('app.webhooks.delivery_when'), priority: 1 },
    { id: 'attempt', label: t('app.webhooks.delivery_attempts'), priority: 3 },
    { id: 'answer', label: t('app.webhooks.delivery_answer'), priority: 2 },
    { id: 'outcome', label: t('app.webhooks.delivery_outcome'), priority: 2, isStatus: true },
  ]);

  const lists = $derived<RecordList[]>([
    { id: 'events', title: t('app.webhooks.event_types'), count: subscription ? String(subscription.event_types.length) : undefined },
    { id: 'deliveries', title: t('app.webhooks.deliveries'), count: deliveryState.status === 'ready' ? String(deliveries.length) : undefined },
  ]);

  const menuItems = $derived<MenuItem[]>(
    !subscription || !mayAutomate
      ? []
      : [
          ...(subscription.state === 'ACTIVE'
            ? [{ id: 'pause', label: t('app.webhooks.pause') }]
            : webhooksIncluded ? [{ id: 'resume', label: t('app.webhooks.resume') }] : []),
          ...(!isChat(subscription.format) ? [{ id: 'rotate', label: t('app.webhooks.rotate_menu') }] : []),
        ],
  );

  const facts = $derived(
    subscription
      ? [
          t('app.webhooks.subscribed_since', { at: formatDateTime(subscription.created_at, messages.locale) }),
          t('app.webhooks.listens_at', { scope: listensAt(subscription, scopes) }),
          isChat(subscription.format) ? t(`app.webhooks.format_${(subscription.format ?? '').toLowerCase()}`) : undefined,
          subscription.room ? t('app.webhooks.room_named', { room: subscription.room }) : undefined,
        ].filter(Boolean).join(' · ')
      : undefined,
  );
</script>

<RecordPage
  record={{
    kind: t('app.webhooks.kind'),
    title,
    back: { label: t('app.webhooks.title'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status: subscription ? { label: stateWord(subscription), tone: stateTone(subscription) } : undefined,
    facts,
    edit: subscription && mayAutomate && subscription.state !== 'ACTIVE' && webhooksIncluded
      ? { label: subscription.state === 'DISABLED' ? t('app.webhooks.switch_back_on') : t('app.webhooks.resume'), onclick: () => void setState('ACTIVE') }
      : undefined,
    menu: menuItems.length > 0
      ? { label: t('app.page.more_for', { name: title }), items: menuItems, onselect: (item: string) => { if (item === 'rotate') rotating = true; else void setState(item === 'pause' ? 'PAUSED' : 'ACTIVE'); } }
      : undefined,
  }}
  {lists}
  danger={subscription && mayAutomate
    ? { title: t('app.page.danger_zone'), acts: [{ id: 'unsubscribe', title: t('app.webhooks.unsubscribe_title'), cost: t('app.webhooks.unsubscribe_note'), actionLabel: t('app.webhooks.unsubscribe_menu') }], onact: () => (removing = true) }
    : undefined}
  refused={webhooks.state.status === 'failed' || (webhooks.state.status === 'ready' && !subscription) ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    {#if item.id === 'events' && subscription}
      <Stack gap="100">
        <div class="chips">
          {#each subscription.event_types as type (type)}<Badge>{eventWord(type)}</Badge>{/each}
        </div>
        {#if subscription.filter}
          <!-- The condition as its author wrote it: configuration, shown to whoever may read it. -->
          <p class="quiet small">{t('app.webhooks.only_when')} <span class="mono">{subscription.filter}</span></p>
        {/if}
      </Stack>
    {:else if item.id === 'deliveries' && subscription}
      <Stack gap="150">
        {#if failure}<FailureNote problem={failure} />{/if}
        {#if subscription.state === 'DISABLED' || subscription.failure_count > 0}
          <Banner tone={subscription.state === 'DISABLED' ? 'danger' : 'warning'}>
            <Stack gap="025">
              <span>
                {subscription.state === 'DISABLED'
                  ? t('app.webhooks.disabled_note', { count: String(subscription.failure_count) })
                  : t('app.webhooks.failing_note', { count: String(subscription.failure_count) })}
              </span>
              {#if subscription.last_error}<span>{sentence(subscription.last_error)}</span>{/if}
            </Stack>
          </Banner>
        {/if}
        <div class="narrow">
          <Select
            label={t('app.webhooks.filter_outcome')}
            bind:value={outcome}
            placeholder={t('app.webhooks.all_outcomes')}
            options={OUTCOMES.map((value) => ({ value, label: t(`app.webhooks.outcome_${value.toLowerCase()}`) }))}
          />
        </div>
        {#if deliveryState.status === 'loading' || deliveryState.status === 'idle'}
          <p class="quiet"><Spinner label={t('app.webhooks.reading_deliveries')} /> <span>{t('app.webhooks.reading_deliveries')}</span></p>
        {:else if deliveryState.status === 'failed'}
          <p class="failure" role="alert">{renderProblem(deliveryState.error, messages).message}</p>
        {:else}
          <DataTable
            label={item.title}
            {columns}
            rows={deliveries}
            key={(each: Delivery) => each.id}
            onopen={(each: Delivery) => (opened = each)}
            rowLabel={(each: Delivery) => formatDateTime(each.created_at, messages.locale)}
            shownLabel={deliveries.length === 0 ? undefined : t('app.page.showing', { count: String(deliveries.length) })}
            more={webhooks.moreAfter(id, outcome || undefined) || arrived || moreFailed
              ? {
                  label: t('app.webhooks.more'),
                  busyLabel: t('app.webhooks.loading_more'),
                  hasMore: Boolean(webhooks.moreAfter(id, outcome || undefined)),
                  isBusy: isLoadingMore,
                  arrivedLabel: arrived,
                  failedLabel: moreFailed,
                  onLoadMore: () => void more(),
                }
              : undefined}
          >
            {#snippet cell(each: Delivery, column: DataColumn)}
              {#if column.id === 'when'}
                <span>{formatDateTime(each.created_at, messages.locale)}</span>
              {:else if column.id === 'attempt'}
                <span class="quiet">{each.attempt > 0 ? t('app.webhooks.attempt', { count: String(each.attempt) }) : ''}</span>
              {:else if column.id === 'answer'}
                <span class="quiet">{each.response_status ? t('app.webhooks.answered', { status: String(each.response_status) }) : (sentence(each.error_code) ?? '')}</span>
              {:else}
                <Badge tone={outcomeTone(each)}>{t(`app.webhooks.outcome_${each.status.toLowerCase()}`)}</Badge>
              {/if}
            {/snippet}
            {#snippet empty()}<EmptyState kind={outcome ? 'filtered' : 'settled'} title={t('app.webhooks.no_deliveries')} />{/snippet}
          </DataTable>
        {/if}
      </Stack>
    {/if}
  {/snippet}
</RecordPage>

<DeliveryDialog webhookId={id} delivery={opened} mayReplay={mayAutomate && webhooksIncluded} {outcome} onClose={() => (opened = undefined)} />
<RotateDialog subscription={rotating ? subscription : undefined} onClose={() => (rotating = false)} onRotated={(value) => (secret = value)} />
<SecretDialog {secret} onClose={() => (secret = undefined)} />
<UnsubscribeDialog subscription={removing ? subscription : undefined} onClose={() => (removing = false)} onRemoved={() => onnavigate(LIST)} />

{#snippet notHere()}
  {#if webhooks.state.status === 'failed'}
    <FailureNote problem={renderProblem(webhooks.state.error, messages)} />
  {:else}
    <EmptyState kind="settled" title={t('app.webhooks.not_here')} />
  {/if}
{/snippet}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .narrow { max-inline-size: var(--layout-field-md); }
</style>
