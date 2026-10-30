<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Service desk* under *How we work*, where the service desk is on: the workspace's business
  // calendars (UC-SVC-01), its SLA policies (UC-SVC-02 check 1), its canned replies (UC-SVC-06
  // check 1) and how the goals of a period stood (UC-SVC-02 check 7), as tabs.
  //
  // **What the reader may never do is not drawn** (P-05): defining and deleting need STRUCTURE at
  // the workspace, and so does the report; a member without it reads the calendars, policies and
  // replies. Each definition replaces the block under its key whole, as the API does.

  import { untrack } from 'svelte';
  import {
    Badge, Button, Checkbox, IconButton, Inline, Input, PageHeader, Select, Skeleton, Stack, Switch, Tabs, Textarea,
  } from '@hubtask/design-system/components';
  import type { BusinessCalendarList, ReplyTemplateList, SlaPolicy, SlaPolicyList, SlaReport } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { calendarsPath, desk, policiesPath, repliesPath, slaReportPath } from '../lib/data/desk.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { keyFrom } from '../lib/data/entrytypes.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { WEEKDAYS, byPriorityOf, byPriorityText, formOf, holidaysOf, hoursOf, reportRows } from '../lib/servicedesk/desk.ts';

  $effect(() => page.entitle(t('app.desk.title')));
  const mayShape = $derived(holdsHere('STRUCTURE') === true);

  const TABS = ['calendars', 'policies', 'replies', 'report'] as const;
  let current = $state<(typeof TABS)[number]>('calendars');

  const calendars = untrack(() => resource<BusinessCalendarList>(calendarsPath));
  const policies = untrack(() => resource<SlaPolicyList>(policiesPath));
  const replies = untrack(() => resource<ReplyTemplateList>(repliesPath));
  const calendarRows = $derived(calendars.state.status === 'ready' ? calendars.state.data.data : undefined);
  const policyRows = $derived(policies.state.status === 'ready' ? policies.state.data.data : undefined);
  const replyRows = $derived(replies.state.status === 'ready' ? replies.state.data.data : undefined);

  let refusal = $state<string | undefined>(undefined);
  let isSaving = $state(false);

  async function attempt(work: () => Promise<unknown>, said: string): Promise<boolean> {
    refusal = undefined;
    isSaving = true;
    try {
      await work();
      toast.show({ text: t(said), tone: 'success' });
      return true;
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
      return false;
    } finally {
      isSaving = false;
    }
  }

  // A calendar: one span on the days chosen, holidays by line.
  let calendar = $state({ key: '', name: '', zone: Intl.DateTimeFormat().resolvedOptions().timeZone, days: ['MONDAY', 'TUESDAY', 'WEDNESDAY', 'THURSDAY', 'FRIDAY'] as string[], from: '08:00', to: '17:00', holidays: '' });

  function editCalendar(key: string) {
    const row = calendarRows?.find((each) => each.key === key);
    if (!row) return;
    const form = formOf(row);
    calendar = { key: row.key, name: row.name, zone: row.zone, days: form.days, from: form.from, to: form.to, holidays: row.holidays.join('\n') };
  }

  async function saveCalendar(event: SubmitEvent) {
    event.preventDefault();
    const key = calendar.key || keyFrom(calendar.name);
    const saved = await attempt(() => desk.defineCalendar(key, {
      name: calendar.name.trim(), zone: calendar.zone.trim(), hours: hoursOf(calendar.days, calendar.from, calendar.to), holidays: holidaysOf(calendar.holidays),
    }), 'app.desk.calendar_saved');
    if (saved) calendar = { ...calendar, key: '', name: '', holidays: '' };
  }

  // A policy: its types, calendar, priority field and the two goals.
  const emptyPolicy = () => ({ key: '', name: '', calendar: '', types: '', priority: '', first: '480', firstBy: '', resolution: '2400', resolutionBy: '', pauses: '', warn: '80', enabled: true, position: '0' });
  let policy = $state(emptyPolicy());
  const words = (text: string) => text.split(',').map((each) => each.trim()).filter((each) => each !== '');

  function editPolicy(row: SlaPolicy) {
    const first = row.goals.find((goal) => goal.kind === 'FIRST_RESPONSE');
    const resolution = row.goals.find((goal) => goal.kind === 'RESOLUTION');
    policy = {
      key: row.key, name: row.name, calendar: row.calendar ?? '', types: row.entry_types.join(', '), priority: row.priority_field ?? '',
      first: String(first?.minutes ?? ''), firstBy: byPriorityText(first), resolution: String(resolution?.minutes ?? ''),
      resolutionBy: byPriorityText(resolution), pauses: (resolution?.pause_statuses ?? first?.pause_statuses ?? []).join(', '),
      warn: String(resolution?.warn_percent ?? first?.warn_percent ?? 80), enabled: row.enabled, position: String(row.position),
    };
  }

  async function savePolicy(event: SubmitEvent) {
    event.preventDefault();
    const firstBy = byPriorityOf(policy.firstBy);
    const resolutionBy = byPriorityOf(policy.resolutionBy);
    if (!firstBy || !resolutionBy) {
      refusal = t('app.desk.by_priority_malformed');
      return;
    }
    const goal = (kind: 'FIRST_RESPONSE' | 'RESOLUTION', minutes: string, by: Record<string, number>) => ({
      kind, minutes: Number(minutes), by_priority: by, pause_statuses: words(policy.pauses), warn_percent: Number(policy.warn),
    });
    const goals = [
      ...(policy.first.trim() ? [goal('FIRST_RESPONSE', policy.first, firstBy)] : []),
      ...(policy.resolution.trim() ? [goal('RESOLUTION', policy.resolution, resolutionBy)] : []),
    ];
    const key = policy.key || keyFrom(policy.name);
    const saved = await attempt(() => desk.definePolicy(key, {
      name: policy.name.trim(), entry_types: words(policy.types), goals, position: Number(policy.position) || 0, enabled: policy.enabled,
      ...(policy.calendar ? { calendar: policy.calendar } : {}), ...(policy.priority.trim() ? { priority_field: policy.priority.trim() } : {}),
    }), 'app.desk.policy_saved');
    if (saved) policy = emptyPolicy();
  }

  // A canned reply.
  let reply = $state({ key: '', name: '', body: '' });
  async function saveReply(event: SubmitEvent) {
    event.preventDefault();
    const key = reply.key || keyFrom(reply.name);
    const saved = await attempt(() => desk.defineReply(key, { name: reply.name.trim(), body: reply.body }), 'app.desk.reply_saved');
    if (saved) reply = { key: '', name: '', body: '' };
  }

  // The report over the last thirty days by default.
  const day = 24 * 60 * 60 * 1000;
  let reportFrom = $state(new Date(Date.now() - 30 * day).toISOString().slice(0, 10));
  let reportTo = $state(new Date(Date.now() + day).toISOString().slice(0, 10));
  let report = $state<SlaReport | undefined>(undefined);
  async function readReport() {
    refusal = undefined;
    const state = await engine.refresh<SlaReport>({
      path: slaReportPath(new Date(reportFrom).toISOString(), new Date(reportTo).toISOString()),
    });
    if (state.status === 'ready') report = state.data;
    else if (state.status === 'failed') refusal = renderProblem(state.error as never, messages).message;
  }
  const rows = $derived(report ? reportRows(report) : []);
  const calendarOptions = $derived([{ value: '', label: t('app.desk.around_the_clock') }, ...(calendarRows ?? []).map((row) => ({ value: row.key, label: row.name }))]);
</script>

<Stack gap="300">
  <PageHeader title={t('app.desk.title')} subtitle={t('app.desk.subtitle')} isTitleInBar={viewport.isCompact} />
  <Tabs label={t('app.desk.title')} selected={current} onselect={(id) => (current = id as typeof current)}
    tabs={TABS.filter((tab) => tab !== 'report' || mayShape).map((tab) => ({ id: tab, label: t(`app.desk.tab.${tab}`) }))}>
    {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}

    {#if current === 'calendars'}
      <Stack gap="150">
        {#if !calendarRows}<Skeleton lines={3} />{:else if calendarRows.length === 0}<p class="quiet">{t('app.desk.calendars_none')}</p>{/if}
        <ul class="rows">
          {#each calendarRows ?? [] as row (row.key)}
            <li>
              <Inline gap="100" align="center" justify="between">
                <span><strong>{row.name}</strong> · {row.zone} · {t('app.desk.spans', { count: String(row.hours.length) })}</span>
                {#if mayShape}
                  <Inline gap="050">
                    <IconButton icon="pencil" size="sm" tone="subtle" label={t('app.desk.edit')} onclick={() => editCalendar(row.key)} />
                    <IconButton icon="trash" size="sm" tone="subtle" label={t('app.desk.delete')}
                      onclick={() => void attempt(() => desk.deleteCalendar(row.key), 'app.desk.deleted')} />
                  </Inline>
                {/if}
              </Inline>
            </li>
          {/each}
        </ul>
        {#if mayShape}
          <form class="panel" onsubmit={(event) => void saveCalendar(event)}>
            <Stack gap="100">
              <Input label={t('app.desk.name')} bind:value={calendar.name} required />
              <Input label={t('app.desk.zone')} hint={t('app.desk.zone_hint')} bind:value={calendar.zone} required />
              <div class="choices" role="group" aria-label={t('app.desk.days')}>
                {#each WEEKDAYS as weekday (weekday)}
                  <Checkbox label={t(`app.desk.day.${weekday.toLowerCase()}`)} checked={calendar.days.includes(weekday)}
                    onchange={(event: Event) => {
                      const on = (event.currentTarget as HTMLInputElement).checked;
                      calendar.days = on ? [...calendar.days, weekday] : calendar.days.filter((each) => each !== weekday);
                    }} />
                {/each}
              </div>
              <div class="fields">
                <Input label={t('app.desk.from')} type="time" bind:value={calendar.from} />
                <Input label={t('app.desk.to')} type="time" bind:value={calendar.to} />
              </div>
              <Textarea label={t('app.desk.holidays')} hint={t('app.desk.holidays_hint')} rows={3} bind:value={calendar.holidays} />
              <div><Button type="submit" tone="secondary" icon="calendar" isBusy={isSaving} busyLabel={t('app.desk.saving')}>
                {calendar.key ? t('app.desk.replace') : t('app.desk.add_calendar')}</Button></div>
            </Stack>
          </form>
        {/if}
      </Stack>
    {:else if current === 'policies'}
      <Stack gap="150">
        {#if !policyRows}<Skeleton lines={3} />{:else if policyRows.length === 0}<p class="quiet">{t('app.desk.policies_none')}</p>{/if}
        <ul class="rows">
          {#each policyRows ?? [] as row (row.key)}
            <li>
              <Inline gap="100" align="center" justify="between">
                <span>
                  <strong>{row.name}</strong> · {row.entry_types.join(', ')}
                  {#if !row.enabled}<Badge size="sm">{t('app.desk.off')}</Badge>{/if}
                </span>
                {#if mayShape}
                  <Inline gap="050">
                    <IconButton icon="pencil" size="sm" tone="subtle" label={t('app.desk.edit')} onclick={() => editPolicy(row)} />
                    <IconButton icon="trash" size="sm" tone="subtle" label={t('app.desk.delete')}
                      onclick={() => void attempt(() => desk.deletePolicy(row.key), 'app.desk.deleted')} />
                  </Inline>
                {/if}
              </Inline>
            </li>
          {/each}
        </ul>
        {#if mayShape}
          <form class="panel" onsubmit={(event) => void savePolicy(event)}>
            <Stack gap="100">
              <Input label={t('app.desk.name')} bind:value={policy.name} required />
              <Input label={t('app.desk.types')} hint={t('app.desk.types_hint')} bind:value={policy.types} required />
              <div class="fields">
                <Select label={t('app.desk.calendar')} options={calendarOptions} bind:value={policy.calendar} />
                <Input label={t('app.desk.priority_field')} hint={t('app.desk.priority_field_hint')} bind:value={policy.priority} />
              </div>
              <div class="fields">
                <Input label={t('app.desk.first_response')} hint={t('app.desk.minutes_hint')} type="number" min="1" bind:value={policy.first} />
                <Input label={t('app.desk.by_priority')} hint={t('app.desk.by_priority_hint')} bind:value={policy.firstBy} />
              </div>
              <div class="fields">
                <Input label={t('app.desk.resolution')} hint={t('app.desk.minutes_hint')} type="number" min="1" bind:value={policy.resolution} />
                <Input label={t('app.desk.by_priority')} hint={t('app.desk.by_priority_hint')} bind:value={policy.resolutionBy} />
              </div>
              <div class="fields">
                <Input label={t('app.desk.pauses')} hint={t('app.desk.pauses_hint')} bind:value={policy.pauses} />
                <Input label={t('app.desk.warn')} type="number" min="1" max="99" bind:value={policy.warn} />
                <Input label={t('app.desk.position')} type="number" min="0" bind:value={policy.position} />
              </div>
              <Switch label={t('app.desk.enabled')} checked={policy.enabled}
                onchange={(event: Event) => (policy.enabled = (event.currentTarget as HTMLInputElement).checked)} />
              <div><Button type="submit" tone="secondary" icon="clock" isBusy={isSaving} busyLabel={t('app.desk.saving')}>
                {policy.key ? t('app.desk.replace') : t('app.desk.add_policy')}</Button></div>
            </Stack>
          </form>
        {/if}
      </Stack>
    {:else if current === 'replies'}
      <Stack gap="150">
        {#if !replyRows}<Skeleton lines={3} />{:else if replyRows.length === 0}<p class="quiet">{t('app.desk.replies_none')}</p>{/if}
        <ul class="rows">
          {#each replyRows ?? [] as row (row.key)}
            <li>
              <Inline gap="100" align="center" justify="between">
                <strong>{row.name}</strong>
                {#if mayShape}
                  <Inline gap="050">
                    <IconButton icon="pencil" size="sm" tone="subtle" label={t('app.desk.edit')} onclick={() => (reply = { key: row.key, name: row.name, body: row.body })} />
                    <IconButton icon="trash" size="sm" tone="subtle" label={t('app.desk.delete')}
                      onclick={() => void attempt(() => desk.deleteReply(row.key), 'app.desk.deleted')} />
                  </Inline>
                {/if}
              </Inline>
              <p class="body">{row.body}</p>
            </li>
          {/each}
        </ul>
        {#if mayShape}
          <form class="panel" onsubmit={(event) => void saveReply(event)}>
            <Stack gap="100">
              <Input label={t('app.desk.name')} bind:value={reply.name} required />
              <Textarea label={t('app.desk.reply_body')} hint={t('app.desk.reply_body_hint')} rows={5} bind:value={reply.body} />
              <div><Button type="submit" tone="secondary" icon="message-square" isBusy={isSaving} busyLabel={t('app.desk.saving')}>
                {reply.key ? t('app.desk.replace') : t('app.desk.add_reply')}</Button></div>
            </Stack>
          </form>
        {/if}
      </Stack>
    {:else}
      <Stack gap="150">
        <div class="fields">
          <Input label={t('app.desk.from')} type="date" bind:value={reportFrom} />
          <Input label={t('app.desk.to')} type="date" bind:value={reportTo} />
          <div><Button tone="secondary" icon="gauge" onclick={() => void readReport()}>{t('app.desk.count')}</Button></div>
        </div>
        {#if report && rows.length === 0}<p class="quiet">{t('app.desk.report_none')}</p>{/if}
        <ul class="rows" aria-label={t('app.desk.tab.report')}>
          {#each rows as row (row.policy + row.goal)}
            <li data-report-row>
              <strong>{policyRows?.find((each) => each.key === row.policy)?.name ?? row.policy}</strong> · {t(`app.portal.goal.${row.goal}`)}
              <span class="quiet">{t('app.desk.report_row', {
                met: String(row.met), breached: String(row.breached), running: String(row.running), paused: String(row.paused),
              })}{row.metShare !== undefined ? ` · ${t('app.desk.met_share', { share: String(row.metShare) })}` : ''}</span>
            </li>
          {/each}
        </ul>
      </Stack>
    {/if}
  </Tabs>
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .rows li { padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); }
  .body { margin: var(--sp-050) 0 0; white-space: pre-wrap; overflow-wrap: anywhere; color: var(--text-secondary); }
  form { margin: 0; max-inline-size: 64ch; }
  .panel { padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 20ch), 1fr)); gap: var(--sp-100); align-items: end; }
  .choices { display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-200); }
</style>
