<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What the rules did, what one would do, and how a failed run is finished (G-07).
  //
  // **This is where the seven statuses earn their distinctness.** `SKIPPED` names *which* condition
  // stopped the run — the contract puts the condition results in the rule's own order precisely so
  // that this question has an answer. `THROTTLED` says the rule protected the workspace from
  // itself. `ABORTED_LOOP` says the causation depth stopped a rule that triggered itself. And
  // `RUNNING` on a row that started long ago is a crash rather than progress, which the screen says
  // rather than showing a spinner forever.
  //
  // **A dry run writes nothing**, and that is said beside the results rather than in a heading —
  // where somebody reading "would have assigned Ada" is looking.
  //
  // **A replay completes a failed run rather than starting a new one.** The actions that already
  // succeeded are named before the button, because confusing a replay with a re-trigger is how
  // somebody sends the same mail twice.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Input, PageHeader, RunStatusBadge, Select, Spinner, Stack } from '@hubtask/design-system/components';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { rules } from '../lib/data/rules.svelte.ts';
  import { runs, type ActionResult, type Run, type TestResult } from '../lib/data/runs.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  // On the shell (ADR-0061, issue 880): the page's head is a `PageHeader`, the title the bar's on
  // a phone. The filters stay beside the list they narrow, under its own heading and above the
  // counting strip, rather than in the head's second row: a row there would sit above the dry
  // run, which they do not filter.
  $effect(() => page.entitle(t('app.runs.title')));

  /** A run that says it is running and started this long ago is a crash, not progress. */
  const STALE_AFTER_MS = 15 * 60 * 1000;

  /**
   * The rule's own screen links here prefiltered (F8-06): `?rule_id=` in the address is the
   * filter's first value. Read once, at the open - the address is where the link put it, not a
   * store this screen writes back into.
   */
  const asked = typeof location === 'object' ? new URLSearchParams(location.search) : new URLSearchParams();
  let ruleFilter = $state(asked.get('rule_id') ?? '');
  let statusFilter = $state('');
  let triggerFilter = $state('');
  /* The window (F8-02): `from` inclusive, `to` exclusive, on the run's own moment. */
  let from = $state('');
  let to = $state('');
  let opened = $state<string | undefined>(undefined);
  let testing = $state('');
  let sampleType = $state('');
  let sampleSubject = $state('');
  let tested = $state<TestResult | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  const reversed = $derived(from !== '' && to !== '' && new Date(to).getTime() <= new Date(from).getTime());
  const filter = $derived({
    ...(ruleFilter ? { ruleId: ruleFilter } : {}),
    ...(statusFilter ? { status: statusFilter } : {}),
    ...(triggerFilter ? { trigger: triggerFilter } : {}),
    ...(from && !reversed ? { from: new Date(from).toISOString() } : {}),
    ...(to && !reversed ? { to: new Date(to).toISOString() } : {}),
  });

  $effect(() => untrack(() => rules.open()));
  $effect(() => {
    const current = filter;
    return untrack(() => runs.open(current));
  });

  const reading = $derived(runs.stateOf(filter));
  const listed = $derived(runs.of(filter));
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const statuses = ['RUNNING', 'WAITING', 'SUCCEEDED', 'SKIPPED', 'FAILED', 'ABORTED_LOOP', 'THROTTLED'];
  const triggers = $derived(manifest.value?.automation?.triggers ?? []);

  /** The strip over the page (F8-07): what the filter shows, counted - not the workspace's totals. */
  const counts = $derived({
    runs: listed.length,
    succeeded: listed.filter((run) => run.status === 'SUCCEEDED').length,
    skipped: listed.filter((run) => run.status === 'SKIPPED').length,
    throttled: listed.filter((run) => run.status === 'THROTTLED').length,
    failed: listed.filter((run) => run.status === 'FAILED' || run.status === 'ABORTED_LOOP').length,
  });

  const ruleName = (id: string) => rules.all.find((rule) => rule.id === id)?.name ?? id;

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : undefined;

  const statusWord = (status: string) =>
    messages.has(`app.runs.status_${status.toLowerCase()}`)
      ? t(`app.runs.status_${status.toLowerCase()}`)
      : status;

  /** A `RUNNING` row whose start is long past is a process that died, not one still working. */
  const isStale = (run: Run) =>
    run.status === 'RUNNING' && Date.now() - new Date(run.started_at).getTime() > STALE_AFTER_MS;

  /** Which condition stopped a skipped run — the first that did not hold. */
  function stoppedBy(run: Run): number | undefined {
    const failed = run.condition_results.find((result) => !result.matched);
    return failed?.index;
  }

  /** What a replay will not do again. Said before the button rather than after it. */
  const alreadyDone = (run: Run) =>
    run.action_results.filter((result) => result.status === 'SUCCEEDED');

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

  async function dryRun(ruleId: string): Promise<void> {
    if (!sampleType.trim()) return;
    await attempt(async () => {
      tested = await runs.dryRun(ruleId, {
        type: sampleType.trim(),
        subject: sampleSubject.trim() || undefined,
      });
    });
  }

  const actionWord = (result: ActionResult) =>
    result.path ? `${result.kind} · ${result.path}` : result.kind;
</script>

<div class="screen">
  <Stack gap="300">
    <PageHeader title={t('app.runs.title')} subtitle={t('app.runs.intro')} isTitleInBar={viewport.isCompact}>
      {#snippet notices()}
        {#if failure}
          <Banner tone="danger" title={failure.message}>
            {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
          </Banner>
        {/if}
      {/snippet}
    </PageHeader>

    <Stack gap="150">
      <h2 class="section">{t('app.runs.try_title')}</h2>
      <p class="quiet small">{t('app.runs.try_hint')}</p>
      <Select
        label={t('app.runs.which_rule')}
        bind:value={testing}
        placeholder={t('app.runs.choose_rule')}
        options={rules.all.map((rule) => ({ value: rule.id, label: rule.name }))}
      />
      {#if testing}
        <Input label={t('app.runs.sample_type')} hint={t('app.runs.sample_type_hint')} bind:value={sampleType} />
        <Input label={t('app.runs.sample_subject')} hint={t('app.runs.sample_subject_hint')} bind:value={sampleSubject} />
        <div class="row">
          <Button tone="primary" isBusy={isWorking} busyLabel={t('app.runs.trying')} onclick={() => void dryRun(testing)}>
            {t('app.runs.try')}
          </Button>
          <Button
            tone="secondary"
            isBusy={isWorking}
            busyLabel={t('app.runs.triggering')}
            onclick={() => void attempt(() => runs.trigger(testing), t('app.runs.triggered_announced'))}
          >
            {t('app.runs.trigger')}
          </Button>
        </div>
      {/if}

      {#if tested}
        <div class="result">
          <Stack gap="100">
            <div class="row">
              <RunStatusBadge
                status={tested.matched ? 'SUCCEEDED' : 'SKIPPED'}
                label={tested.matched ? t('app.runs.would_act') : t('app.runs.would_not_act')}
                isDryRun
                dryRunLabel={t('app.runs.dry_run')}
              />
            </div>
            <!-- Beside the results rather than in a heading: this is where somebody reading
                 "would have assigned Ada" is actually looking. -->
            <p class="quiet small">{t('app.runs.wrote_nothing')}</p>

            {#each tested.condition_results as result (result.index)}
              <p class="line">
                {#if result.error_code}
                  <!-- Not the same as a condition that did not hold: this one could not be
                       evaluated at all. -->
                  <Badge tone="danger">{t('app.runs.condition_broken')}</Badge>
                {:else}
                  <Badge tone={result.matched ? 'success' : 'neutral'}>
                    {result.matched ? t('app.runs.condition_held') : t('app.runs.condition_did_not')}
                  </Badge>
                {/if}
                <span>{t('app.runs.condition_index', { index: String(result.index + 1) })}</span>
              </p>
            {/each}

            <ul class="actions">
              {#each tested.actions as action (action.path)}
                <li>
                  <Badge tone={action.would_run ? 'info' : 'neutral'}>
                    {action.would_run ? t('app.runs.would_run') : t('app.runs.would_not_run')}
                  </Badge>
                  <span class="mono">{action.kind} · {action.path}</span>
                  {#if action.summary}<span class="quiet">{action.summary}</span>{/if}
                </li>
              {/each}
            </ul>
            <!-- Both arms appear, which a real run's log does not do: the honest answer to "what
                 would happen" includes "and what if it had not". -->
            <p class="quiet small">{t('app.runs.both_arms')}</p>
          </Stack>
        </div>
      {/if}
    </Stack>

    <Stack gap="150">
      <h2 class="section">{t('app.runs.list_title')}</h2>
      <div class="row">
        <Select
          label={t('app.runs.filter_rule')}
          bind:value={ruleFilter}
          placeholder={t('app.runs.all_rules')}
          options={rules.all.map((rule) => ({ value: rule.id, label: rule.name }))}
        />
        <Select
          label={t('app.runs.filter_status')}
          bind:value={statusFilter}
          placeholder={t('app.runs.all_statuses')}
          options={statuses.map((status) => ({ value: status, label: statusWord(status) }))}
        />
        <Select
          label={t('app.runs.filter_trigger')}
          bind:value={triggerFilter}
          placeholder={t('app.runs.all_triggers')}
          options={triggers.map((kind) => ({ value: kind, label: messages.has(`app.rules.trigger_${kind.toLowerCase()}`) ? t(`app.rules.trigger_${kind.toLowerCase()}`) : kind }))}
        />
        <Input label={t('app.runs.filter_from')} type="datetime-local" bind:value={from} />
        <Input label={t('app.runs.filter_to')} type="datetime-local" bind:value={to} error={reversed ? t('app.runs.window_reversed') : undefined} />
      </div>

      <!-- What the filter shows, counted: a reader narrowing to one rule and a week reads the
           week's numbers here, not the workspace's. -->
      <dl class="strip" data-strip>
        {#each [['runs', counts.runs], ['succeeded', counts.succeeded], ['skipped', counts.skipped], ['throttled', counts.throttled], ['failed', counts.failed]] as [key, value] (key)}
          <div class="stat"><dd>{value}</dd><dt>{t(`app.runs.strip_${key}`)}</dt></div>
        {/each}
      </dl>

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="waiting"><Spinner label={t('app.runs.reading')} /> <span>{t('app.runs.reading')}</span></p>
      {:else if refusal}
        <Banner tone="danger" title={refusal.message}>
          {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
        </Banner>
      {:else}
        {#each listed as run (run.id)}
          <div class="run">
            <Stack gap="100">
              <div class="row">
                <RunStatusBadge
                  status={run.status}
                  label={isStale(run) ? t('app.runs.status_crashed') : statusWord(run.status)}
                  isDryRun={run.is_dry_run ?? false}
                  dryRunLabel={t('app.runs.dry_run')}
                />
                <span class="name">{ruleName(run.rule_id)}</span>
                <span class="quiet small">{when(run.started_at)}</span>
              </div>

              <p class="quiet small">
                {t('app.runs.started_by', { trigger: messages.has(`app.rules.trigger_${run.trigger.toLowerCase()}`) ? t(`app.rules.trigger_${run.trigger.toLowerCase()}`) : run.trigger })}
                {#if run.triggered_by}
                  · {t('app.runs.pulled_by', { name: accounts.nameOf(run.triggered_by) ?? t('app.people.unnamed') })}
                {/if}
              </p>

              {#if run.status === 'SKIPPED' && stoppedBy(run) !== undefined}
                <!-- The question somebody asks first, and the reason the contract keeps the
                     condition results in the rule's own order. -->
                <p class="quiet small">{t('app.runs.skipped_by', { index: String((stoppedBy(run) ?? 0) + 1) })}</p>
              {:else if run.status === 'THROTTLED'}
                <p class="quiet small">{t('app.runs.throttled_note')}</p>
              {:else if run.status === 'ABORTED_LOOP'}
                <p class="quiet small">{t('app.runs.aborted_note', { depth: String(run.causation_depth) })}</p>
              {:else if isStale(run)}
                <p class="quiet small">{t('app.runs.crashed_note')}</p>
              {/if}

              {#if run.error_code}
                <!-- The catalogue's sentence for it, not the code shown raw. -->
                <p class="failure" role="alert">{messages.has(run.error_code) ? t(run.error_code) : run.error_code}</p>
              {/if}

              <div class="row">
                <Button
                  size="sm"
                  tone="subtle"
                  onclick={() => {
                    opened = opened === run.id ? undefined : run.id;
                    if (opened) void runs.read(run.id);
                  }}
                >
                  {opened === run.id ? t('app.runs.hide_detail') : t('app.runs.show_detail')}
                </Button>
              </div>

              {#if opened === run.id}
                {@const detail = runs.detail(run.id) ?? run}
                <ul class="actions">
                  {#each detail.action_results as result (result.index)}
                    <li>
                      <Badge
                        tone={result.status === 'SUCCEEDED' ? 'success' : result.status === 'FAILED' ? 'danger' : 'neutral'}
                      >
                        {statusWord(result.status)}
                      </Badge>
                      <span class="mono">{actionWord(result)}</span>
                      {#if result.matched !== undefined}
                        <span class="quiet small">
                          {result.matched ? t('app.runs.branch_then_ran') : t('app.runs.branch_else_ran')}
                        </span>
                      {/if}
                      {#if result.error_code}
                        <span class="failure">
                          {messages.has(result.error_code) ? t(result.error_code) : result.error_code}
                        </span>
                      {/if}
                    </li>
                  {/each}
                </ul>

                {#if detail.status === 'FAILED'}
                  <!-- Said before the button: a replay finishes this run, it does not start a new
                       one, and what already succeeded is not done twice. -->
                  <Banner tone="info">
                    {t('app.runs.replay_note', { count: String(alreadyDone(detail).length) })}
                  </Banner>
                  <div>
                    <Button
                      tone="primary"
                      isBusy={isWorking}
                      busyLabel={t('app.runs.replaying')}
                      onclick={() => void attempt(() => runs.replay(detail.id), t('app.runs.replayed_announced'))}
                    >
                      {t('app.runs.replay')}
                    </Button>
                  </div>
                {/if}
              {/if}
            </Stack>
          </div>
        {:else}
          <p class="quiet">{t('app.runs.none')}</p>
        {/each}

        {#if runs.moreAfter(filter)}
          <div>
            <Button tone="secondary" onclick={() => void runs.more(filter)}>{t('app.runs.more')}</Button>
          </div>
        {/if}
      {/if}
    </Stack>
  </Stack>
</div>

<style>
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); }

  .run,
  .result {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .strip { margin: 0; display: grid; grid-template-columns: repeat(auto-fit, minmax(12ch, 1fr)); gap: var(--sp-100); }

  .stat { padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); }

  .stat dd { margin: 0; font-family: var(--font-display); font-size: var(--fs-400); font-weight: var(--fw-semibold); font-variant-numeric: tabular-nums; }

  .stat dt { font-size: var(--fs-075); color: var(--text-subtle); }

  .line { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .actions { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }

  .actions li { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .mono { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
</style>
