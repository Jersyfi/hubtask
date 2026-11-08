<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Events to the purchase platform (UC-INS-14): the addresses the installation's events are pushed
  // to - workspace created, plan changed, suspended, active again, deletion requested, cancelled and
  // done, owner changed, limit reached, AI budget spent, export finished - identifiers and states
  // only.
  //
  // **An enforcing instance file decides.** Its subscriptions come marked `from_file`, and while one
  // stands the screen offers no add, pause or remove - the server would refuse each.
  //
  // **Never switched off silently.** Unlike a workspace's webhook, a subscription stays on however
  // long the platform is unreachable: what did not arrive waits as dead letters, counted on its row,
  // and the notice says the subscription stays on and the platform can read what it missed itself.
  // Redelivering is offered only where there is something to redeliver and the subscription is on.
  //
  // **Adding one needs a fresh proof**, beyond the raised hour: every workspace's reference goes to
  // that address. The secret is shown once and never again.

  import { tick, untrack } from 'svelte';

  import {
    Badge,
    Banner,
    Button,
    DataTable,
    Dialog,
    EmptyState,
    FilterBar,
    Input,
    ListPage,
    OneTimeSecret,
    Spinner,
    Stack,
    type DataColumn,
    type MenuItem,
    type PageAction,
  } from '@hubtask/design-system/components';
  import { actionsOfHead } from '../lib/palette/actions.ts';
  import { commands } from '../lib/palette/commands.svelte.ts';
  import type { PlatformSubscription, PlatformSubscriptionSecret } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { platformSubscriptions } from '../lib/data/platformsubscriptions.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { focusFirst } from '../lib/forms/missing.ts';

  /** Whether *Add* was pressed without an address: marked at the field (UC-WRK-32 check 3). */
  let targetMissing = $state(false);
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let isAdding = $state(false);
  let target = $state('');
  let minted = $state<PlatformSubscriptionSecret | undefined>(undefined);
  let removing = $state<PlatformSubscription | undefined>(undefined);
  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  // Single mode offers none of this (UC-INS-14 check 7): nothing is read, and the screen says why
  // to whoever reaches its address.
  // Read only once the manifest says multi mode: before it arrives nothing is offered.
  const isSingle = $derived(manifest.isSingleTenant);
  const isMulti = $derived(manifest.isMultiTenant);

  $effect(() => {
    if (!isMulti || !instance.isElevated) return;
    const stop = untrack(() => platformSubscriptions.openSubscriptions());
    return stop;
  });

  const reading = $derived(platformSubscriptions.subscriptionsState);
  const subscriptions = $derived(platformSubscriptions.subscriptions);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const waiting = $derived(subscriptions.some((each) => each.dead_letters > 0));
  const isFromFile = $derived(subscriptions.some((each) => each.from_file));

  const columns = $derived<DataColumn[]>([
    { id: 'target', label: t('app.instance.platform_column_target'), priority: 1 },
    { id: 'delivery', label: t('app.instance.platform_column_delivery'), priority: 2, isStatus: true },
    { id: 'last', label: t('app.instance.platform_column_last'), priority: 3 },
  ]);

  let search = $state('');
  const shown = $derived(subscriptions.filter((each) => matchesSearch(search, [each.target_url])));

  /**
   * A subscription's ⋯: deliver again what waits, pause or resume, and remove it last. One the file
   * sets offers only the redelivery; in the menu, so a phone's sheet offers it too.
   */
  function rowMenu(subscription: PlatformSubscription) {
    const items: MenuItem[] = [
      ...(subscription.dead_letters > 0 && subscription.state === 'ACTIVE' ? [{ id: 'redeliver', label: t('app.instance.platform_redeliver') }] : []),
      ...(subscription.from_file
        ? []
        : [
            { id: 'state', label: subscription.state === 'PAUSED' ? t('app.instance.platform_resume') : t('app.instance.platform_pause') },
            { id: 'remove', label: `${t('app.instance.platform_remove')}…`, isDestructive: true, hasSeparatorBefore: true },
          ]),
    ];
    return items.length === 0 ? undefined : { label: t('app.page.more_for', { name: subscription.target_url }), items };
  }

  function onRowMenu(subscription: PlatformSubscription, id: string): void {
    if (id === 'redeliver') {
      void run(`redeliver:${subscription.id}`, () => platformSubscriptions.redeliver(subscription.id), t('app.instance.platform_redelivered_announced'));
    } else if (id === 'state') {
      void run(`state:${subscription.id}`, () => platformSubscriptions.setState(subscription.id, subscription.state === 'PAUSED' ? 'ACTIVE' : 'PAUSED'));
    } else {
      removing = subscription;
      failure = undefined;
    }
  }

  const last = (subscription: PlatformSubscription): string =>
    subscription.last_delivery_at
      ? formatRelative(subscription.last_delivery_at, messages.locale)
      : t('app.instance.platform_never_delivered');

  async function run(id: string, call: () => Promise<unknown>, said?: string): Promise<boolean> {
    working = id;
    failure = undefined;
    try {
      await call();
      if (said) announcer.say(said);
      return true;
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
      return false;
    } finally {
      working = undefined;
    }
  }

  async function add(): Promise<void> {
    targetMissing = target.trim() === '';
    if (targetMissing) {
      void tick().then(() => focusFirst(document));
      return;
    }
    let answer: PlatformSubscriptionSecret | undefined;
    const added = await run('add', async () => {
      answer = await platformSubscriptions.subscribe(target.trim());
    });
    if (added && answer) {
      minted = answer;
      isAdding = false;
      target = '';
    }
  }

  $effect(() => page.entitle(t('app.instance.platform')));

  /** The head's one action, handed to the head and to the palette alike (UC-WRK-27 check 2). */
  const headPrimary = $derived<PageAction | undefined>(
    !isMulti || isFromFile ? undefined : { label: t('app.instance.platform_add'), icon: 'plus', onclick: () => ((isAdding = true), (failure = undefined)) },
  );
  $effect(() => commands.offer('page', actionsOfHead('page', { primary: headPrimary })));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <ListPage
      head={{ title: t('app.instance.platform'), breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.platform') }), primary: headPrimary }}
      refused={door ?? (unreadable ? failed : undefined)}
    >
      {#snippet intro()}
        <Stack gap="100">
          <p class="quiet">{t('app.instance.platform_sent')}</p>
          {#if !isSingle && waiting}
            <Banner tone="warning" title={t('app.instance.platform_unreachable_title')}>{t('app.instance.platform_unreachable_body')}</Banner>
          {/if}
        </Stack>
      {/snippet}
      {#snippet filters()}
        {#if !isSingle}
          <FilterBar
            searchLabel={t('app.instance.platform_search')}
            clearSearchLabel={t('app.page.clear_search')}
            bind:search
            countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(subscriptions.length) }) : undefined}
          />
        {/if}
      {/snippet}

      {#if failure && !removing && !isAdding}<FailureNote problem={failure} />{/if}
      {#if isSingle}
        <Banner tone="info">{t('app.instance.platform_single')}</Banner>
      {:else if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else}
        <DataTable
          label={t('app.instance.platform')}
          {columns}
          rows={shown}
          key={(subscription: PlatformSubscription) => subscription.id}
          rowLabel={(subscription: PlatformSubscription) => subscription.target_url}
          menu={rowMenu}
          onmenu={onRowMenu}
          actionsLabel={t('app.page.row_actions')}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          shownLabel={subscriptions.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
        >
          {#snippet cell(subscription: PlatformSubscription, column: DataColumn)}
            {#if column.id === 'target'}
              <span class="mono">{subscription.target_url}</span>
            {:else if column.id === 'delivery'}
              {#if subscription.state === 'PAUSED'}
                <Badge tone="neutral">{t('app.instance.platform_paused')}</Badge>
              {:else if subscription.dead_letters > 0}
                <Badge tone="warning">{t('app.instance.platform_dead_letters', { count: subscription.dead_letters })}</Badge>
              {:else}
                <Badge tone="success">{t('app.instance.platform_delivering')}</Badge>
              {/if}
              {#if working === `state:${subscription.id}` || working === `redeliver:${subscription.id}`}<Spinner label={t('app.instance.working')} />{/if}
            {:else}
              <span class="quiet">{last(subscription)}</span>
            {/if}
          {/snippet}
          {#snippet empty()}
            {#if subscriptions.length === 0}
              <EmptyState kind="unused" title={t('app.instance.platform_none')} />
            {:else}
              <EmptyState kind="filtered" title={t('app.instance.platform_none_found')}>
                {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
              </EmptyState>
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </ListPage>
  {/snippet}
</InstanceGate>

{#snippet failed()}
  {#if unreadable}<FailureNote problem={unreadable} />{/if}
{/snippet}

<!-- The secret once, in a dialog of its own: it is shown here and nowhere else, ever. -->
<Dialog size="s" title={t('app.instance.platform_secret_title')} isOpen={minted !== undefined} dismissLabel={t('app.apps.done')} onClose={() => (minted = undefined)}>
  {#if minted}
    <OneTimeSecret
      value={minted.secret}
      label={t('app.instance.platform_secret_title')}
      hint={t('app.instance.platform_secret_hint')}
      revealLabel={t('app.apps.reveal')}
      hideLabel={t('app.apps.hide')}
      copyLabel={t('app.apps.copy')}
      copiedLabel={t('app.apps.copied')}
      acknowledgementLabel={t('app.apps.kept')}
      notAcknowledgedReason={t('app.apps.keep_first')}
      dismissLabel={t('app.apps.done')}
      onDismiss={() => (minted = undefined)}
    />
  {/if}
</Dialog>

<Dialog
  size="s"
  title={t('app.instance.platform_add')}
  isOpen={isAdding}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (isAdding = false)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isAdding = false)}>{t('app.instance.cancel')}</Button>
    <Button
      tone="primary"
      isBusy={working === 'add'}
      busyLabel={t('app.instance.working')}
      onclick={() => void add()}
    >
      {t('app.instance.platform_add_confirm')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure}
      <FailureNote problem={failure} />
    {/if}
    <Input
      label={t('app.instance.platform_target_label')}
      hint={t('app.instance.platform_target_hint')}
      bind:value={target}
      type="url"
      autocomplete="off"
      spellcheck={false}
      isRequired
      error={targetMissing && !target.trim() ? t('app.instance.platform_target_required') : undefined}
    />
  </Stack>
</Dialog>

<Dialog
  size="s"
  title={t('app.instance.platform_remove_title')}
  isOpen={removing !== undefined}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (removing = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (removing = undefined)}>{t('app.instance.keep')}</Button>
    <Button
      tone="danger"
      isBusy={working === 'remove'}
      busyLabel={t('app.instance.working')}
      onclick={async () => {
        const removed = await run('remove', () => platformSubscriptions.remove(removing?.id ?? ''),
          t('app.instance.platform_removed_announced'));
        if (removed) removing = undefined;
      }}
    >
      {t('app.instance.platform_remove')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure}
      <FailureNote problem={failure} />
    {/if}
    <Banner tone="warning">{t('app.instance.platform_remove_note', { target: removing?.target_url ?? '' })}</Banner>
  </Stack>
</Dialog>

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
</style>
