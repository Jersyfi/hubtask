<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What an external system is told, and what happened to each telling (`automation.md` §5).
  //
  // **The secret appears once, and this screen keeps no copy.** It is held in one `$state` for as
  // long as the panel is open and dropped when it closes; nothing writes it to storage, to the
  // address, or to a message.
  //
  // **The event types come from the manifest**, never from a list typed here: a type this build
  // does not emit is refused rather than stored, so the picker offers exactly what the server
  // accepts and a subscription cannot quietly wait for something that will never arrive.
  //
  // **The target URL is not validated here.** A private range or the cloud metadata address is
  // refused by the guarded client unless the installation deliberately released private networks
  // (T-07), and that is the installation's answer rather than this screen's — so the refusal is
  // rendered where the field is, and nothing is predicted.
  //
  // **The condition is written, not assembled.** It is the automation's language over `event`,
  // `data`, `actor` and `now`, compiled by the server when it is saved and refused naming where -
  // this screen does not parse it, because a second parser would be a second dialect. The scope
  // offers the hubs and collections the reader sees, as the rule editor does; the form is open
  // only to whoever may automate the workspace, which reaches each of them.
  //
  // **The three states say different things.** `PAUSED` is somebody's decision, `DISABLED` is what
  // sustained unreachability concluded — so the failure count and the last error are beside the
  // control that re-enables it, because whoever presses it should know what they are re-enabling.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Checkbox, Input, LoadMore, OneTimeSecret, PageHeader, Select, Spinner, Stack, Textarea } from '@hubtask/design-system/components';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import {
    webhooks,
    type Delivery,
    type Subscription,
    type SubscriptionWithSecret,
  } from '../lib/data/webhooks.svelte.ts';
  import { scopeChoiceOf, subscriptionBody } from '../lib/data/webhooks.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { mayHere } from '../lib/data/capability.svelte.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  /** The outcomes the contract lets a caller narrow to. `DEAD_LETTER` is the one operators want. */
  const OUTCOMES = ['PENDING', 'SUCCEEDED', 'FAILED', 'DEAD_LETTER', 'FILTER_FAILED'];

  /** The rotation choices, in seconds. Zero is its own decision and says so. */
  const GRACE_CHOICES = ['3600', '86400', '604800', '0'];

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  // The one place a secret exists in this client, for as long as its panel is open.
  let secret = $state<SubscriptionWithSecret | undefined>(undefined);

  let draftUrl = $state('');
  let draftTypes = $state<readonly string[]>([]);
  let draftCondition = $state('');
  let draftScope = $state('TENANT');

  let opened = $state('');
  let outcome = $state('');
  let rotating = $state('');
  let grace = $state('86400');
  let removing = $state('');

  $effect(() => untrack(() => webhooks.open()));
  $effect(() => untrack(() => containers.start()));
  $effect(() => {
    const stops = containers.hubs.map((hub) => untrack(() => containers.openLevel(hub.id)));
    return () => stops.forEach((stop) => stop());
  });

  /** Where a subscription may listen: the workspace, a hub with its collections, or one collection. */
  const scopes = $derived([
    { value: 'TENANT', label: t('app.rules.scope_tenant') },
    ...containers.hubs.map((hub) => ({ value: `HUB:${hub.id}`, label: t('app.rules.scope_hub', { name: hub.name }) })),
    ...containers.hubs.flatMap((hub) =>
      containers.collectionsOf(hub.id).map((collection) => ({
        value: `COLLECTION:${collection.id}`,
        label: t('app.rules.scope_collection', { name: `${hub.name} · ${collection.name}` }),
      })),
    ),
  ]);

  /** Where a stored subscription listens, in the picker's words; a container this reader does not see is named by nothing more. */
  const listensAt = (subscription: Subscription) => {
    const choice = scopeChoiceOf(subscription.scope);
    return scopes.find((each) => each.value === choice)?.label ?? t('app.webhooks.scope_unseen');
  };

  /** What the last press of *Show more* brought, said by `LoadMore`'s live region; cleared by another subscription or outcome. */
  let arrived = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  /** Why the last press of *Show more* brought nothing; cleared by the next press and by another subscription or outcome. */
  let moreFailed = $state<string | undefined>(undefined);
  $effect(() => {
    void opened;
    void outcome;
    arrived = undefined;
    moreFailed = undefined;
  });

  /** A subscription's next page of deliveries, appended, and what arrived said once it has (P-13). */
  async function moreDeliveries(webhookId: string): Promise<void> {
    const status = outcome || undefined;
    const before = webhooks.deliveriesOf(webhookId, status).length;
    moreFailed = undefined;
    isLoadingMore = true;
    try {
      await webhooks.more(webhookId, status);
      arrived = t('app.webhooks.arrived', { count: webhooks.deliveriesOf(webhookId, status).length - before });
    } catch (error) {
      // Beside the button that asked, not in the page's banner: the deliveries already shown are still right.
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }

  const reading = $derived(webhooks.state);
  const listed = $derived(webhooks.all);
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  // Exactly what the manifest declares, and nothing while it has not been read: a picker offering
  // a guess would be offering a choice the server refuses at the end.
  const eventTypes = $derived<readonly string[]>(manifest.value?.event_types ?? []);

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : undefined;

  const sentence = (code: string | null | undefined) =>
    code ? (messages.has(code) ? t(code) : code) : undefined;

  const stateWord = (subscription: Subscription) =>
    t(`app.webhooks.state_${subscription.state.toLowerCase()}`);

  const stateTone = (subscription: Subscription) =>
    subscription.state === 'ACTIVE' ? 'success' : subscription.state === 'PAUSED' ? 'neutral' : 'danger';

  const outcomeTone = (delivery: Delivery) =>
    delivery.status === 'SUCCEEDED'
      ? 'success'
      : delivery.status === 'DEAD_LETTER'
        ? 'danger'
        : delivery.status === 'FAILED' || delivery.status === 'FILTER_FAILED'
          ? 'warning'
          : 'neutral';

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
      // Said out loud on success (4.1.3): what changed is on the screen, and a reader who cannot
      // see the screen is told the same thing once, through the one live region.
      if (said) announcer.say(said);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  function toggleType(type: string, wanted: boolean): void {
    draftTypes = wanted
      ? [...draftTypes, type]
      : draftTypes.filter((declared) => declared !== type);
  }

  async function subscribe(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!draftUrl.trim() || draftTypes.length === 0) return;
    await attempt(async () => {
      secret = await webhooks.subscribe(
        subscriptionBody({
          target_url: draftUrl,
          event_types: draftTypes,
          condition: draftCondition,
          scope: draftScope,
        }),
      );
      draftUrl = '';
      draftTypes = [];
      draftCondition = '';
      draftScope = 'TENANT';
    }, t('app.webhooks.subscribed_announced'));
  }

  async function openDeliveries(webhookId: string): Promise<void> {
    if (opened === webhookId) {
      opened = '';
      return;
    }
    opened = webhookId;
    await attempt(() => webhooks.readDeliveries(webhookId, outcome || undefined));
  }
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.webhooks.title')));

  /** Whether the reader may change the workspace's automation (`AUTOMATION`), as the server asks. */
  const mayAutomate = $derived(mayHere('AUTOMATION').status === 'permitted');
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.webhooks.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'webhooks', label: t('app.webhooks.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

    <Stack gap="300">
    <p class="quiet">{t('app.webhooks.intro')}</p>

    {#if secret}
      <!-- The only time this value exists anywhere but the server's sealed copy. The
           acknowledgement is required, because a secret scrolled past is a secret nobody kept. -->
      <OneTimeSecret
        value={secret.secret}
        label={t('app.webhooks.secret_title')}
        hint={t('app.webhooks.secret_hint')}
        revealLabel={t('app.webhooks.reveal')}
        hideLabel={t('app.webhooks.hide')}
        copyLabel={t('app.webhooks.copy')}
        copiedLabel={t('app.webhooks.copied')}
        acknowledgementLabel={t('app.webhooks.kept')}
        notAcknowledgedReason={t('app.webhooks.keep_first')}
        dismissLabel={t('app.webhooks.done')}
        onDismiss={() => (secret = undefined)}
      />
    {/if}

    {#if failure}
      <Banner tone="danger" title={failure.message}>
        <Stack gap="050">
          {#each [...failure.fields] as [path, message] (path)}
            <span>{message}</span>
          {/each}
          {#if failure.reference}
            <span>{t('app.error_reference', { request_id: failure.reference })}</span>
          {/if}
        </Stack>
      </Banner>
    {/if}

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting">
        <Spinner label={t('app.webhooks.reading')} /> <span>{t('app.webhooks.reading')}</span>
      </p>
    {:else if refusal}
      <Banner tone="danger" title={refusal.message}>
        {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
      </Banner>
    {:else}
      {#each listed as subscription (subscription.id)}
        <section class="panel">
          <Stack gap="100">
            <div class="row">
              <span class="mono target">{subscription.target_url}</span>
              <Badge tone={stateTone(subscription)}>{stateWord(subscription)}</Badge>
            </div>

            <p class="quiet small">
              {t('app.webhooks.subscribed_since', { at: when(subscription.created_at) ?? '' })}
            </p>

            <div class="row">
              {#each subscription.event_types as type (type)}
                <Badge>{type}</Badge>
              {/each}
            </div>

            <p class="quiet small">{t('app.webhooks.listens_at', { scope: listensAt(subscription) })}</p>
            {#if subscription.filter}
              <!-- The condition as its author wrote it: configuration, shown to whoever may read it. -->
              <p class="quiet small">
                {t('app.webhooks.only_when')} <span class="mono">{subscription.filter}</span>
              </p>
            {/if}

            {#if subscription.state === 'DISABLED' || subscription.failure_count > 0}
              <!-- What the person pressing "switch it back on" is switching back on. `DISABLED` is
                   a conclusion the system reached, not a setting somebody chose. -->
              <Banner tone={subscription.state === 'DISABLED' ? 'danger' : 'warning'}>
                <Stack gap="025">
                  <span>
                    {subscription.state === 'DISABLED'
                      ? t('app.webhooks.disabled_note', { count: String(subscription.failure_count) })
                      : t('app.webhooks.failing_note', { count: String(subscription.failure_count) })}
                  </span>
                  {#if subscription.last_error}
                    <span>{sentence(subscription.last_error)}</span>
                  {/if}
                </Stack>
              </Banner>
            {/if}

            <div class="row">
              {#if !mayAutomate}
                <!-- Pause, rotate and unsubscribe are absent; the form's gate below says why. -->
              {:else if subscription.state === 'ACTIVE'}
                <Button
                  size="sm"
                  tone="secondary"
                  isBusy={isWorking}
                  busyLabel={t('app.webhooks.saving')}
                  onclick={() =>
                    void attempt(() =>
                      webhooks.update(subscription.id, { state: 'PAUSED' }, subscription.version), t('app.webhooks.paused_announced')
                    )}
                >
                  {t('app.webhooks.pause')}
                </Button>
              {:else}
                <Button
                  size="sm"
                  tone="primary"
                  isBusy={isWorking}
                  busyLabel={t('app.webhooks.saving')}
                  onclick={() =>
                    void attempt(() =>
                      webhooks.update(subscription.id, { state: 'ACTIVE' }, subscription.version), t('app.webhooks.resumed_announced')
                    )}
                >
                  {t('app.webhooks.resume')}
                </Button>
              {/if}
              <Button size="sm" tone="subtle" onclick={() => void openDeliveries(subscription.id)}>
                {opened === subscription.id
                  ? t('app.webhooks.hide_deliveries')
                  : t('app.webhooks.show_deliveries')}
              </Button>
              {#if mayAutomate}
              <Button
                size="sm"
                tone="secondary"
                onclick={() => (rotating = rotating === subscription.id ? '' : subscription.id)}
              >
                {t('app.webhooks.rotate')}
              </Button>
              <Button
                size="sm"
                tone="danger"
                onclick={() => (removing = removing === subscription.id ? '' : subscription.id)}
              >
                {t('app.webhooks.unsubscribe')}
              </Button>
              {/if}
            </div>

            {#if rotating === subscription.id}
              <!-- What rotation costs, before it is pressed. Signatures use the new secret from
                   the moment of the call; the grace period is the subscriber's side of the check,
                   which is the half that cannot be deployed atomically. -->
              <Banner tone="warning" title={t('app.webhooks.rotate_title')}>
                <Stack gap="100">
                  <span>{t('app.webhooks.rotate_note')}</span>
                  <Select
                    label={t('app.webhooks.grace')}
                    hint={t('app.webhooks.grace_hint')}
                    bind:value={grace}
                    options={GRACE_CHOICES.map((seconds) => ({
                      value: seconds,
                      label: t(`app.webhooks.grace_${seconds}`),
                    }))}
                  />
                  <div class="row">
                    <Button
                      size="sm"
                      tone="primary"
                      isBusy={isWorking}
                      busyLabel={t('app.webhooks.rotating')}
                      onclick={() =>
                        void attempt(async () => {
                          secret = await webhooks.rotate(
                            subscription.id,
                            Number.parseInt(grace, 10),
                          );
                          rotating = '';
                        }, t('app.webhooks.rotated_announced'))}
                    >
                      {t('app.webhooks.rotate_confirm')}
                    </Button>
                    <Button size="sm" tone="subtle" onclick={() => (rotating = '')}>
                      {t('app.workspace.cancel')}
                    </Button>
                  </div>
                </Stack>
              </Banner>
            {/if}

            {#if removing === subscription.id}
              <Banner tone="danger" title={t('app.webhooks.unsubscribe_title')}>
                <Stack gap="100">
                  <!-- The deliveries go with it: a log of attempts against an address the
                       workspace no longer knows is a record of nothing. -->
                  <span>{t('app.webhooks.unsubscribe_note')}</span>
                  <div class="row">
                    <Button
                      size="sm"
                      tone="danger"
                      isBusy={isWorking}
                      busyLabel={t('app.webhooks.unsubscribing')}
                      onclick={() =>
                        void attempt(async () => {
                          await webhooks.unsubscribe(subscription.id);
                          removing = '';
                        }, t('app.webhooks.unsubscribed_announced'))}
                    >
                      {t('app.webhooks.unsubscribe_confirm')}
                    </Button>
                    <Button size="sm" tone="subtle" onclick={() => (removing = '')}>
                      {t('app.workspace.cancel')}
                    </Button>
                  </div>
                </Stack>
              </Banner>
            {/if}

            {#if opened === subscription.id}
              {@const state = webhooks.deliveryState(subscription.id, outcome || undefined)}
              <Stack gap="100">
                <Select
                  label={t('app.webhooks.filter_outcome')}
                  bind:value={outcome}
                  placeholder={t('app.webhooks.all_outcomes')}
                  options={OUTCOMES.map((value) => ({
                    value,
                    label: t(`app.webhooks.outcome_${value.toLowerCase()}`),
                  }))}
                  onchange={() =>
                    void attempt(() =>
                      webhooks.readDeliveries(subscription.id, outcome || undefined),
                    )}
                />

                {#if state.status === 'loading'}
                  <p class="waiting">
                    <Spinner label={t('app.webhooks.reading_deliveries')} />
                    <span>{t('app.webhooks.reading_deliveries')}</span>
                  </p>
                {:else if state.status === 'failed'}
                  <p class="failure" role="alert">{renderProblem(state.error, messages).message}</p>
                {:else}
                  <ul class="deliveries">
                    {#each webhooks.deliveriesOf(subscription.id, outcome || undefined) as delivery (delivery.id)}
                      <li>
                        <Stack gap="025">
                          <div class="row">
                            <Badge tone={outcomeTone(delivery)}>
                              {t(`app.webhooks.outcome_${delivery.status.toLowerCase()}`)}
                            </Badge>
                            {#if delivery.attempt > 0}
                              <span class="quiet small">
                                {t('app.webhooks.attempt', { count: String(delivery.attempt) })}
                              </span>
                            {/if}
                            {#if delivery.response_status}
                              <span class="quiet small">
                                {t('app.webhooks.answered', {
                                  status: String(delivery.response_status),
                                })}
                              </span>
                            {/if}
                            <span class="quiet small">{when(delivery.created_at)}</span>
                          </div>
                          <span class="mono small">{delivery.event_id}</span>
                          {#if delivery.error_code}
                            <!-- The code as a sentence, never the target's response body. -->
                            <span class="failure">{sentence(delivery.error_code)}</span>
                          {/if}
                          {#if delivery.next_attempt_at}
                            <span class="quiet small">
                              {t('app.webhooks.next_attempt', {
                                at: when(delivery.next_attempt_at) ?? '',
                              })}
                            </span>
                          {/if}
                          {#if delivery.status === 'DEAD_LETTER' && mayAutomate}
                            <Stack gap="025">
                              <!-- Said before the button: the replay carries the same event id, so
                                   a subscriber that deduplicates recognises the repeat. That is
                                   what makes it safe to press. -->
                              <span class="quiet small">{t('app.webhooks.replay_note')}</span>
                              <div>
                                <Button
                                  size="sm"
                                  tone="primary"
                                  isBusy={isWorking}
                                  busyLabel={t('app.webhooks.replaying')}
                                  onclick={() =>
                                    void attempt(async () => {
                                      await webhooks.replay(subscription.id, delivery.id);
                                      await webhooks.readDeliveries(
                                        subscription.id,
                                        outcome || undefined,
                                      );
                                    }, t('app.webhooks.replayed_announced'))}
                                >
                                  {t('app.webhooks.replay')}
                                </Button>
                              </div>
                            </Stack>
                          {/if}
                        </Stack>
                      </li>
                    {:else}
                      <li class="quiet">{t('app.webhooks.no_deliveries')}</li>
                    {/each}
                  </ul>

                  <!-- Drawn whole while something is to be said after the last page: the live
                       regions go with the component, and one removed with the button is one
                       nobody hears. -->
                  {#if webhooks.moreAfter(subscription.id, outcome || undefined) || arrived || moreFailed}
                    <LoadMore
                      label={t('app.webhooks.more')}
                      busyLabel={t('app.webhooks.loading_more')}
                      hasMore={Boolean(webhooks.moreAfter(subscription.id, outcome || undefined))}
                      isBusy={isLoadingMore}
                      arrivedLabel={arrived}
                      failedLabel={moreFailed}
                      onLoadMore={() => void moreDeliveries(subscription.id)}
                    />
                  {/if}
                {/if}
              </Stack>
            {/if}
          </Stack>
        </section>
      {:else}
        <p class="quiet">{t('app.webhooks.none')}</p>
      {/each}
    {/if}

    <WorkspaceGate permission="AUTOMATION">
      <form class="panel" onsubmit={subscribe}>
        <Stack gap="150">
          <h2 class="section">{t('app.webhooks.add_title')}</h2>
          <Input
            label={t('app.webhooks.target_url')}
            hint={t('app.webhooks.target_url_hint')}
            bind:value={draftUrl}
            isRequired
          />

          <fieldset class="types">
            <legend>{t('app.webhooks.event_types')}</legend>
            {#if eventTypes.length === 0}
              <!-- Nothing is knowable before the manifest is read, and a guess would be a choice the
                   server refuses at the end. -->
              <p class="quiet small">{t('app.webhooks.types_unknown')}</p>
            {:else}
              <div class="type-grid">
                {#each eventTypes as type (type)}
                  <Checkbox
                    label={type}
                    checked={draftTypes.includes(type)}
                    onchange={(event) =>
                      toggleType(type, (event.currentTarget as HTMLInputElement).checked)}
                  />
                {/each}
              </div>
            {/if}
          </fieldset>
          <p class="quiet small">{t('app.webhooks.types_note')}</p>

          <Select
            label={t('app.webhooks.scope')}
            hint={t('app.webhooks.scope_hint')}
            error={failure?.fields.get('/scope') ?? failure?.fields.get('/scope/id')}
            bind:value={draftScope}
            options={scopes}
          />
          <Textarea
            label={t('app.webhooks.condition')}
            hint={t('app.webhooks.condition_hint')}
            error={failure?.fields.get('/filter')}
            bind:value={draftCondition}
            rows={2}
            spellcheck="false"
          />
          <p class="quiet small">{t('app.webhooks.egress_note')}</p>

          <div>
            <Button
              type="submit"
              tone="primary"
              isBusy={isWorking}
              busyLabel={t('app.webhooks.subscribing')}
            >
              {t('app.webhooks.subscribe')}
            </Button>
          </div>
        </Stack>
      </form>
    </WorkspaceGate>
    </Stack>
</Stack>

<style>
  /* The screen takes the region it is given, and what needs a measure carries one: prose has the
     one `app.css` gives every paragraph, fields have `.fields`, and a table or a list has none
     (ADR-0065 decision 2). A 60ch column around the whole of it would be a document's measure
     around a screen that is not a document. */

  .section {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
  }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

  .waiting {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  /* A form on a surface is still a form: its fields keep a measure while the lists and tables of
     the screen take the region (ADR-0065 decision 2). The trail's filters are the exception and
     say so themselves - they are a grid of their own. */
  form.panel { max-inline-size: 52ch; }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .deliveries { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-150); }

  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }

  .target { font-weight: var(--fw-medium); }

  .types { margin: 0; padding: 0; border: none; }

  .types legend {
    padding: 0 0 var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  /* A column count the content decides, so a build with fifty event types does not become a list
     fifty rows long on a wide screen. */
  .type-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(22ch, 1fr));
    gap: var(--sp-050) var(--sp-150);
  }
</style>
