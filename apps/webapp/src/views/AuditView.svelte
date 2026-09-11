<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The trail: queried, verified, and exported to where it went (`audit.md` §3–§5).
  //
  // **An entry carries no user content, by design (ADR-0017)**, so an actor is an identifier plus
  // the label that was valid at the time. The screen resolves the identifier through the accounts
  // store the way the activity feed does, and falls back to the stored label — which is precisely
  // what survives an erasure, and therefore the marker an erased account leaves behind.
  //
  // **A break is a finding, not an error.** `:verify` answering `valid: false` with a sequence
  // number is the thing an audit trail exists to produce; rendering it as a failed request would
  // hide it. So the two outcomes are two different renderings, and the break says where it starts.
  //
  // **The export goes to a backup target and the browser never sees the archive.** ADR-0047 named
  // exactly one media origin in the policy and a backup target is not it, so what the screen shows
  // when the job ends is *where* the archive was written — not a download.
  //
  // **There is no filter on an export**, and the screen says so where somebody would otherwise
  // look for the filters they just used: an export narrowed before it was signed would be evidence
  // about somebody's selection rather than about an interval.

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
  } from '@hubtask/design-system/components';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { audit, readVerification, type Entry, type Query } from '../lib/data/audit.svelte.ts';
  import { backup } from '../lib/data/backup.svelte.ts';
  import { jobs, type Watch } from '../lib/data/jobs.svelte.ts';
  import { isTerminal } from '../lib/data/jobs.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const OUTCOMES = ['SUCCESS', 'DENIED', 'FAILED'];
  const FORMATS = ['JSONL', 'CSV'];

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  let from = $state('');
  let to = $state('');
  let action = $state('');
  let actorId = $state('');
  let targetType = $state('');
  let targetId = $state('');
  let outcome = $state('');

  // The query the listing is reading. Separate from the fields, so that typing into a filter does
  // not send a request per keystroke: the reader presses the button.
  let applied = $state<Query>({});

  let verifyFrom = $state('');
  let verifyTo = $state('');

  let exportFormat = $state('JSONL');
  let exportTarget = $state('');
  let exportJob = $state('');
  let exportedTo = $state<{ target: string; from: string; to: string } | undefined>(undefined);

  $effect(() => {
    const query = applied;
    return untrack(() => audit.open(query));
  });
  $effect(() => untrack(() => backup.open()));

  const reading = $derived(audit.stateOf(applied));
  const entries = $derived(audit.of(applied));
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );
  // Read into a finding rather than branched on in the template: a break is not an error, and
  // which of the two facts an answer is deserves a test that a `{#if}` cannot have.
  const finding = $derived(audit.verification ? readVerification(audit.verification) : undefined);
  const watch = $derived<Watch | undefined>(exportJob ? jobs.of(exportJob) : undefined);

  // The names behind the identifiers, asked for once each.
  $effect(() => {
    const listed = entries;
    untrack(() => accounts.resolve(listed.map((entry) => entry.actor?.id)));
  });

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : undefined;

  const sentence = (code: string | null | undefined) =>
    code ? (messages.has(code) ? t(code) : code) : undefined;

  /**
   * Who acted, in this order: the name the accounts store resolved, the label the entry stored,
   * and then the sentence that is true of every actor.
   *
   * The stored label is what an erasure leaves behind — an entry that only pointed at a foreign
   * key would become unreadable the moment the account went, which is the failure `audit.md` §2
   * describes. So an anonymised actor reads as its marker rather than as nothing.
   */
  function actorOf(entry: Entry): string {
    const kind = entry.actor?.type ?? 'SYSTEM';
    if (kind === 'SYSTEM') return t('app.audit.actor_system');
    return (
      accounts.nameOf(entry.actor?.id) ??
      entry.actor?.label ??
      t('app.audit.actor_gone')
    );
  }

  const outcomeTone = (entry: Entry) =>
    entry.outcome === 'SUCCESS' ? 'success' : entry.outcome === 'DENIED' ? 'warning' : 'danger';

  const severityTone = (entry: Entry) =>
    entry.severity === 'CRITICAL' ? 'danger' : entry.severity === 'WARNING' ? 'warning' : 'neutral';

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

  function apply(event: SubmitEvent): void {
    event.preventDefault();
    applied = {
      ...(from ? { from: new Date(from).toISOString() } : {}),
      ...(to ? { to: new Date(to).toISOString() } : {}),
      ...(action.trim() ? { action: action.trim() } : {}),
      ...(actorId.trim() ? { actorId: actorId.trim() } : {}),
      ...(targetType.trim() ? { targetType: targetType.trim() } : {}),
      ...(targetId.trim() ? { targetId: targetId.trim() } : {}),
      ...(outcome ? { outcome } : {}),
    };
  }

  // The report has arrived once the job is over. Read once, rather than a second polling loop.
  $effect(() => {
    const current = watch;
    if (!current || current.watching || !isTerminal(current.job.status)) return;
    if (current.job.status !== 'SUCCEEDED') return;
    untrack(() => {
      const target = backup.all.find((each) => each.id === exportTarget);
      exportedTo = {
        target: target?.name ?? exportTarget,
        from: verifyFrom || from,
        to: verifyTo || to,
      };
    });
  });
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.audit.title')}</h1>
    <p class="quiet">{t('app.audit.intro')}</p>

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

    <form class="panel" onsubmit={apply}>
      <Stack gap="150">
        <h2 class="section">{t('app.audit.query_title')}</h2>
        <div class="fields">
          <Input label={t('app.audit.from')} type="datetime-local" bind:value={from} />
          <Input label={t('app.audit.to')} type="datetime-local" bind:value={to} />
          <Input
            label={t('app.audit.action')}
            hint={t('app.audit.action_hint')}
            bind:value={action}
          />
          <Input label={t('app.audit.actor')} hint={t('app.audit.actor_hint')} bind:value={actorId} />
          <Input
            label={t('app.audit.target_type')}
            hint={t('app.audit.target_type_hint')}
            bind:value={targetType}
          />
          <Input label={t('app.audit.target_id')} bind:value={targetId} />
          <Select
            label={t('app.audit.outcome')}
            bind:value={outcome}
            placeholder={t('app.audit.any_outcome')}
            options={OUTCOMES.map((value) => ({
              value,
              label: t(`app.audit.outcome_${value.toLowerCase()}`),
            }))}
          />
        </div>
        <div>
          <Button type="submit" tone="primary">{t('app.audit.apply')}</Button>
        </div>
        <!-- The one thing about reading the trail that is worth saying out loud. -->
        <p class="quiet small">{t('app.audit.reading_is_not_recorded')}</p>
      </Stack>
    </form>

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting">
        <Spinner label={t('app.audit.reading')} /> <span>{t('app.audit.reading')}</span>
      </p>
    {:else if refusal}
      <Banner tone="danger" title={refusal.message}>
        {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
      </Banner>
    {:else}
      <ul class="entries">
        {#each entries as entry (entry.id)}
          <li class="entry">
            <Stack gap="025">
              <div class="row">
                <Badge tone={outcomeTone(entry)}>
                  {t(`app.audit.outcome_${entry.outcome.toLowerCase()}`)}
                </Badge>
                {#if entry.severity && entry.severity !== 'INFO'}
                  <Badge tone={severityTone(entry)}>
                    {t(`app.audit.severity_${entry.severity.toLowerCase()}`)}
                  </Badge>
                {/if}
                <span class="mono">{entry.action}</span>
                {#if entry.seq !== undefined}
                  <span class="quiet small">{t('app.audit.sequence', { seq: String(entry.seq) })}</span>
                {/if}
              </div>
              <p class="quiet small">
                {t('app.audit.by', { actor: actorOf(entry) })}
                · {when(entry.occurred_at)}
                {#if entry.context?.channel}· {entry.context.channel}{/if}
              </p>
              {#if entry.target?.type}
                <p class="quiet small">
                  {t('app.audit.about', {
                    type: entry.target.type,
                    what: entry.target.label ?? entry.target.id ?? '',
                  })}
                </p>
              {/if}
              {#each entry.changes ?? [] as change (change.field)}
                <p class="quiet small">
                  {#if change.from_hash !== undefined && change.from_hash !== null}
                    <!-- A sensitive field: that it changed, and two hashes, which makes two
                         entries comparable without either being readable. -->
                    {t('app.audit.changed_sensitive', { field: change.field ?? '' })}
                  {:else}
                    {t('app.audit.changed', {
                      field: change.field ?? '',
                      to: String(change.to ?? ''),
                    })}
                  {/if}
                </p>
              {/each}
            </Stack>
          </li>
        {:else}
          <li class="quiet">{t('app.audit.none')}</li>
        {/each}
      </ul>

      {#if audit.moreAfter(applied)}
        <div>
          <Button tone="secondary" onclick={() => void attempt(() => audit.more(applied))}>
            {t('app.audit.more')}
          </Button>
        </div>
      {/if}
    {/if}

    <Stack gap="150">
      <h2 class="section">{t('app.audit.verify_title')}</h2>
      <p class="quiet small">{t('app.audit.verify_intro')}</p>
      <div class="fields">
        <Input label={t('app.audit.from')} type="datetime-local" bind:value={verifyFrom} />
        <Input label={t('app.audit.to')} type="datetime-local" bind:value={verifyTo} />
      </div>
      <div>
        <Button
          tone="secondary"
          isBusy={isWorking}
          busyLabel={t('app.audit.verifying')}
          disabledReason={verifyFrom && verifyTo ? undefined : t('app.audit.period_first')}
          onclick={() =>
            void attempt(() =>
              audit.verify(new Date(verifyFrom).toISOString(), new Date(verifyTo).toISOString()),
            )}
        >
          {t('app.audit.verify')}
        </Button>
      </div>

      {#if finding?.kind === 'holds'}
        <Banner tone="success" title={t('app.audit.chain_holds')}>
          <Stack gap="050">
            <span>{t('app.audit.chain_checked', { count: String(finding.checked) })}</span>
            {#if finding.sealedUntil}
              <span>{t('app.audit.sealed_until', { at: when(finding.sealedUntil) ?? '' })}</span>
            {:else}
              <!-- What the check does and does not prove. Inside the database only. -->
              <span>{t('app.audit.never_anchored')}</span>
            {/if}
          </Stack>
        </Banner>
      {:else if finding?.kind === 'broken'}
        <!-- The finding this whole trail exists to produce, rendered as a finding rather than as
             an error message: it names where the chain stops holding, which is where an
             investigation starts. -->
        <Banner tone="danger" title={t('app.audit.chain_broken')}>
          <Stack gap="050">
            {#if finding.firstBrokenSeq !== undefined}
              <span>{t('app.audit.broken_at', { seq: String(finding.firstBrokenSeq) })}</span>
            {/if}
            {#if finding.gapCount > 0}
              <span>{t('app.audit.gaps', { count: String(finding.gapCount) })}</span>
              {#if finding.gaps.length > 0}
                <span class="mono small">{finding.gaps.join(', ')}</span>
              {/if}
            {/if}
            <span>{t('app.audit.break_is_recorded')}</span>
          </Stack>
        </Banner>
      {/if}
    </Stack>

    <Stack gap="150">
      <h2 class="section">{t('app.audit.export_title')}</h2>
      <p class="quiet small">{t('app.audit.export_intro')}</p>
      <!-- Said where somebody would otherwise look for the filters they just used. -->
      <p class="quiet small">{t('app.audit.export_has_no_filter')}</p>

      <div class="fields">
        <Select
          label={t('app.audit.format')}
          bind:value={exportFormat}
          options={FORMATS.map((value) => ({
            value,
            label: t(`app.audit.format_${value.toLowerCase()}`),
          }))}
        />
        <Select
          label={t('app.audit.export_target')}
          bind:value={exportTarget}
          placeholder={t('app.audit.choose_target')}
          options={backup.all.map((target) => ({ value: target.id, label: target.name }))}
        />
      </div>

      <div>
        <Button
          tone="primary"
          isBusy={isWorking}
          busyLabel={t('app.audit.exporting')}
          disabledReason={exportTarget && verifyFrom && verifyTo
            ? undefined
            : t('app.audit.export_needs')}
          onclick={() =>
            void attempt(async () => {
              exportedTo = undefined;
              const accepted = await audit.export({
                from: new Date(verifyFrom).toISOString(),
                to: new Date(verifyTo).toISOString(),
                format: exportFormat as 'JSONL' | 'CSV',
                target_id: exportTarget,
              });
              exportJob = accepted.job_id;
            })}
        >
          {t('app.audit.export')}
        </Button>
      </div>
      <p class="quiet small">{t('app.audit.export_is_audited')}</p>

      {#if watch}
        {#if watch.watching}
          <ProgressBar
            label={t('app.audit.export_title')}
            value={watch.job.progress === null || watch.job.progress === undefined
              ? undefined
              : Math.round(watch.job.progress * 100)}
            valueLabel={watch.job.progress === null || watch.job.progress === undefined
              ? t('app.audit.progress_unknown')
              : t('app.audit.progress_share', {
                  percent: String(Math.round(watch.job.progress * 100)),
                })}
          />
        {:else if watch.job.status === 'SUCCEEDED' && exportedTo}
          <!-- Where it went, and no download: the archive lies at a target the browser holds no
               credential for and the policy names no origin for (ADR-0047). -->
          <Banner tone="success" title={t('app.audit.export_done')}>
            <Stack gap="050">
              <span>{t('app.audit.export_written_to', { target: exportedTo.target })}</span>
              <span>{t('app.audit.export_no_download')}</span>
            </Stack>
          </Banner>
        {:else if watch.unreachable}
          <p class="failure">{sentence(watch.unreachable)}</p>
        {:else if watch.job.status !== 'SUCCEEDED'}
          <p class="failure">{sentence(watch.job.error_code) ?? t('app.audit.export_failed')}</p>
        {/if}
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

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  /* The filters lay themselves out by how much room there is, so a narrow window is one column
     rather than a row that scrolls sideways. */
  .fields {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(20ch, 1fr));
    gap: var(--sp-150);
  }

  .entries { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-150); }

  .entry {
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }

  .mono { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
</style>
