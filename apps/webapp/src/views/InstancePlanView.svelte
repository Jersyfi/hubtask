<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One plan, edited (UC-INS-13 checks 1, 3, 7; multi-tenancy.md §4.2): the installation's
  // *Defaults* one level down - the same areas but the language, each key with the control of its
  // kind and its lock - and the three features a plan may leave out, as switches.
  //
  // **The level above is drawn where it decides.** A key the installation locks is no plan's to
  // set: it shows the installation's value and no control (P-05). A value the plan stored before
  // that lock landed is not in force, and any save that keeps it is refused, so it is said and can
  // be removed. An undecided key says what it falls back to.
  //
  // **A save reaches every workspace on the plan at once**, so it asks first and names how many. It
  // carries the version it read: somebody who saved first is told, never overwritten.

  import { untrack } from 'svelte';

  import { Banner, Button, Callout, Dialog, Input, PageHeader, Spinner, Stack, Switch, Table } from '@hubtask/design-system/components';
  import type { InstanceSetting, InstanceSettings, Plan, PlanFeature } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import InstanceValueField from '../lib/instance/InstanceValueField.svelte';
  import ValueSource from '../lib/workspace/ValueSource.svelte';
  import { PLAN_AREAS, choiceCode, fieldOf, isBytes, lockCode, nameCode, planBodyOf, rowsOf, type Area, type PlanAreas } from '../lib/instance/values.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { plans } from '../lib/data/plans.svelte.ts';
  import { localesOf, zoneOptions } from '../lib/data/preferences.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    name: string;
    onnavigate: (path: string) => void;
  }

  const { name, onnavigate }: Props = $props();

  const FEATURES: readonly PlanFeature[] = ['AUTOMATION_RULES', 'WEBHOOKS', 'OWN_SIGN_IN_PROVIDERS'];

  $effect(() => {
    if (!manifest.isMultiTenant) return;
    const stops = untrack(() => [plans.open(), instance.openSettings()]);
    return () => stops.forEach((stop) => stop());
  });

  const reading = $derived(plans.state);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const plan = $derived(plans.named(name));
  const level = $derived(instance.settings.status === 'ready' ? instance.settings.data : undefined);
  const enforced = $derived(level?.is_enforced_from_file === true);
  const locales = $derived(localesOf(manifest.value).map((each) => each.locale));

  $effect(() => page.entitle(plan?.display_name ?? t('app.instance.plans')));

  // The draft: the plan's areas, its name and what it leaves out, taken once per version read.
  let areas = $state<PlanAreas>({});
  let displayName = $state('');
  let leftOut = $state<PlanFeature[]>([]);
  let takenVersion = $state<number | undefined>(undefined);
  let isConfirming = $state(false);
  let isRemoving = $state(false);
  let isWorking = $state(false);
  let saved = $state(false);
  let conflict = $state(false);
  let gone = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  function take(from: Plan): void {
    const copy = JSON.parse(JSON.stringify(from)) as Plan;
    areas = Object.fromEntries(PLAN_AREAS.map((area) => [area, copy[area] ?? {}])) as PlanAreas;
    displayName = copy.display_name;
    leftOut = [...(copy.left_out ?? [])];
    takenVersion = copy.version;
  }

  $effect(() => {
    if (plan && takenVersion === undefined) untrack(() => take(plan));
  });

  const rows = $derived(
    PLAN_AREAS.flatMap((area) => rowsOf({ [area]: plan?.[area] ?? {} } as InstanceSettings)).filter((row) => row.area !== 'localisation'),
  );
  const groups = $derived(
    PLAN_AREAS.filter((area) => !(area === 'ai' && manifest.isSingleTenant))
      .map((area) => ({ area, rows: rows.filter((row) => row.area === area) }))
      .filter((group) => group.rows.length > 0),
  );

  /** What the installation decided for a key, where it decided it. */
  const above = (area: Area, key: string): InstanceSetting | undefined => level?.[area]?.[key];
  const lockedAbove = (area: Area, key: string): boolean => above(area, key)?.locked === true;

  /** A value in words: a choice by its name, a flag on or off, a limit's zero as unlimited. */
  function said(area: Area, key: string, setting: InstanceSetting): string {
    const value = setting.value;
    if (Array.isArray(value)) return value.map((each) => t(choiceCode(area, key, String(each)))).join(', ');
    switch (setting.kind) {
      case 'CHOICE':
        return t(choiceCode(area, key, String(value)));
      case 'FLAG':
        return value === true ? t('app.instance.flag_on') : t('app.instance.flag_off');
      case 'COUNT':
        if (value === 0 && area === 'quotas') return t('app.instance.value_unlimited');
        return isBytes(area, key) && typeof value === 'number' ? formatBytes(value, messages.locale) : String(value);
      default:
        return String(value ?? '');
    }
  }

  const current = (area: Area, key: string): InstanceSetting | undefined => {
    const held = areas[area]?.[key];
    return held?.set ? held : undefined;
  };

  function put(area: Area, key: string, declared: InstanceSetting, next: InstanceSetting | undefined): void {
    const entry: InstanceSetting = next ?? { ...declared, set: false, value: undefined, locked: false };
    areas = { ...areas, [area]: { ...(areas[area] ?? {}), [key]: entry } };
  }

  function toggle(feature: PlanFeature, included: boolean): void {
    leftOut = included ? leftOut.filter((each) => each !== feature) : FEATURES.filter((each) => each === feature || leftOut.includes(each));
  }

  function failed(cause: unknown): void {
    if (cause instanceof TransportError && cause.isVersionConflict) {
      conflict = true;
      return;
    }
    if (cause instanceof TransportError && cause.detailCode === 'admin.plan_not_found') {
      gone = true;
      return;
    }
    failure = cause instanceof TransportError
      ? renderProblem(cause, messages)
      : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
  }

  async function save(): Promise<void> {
    if (!plan || takenVersion === undefined) return;
    isWorking = true;
    failure = undefined;
    saved = false;
    try {
      const written = await plans.change(plan.name, planBodyOf(displayName.trim(), leftOut, areas) as never, takenVersion);
      take(written);
      saved = true;
    } catch (cause) {
      failed(cause);
    } finally {
      isWorking = false;
      isConfirming = false;
    }
  }

  async function readAgain(): Promise<void> {
    conflict = false;
    await plans.reload();
    const fresh = plans.named(name);
    if (fresh) take(fresh);
    else gone = true;
  }

  async function remove(): Promise<void> {
    if (!plan) return;
    isWorking = true;
    failure = undefined;
    try {
      await plans.remove(plan.name);
      onnavigate('/instance/plans');
    } catch (cause) {
      failed(cause);
    } finally {
      isWorking = false;
      isRemoving = false;
    }
  }

  const columns = $derived([
    { id: 'setting', label: t('app.instance.column_setting') },
    { id: 'value', label: t('app.instance.column_value') },
    { id: 'lock', label: t('app.instance.workspaces_may') },
  ]);
</script>

<Stack gap="300">
  <PageHeader
    title={plan?.display_name ?? t('app.instance.plans')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={installationTrail(
      { id: 'plans', label: t('app.instance.plans'), href: '/instance/plans' },
      { id: 'plan', label: plan?.display_name ?? name },
    )}
  />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      {#if !manifest.isMultiTenant}
        <p class="prose">{t('app.instance.plans_single')}</p>
      {:else if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /> <span>{t('app.instance.reading')}</span></p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else if gone || !plan}
        <Banner tone="warning" title={t('app.instance.plan_gone')}>
          <a href="/instance/plans">{t('app.instance.plan_back')}</a>
        </Banner>
      {:else}
        <p class="quiet-line">
          <code>{plan.name}</code> · {t('app.instance.plan_workspaces', { count: plan.workspaces })}
        </p>

        {#if enforced}
          <Banner tone="info" title={t('app.instance.source_file_enforced')}>{level?.source ?? ''}</Banner>
          {#each groups as group (group.area)}
            <section aria-labelledby={`read-${group.area}`}>
              <Stack gap="100">
                <h2 id={`read-${group.area}`} class="section">{t(`app.instance.area_${group.area}`)}</h2>
                <Table label={t(`app.instance.area_${group.area}`)} isLabelHidden {columns}>
                  {#each group.rows as row (row.key)}
                    <tr>
                      <td>{t(nameCode(row.area, row.key))}</td>
                      <td>{row.setting.set ? said(row.area, row.key, row.setting) : t('app.instance.value_undecided')}</td>
                      <td>{lockCode(row.setting, row.setting.locked) ? t(lockCode(row.setting, row.setting.locked) ?? '') : ''}</td>
                    </tr>
                  {/each}
                </Table>
              </Stack>
            </section>
          {/each}
          <p class="quiet-line">
            {t('app.instance.plan_features')}:
            {FEATURES.filter((feature) => !(plan.left_out ?? []).includes(feature)).map((feature) => t(`app.instance.feature.${feature.toLowerCase()}`)).join(', ') || '–'}
          </p>
        {:else}
          {#if conflict}
            <!-- Somebody saved first: nothing of theirs is overwritten, and nothing of this draft is
                 lost until it is read again. -->
            <div role="alert">
              <Callout tone="warning" title={t('app.instance.plan_conflict_title')}>
                {t('app.instance.plan_conflict_body')}
                <Button size="sm" tone="secondary" onclick={() => void readAgain()}>{t('app.instance.plan_reload')}</Button>
              </Callout>
            </div>
          {:else if failure && !failure.fields.size}
            <Banner tone="danger" title={failure.message}>
              {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
            </Banner>
          {:else if saved}
            <Banner tone="success">{t('app.instance.plan_saved')}</Banner>
          {/if}

          <form
            class="panel"
            onsubmit={(event) => {
              event.preventDefault();
              isConfirming = true;
            }}
          >
            <Stack gap="300">
              <Input
                label={t('app.instance.plan_display_name')}
                bind:value={displayName}
                error={failure?.fields.get('/display_name')}
                isRequired
              />

              <section class="group" aria-labelledby="plan-features">
                <Stack gap="150">
                  <h3 id="plan-features" class="area">{t('app.instance.plan_features')}</h3>
                  <p class="quiet-line">{t('app.instance.plan_features_hint')}</p>
                  {#each FEATURES as feature (feature)}
                    <Switch
                      label={t(`app.instance.feature.${feature.toLowerCase()}`)}
                      checked={!leftOut.includes(feature)}
                      onchange={(event) => toggle(feature, (event.currentTarget as HTMLInputElement).checked)}
                    />
                  {/each}
                </Stack>
              </section>

              {#each groups as group (group.area)}
                <section class="group" aria-labelledby={`area-${group.area}`}>
                  <Stack gap="150">
                    <h3 id={`area-${group.area}`} class="area">{t(`app.instance.area_${group.area}`)}</h3>
                    {#if group.area === 'quotas'}
                      <p class="quiet-line">{t('app.instance.plan_limits_exception')}</p>
                    {/if}
                    {#each group.rows as row (row.key)}
                      {@const installation = above(row.area, row.key)}
                      {#if lockedAbove(row.area, row.key)}
                        <!-- The installation's lock binds the plan: its value, the chip that says
                             whose it is (UC-WRK-32 check 5), and no control. -->
                        {@const inForce = installation?.set ? said(row.area, row.key, installation) : t('app.instance.value_undecided')}
                        <div class="locked" data-value={`${row.area}.${row.key}`}>
                          <span class="label">{t(nameCode(row.area, row.key))}</span>
                          <span class="in-force">{inForce} <ValueSource source="INSTANCE" lock="INSTANCE" value={inForce} /></span>
                          {#if current(row.area, row.key)}
                            <p class="overtaken">
                              {t('app.instance.plan_value_overtaken', { value: said(row.area, row.key, current(row.area, row.key) as InstanceSetting) })}
                              <Button size="sm" tone="subtle" onclick={() => put(row.area, row.key, row.setting, undefined)}>
                                {t('app.instance.plan_value_remove')}
                              </Button>
                            </p>
                          {/if}
                          {#if failure?.fields.get(fieldOf(row.area, row.key))}
                            <p class="refusal" role="alert">{failure?.fields.get(fieldOf(row.area, row.key))}</p>
                          {/if}
                        </div>
                      {:else}
                        <InstanceValueField
                          area={row.area}
                          key={row.key}
                          declared={row.setting}
                          current={current(row.area, row.key)}
                          {locales}
                          zones={zoneOptions('')}
                          error={failure?.fields.get(fieldOf(row.area, row.key))}
                          onchange={(next) => put(row.area, row.key, row.setting, next)}
                        />
                        {#if !current(row.area, row.key)}
                          <p class="fallback">
                            {installation?.set
                              ? t('app.instance.plan_falls_back', { value: said(row.area, row.key, installation) })
                              : t('app.instance.plan_falls_back_default')}
                          </p>
                        {/if}
                      {/if}
                    {/each}
                  </Stack>
                </section>
              {/each}

              <div class="row">
                <Button type="submit" tone="primary">{t('app.instance.plan_save')}</Button>
                {#if plan.workspaces === 0}
                  <Button tone="subtle" icon="trash" onclick={() => (isRemoving = true)}>{t('app.instance.plan_remove')}</Button>
                {/if}
              </div>
            </Stack>
          </form>
        {/if}
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<Dialog
  title={t('app.instance.plan_confirm_title', { name: displayName.trim() || (plan?.display_name ?? '') })}
  isOpen={isConfirming}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (isConfirming = false)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isConfirming = false)}>{t('app.instance.cancel')}</Button>
    <Button tone="primary" isBusy={isWorking} busyLabel={t('app.instance.working')} onclick={() => void save()}>
      {t('app.instance.plan_confirm')}
    </Button>
  {/snippet}
  <p class="prose">{t('app.instance.plan_confirm_body', { count: plan?.workspaces ?? 0 })}</p>
</Dialog>

<Dialog
  title={t('app.instance.plan_remove_title', { name: plan?.display_name ?? '' })}
  isOpen={isRemoving}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (isRemoving = false)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isRemoving = false)}>{t('app.instance.cancel')}</Button>
    <Button tone="danger" isBusy={isWorking} busyLabel={t('app.instance.working')} onclick={() => void remove()}>
      {t('app.instance.plan_remove_confirm')}
    </Button>
  {/snippet}
  <p class="prose">{t('app.instance.plan_remove_body', { short: plan?.name ?? '' })}</p>
</Dialog>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-line { margin: 0; max-inline-size: 60ch; color: var(--text-secondary); font-size: var(--fs-100); }
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .area { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  form { margin: 0; max-inline-size: 80ch; }
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .locked { display: grid; gap: var(--sp-050); }
  .label { font-weight: var(--fw-medium); }
  .overtaken, .fallback { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .refusal { margin: 0; color: var(--text-danger); font-size: var(--fs-100); }
  code { font-family: var(--font-mono); }
</style>
