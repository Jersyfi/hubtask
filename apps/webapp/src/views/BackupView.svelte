<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where the archives go, when they are written, and whether they open.
  //
  // **The first screen that watches a job**, and the pattern the next three reuse. `POST /backups`
  // answers a `JobRef` rather than a result; the answer arrives at `GET /jobs/{jobId}`, and the
  // watching is `lib/data/jobs.svelte.ts`'s rather than this file's. `progress: null` is the
  // indeterminate case rather than zero — a bar at zero would claim nothing has happened on a job
  // that is halfway through.
  //
  // **"We have backups" and "we have backups that open" are different claims**, and the screen
  // keeps them apart: a run says when it was last verified, and a target that has never had a
  // verified run says so in as many words.
  //
  // **What is at the target is read from the target.** There is no listing of runs in the
  // contract, and that is deliberate rather than missing: the manifests where the archives lie
  // still answer after a total loss, which is exactly the moment somebody needs them. An archive's
  // id is its run's id, so a row's verification is one read away.
  //
  // **No credential is ever read back.** A target's credentials go in at creation and the contract
  // answers `config` without them; there is nothing here to show and nothing to ask for.

  import { untrack } from 'svelte';

  import {
    Badge,
    Banner,
    Button,
    Input,
    ProgressBar,
    Select,
    Spinner,
    Stack,
    Switch,
  } from '@hubtask/design-system/components';

  import { backup, type Archive, type Schedule, type Target } from '../lib/data/backup.svelte.ts';
  import { generations, readRule } from '../lib/data/backup.ts';
  import { jobs, type Watch } from '../lib/data/jobs.svelte.ts';
  import { isTerminal, mayCancel } from '../lib/data/jobs.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  /** The kinds a `config` shape is written for here. The rest are the operator's, through hubctl. */
  const KINDS = ['LOCAL', 'S3', 'SFTP'] as const;

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  // The target form. `credentials` is write-only by construction: it is never read back into it.
  let draftName = $state('');
  let draftKind = $state<string>('LOCAL');
  let draftPath = $state('');
  let draftEndpoint = $state('');
  let draftBucket = $state('');
  let draftAccessKey = $state('');
  let draftSecretKey = $state('');
  let draftHost = $state('');
  let draftUser = $state('');
  let draftPassword = $state('');

  // The schedule form.
  // Empty rather than undefined: `Select`'s `value` prop has a default, and Svelte refuses a
  // binding whose initial value is undefined (`props_invalid_value`).
  let scheduleTarget = $state('');
  let scheduleRule = $state('FREQ=DAILY;BYHOUR=3;BYMINUTE=0');
  let scheduleZone = $state(Intl.DateTimeFormat().resolvedOptions().timeZone);
  let scheduleMinKeep = $state('3');

  // What is on screen rather than what exists: a target whose archives somebody opened, the run
  // a started job produced, and the confirmation a delete is behind.
  let openedTarget = $state<string | undefined>(undefined);
  let startedJob = $state<string | undefined>(undefined);
  let verifying = $state<string | undefined>(undefined);
  let removing = $state<string | undefined>(undefined);

  $effect(() => untrack(() => backup.open()));

  const reading = $derived(backup.targets);
  const targets = $derived(backup.all);
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );
  const schedules = $derived(
    backup.schedules.status === 'ready' ? backup.schedules.data : ([] as readonly Schedule[]),
  );
  const watched = $derived<Watch | undefined>(startedJob ? jobs.of(startedJob) : undefined);

  // A job that has just ended is what makes the rows on screen out of date, and re-reading is the
  // answer rather than a second polling loop: the watcher already knows when to stop.
  $effect(() => {
    const watch = watched;
    if (!watch || watch.watching || !isTerminal(watch.job.status)) return;
    const target = openedTarget;
    void untrack(async () => {
      // `refresh` rather than the cached answer: the server's listing of the target is from
      // before the run that just ended, and reading the runs is what `readArchives` does after.
      if (target) await backup.readArchives(target, true);
    });
  });

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : undefined;

  const sentence = (code: string | null | undefined) =>
    code ? (messages.has(code) ? t(code) : code) : undefined;

  const nameOf = (targetId: string) =>
    targets.find((target) => target.id === targetId)?.name ?? targetId;

  /** The rule as what it means. Never as when it next fires — that is `next_run_at`, the server's. */
  function ruleSentence(schedule: Schedule): string {
    const reading = readRule(schedule.rrule);
    if (reading.partial || !reading.frequency) return schedule.rrule;
    const at = reading.at
      ? t('app.backup.rule_at', {
          time: `${String(reading.at.hour).padStart(2, '0')}:${String(reading.at.minute).padStart(2, '0')}`,
        })
      : '';
    const days = reading.weekdays
      .map((day) => t(`app.backup.weekday_${day.toLowerCase()}`))
      .join(', ');
    const every =
      reading.interval === 1
        ? t(`app.backup.rule_${reading.frequency.toLowerCase()}`)
        : t(`app.backup.rule_every_${reading.frequency.toLowerCase()}`, {
            interval: String(reading.interval),
          });
    return [every, days, at].filter((part) => part !== '').join(' ');
  }

  /** The archive rows of one target, once somebody has opened it. */
  const archivesOf = (targetId: string) => backup.archivesAt(targetId);

  /** Whether this target has ever had a run verified, from the rows this tab has read. */
  function lastVerified(list: readonly Archive[]): string | undefined {
    const stamps = list
      .map((archive) => backup.runOf(archive.archive_id)?.verified_at)
      .filter((stamp): stamp is string => typeof stamp === 'string');
    return stamps.sort().at(-1);
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

  /** What the chosen kind needs, as the contract's own `config`. Nothing here is a credential. */
  function configuration(): Record<string, unknown> {
    if (draftKind === 'S3') return { endpoint: draftEndpoint.trim(), bucket: draftBucket.trim() };
    if (draftKind === 'SFTP') return { host: draftHost.trim(), path: draftPath.trim() };
    return { path: draftPath.trim() };
  }

  /** What goes in once and is never answered. */
  function credentials(): Record<string, unknown> | undefined {
    if (draftKind === 'S3' && draftAccessKey) {
      return { access_key: draftAccessKey, secret_key: draftSecretKey };
    }
    if (draftKind === 'SFTP' && draftUser) {
      return { username: draftUser, password: draftPassword };
    }
    return undefined;
  }

  async function createTarget(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!draftName.trim()) return;
    await attempt(async () => {
      await backup.createTarget({
        name: draftName.trim(),
        kind: draftKind as Target['kind'],
        config: configuration(),
        ...(credentials() ? { credentials: credentials() } : {}),
      });
      draftName = '';
      draftPath = '';
      draftEndpoint = '';
      draftBucket = '';
      draftAccessKey = '';
      draftSecretKey = '';
      draftHost = '';
      draftUser = '';
      draftPassword = '';
    });
  }

  async function createSchedule(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!scheduleTarget) return;
    const floor = Number.parseInt(scheduleMinKeep, 10);
    await attempt(() =>
      backup.createSchedule({
        target_id: scheduleTarget,
        scope: { kind: 'TENANT' },
        rrule: scheduleRule.trim(),
        timezone: scheduleZone,
        ...(Number.isFinite(floor) && floor > 0 ? { retention: { min_keep: floor } } : {}),
      }),
    );
  }

  async function start(targetId: string): Promise<void> {
    await attempt(async () => {
      const accepted = await backup.start({ target_id: targetId });
      startedJob = accepted.job_id;
      openedTarget = targetId;
      verifying = undefined;
    });
  }

  async function verify(runId: string): Promise<void> {
    await attempt(async () => {
      const accepted = await backup.verify(runId);
      startedJob = accepted.job_id;
      verifying = runId;
    });
  }

  async function openTarget(targetId: string): Promise<void> {
    if (openedTarget === targetId) {
      openedTarget = undefined;
      return;
    }
    openedTarget = targetId;
    await attempt(async () => {
      await backup.readArchives(targetId);
      const list = backup.archivesAt(targetId);
      if (list.status !== 'ready') return;
      // Each archive's run is what carries `verified_at`; the manifest carries only whether the
      // checksums were checked at the target the last time somebody looked.
      for (const archive of list.data) await backup.readRun(archive.archive_id);
    });
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.backup.title')}</h1>
    <p class="quiet">{t('app.backup.intro')}</p>

    {#if failure}
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
      </Banner>
    {/if}

    {#if watched}
      <!-- The job, wherever it came from. One place for it rather than one per button: a screen
           with three watchers would be three loops disagreeing about which is the current one. -->
      <section class="panel">
        <Stack gap="100">
          <h2 class="section">
            {verifying ? t('app.backup.verifying_title') : t('app.backup.running_title')}
          </h2>
          {#if watched.watching}
            <ProgressBar
              label={verifying ? t('app.backup.verifying_title') : t('app.backup.running_title')}
              value={watched.job.progress === null || watched.job.progress === undefined
                ? undefined
                : Math.round(watched.job.progress * 100)}
              valueLabel={watched.job.progress === null || watched.job.progress === undefined
                ? t('app.backup.progress_unknown')
                : t('app.backup.progress_share', {
                    percent: String(Math.round(watched.job.progress * 100)),
                  })}
            />
            {#if mayCancel(watched.job.status)}
              <div>
                <Button
                  size="sm"
                  tone="secondary"
                  onclick={() => void attempt(() => jobs.cancel(watched!.job.job_id))}
                >
                  {t('app.backup.cancel')}
                </Button>
              </div>
              <!-- What a cancellation cannot take back, said where the button is rather than
                   after it has been pressed. -->
              <p class="quiet small">{t('app.backup.cancel_note')}</p>
            {/if}
          {:else if watched.job.status === 'SUCCEEDED'}
            <p class="good">
              {verifying ? t('app.backup.verify_done') : t('app.backup.run_done')}
            </p>
          {:else if watched.job.status === 'CANCELLED'}
            <p class="quiet">{t('app.backup.run_cancelled')}</p>
          {:else if watched.unreachable}
            <!-- A job whose status could not be read is not a job that failed, and saying so
                 would report a failure the server never reported. -->
            <p class="failure">{sentence(watched.unreachable)}</p>
          {:else}
            <p class="failure">
              {sentence(watched.job.error_code) ?? t('app.backup.run_failed')}
            </p>
          {/if}
        </Stack>
      </section>
    {/if}

    <Stack gap="150">
      <h2 class="section">{t('app.backup.targets_title')}</h2>

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="waiting">
          <Spinner label={t('app.backup.reading')} /> <span>{t('app.backup.reading')}</span>
        </p>
      {:else if refusal}
        <Banner tone="danger" title={refusal.message}>
          {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
        </Banner>
      {:else}
        {#each targets as target (target.id)}
          {@const probe = backup.probeOf(target.id)}
          {@const archives = archivesOf(target.id)}
          <section class="panel">
            <Stack gap="100">
              <div class="row">
                <span class="name">{target.name}</span>
                <Badge>{target.kind}</Badge>
                {#if target.encryption_mode === 'NONE'}
                  <Badge tone="warning">{t('backup.target_unencrypted')}</Badge>
                {/if}
              </div>

              {#each target.warnings ?? [] as warning (warning)}
                <!-- What the target says about itself, in the catalogue's words. -->
                <p class="warning">{sentence(warning)}</p>
              {/each}

              <p class="quiet small">
                {#if target.last_test_at}
                  {target.last_test_ok
                    ? t('app.backup.tested_ok', { at: when(target.last_test_at) ?? '' })
                    : t('app.backup.tested_failed', { at: when(target.last_test_at) ?? '' })}
                {:else}
                  {t('app.backup.never_tested')}
                {/if}
              </p>

              {#if probe}
                <p class={probe.ok ? 'good' : 'failure'}>
                  {#if probe.ok}
                    {t('app.backup.probe_ok', { ms: String(Math.round(probe.latency_ms)) })}
                    {#if probe.free_bytes}
                      · {t('app.backup.probe_free', {
                        size: formatBytes(probe.free_bytes, messages.locale),
                      })}
                    {/if}
                  {:else}
                    {sentence(probe.error_code) ?? t('app.backup.probe_failed')}
                    {#if !probe.writable}· {t('app.backup.probe_not_writable')}{/if}
                  {/if}
                </p>
              {/if}

              <div class="row">
                <Button
                  size="sm"
                  tone="secondary"
                  isBusy={isWorking}
                  busyLabel={t('app.backup.testing')}
                  onclick={() => void attempt(() => backup.test(target.id))}
                >
                  {t('app.backup.test')}
                </Button>
                <Button
                  size="sm"
                  tone="primary"
                  isBusy={isWorking}
                  busyLabel={t('app.backup.starting')}
                  onclick={() => void start(target.id)}
                >
                  {t('app.backup.start')}
                </Button>
                <Button size="sm" tone="subtle" onclick={() => void openTarget(target.id)}>
                  {openedTarget === target.id
                    ? t('app.backup.hide_archives')
                    : t('app.backup.show_archives')}
                </Button>
                <Button
                  size="sm"
                  tone="danger"
                  onclick={() => (removing = removing === target.id ? undefined : target.id)}
                >
                  {t('app.backup.remove')}
                </Button>
              </div>

              {#if removing === target.id}
                <!-- The one sentence somebody deleting a target needs: the archives stay where
                     they are. Hubtask never deletes a file it did not write, and it does not
                     delete the ones it did on the way out either. -->
                <Banner tone="warning" title={t('app.backup.remove_title')}>
                  <Stack gap="100">
                    <span>{t('app.backup.remove_note')}</span>
                    <div class="row">
                      <Button
                        size="sm"
                        tone="danger"
                        isBusy={isWorking}
                        busyLabel={t('app.backup.removing')}
                        onclick={() =>
                          void attempt(async () => {
                            await backup.deleteTarget(target.id);
                            removing = undefined;
                          })}
                      >
                        {t('app.backup.remove_confirm')}
                      </Button>
                      <Button size="sm" tone="subtle" onclick={() => (removing = undefined)}>
                        {t('app.workspace.cancel')}
                      </Button>
                    </div>
                  </Stack>
                </Banner>
              {/if}

              {#if openedTarget === target.id}
                {#if archives.status === 'loading'}
                  <p class="waiting">
                    <Spinner label={t('app.backup.reading_archives')} />
                    <span>{t('app.backup.reading_archives')}</span>
                  </p>
                {:else if archives.status === 'ready'}
                  {@const verified = lastVerified(archives.data)}
                  <p class="quiet small">
                    {verified
                      ? t('app.backup.last_verified', { at: when(verified) ?? '' })
                      : t('app.backup.never_verified')}
                  </p>
                  <ul class="archives">
                    {#each archives.data as archive (archive.archive_id)}
                      {@const run = backup.runOf(archive.archive_id)}
                      <li>
                        <Stack gap="025">
                          <div class="row">
                            <span class="mono">{archive.path}</span>
                            <Badge>{archive.mode}</Badge>
                            {#if !archive.complete}
                              <!-- Not damaged: a run still going, or one that died. -->
                              <Badge tone="warning">{t('app.backup.archive_incomplete')}</Badge>
                            {/if}
                            {#if run?.verify_ok === true}
                              <Badge tone="success">{t('app.backup.archive_opens')}</Badge>
                            {:else if run?.verify_ok === false}
                              <Badge tone="danger">{t('app.backup.archive_broken')}</Badge>
                            {/if}
                          </div>
                          <span class="quiet small">
                            {when(archive.created_at)}
                            {#if archive.size_bytes}
                              · {formatBytes(archive.size_bytes, messages.locale)}
                            {/if}
                            {#if run?.verified_at}
                              · {t('app.backup.verified_at', { at: when(run.verified_at) ?? '' })}
                            {:else}
                              · {t('app.backup.not_verified')}
                            {/if}
                          </span>
                          <div>
                            <Button
                              size="sm"
                              tone="secondary"
                              isBusy={isWorking}
                              busyLabel={t('app.backup.verifying')}
                              onclick={() => void verify(archive.archive_id)}
                            >
                              {t('app.backup.verify')}
                            </Button>
                          </div>
                        </Stack>
                      </li>
                    {:else}
                      <li class="quiet">{t('app.backup.no_archives')}</li>
                    {/each}
                  </ul>
                {:else if archives.status === 'failed'}
                  <p class="failure">{renderProblem(archives.error, messages).message}</p>
                {/if}
              {/if}
            </Stack>
          </section>
        {:else}
          <p class="quiet">{t('app.backup.no_targets')}</p>
        {/each}
      {/if}

      <form class="panel" onsubmit={createTarget}>
        <Stack gap="150">
          <h3 class="section">{t('app.backup.add_target')}</h3>
          <Input label={t('app.backup.target_name')} bind:value={draftName} isRequired />
          <Select
            label={t('app.backup.target_kind')}
            bind:value={draftKind}
            options={KINDS.map((kind) => ({ value: kind, label: kind }))}
          />
          {#if draftKind === 'LOCAL'}
            <Input
              label={t('app.backup.target_path')}
              hint={t('app.backup.target_path_hint')}
              bind:value={draftPath}
            />
          {:else if draftKind === 'S3'}
            <Input label={t('app.backup.target_endpoint')} bind:value={draftEndpoint} />
            <Input label={t('app.backup.target_bucket')} bind:value={draftBucket} />
            <Input label={t('app.backup.target_access_key')} bind:value={draftAccessKey} />
            <Input
              label={t('app.backup.target_secret_key')}
              type="password"
              bind:value={draftSecretKey}
            />
          {:else}
            <Input label={t('app.backup.target_host')} bind:value={draftHost} />
            <Input label={t('app.backup.target_path')} bind:value={draftPath} />
            <Input label={t('app.backup.target_user')} bind:value={draftUser} />
            <Input
              label={t('app.backup.target_password')}
              type="password"
              bind:value={draftPassword}
            />
          {/if}
          <!-- Said where the credentials are typed: they go in and the contract answers `config`
               without them, so there is nothing to come back and check later. -->
          <p class="quiet small">{t('app.backup.credentials_write_only')}</p>
          <div>
            <Button
              type="submit"
              tone="primary"
              isBusy={isWorking}
              busyLabel={t('app.backup.adding')}
            >
              {t('app.backup.add')}
            </Button>
          </div>
        </Stack>
      </form>
    </Stack>

    <Stack gap="150">
      <h2 class="section">{t('app.backup.schedules_title')}</h2>
      <p class="quiet small">{t('app.backup.schedules_intro')}</p>

      {#each schedules as schedule (schedule.id)}
        <section class="panel">
          <Stack gap="100">
            <div class="row">
              <span class="name">{ruleSentence(schedule)}</span>
              <Badge tone={schedule.enabled === false ? 'neutral' : 'success'}>
                {schedule.enabled === false ? t('app.backup.off') : t('app.backup.on')}
              </Badge>
            </div>
            <p class="quiet small">
              {t('app.backup.schedule_of', { target: nameOf(schedule.target_id) })}
              · {t('app.backup.schedule_zone', { zone: schedule.timezone ?? 'UTC' })}
            </p>
            <p class="quiet small">
              {#if schedule.next_run_at}
                {t('app.backup.next_run', { at: when(schedule.next_run_at) ?? '' })}
              {:else if schedule.enabled === false}
                {t('app.backup.no_next_run_off')}
              {:else}
                <!-- A rule may be perfectly good and simply spent, which is not an error. -->
                {t('app.backup.no_next_run_spent')}
              {/if}
            </p>

            <ul class="plan">
              {#each generations(schedule.retention) as line (line.kind)}
                <li class="quiet small">
                  {t(`app.backup.keep_${line.kind}`, { count: String(line.keep) })}
                </li>
              {/each}
            </ul>
            <!-- A floor under all of them rather than a sixth generation: a retention rule may
                 never result in no backup being left (`backup-restore.md` §6). -->
            <p class="quiet small">
              {t('app.backup.min_keep', { count: String(schedule.retention?.min_keep ?? 3) })}
            </p>

            <div class="row">
              <!-- The write happens on the switch rather than behind a save button: `enabled`
                   is one field with two values, and a form around it would be a form with one
                   control in it. -->
              <Switch
                label={t('app.backup.enabled')}
                checked={schedule.enabled !== false}
                onchange={(event) =>
                  void attempt(() =>
                    backup.updateSchedule(schedule.id, {
                      enabled: (event.currentTarget as HTMLInputElement).checked,
                    }),
                  )}
              />
              <Button
                size="sm"
                tone="danger"
                isBusy={isWorking}
                busyLabel={t('app.backup.removing')}
                onclick={() => void attempt(() => backup.deleteSchedule(schedule.id))}
              >
                {t('app.backup.remove_schedule')}
              </Button>
            </div>
            <p class="quiet small">{t('app.backup.switch_off_rather_than_delete')}</p>
          </Stack>
        </section>
      {:else}
        <p class="quiet">{t('app.backup.no_schedules')}</p>
      {/each}

      {#if targets.length > 0}
        <form class="panel" onsubmit={createSchedule}>
          <Stack gap="150">
            <h3 class="section">{t('app.backup.add_schedule')}</h3>
            <Select
              label={t('app.backup.schedule_target')}
              bind:value={scheduleTarget}
              placeholder={t('app.backup.choose_target')}
              options={targets.map((target) => ({ value: target.id, label: target.name }))}
            />
            <Input
              label={t('app.backup.schedule_rule')}
              hint={t('app.backup.schedule_rule_hint')}
              bind:value={scheduleRule}
            />
            <Input label={t('app.backup.schedule_zone_field')} bind:value={scheduleZone} />
            <Input
              label={t('app.backup.schedule_min_keep')}
              hint={t('app.backup.schedule_min_keep_hint')}
              type="number"
              bind:value={scheduleMinKeep}
            />
            <p class="quiet small">{t('app.backup.rule_is_the_servers')}</p>
            <div>
              <Button
                type="submit"
                tone="primary"
                isBusy={isWorking}
                busyLabel={t('app.backup.adding')}
              >
                {t('app.backup.add')}
              </Button>
            </div>
          </Stack>
        </form>
      {/if}
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

  .good { margin: 0; color: var(--text-success); font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

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

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .archives,
  .plan { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }

  .mono { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
</style>
