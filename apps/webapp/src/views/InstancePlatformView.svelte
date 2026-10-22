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

  import { untrack } from 'svelte';

  import {
    Badge,
    Banner,
    Button,
    Dialog,
    Input,
    OneTimeSecret,
    PageHeader,
    Spinner,
    Stack,
    Table,
    type PageAction,
  } from '@hubtask/design-system/components';
  import { actionsOfHead } from '../lib/palette/actions.ts';
  import { commands } from '../lib/palette/commands.svelte.ts';
  import type { PlatformSubscription, PlatformSubscriptionSecret } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { platformSubscriptions } from '../lib/data/platformsubscriptions.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

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
    if (!isMulti) return;
    const stop = untrack(() => platformSubscriptions.openSubscriptions());
    return stop;
  });

  const reading = $derived(platformSubscriptions.subscriptionsState);
  const subscriptions = $derived(platformSubscriptions.subscriptions);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const waiting = $derived(subscriptions.some((each) => each.dead_letters > 0));
  const isFromFile = $derived(subscriptions.some((each) => each.from_file));

  const columns = $derived([
    { id: 'target', label: t('app.instance.platform_column_target') },
    { id: 'delivery', label: t('app.instance.platform_column_delivery') },
    { id: 'last', label: t('app.instance.platform_column_last') },
    { id: 'actions', label: t('app.instance.column_actions'), isLabelHidden: true },
  ]);

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
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
      return false;
    } finally {
      working = undefined;
    }
  }

  async function add(): Promise<void> {
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

<Stack gap="300">
  <PageHeader
    title={t('app.instance.platform')}
    isTitleInBar={viewport.isCompact}
    primary={headPrimary}
  />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
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

      {#if failure && !removing && !isAdding}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {/if}

      {#if isSingle}
        <Banner tone="info">{t('app.instance.platform_single')}</Banner>
      {:else if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else}
        {#if subscriptions.length === 0}
          <Banner tone="info">{t('app.instance.platform_none')}</Banner>
        {:else}
          {#snippet delivery(subscription: PlatformSubscription)}
            {#if subscription.state === 'PAUSED'}
              <Badge tone="neutral">{t('app.instance.platform_paused')}</Badge>
            {:else if subscription.dead_letters > 0}
              <Badge tone="warning">{t('app.instance.platform_dead_letters', { count: subscription.dead_letters })}</Badge>
            {:else}
              <Badge tone="success">{t('app.instance.platform_delivering')}</Badge>
            {/if}
          {/snippet}
          {#snippet controls(subscription: PlatformSubscription)}
            <div class="row">
              {#if subscription.dead_letters > 0 && subscription.state === 'ACTIVE'}
                <Button
                  isBusy={working === `redeliver:${subscription.id}`}
                  busyLabel={t('app.instance.working')}
                  onclick={() =>
                    void run(`redeliver:${subscription.id}`, () => platformSubscriptions.redeliver(subscription.id),
                      t('app.instance.platform_redelivered_announced'))}
                >
                  {t('app.instance.platform_redeliver')}
                </Button>
              {/if}
              {#if !subscription.from_file}
                <Button
                  tone="subtle"
                  isBusy={working === `state:${subscription.id}`}
                  busyLabel={t('app.instance.working')}
                  onclick={() =>
                    void run(`state:${subscription.id}`, () =>
                      platformSubscriptions.setState(subscription.id, subscription.state === 'PAUSED' ? 'ACTIVE' : 'PAUSED'))}
                >
                  {subscription.state === 'PAUSED' ? t('app.instance.platform_resume') : t('app.instance.platform_pause')}
                </Button>
                <Button tone="danger" onclick={() => ((removing = subscription), (failure = undefined))}>
                  {t('app.instance.platform_remove')}
                </Button>
              {/if}
            </div>
          {/snippet}
          {#if viewport.isCompact}
            <!-- A phone has no room for four columns: each subscription reads top to bottom, its
                 address whole on a line of its own. -->
            <ul class="cards">
              {#each subscriptions as subscription (subscription.id)}
                <li class="card">
                  <span class="mono">{subscription.target_url}</span>
                  <span class="meta">{@render delivery(subscription)}<span class="quiet">{last(subscription)}</span></span>
                  {@render controls(subscription)}
                </li>
              {/each}
            </ul>
          {:else}
            <Table label={t('app.instance.platform')} isLabelHidden columns={columns}>
              {#each subscriptions as subscription (subscription.id)}
                <tr>
                  <td><span class="mono">{subscription.target_url}</span></td>
                  <td>{@render delivery(subscription)}</td>
                  <td><span class="quiet">{last(subscription)}</span></td>
                  <td>{@render controls(subscription)}</td>
                </tr>
              {/each}
            </Table>
          {/if}
        {/if}

        {#if waiting}
          <Banner tone="warning" title={t('app.instance.platform_unreachable_title')}>
            {t('app.instance.platform_unreachable_body')}
          </Banner>
        {/if}
        <p class="quiet prose">{t('app.instance.platform_sent')}</p>
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<Dialog
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
      disabledReason={target.trim() === '' ? t('app.instance.platform_target_required') : undefined}
      onclick={() => void add()}
    >
      {t('app.instance.platform_add_confirm')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure}
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
      </Banner>
    {/if}
    <Input
      label={t('app.instance.platform_target_label')}
      hint={t('app.instance.platform_target_hint')}
      bind:value={target}
      type="url"
      autocomplete="off"
      spellcheck={false}
      isRequired
    />
  </Stack>
</Dialog>

<Dialog
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
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
      </Banner>
    {/if}
    <Banner tone="warning">{t('app.instance.platform_remove_note', { target: removing?.target_url ?? '' })}</Banner>
  </Stack>
</Dialog>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .cards { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .card {
    display: grid; gap: var(--sp-100); padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }
  .meta { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
</style>
