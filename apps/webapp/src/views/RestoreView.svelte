<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The most dangerous screen in the product, and the one built to make the safe path the easy one.
  //
  // **A restore is always rehearsed first.** `dry_run` is true in the contract and this screen
  // never flips it silently: the dry run goes first, its report is shown — what would be created,
  // overwritten, skipped — and only then is the *same request* offered without it. The second
  // request is composed from the first (`asked`), so what was reviewed is what runs; a screen that
  // rebuilt it from its fields could rebuild it from fields that changed while the report was
  // being read.
  //
  // **`REPLACE_TENANT` ends credentials, and the screen says so before the confirmation.** The
  // mode replaces the workspace's rows, and the accounts, sessions and tokens that could sign in
  // to it are among them: a person who runs it may not be able to sign back in. That sentence is
  // in the catalogue and on the screen — not in a tooltip.
  //
  // **Three doors, in this order.** The exact workspace name typed; a step-up token, carried in
  // the *body* because this is the one operation where the contract puts the proof there; and
  // `create_safety_backup`, on by default, whose switching off is its own confirmation naming what
  // it gives up.
  //
  // **`INSTANCE` and `NEW_TENANT` are not offered**, and the reason is stated rather than left as
  // an absence: they cross or create a tenant, which is the installation operator's business. They
  // are not in `MODES` at all, so nothing here can compose one.

  import { untrack } from 'svelte';

  import {
    Badge,
    Banner,
    Button,
    Checkbox,
    Input,
    ProgressBar,
    Select,
    Spinner,
    Stack,
    Switch,
  } from '@hubtask/design-system/components';

  import { backup, type Archive } from '../lib/data/backup.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { jobs, type Watch } from '../lib/data/jobs.svelte.ts';
  import { isTerminal, mayCancel } from '../lib/data/jobs.ts';
  import { restores, type Asked, type Mode, type Run } from '../lib/data/restore.svelte.ts';
  import { isDestructive, MODES } from '../lib/data/restore.ts';
  import { stepUp } from '../lib/data/stepup.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  /** What a collision may become. `SKIP` is the contract's default and the safe one. */
  const CONFLICT_RULES = ['SKIP', 'OVERWRITE', 'DUPLICATE'];

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  let targetId = $state('');
  let archiveId = $state('');
  let mode = $state<string>('INSPECT');
  let conflictRule = $state('SKIP');
  let safetyBackup = $state(true);
  let safetyOffAcknowledged = $state(false);
  let typedName = $state('');
  let costAcknowledged = $state(false);
  /** Which containers a selective restore names. Composed, never typed. */
  let chosen = $state<readonly string[]>([]);

  /** What the dry run was asked for. The second request is this one with `dry_run: false`. */
  let asked = $state<Asked | undefined>(undefined);
  /**
   * The form as it stood when the rehearsal was asked for.
   *
   * A report describes one request. Leaving it on screen after somebody has changed the mode
   * would be a report about a restore they are no longer composing — and the button beneath it
   * would run the old one, which is right and reads as wrong. So the report is shown only while
   * the form still matches it.
   */
  let rehearsedFrom = $state('');
  let rehearsalId = $state('');
  let realId = $state('');
  let watchedJob = $state('');

  $effect(() => untrack(() => backup.open()));
  $effect(() => untrack(() => workspace.open()));
  $effect(() => untrack(() => containers.start()));
  $effect(() => {
    const chosen = targetId;
    if (!chosen) return;
    void untrack(() => backup.readArchives(chosen));
  });

  const targets = $derived(backup.all);
  const archives = $derived(targetId ? backup.archivesAt(targetId) : { status: 'idle' as const });
  const watch = $derived<Watch | undefined>(watchedJob ? jobs.of(watchedJob) : undefined);
  /** Everything a request is made of, as one comparable value. */
  const signature = $derived(
    [targetId, archiveId, mode, conflictRule, [...chosen].sort().join(','), String(safetyBackup)].join('|'),
  );
  const rehearsal = $derived<Run | undefined>(
    rehearsalId && signature === rehearsedFrom ? restores.of(rehearsalId) : undefined,
  );
  const real = $derived<Run | undefined>(realId ? restores.of(realId) : undefined);
  const name = $derived(workspace.workspace?.display_name ?? '');
  const chosenMode = $derived(mode as Mode);
  const destructive = $derived(isDestructive(chosenMode));

  /** Whether the three doors are all open. Read once, so the button and the hint agree. */
  const doorsOpen = $derived(
    (chosenMode !== 'SELECTIVE' || chosen.length > 0) &&
      (!destructive ||
        (typedName.trim() === name && name !== '' && costAcknowledged &&
          (safetyBackup || safetyOffAcknowledged))),
  );

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : undefined;

  const sentence = (code: string | null | undefined) =>
    code ? (messages.has(code) ? t(code) : code) : undefined;

  const modeWord = (which: string) => t(`app.restore.mode_${which.toLowerCase()}`);

  /**
   * The archive the picker names, found by its **path**.
   *
   * `RestoreRequest.archive_id` is the path at the target rather than the manifest's
   * `archive_id`, and that is the contract's shape rather than a slip on this screen: a restore
   * has to be possible when the database that recorded the run is gone, so what names an archive
   * is where it lies. `hubctl restore` takes the same value from the same listing.
   */
  const chosenArchive = $derived<Archive | undefined>(
    archives.status === 'ready'
      ? archives.data.find((archive) => archive.path === archiveId)
      : undefined,
  );

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

  /** What this attempt is asking for, as one value, so the real run is the rehearsal again. */
  function compose(): Asked {
    return {
      target_id: targetId,
      archive_id: archiveId,
      mode: chosenMode,
      // Always this workspace. The two modes that mint or cross a tenant are the operator's and
      // are not offered, so there is nothing to choose — and the server refuses a request that
      // does not name one.
      ...(workspace.workspace?.id ? { target_tenant_id: workspace.workspace.id } : {}),
      ...(chosenMode === 'SELECTIVE' ? { selection: { container_ids: chosen } } : {}),
      conflict_rule: conflictRule as Asked['conflict_rule'],
      ...(destructive
        ? { create_safety_backup: safetyBackup, confirmation: typedName.trim() }
        : {}),
    };
  }

  /** The rehearsal. Writes nothing, and is the only way to reach the second button. */
  async function rehearse(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!targetId || !archiveId) return;
    await attempt(async () => {
      const request = compose();
      // The proof is demanded for a destructive mode on the dry run too — the server decides, and
      // `stepUp.around` is what turns its refusal into the prompt rather than a dead end.
      const accepted = await stepUp.around((token) => restores.start(request, true, token));
      asked = request;
      realId = '';
      rehearsedFrom = signature;
      rehearsalId = restores.idOf(accepted) ?? '';
      watchedJob = accepted.job_id;
    });
  }

  /**
   * The same request, without `dry_run`.
   *
   * `asked` rather than `compose()`: the fields may have moved while the report was being read,
   * and what was reviewed is what has to run.
   */
  async function runForReal(): Promise<void> {
    const request = asked;
    if (!request) return;
    await attempt(async () => {
      const accepted = await stepUp.around((token) => restores.start(request, false, token));
      realId = restores.idOf(accepted) ?? '';
      watchedJob = accepted.job_id;
    });
  }

  // A finished job means the report has arrived. Read it once, rather than polling for it: the
  // watcher already knows when to stop.
  $effect(() => {
    const current = watch;
    if (!current || current.watching || !isTerminal(current.job.status)) return;
    const rehearsed = rehearsalId;
    const done = realId;
    void untrack(async () => {
      if (rehearsed) await restores.read(rehearsed);
      if (done) await restores.read(done);
    });
  });

  /** One line of a report, skipping the counts a mode cannot produce. */
  function lines(run: Run): readonly { kind: string; count: number }[] {
    const report = run.report ?? {};
    return (
      [
        { kind: 'new', count: report.new ?? 0 },
        { kind: 'overwritten', count: report.overwritten ?? 0 },
        { kind: 'skipped', count: report.skipped ?? 0 },
        { kind: 'duplicated', count: report.duplicated ?? 0 },
        { kind: 'conflicts', count: report.conflicts ?? 0 },
        { kind: 'deleted', count: report.deleted ?? 0 },
        { kind: 'media', count: report.media ?? 0 },
      ] as const
    ).filter((line) => line.count > 0);
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.restore.title')}</h1>
    <p class="quiet">{t('app.restore.intro')}</p>

    <!-- Stated rather than left as an absence: a control that is simply missing tells a reader
         nothing about why, and the alternative is somebody's to know. -->
    <Banner tone="info" title={t('app.restore.operators_title')}>
      {t('app.restore.operators_note')}
    </Banner>

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

    <form class="panel" onsubmit={rehearse}>
      <Stack gap="150">
        <h2 class="section">{t('app.restore.what_title')}</h2>

        <Select
          label={t('app.restore.target')}
          bind:value={targetId}
          placeholder={t('app.restore.choose_target')}
          options={targets.map((target) => ({ value: target.id, label: target.name }))}
        />

        {#if targetId}
          {#if archives.status === 'loading'}
            <p class="waiting">
              <Spinner label={t('app.restore.reading_archives')} />
              <span>{t('app.restore.reading_archives')}</span>
            </p>
          {:else if archives.status === 'ready'}
            <!-- Composed from the listing at the target rather than typed: an archive identifier
                 nobody can check is an archive identifier somebody mistypes. -->
            <Select
              label={t('app.restore.archive')}
              bind:value={archiveId}
              placeholder={t('app.restore.choose_archive')}
              options={archives.data.map((archive) => ({
                value: archive.path,
                label: `${when(archive.created_at) ?? archive.path} · ${archive.mode}`,
              }))}
            />
            {#if chosenArchive}
              <p class="quiet small">
                {chosenArchive.path}
                {#if chosenArchive.size_bytes}
                  · {formatBytes(chosenArchive.size_bytes, messages.locale)}
                {/if}
                {#if chosenArchive.item_count !== undefined}
                  · {t('app.restore.archive_items', { count: String(chosenArchive.item_count) })}
                {/if}
              </p>
              {#if chosenArchive.complete === false}
                <p class="warning">{t('app.restore.archive_incomplete')}</p>
              {/if}
            {/if}
          {:else if archives.status === 'failed'}
            <p class="failure">{renderProblem(archives.error, messages).message}</p>
          {/if}
        {/if}

        <Select
          label={t('app.restore.mode')}
          bind:value={mode}
          options={MODES.map((which) => ({ value: which, label: modeWord(which) }))}
        />
        <p class="quiet small">{t(`app.restore.mode_${mode.toLowerCase()}_note`)}</p>

        {#if mode === 'SELECTIVE'}
          <fieldset class="selection">
            <legend>{t('app.restore.selection')}</legend>
            <!-- Composed from what is here rather than typed: an archive holds identifiers, and an
                 identifier somebody types is an identifier somebody mistypes. -->
            {#each containers.hubs as container (container.id)}
              <Checkbox
                label={container.name}
                checked={chosen.includes(container.id)}
                onchange={(event) => {
                  const wanted = (event.currentTarget as HTMLInputElement).checked;
                  chosen = wanted
                    ? [...chosen, container.id]
                    : chosen.filter((id) => id !== container.id);
                }}
              />
            {/each}
          </fieldset>
          <p class="quiet small">{t('app.restore.selection_note')}</p>
        {/if}

        {#if mode === 'SELECTIVE' || mode === 'MERGE'}
          <Select
            label={t('app.restore.conflict_rule')}
            hint={t('app.restore.conflict_rule_hint')}
            bind:value={conflictRule}
            options={CONFLICT_RULES.map((rule) => ({
              value: rule,
              label: t(`app.restore.conflict_${rule.toLowerCase()}`),
            }))}
          />
        {/if}

        {#if destructive}
          <!-- The first door, and the sentence that has to be read before the other two. The
               catalogue carries it, because it is a fact about the operation rather than a
               component's caption. -->
          <Banner tone="danger" title={t('app.restore.replace_title')}>
            <Stack gap="100">
              <span>{t('app.restore.replace_note')}</span>
              <span>{t('app.restore.credentials_note')}</span>
            </Stack>
          </Banner>

          <Checkbox
            label={t('app.restore.acknowledge_cost')}
            bind:checked={costAcknowledged}
          />

          <Input
            label={t('app.restore.type_name', { name })}
            hint={t('app.restore.type_name_hint')}
            bind:value={typedName}
          />

          <Switch
            label={t('app.restore.safety_backup')}
            hint={t('app.restore.safety_backup_hint')}
            bind:checked={safetyBackup}
          />
          {#if !safetyBackup}
            <!-- Its own confirmation, naming what it gives up. Turning the copy off is a second
                 decision, not a side effect of the first. -->
            <Banner tone="danger" title={t('app.restore.no_safety_title')}>
              <Stack gap="100">
                <span>{t('app.restore.no_safety_note')}</span>
                <Checkbox
                  label={t('app.restore.acknowledge_no_safety')}
                  bind:checked={safetyOffAcknowledged}
                />
              </Stack>
            </Banner>
          {/if}
        {/if}

        <div>
          <Button
            type="submit"
            tone="primary"
            isBusy={isWorking}
            busyLabel={t('app.restore.rehearsing')}
            disabledReason={doorsOpen ? undefined : t('app.restore.doors_closed')}
          >
            {t('app.restore.rehearse')}
          </Button>
        </div>
        <p class="quiet small">{t('app.restore.rehearse_note')}</p>
      </Stack>
    </form>

    {#if watch}
      <section class="panel">
        <Stack gap="100">
          <h2 class="section">
            {realId ? t('app.restore.running_title') : t('app.restore.rehearsing_title')}
          </h2>
          {#if watch.watching}
            <ProgressBar
              label={realId ? t('app.restore.running_title') : t('app.restore.rehearsing_title')}
              value={watch.job.progress === null || watch.job.progress === undefined
                ? undefined
                : Math.round(watch.job.progress * 100)}
              valueLabel={watch.job.progress === null || watch.job.progress === undefined
                ? t('app.restore.progress_unknown')
                : t('app.restore.progress_share', {
                    percent: String(Math.round(watch.job.progress * 100)),
                  })}
            />
            {#if mayCancel(watch.job.status)}
              <div>
                <Button
                  size="sm"
                  tone="secondary"
                  onclick={() => void attempt(() => jobs.cancel(watch!.job.job_id))}
                >
                  {t('app.restore.cancel')}
                </Button>
              </div>
            {/if}
          {:else if watch.unreachable}
            <p class="failure">{sentence(watch.unreachable)}</p>
          {:else if watch.job.status !== 'SUCCEEDED'}
            <p class="failure">
              {sentence(watch.job.error_code) ?? t('app.restore.failed')}
            </p>
          {/if}
        </Stack>
      </section>
    {/if}

    {#if rehearsal}
      <section class="panel">
        <Stack gap="150">
          <div class="row">
            <h2 class="section">{t('app.restore.report_title')}</h2>
            <Badge tone="info">{t('app.restore.was_a_rehearsal')}</Badge>
          </div>

          {#if rehearsal.status !== 'SUCCEEDED'}
            <!-- A rehearsal that did not finish has no report, and an empty report rendered as
                 "nothing would change" would be this screen telling somebody a restore is safe
                 because it could not read the archive. -->
            <Banner tone="danger" title={t('app.restore.rehearsal_failed')}>
              {sentence(rehearsal.error_code) ?? t('app.restore.failed')}
            </Banner>
          {/if}
          <p class="quiet small">
            {modeWord(rehearsal.mode)} · {rehearsal.source_archive}
          </p>

          {#if rehearsal.status === 'SUCCEEDED'}
            <ul class="report">
              {#each lines(rehearsal) as line (line.kind)}
                <li class="quiet small">
                  {t(`app.restore.report_${line.kind}`, { count: String(line.count) })}
                </li>
              {:else}
                <li class="quiet small">{t('app.restore.report_nothing')}</li>
              {/each}
            </ul>
          {/if}

          {#each Object.entries(rehearsal.report?.withheld ?? {}) as [reason, count] (reason)}
            <!-- What the restore would deliberately not bring back, and why. -->
            <p class="quiet small">
              {messages.has(`app.restore.withheld_${reason}`)
                ? t(`app.restore.withheld_${reason}`, { count: String(count) })
                : t('app.restore.withheld_other', { reason, count: String(count) })}
            </p>
          {/each}

          <p class="quiet small">{t('app.restore.rehearsal_wrote_nothing')}</p>

          {#if !realId && rehearsal.status === 'SUCCEEDED'}
            <!-- The second request, composed from the first. Offered only once a report exists. -->
            <div>
              <Button
                tone="danger"
                isBusy={isWorking}
                busyLabel={t('app.restore.running')}
                onclick={() => void runForReal()}
              >
                {t('app.restore.run_for_real')}
              </Button>
            </div>
            <p class="quiet small">{t('app.restore.run_for_real_note')}</p>
          {/if}
        </Stack>
      </section>
    {/if}

    {#if real}
      <section class="panel">
        <Stack gap="150">
          <div class="row">
            <h2 class="section">{t('app.restore.done_title')}</h2>
            <Badge tone={real.status === 'SUCCEEDED' ? 'success' : 'danger'}>{real.status}</Badge>
          </div>
          <ul class="report">
            {#each lines(real) as line (line.kind)}
              <li class="quiet small">
                {t(`app.restore.report_${line.kind}`, { count: String(line.count) })}
              </li>
            {/each}
          </ul>
          {#if real.safety_backup_run_id}
            <!-- The way back is a run identifier rather than a search at the target. -->
            <p class="quiet small">
              {t('app.restore.safety_taken', { id: real.safety_backup_run_id })}
            </p>
          {/if}
          {#if real.error_code}
            <p class="failure">{sentence(real.error_code)}</p>
          {/if}
        </Stack>
      </section>
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

  .section {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
  }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

  .warning { margin: 0; color: var(--text-warning); font-size: var(--fs-075); }

  .waiting {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .selection { margin: 0; padding: 0; border: none; display: grid; gap: var(--sp-050); }

  .selection legend {
    padding: 0 0 var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .report { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
</style>
