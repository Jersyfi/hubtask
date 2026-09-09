<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The rules, and the editor behind them (G-05, `automation.md` §1).
  //
  // **A rule is created switched off.** There is no "enabled" control on the form, because there is
  // no such field to write: `:enable` and `:disable` are separate calls so the trail says which of
  // the two somebody did, and writing what a rule would do is a different decision from letting it
  // loose on the workspace.
  //
  // **Every list is read rather than compiled in.** The triggers, the action kinds and the event
  // types come from `/meta/capabilities`, so an installation that serves one more action gets one
  // more option without a release of this client.
  //
  // **A scope includes its descendants**, and the screen says so because it is the question
  // everybody asks: a rule on a hub sees what happens in its collections, by the ordinary rule that
  // a permission held at a hub applies downwards.
  //
  // **Nothing is pre-empted.** Writing a rule needs the automation permission *and* the rights the
  // rule's own actions need — the second half depends on what each action's use case demands of the
  // account the rule runs as, which no client can compute. The server refuses and this renders it.

  import { untrack } from 'svelte';

  import { AutomationRuleCard, Banner, Button, Input, Select, Spinner, Stack, Textarea } from '@hubtask/design-system/components';

  import ActionList from '../lib/automation/ActionList.svelte';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { rules, type DraftAction, type Rule, type RuleAction } from '../lib/data/rules.svelte.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const TENANT = { scopeType: 'TENANT' } as const;

  let isWriting = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  // The draft. One rule at a time, because a form that held several would be a form nobody can
  // tell apart from the list behind it.
  let name = $state('');
  let scopeType = $state('TENANT');
  let scopeId = $state('');
  let runAs = $state('');
  let triggerKind = $state('');
  let eventType = $state('');
  let rrule = $state('');
  let timezone = $state('');
  let conditions = $state<string[]>([]);
  let actions = $state<DraftAction[]>([]);
  let maxRuns = $state('');
  let dedupe = $state('');
  let onError = $state('STOP');

  $effect(() => untrack(() => rules.open()));
  $effect(() => untrack(() => containers.start()));
  $effect(() => untrack(() => serviceAccounts.open()));
  $effect(() => untrack(() => people.openScope(TENANT)));
  $effect(() => {
    for (const hub of containers.hubs) untrack(() => containers.openLevel(hub.id));
  });

  const reading = $derived(rules.state);
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  /** The vocabulary, from the manifest and nowhere else. */
  const triggers = $derived(manifest.value?.automation?.triggers ?? []);
  const actionKinds = $derived(manifest.value?.automation?.actions ?? []);
  const eventTypes = $derived(manifest.value?.event_types ?? []);

  /** Who a rule may run as: the service accounts first, because that is what it usually is. */
  const runners = $derived([
    ...serviceAccounts.all.map((account) => ({
      value: account.id,
      label: t('app.rules.service_account_named', { name: account.display_name }),
    })),
    ...people
      .candidates({})
      .map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  ]);

  const scopeChoices = $derived([
    { value: 'TENANT', label: t('app.rules.scope_tenant') },
    ...containers.hubs.map((hub) => ({ value: `HUB:${hub.id}`, label: t('app.rules.scope_hub', { name: hub.name }) })),
    ...containers.hubs.flatMap((hub) =>
      containers.collectionsOf(hub.id).map((collection) => ({
        value: `COLLECTION:${collection.id}`,
        label: t('app.rules.scope_collection', { name: `${hub.name} · ${collection.name}` }),
      })),
    ),
  ]);

  let chosenScope = $state('TENANT');
  $effect(() => {
    const [type, id] = chosenScope.split(':');
    scopeType = type ?? 'TENANT';
    scopeId = id ?? '';
  });

  /**
   * The parameters, parsed on the way out.
   *
   * Typed as JSON and parsed here rather than as it is typed: a half-written object is something
   * the reader can still see and finish, and a parse error that ate their input is not.
   */
  function paramsOf(action: DraftAction): Record<string, unknown> | undefined {
    if (action.params) return action.params;
    const written = action.paramsText?.trim();
    if (!written) return undefined;
    try {
      const parsed: unknown = JSON.parse(written);
      return typeof parsed === 'object' && parsed !== null ? (parsed as Record<string, unknown>) : undefined;
    } catch {
      // Left for the server to refuse by name. A client that invented a message here would be
      // inventing one about a shape the use case behind the kind defines, not this screen.
      return undefined;
    }
  }

  function shaped(list: readonly DraftAction[]): RuleAction[] {
    return list
      .filter((action) => action.kind !== '')
      .map((action) => ({
        kind: action.kind,
        ...(paramsOf(action) ? { params: paramsOf(action) } : {}),
        ...(action.then ? { then: shaped(action.then) } : {}),
        ...(action.else && action.else.length > 0 ? { else: shaped(action.else) } : {}),
      }));
  }

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

  async function write(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!name.trim() || !runAs || !triggerKind) return;
    await attempt(async () => {
      await rules.write({
        name: name.trim(),
        scope: { type: scopeType, ...(scopeId ? { id: scopeId } : {}) },
        run_as: runAs,
        trigger: {
          kind: triggerKind,
          ...(triggerKind === 'EVENT' && eventType ? { event_type: eventType } : {}),
          ...(triggerKind === 'SCHEDULE' && rrule ? { rrule } : {}),
          ...(triggerKind === 'SCHEDULE' && timezone ? { timezone } : {}),
        },
        conditions: conditions.filter((expr) => expr.trim() !== '').map((expr) => ({ expr: expr.trim() })),
        actions: shaped(actions),
        ...(maxRuns || dedupe
          ? {
              throttle: {
                ...(maxRuns ? { max_runs_per_hour: Number(maxRuns) } : {}),
                ...(dedupe ? { dedupe_key_expr: dedupe } : {}),
              },
            }
          : {}),
        on_error: onError,
      });
      isWriting = false;
      name = '';
      conditions = [];
      actions = [];
      maxRuns = '';
      dedupe = '';
    });
  }

  /** What a card says about a rule, in words rather than tokens. */
  function triggerWord(rule: Rule): string {
    const code = `app.rules.trigger_${rule.trigger.kind.toLowerCase()}`;
    const said = messages.has(code) ? t(code) : rule.trigger.kind;
    return rule.trigger.event_type ? `${said} · ${rule.trigger.event_type}` : said;
  }

  const runnerName = (id: string) =>
    serviceAccounts.all.find((account) => account.id === id)?.display_name ??
    accounts.nameOf(id) ??
    t('app.people.unnamed');
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.rules.title')}</h1>
    <p class="quiet">{t('app.rules.intro')}</p>

    {#if failure}
      <!-- The server's own words, including the refusal a client cannot predict: a rule whose
           actions exceed what its writer may do. -->
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
      </Banner>
    {/if}

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting"><Spinner label={t('app.rules.reading')} /> <span>{t('app.rules.reading')}</span></p>
    {:else if refusal}
      <Banner tone="danger" title={refusal.message}>
        {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
      </Banner>
    {:else}
      <Stack gap="150">
        {#each rules.all as rule (rule.id)}
          <Stack gap="100">
            <AutomationRuleCard
              name={rule.name}
              trigger={{ label: t('app.rules.starts_on'), value: triggerWord(rule) }}
              actions={{ label: t('app.rules.actions'), value: String(rule.actions.length) }}
              runAs={{ label: t('app.rules.runs_as'), value: runnerName(rule.run_as) }}
              isEnabled={rule.enabled}
              stateLabel={rule.enabled ? t('app.rules.on') : t('app.rules.off')}
              failureLabel={rule.failure_count > 0
                ? t('app.rules.failing', { count: String(rule.failure_count) })
                : undefined}
            />
            <div class="row">
              {#if rule.enabled}
                <Button size="sm" tone="secondary" isBusy={isWorking} busyLabel={t('app.rules.working')}
                  onclick={() => void attempt(() => rules.disable(rule.id))}>
                  {t('app.rules.disable')}
                </Button>
              {:else}
                <Button size="sm" tone="primary" isBusy={isWorking} busyLabel={t('app.rules.working')}
                  onclick={() => void attempt(() => rules.enable(rule.id))}>
                  {t('app.rules.enable')}
                </Button>
              {/if}
              <Button size="sm" tone="subtle" isBusy={isWorking} busyLabel={t('app.rules.working')}
                onclick={() => void attempt(() => rules.remove(rule.id))}>
                {t('app.rules.delete')}
              </Button>
            </div>
          </Stack>
        {:else}
          <p class="quiet">{t('app.rules.none')}</p>
        {/each}
      </Stack>
    {/if}

    {#if isWriting}
      <form onsubmit={write}>
        <Stack gap="200">
          <h2 class="section">{t('app.rules.new_title')}</h2>
          <p class="quiet small">{t('app.rules.created_off')}</p>

          <Input label={t('app.rules.name')} bind:value={name} isRequired />

          <Select
            label={t('app.rules.scope')}
            hint={t('app.rules.scope_hint')}
            bind:value={chosenScope}
            options={scopeChoices}
          />

          <Select
            label={t('app.rules.runs_as')}
            hint={t('app.rules.runs_as_hint')}
            bind:value={runAs}
            placeholder={t('app.rules.choose_runner')}
            options={runners}
          />

          <Select
            label={t('app.rules.starts_on')}
            bind:value={triggerKind}
            placeholder={t('app.rules.choose_trigger')}
            options={triggers.map((kind) => ({
              value: kind,
              label: messages.has(`app.rules.trigger_${kind.toLowerCase()}`)
                ? t(`app.rules.trigger_${kind.toLowerCase()}`)
                : kind,
            }))}
          />

          {#if triggerKind === 'EVENT'}
            <Select
              label={t('app.rules.event_type')}
              hint={t('app.rules.event_type_hint')}
              bind:value={eventType}
              placeholder={t('app.rules.choose_event')}
              options={eventTypes.map((type) => ({ value: type, label: type }))}
            />
          {:else if triggerKind === 'SCHEDULE'}
            <Input label={t('app.rules.rrule')} hint={t('app.rules.rrule_hint')} bind:value={rrule} />
            <Input label={t('app.rules.timezone')} hint={t('app.rules.timezone_hint')} bind:value={timezone} />
          {/if}

          <Stack gap="100">
            <span class="label">{t('app.rules.conditions')}</span>
            <p class="quiet small">{t('app.rules.conditions_hint')}</p>
            {#each conditions as condition, index (index)}
              <div class="row">
                <Input
                  label={t('app.rules.condition')}
                  value={condition}
                  error={failure?.fields.get(`/conditions/${index}/expr`)}
                  oninput={(event: Event) => {
                    const written = (event.currentTarget as HTMLInputElement).value;
                    conditions = conditions.map((each, at) => (at === index ? written : each));
                  }}
                />
                <Button size="sm" tone="subtle" onclick={() => (conditions = conditions.filter((_, at) => at !== index))}>
                  {t('app.rules.remove_condition')}
                </Button>
              </div>
            {/each}
            <div>
              <Button size="sm" tone="secondary" onclick={() => (conditions = [...conditions, ''])}>
                {t('app.rules.add_condition')}
              </Button>
            </div>
          </Stack>

          <Stack gap="100">
            <span class="label">{t('app.rules.actions')}</span>
            <p class="quiet small">{t('app.rules.actions_hint')}</p>
            <ActionList {actions} kinds={actionKinds} onchange={(next) => (actions = next)} />
          </Stack>

          <Input label={t('app.rules.max_runs')} hint={t('app.rules.max_runs_hint')} bind:value={maxRuns} type="number" />
          <Textarea label={t('app.rules.dedupe')} hint={t('app.rules.dedupe_hint')} bind:value={dedupe} rows={2} spellcheck={false} />

          <Select
            label={t('app.rules.on_error')}
            hint={t('app.rules.on_error_hint')}
            bind:value={onError}
            options={[
              { value: 'STOP', label: t('app.rules.on_error_stop') },
              { value: 'CONTINUE', label: t('app.rules.on_error_continue') },
              { value: 'RETRY', label: t('app.rules.on_error_retry') },
            ]}
          />

          <div class="row">
            <Button type="submit" tone="primary" isBusy={isWorking} busyLabel={t('app.rules.writing')}>
              {t('app.rules.write')}
            </Button>
            <Button tone="subtle" onclick={() => (isWriting = false)}>{t('app.rules.cancel')}</Button>
          </div>
        </Stack>
      </form>
    {:else}
      <div>
        <Button tone="primary" onclick={() => (isWriting = true)}>{t('app.rules.new_rule')}</Button>
      </div>
    {/if}
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

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .label { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .row { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  form { margin: 0; }
</style>
