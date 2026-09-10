<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The rules that delete things, and the preview that has to come first.
  //
  // **A rule is written switched off, previewed, and only then armed.** `:preview` is a `POST` on
  // a stored rule, so the rule has to exist before anybody can see what it would catch — and a
  // rule that existed *and was enabled* before it had been looked at would be exactly the thing
  // the preview exists to prevent. So the form writes `enabled: false`, the preview runs, and
  // arming it is a second, deliberate press.
  //
  // **The mode a broad rule lands in is said where it lands.** `data-retention.md` §7: a new rule
  // whose first run would affect more than five per cent of the holdings is stored as
  // `NOTIFY_ONLY` whatever was asked for. The server does that; the screen compares what came back
  // with what was asked and says so, rather than letting somebody discover it at the first pass
  // that removed nothing.
  //
  // **`:retain` is not a hold**, and the screen says which is which: one takes a single entry out
  // of a running period, the other is an instruction that overrides every rule and reaches
  // everything under it. Confusing them is how somebody believes a case is protected when one
  // object is.
  //
  // **The kinds come from the manifest.** `data-retention.md` §3 says a new kind is immediately
  // configurable with no code change to the engine; a list typed here would be the one place that
  // still needed one. A kind whose `actions` is empty is named by the catalogue and removed by
  // nothing, and the picker says so rather than offering a rule that would be refused.

  import { untrack } from 'svelte';

  import {
    Badge,
    Banner,
    Button,
    Input,
    Select,
    Spinner,
    Stack,
    Textarea,
  } from '@hubtask/design-system/components';

  import type { RetentionDataKind as DataKind } from '@hubtask/sync-engine';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { policies, type Policy, type RetentionAction } from '../lib/data/policies.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  /** §7's five per cent: above this on a first run and the rule is stored as an announcement. */
  const BROAD_SHARE = 0.05;

  /** What a hold can cover. `TENANT` names nothing because it covers everything. */
  const HOLD_SCOPES = ['TENANT', 'CONTAINER', 'ITEM', 'ACCOUNT'];

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  let draftKind = $state('');
  let draftDays = $state('30');
  let draftAction = $state<string>('NOTIFY_ONLY');
  let draftJustification = $state('');

  let effectiveIn = $state('');

  let holdScope = $state('TENANT');
  let holdTarget = $state('');
  let holdReason = $state('');
  let showReleased = $state(false);
  let releasing = $state('');
  let releaseReason = $state('');

  /** Set when a rule came back in a mode nobody asked for. Cleared by the next write. */
  let landedIn = $state<{ policyId: string; asked: string } | undefined>(undefined);

  $effect(() => untrack(() => policies.open()));
  $effect(() => {
    const released = showReleased;
    return untrack(() => policies.openHolds(released));
  });
  $effect(() => untrack(() => containers.start()));
  $effect(() => {
    const where = effectiveIn;
    if (!where) return;
    return untrack(() => policies.open({ containerId: where, effective: true }));
  });

  const reading = $derived(policies.stateOf());
  const listed = $derived(policies.of());
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );
  const effective = $derived(
    effectiveIn ? policies.of({ containerId: effectiveIn, effective: true }) : [],
  );

  /** The catalogue, from the manifest and from nowhere else. */
  const kinds = $derived<readonly DataKind[]>(manifest.value?.retention_data_kinds ?? []);
  const chosenKind = $derived(kinds.find((kind) => kind.data_kind === draftKind));
  /** What this build can do to the chosen kind. Empty means the catalogue names it and nothing removes it. */
  const actions = $derived<readonly string[]>(chosenKind?.actions ?? []);

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : undefined;

  const actionWord = (action: string) =>
    messages.has(`app.retention.action_${action.toLowerCase()}`)
      ? t(`app.retention.action_${action.toLowerCase()}`)
      : action;

  const containerName = (id: string | null | undefined) =>
    (id ? containers.hubs.find((container) => container.id === id)?.name : undefined) ?? id ?? '';

  const scopeWord = (policy: Policy) =>
    policy.scope?.kind === 'TENANT' || !policy.scope?.id
      ? t('app.retention.scope_workspace')
      : containerName(policy.scope.id);

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  /**
   * Writes the rule **switched off**, then previews it at once.
   *
   * Off, because arming a rule nobody has looked at is what the preview exists to prevent — and
   * because `:preview` needs a stored rule to run against.
   */
  async function write(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const days = Number.parseInt(draftDays, 10);
    if (!draftKind || !Number.isFinite(days)) return;
    await attempt(async () => {
      const asked = draftAction;
      const written = await policies.create({
        scope: { kind: 'TENANT' },
        data_kind: draftKind,
        retain_days: days,
        action: asked as RetentionAction,
        enabled: false,
        ...(draftJustification.trim() ? { justification: draftJustification.trim() } : {}),
      });
      // §7's switch, as the caller sees it: what came back is not what was asked for.
      landedIn =
        written.action === asked ? undefined : { policyId: written.id, asked };
      draftJustification = '';
      await policies.preview(written.id);
    });
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.retention.title')}</h1>
    <p class="quiet">{t('app.retention.intro')}</p>

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

    {#if landedIn}
      <!-- Said where it happened rather than discovered at the first pass that removed nothing. -->
      <Banner tone="warning" title={t('app.retention.landed_title')}>
        {t('app.retention.landed_note', { asked: actionWord(landedIn.asked) })}
      </Banner>
    {/if}

    <Stack gap="150">
      <h2 class="section">{t('app.retention.rules_title')}</h2>

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="waiting">
          <Spinner label={t('app.retention.reading')} /> <span>{t('app.retention.reading')}</span>
        </p>
      {:else if refusal}
        <Banner tone="danger" title={refusal.message}>
          {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
        </Banner>
      {:else}
        {#each listed as policy (policy.id)}
          {@const preview = policies.previewOf(policy.id)}
          <section class="panel">
            <Stack gap="100">
              <div class="row">
                <span class="name">{policy.data_kind}</span>
                <Badge tone={policy.action === 'NOTIFY_ONLY' ? 'info' : 'warning'}>
                  {actionWord(policy.action)}
                </Badge>
                <Badge tone={policy.enabled === false ? 'neutral' : 'success'}>
                  {policy.enabled === false ? t('app.retention.off') : t('app.retention.on')}
                </Badge>
              </div>

              <p class="quiet small">
                {t('app.retention.after_days', { days: String(policy.retain_days) })}
                · {t('app.retention.at_scope', { scope: scopeWord(policy) })}
              </p>

              {#if policy.justification}
                <!-- The sentence somebody will read in an audit, kept beside the period it
                     justifies. -->
                <p class="quiet small">
                  {t('app.retention.justified', { reason: policy.justification })}
                </p>
              {/if}

              {#if preview}
                <div class="preview">
                  <Stack gap="050">
                    <p class="quiet small">
                      {t('app.retention.preview_matched', { count: String(preview.matched ?? 0) })}
                      {#if preview.share_of_scope !== undefined}
                        · {t('app.retention.preview_share', {
                          percent: String(Math.round((preview.share_of_scope ?? 0) * 100)),
                        })}
                      {/if}
                    </p>
                    {#if (preview.share_of_scope ?? 0) > BROAD_SHARE}
                      <p class="warning">{t('app.retention.preview_broad')}</p>
                    {/if}
                    {#each Object.entries(preview.blocked ?? {}) as [reason, count] (reason)}
                      <!-- What the rule would *not* take, and why. A legal hold is the one an
                           operator is usually looking for. -->
                      <p class="quiet small">
                        {messages.has(`app.retention.blocked_${reason}`)
                          ? t(`app.retention.blocked_${reason}`, { count: String(count) })
                          : t('app.retention.blocked_other', { reason, count: String(count) })}
                      </p>
                    {/each}
                    {#if (preview.samples ?? []).length > 0}
                      <ul class="samples">
                        {#each preview.samples ?? [] as sample (sample.id)}
                          <li class="quiet small">
                            {sample.title} · {when(sample.effective_at)}
                          </li>
                        {/each}
                      </ul>
                    {/if}
                    <p class="quiet small">{t('app.retention.preview_wrote_nothing')}</p>
                  </Stack>
                </div>
              {/if}

              <div class="row">
                <Button
                  size="sm"
                  tone="secondary"
                  isBusy={isWorking}
                  busyLabel={t('app.retention.previewing')}
                  onclick={() => void attempt(() => policies.preview(policy.id))}
                >
                  {t('app.retention.preview')}
                </Button>
                {#if policy.enabled === false}
                  <!-- Armed only after somebody has seen what it would do. -->
                  <Button
                    size="sm"
                    tone="danger"
                    isBusy={isWorking}
                    busyLabel={t('app.retention.arming')}
                    disabledReason={preview ? undefined : t('app.retention.preview_first')}
                    onclick={() =>
                      void attempt(() => policies.update(policy.id, { enabled: true }))}
                  >
                    {t('app.retention.arm')}
                  </Button>
                {:else}
                  <Button
                    size="sm"
                    tone="secondary"
                    isBusy={isWorking}
                    busyLabel={t('app.retention.saving')}
                    onclick={() =>
                      void attempt(() => policies.update(policy.id, { enabled: false }))}
                  >
                    {t('app.retention.disarm')}
                  </Button>
                {/if}
                <Button
                  size="sm"
                  tone="subtle"
                  isBusy={isWorking}
                  busyLabel={t('app.retention.withdrawing')}
                  onclick={() => void attempt(() => policies.withdraw(policy.id))}
                >
                  {t('app.retention.withdraw')}
                </Button>
              </div>
              <p class="quiet small">{t('app.retention.withdraw_note')}</p>
            </Stack>
          </section>
        {:else}
          <p class="quiet">{t('app.retention.no_rules')}</p>
        {/each}
      {/if}

      <form class="panel" onsubmit={write}>
        <Stack gap="150">
          <h3 class="section">{t('app.retention.write_title')}</h3>
          <p class="quiet small">{t('app.retention.write_note')}</p>
          <Select
            label={t('app.retention.kind')}
            bind:value={draftKind}
            placeholder={t('app.retention.choose_kind')}
            options={kinds.map((kind) => ({
              value: kind.data_kind ?? '',
              label: kind.data_kind ?? '',
            }))}
          />
          {#if draftKind && actions.length === 0}
            <!-- Named by the catalogue and removed by nothing. Not the same as a kind that does
                 not exist, and a rule for it would be refused. -->
            <p class="warning">{t('app.retention.kind_not_swept')}</p>
          {:else if draftKind}
            <Select
              label={t('app.retention.action')}
              bind:value={draftAction}
              options={actions.map((action) => ({ value: action, label: actionWord(action) }))}
            />
          {/if}
          <Input
            label={t('app.retention.days')}
            hint={chosenKind?.max_days
              ? t('app.retention.days_hint_ceiling', { days: String(chosenKind.max_days) })
              : t('app.retention.days_hint')}
            type="number"
            bind:value={draftDays}
          />
          <Textarea
            label={t('app.retention.justification')}
            hint={t('app.retention.justification_hint')}
            bind:value={draftJustification}
            rows={2}
          />
          <div>
            <Button
              type="submit"
              tone="primary"
              isBusy={isWorking}
              busyLabel={t('app.retention.writing')}
            >
              {t('app.retention.write')}
            </Button>
          </div>
        </Stack>
      </form>
    </Stack>

    <Stack gap="150">
      <h2 class="section">{t('app.retention.effective_title')}</h2>
      <p class="quiet small">{t('app.retention.effective_intro')}</p>
      <Select
        label={t('app.retention.effective_where')}
        bind:value={effectiveIn}
        placeholder={t('app.retention.choose_container')}
        options={containers.hubs.map((container) => ({
          value: container.id,
          label: container.name,
        }))}
      />
      {#if effectiveIn}
        {#each effective as policy (policy.id)}
          <p class="line">
            <Badge tone={policy.in_force ? 'success' : 'neutral'}>
              {policy.in_force ? t('app.retention.in_force') : t('app.retention.overridden')}
            </Badge>
            <span>{policy.data_kind}</span>
            <span class="quiet small">
              {actionWord(policy.action)}
              · {t('app.retention.after_days', { days: String(policy.retain_days) })}
              · {t('app.retention.comes_from', { scope: scopeWord(policy) })}
            </span>
          </p>
        {:else}
          <p class="quiet">{t('app.retention.no_effective')}</p>
        {/each}
      {/if}
    </Stack>

    <Stack gap="150">
      <h2 class="section">{t('app.retention.holds_title')}</h2>
      <!-- Beside the rules rather than on a page of its own: a hold overrides the workspace's own
           periods, and it belongs where somebody reads what those periods are. -->
      <p class="quiet small">{t('app.retention.holds_intro')}</p>

      {#each policies.placed as hold (hold.id)}
        <section class="panel">
          <Stack gap="050">
            <div class="row">
              <Badge tone={hold.released_at ? 'neutral' : 'danger'}>
                {hold.released_at ? t('app.retention.hold_released') : t('app.retention.hold_in_force')}
              </Badge>
              <span class="name">
                {hold.scope.kind === 'TENANT'
                  ? t('app.retention.scope_workspace')
                  : `${hold.scope.kind} ${hold.scope.id ?? ''}`}
              </span>
            </div>
            <p class="quiet small">{t('app.retention.hold_reason', { reason: hold.reason })}</p>
            <p class="quiet small">{t('app.retention.hold_placed', { at: when(hold.placed_at) ?? '' })}</p>
            {#if hold.released_at}
              <p class="quiet small">
                {t('app.retention.hold_lifted', {
                  at: when(hold.released_at) ?? '',
                  reason: hold.released_reason ?? '',
                })}
              </p>
            {:else if releasing === hold.id}
              <Input label={t('app.retention.release_reason')} bind:value={releaseReason} isRequired />
              <div class="row">
                <Button
                  size="sm"
                  tone="danger"
                  isBusy={isWorking}
                  busyLabel={t('app.retention.releasing')}
                  onclick={() =>
                    void attempt(async () => {
                      await policies.release(hold.id, releaseReason.trim());
                      releasing = '';
                      releaseReason = '';
                    })}
                >
                  {t('app.retention.release_confirm')}
                </Button>
                <Button size="sm" tone="subtle" onclick={() => (releasing = '')}>
                  {t('app.workspace.cancel')}
                </Button>
              </div>
            {:else}
              <div>
                <Button size="sm" tone="secondary" onclick={() => (releasing = hold.id)}>
                  {t('app.retention.release')}
                </Button>
              </div>
            {/if}
          </Stack>
        </section>
      {:else}
        <p class="quiet">{t('app.retention.no_holds')}</p>
      {/each}

      <div>
        <Button size="sm" tone="subtle" onclick={() => (showReleased = !showReleased)}>
          {showReleased ? t('app.retention.hide_released') : t('app.retention.show_released')}
        </Button>
      </div>

      <form
        class="panel"
        onsubmit={(event) => {
          event.preventDefault();
          if (!holdReason.trim()) return;
          void attempt(async () => {
            await policies.place(
              { kind: holdScope, id: holdTarget.trim() || null },
              holdReason.trim(),
            );
            holdReason = '';
            holdTarget = '';
          });
        }}
      >
        <Stack gap="150">
          <h3 class="section">{t('app.retention.place_title')}</h3>
          <Select
            label={t('app.retention.hold_scope')}
            bind:value={holdScope}
            options={HOLD_SCOPES.map((kind) => ({
              value: kind,
              label: t(`app.retention.hold_scope_${kind.toLowerCase()}`),
            }))}
          />
          {#if holdScope !== 'TENANT'}
            <Input
              label={t('app.retention.hold_target')}
              hint={t('app.retention.hold_target_hint')}
              bind:value={holdTarget}
            />
          {/if}
          <Textarea
            label={t('app.retention.hold_reason_field')}
            hint={t('app.retention.hold_reason_hint')}
            bind:value={holdReason}
            rows={2}
            isRequired
          />
          <p class="quiet small">{t('app.retention.hold_not_retain')}</p>
          <div>
            <Button
              type="submit"
              tone="primary"
              isBusy={isWorking}
              busyLabel={t('app.retention.placing')}
            >
              {t('app.retention.place')}
            </Button>
          </div>
        </Stack>
      </form>
    </Stack>
  </Stack>
</div>

<style>
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .section {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
  }

  h3.section { font-size: var(--fs-200); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .warning { margin: 0; color: var(--text-warning); font-size: var(--fs-075); }

  .waiting {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .preview {
    padding: var(--sp-150);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
  }

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .line { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .samples { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
</style>
