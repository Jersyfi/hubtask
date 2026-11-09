<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The service desk's settings (page-system §9.6): a settings page of the workspace's business
  // calendars (UC-SVC-01), its SLA policies (UC-SVC-02 check 1), its canned replies (UC-SVC-06
  // check 1) and the portal's organisations (UC-SVC-11 check 5), each at `/desk/settings/<id>`.
  // The desk's last tab (the navigation concept §4): the home's head and `ViewBar` stay, the
  // section list beside the section, and the home's *New request* is not offered on it. How the
  // goals of a period stood is the home's *Reports* (DeskReportsView).
  //
  // **What the reader may never do is not drawn** (P-05): defining and deleting need STRUCTURE at
  // the workspace; a member without it reads the calendars, policies and
  // replies. Each definition replaces the block under its key whole, as the API does.

  import { untrack } from 'svelte';
  import {
    EmptyState, Badge, Button, Checkbox, CheckboxGroup, DataTable, Dialog, Disclosure, Inline, Input, Select, SettingsLayout, Skeleton, Stack, Switch, Textarea,
    type DataColumn, type DataRowMenu, type SectionLink,
  } from '@hubtask/design-system/components';
  import DurationInput from '../lib/forms/DurationInput.svelte';
  import ZonePicker from '../lib/forms/ZonePicker.svelte';
  import type {
    BusinessCalendar, BusinessCalendarList, ReplyTemplate, ReplyTemplateList, RequesterOrganisation, RequesterOrganisationList, SlaGoal, SlaPolicy, SlaPolicyDefine,
    SlaPolicyList,
  } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { calendarsPath, desk, organisationsPath, policiesPath, repliesPath } from '../lib/data/desk.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { customFields } from '../lib/data/customfields.svelte.ts';
  import { entryTypes } from '../lib/data/entrytypes.svelte.ts';
  import { statusLabel, workflows } from '../lib/data/workflows.svelte.ts';
  import { keyFrom } from '../lib/data/entrytypes.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { deviceZone } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';
  import DraftPublish from '../lib/settings/DraftPublish.svelte';
  import TableSection from '../lib/page/TableSection.svelte';
  import { WEEKDAYS, byPriorityOf, dayLengthOf, byPriorityText, formOf, holidaysOf, hoursOf, minutesFor, policyDefinitionOf, textOf, withMinutes, wordsOf } from '../lib/servicedesk/desk.ts';
  import ProductTabs from '../lib/product/ProductTabs.svelte';
  import { productHead } from '../lib/product/frame.svelte.ts';
  import { domainsOf } from '../lib/servicedesk/organisations.ts';
  import { ITIL_TEMPLATE } from '../lib/servicedesk/desk.ts';

  interface Props {
    /** The section the address names (`/desk/settings/<id>`); none is the list of sections. */
    section?: string;
    onnavigate?: (path: string) => void;
    /** Replaces the address: a move between sections. */
    onreplace?: (path: string) => void;
  }
  const { section, onnavigate, onreplace }: Props = $props();
  const BASE = '/desk/settings';
  const title = $derived(t('app.desk.settings_title'));
  $effect(() => page.entitle(title));
  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  // An empty calendars, policies or replies section starts from the ITIL desk template (§7.4): its
  // workspace part's plan, in words, at the templates - the calendar and the P1-P4 policies with
  // the types, priorities and statuses they measure, which a policy alone would name in vain.
  const offersItil = $derived(mayShape && onnavigate !== undefined);
  const startFromItil = () => onnavigate?.(`/administration/templates?template=${ITIL_TEMPLATE}`);

  const offered = ['calendars', 'policies', 'replies', 'organisations'] as const;
  const named = $derived(offered.find((tab) => tab === section));
  const current = $derived(named ?? 'calendars');
  const view = $derived<'list' | 'detail'>(named ? 'detail' : 'list');

  const calendars = untrack(() => resource<BusinessCalendarList>(calendarsPath));
  const policies = untrack(() => resource<SlaPolicyList>(policiesPath));
  const replies = untrack(() => resource<ReplyTemplateList>(repliesPath));
  const calendarRows = $derived(calendars.state.status === 'ready' ? calendars.state.data.data : undefined);
  const policyRows = $derived(policies.state.status === 'ready' ? policies.state.data.data : undefined);
  const replyRows = $derived(replies.state.status === 'ready' ? replies.state.data.data : undefined);
  const organisations = untrack(() => resource<RequesterOrganisationList>(organisationsPath));
  $effect(() => untrack(() => groups.open()));
  const organisationRows = $derived(organisations.state.status === 'ready' ? organisations.state.data.data : undefined);

  let refusal = $state<string | undefined>(undefined);
  let isSaving = $state(false);
  /**
   * The form open in its dialog, from *Add …* or a row's pencil (user-experience.md §7.4): a list
   * stays a list, and a form is never open before somebody asked for it.
   */
  let editing = $state<'calendar' | 'policy' | 'reply' | 'organisation' | undefined>(undefined);
  const close = () => {
    editing = undefined;
    refusal = undefined;
  };

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
  let calendar = $state({ key: '', name: '', zone: deviceZone(), days: ['MONDAY', 'TUESDAY', 'WEDNESDAY', 'THURSDAY', 'FRIDAY'] as string[], from: '08:00', to: '17:00', holidays: '' });

  function editCalendar(key: string) {
    const row = calendarRows?.find((each) => each.key === key);
    if (!row) return;
    const form = formOf(row);
    calendar = { key: row.key, name: row.name, zone: row.zone, days: form.days, from: form.from, to: form.to, holidays: row.holidays.join('\n') };
    editing = 'calendar';
  }
  function addCalendar() {
    calendar = { key: '', name: '', zone: deviceZone(), days: ['MONDAY', 'TUESDAY', 'WEDNESDAY', 'THURSDAY', 'FRIDAY'], from: '08:00', to: '17:00', holidays: '' };
    editing = 'calendar';
  }

  async function saveCalendar(event: SubmitEvent) {
    event.preventDefault();
    const key = calendar.key || keyFrom(calendar.name);
    const saved = await attempt(() => desk.defineCalendar(key, {
      name: calendar.name.trim(), zone: calendar.zone.trim(), hours: hoursOf(calendar.days, calendar.from, calendar.to), holidays: holidaysOf(calendar.holidays),
    }), 'app.desk.calendar_saved');
    if (saved) {
      calendar = { ...calendar, key: '', name: '', holidays: '' };
      close();
    }
  }

  // A policy: its types, calendar, priority field and the two goals.
  const emptyPolicy = () => ({ key: '', name: '', group: '', calendar: '', types: '', organisations: '', priority: '', first: '480', firstBy: '', next: '', nextBy: '', resolution: '2400', resolutionBy: '', pauses: '', warn: '80', enabled: true, position: '0' });
  let policy = $state(emptyPolicy());
  const words = (text: string) => text.split(',').map((each) => each.trim()).filter((each) => each !== '');

  function editPolicy(row: SlaPolicy) {
    const first = row.goals.find((goal) => goal.kind === 'FIRST_RESPONSE');
    const next = row.goals.find((goal) => goal.kind === 'NEXT_RESPONSE');
    const resolution = row.goals.find((goal) => goal.kind === 'RESOLUTION');
    policy = {
      key: row.key, name: row.name, group: row.assignee_group ?? '', calendar: row.calendar ?? '', types: row.entry_types.join(', '), organisations: row.organisations.join(', '), priority: row.priority_field ?? '',
      first: String(first?.minutes ?? ''), firstBy: byPriorityText(first), next: String(next?.minutes ?? ''), nextBy: byPriorityText(next), resolution: String(resolution?.minutes ?? ''),
      resolutionBy: byPriorityText(resolution), pauses: (resolution?.pause_statuses ?? first?.pause_statuses ?? []).join(', '),
      warn: String(resolution?.warn_percent ?? first?.warn_percent ?? 80), enabled: row.enabled, position: String(row.position),
    };
    editing = 'policy';
  }
  function addPolicy() {
    policy = emptyPolicy();
    editing = 'policy';
  }

  /** The goals as the form holds them; undefined where a priority's targets do not read. */
  function policyGoals(): SlaPolicyDefine['goals'] | undefined {
    const firstBy = byPriorityOf(policy.firstBy);
    const nextBy = byPriorityOf(policy.nextBy);
    const resolutionBy = byPriorityOf(policy.resolutionBy);
    if (!firstBy || !nextBy || !resolutionBy) return undefined;
    const goal = (kind: SlaGoal['kind'], minutes: string, by: Record<string, number>) => ({
      kind, minutes: Number(minutes), by_priority: by, pause_statuses: words(policy.pauses), warn_percent: Number(policy.warn),
    });
    return [
      ...(policy.first.trim() ? [goal('FIRST_RESPONSE', policy.first, firstBy)] : []),
      // An OLA promises a first response and a resolution only (UC-SVC-02 check 10).
      ...(policy.next.trim() && !policy.group ? [goal('NEXT_RESPONSE', policy.next, nextBy)] : []),
      ...(policy.resolution.trim() ? [goal('RESOLUTION', policy.resolution, resolutionBy)] : []),
    ];
  }
  const policyBlocked = $derived(policyGoals() ? undefined : t('app.desk.by_priority_malformed'));

  /** A new policy is defined at once: nothing measures by it yet. One in use changes through its draft (ui.md A4). */
  async function savePolicy(event: SubmitEvent) {
    event.preventDefault();
    // Enter in a field of a policy in use submits the form: it goes through the draft's buttons only.
    if (policy.key) return;
    const goals = policyGoals();
    if (!goals) {
      refusal = t('app.desk.by_priority_malformed');
      return;
    }
    const key = policy.key || keyFrom(policy.name);
    const saved = await attempt(() => desk.definePolicy(key, policyDefinitionOf(policy, goals)), 'app.desk.policy_saved');
    if (saved) {
      policy = emptyPolicy();
      close();
    }
  }

  /** A draft's block read back into the form, as a policy row is. */
  function editDraftedPolicy(key: string, block: Record<string, unknown>) {
    editPolicy({ ...(block as Partial<SlaPolicy>), key, name: String(block.name ?? ''), entry_types: (block.entry_types as string[] | undefined) ?? [],
      organisations: (block.organisations as string[] | undefined) ?? [], goals: (block.goals as SlaPolicy['goals'] | undefined) ?? [],
      position: Number(block.position ?? 0), enabled: block.enabled !== false } as SlaPolicy);
  }

  // A canned reply.
  let reply = $state({ key: '', name: '', body: '' });
  async function saveReply(event: SubmitEvent) {
    event.preventDefault();
    const key = reply.key || keyFrom(reply.name);
    const saved = await attempt(() => desk.defineReply(key, { name: reply.name.trim(), body: reply.body }), 'app.desk.reply_saved');
    if (saved) {
      reply = { key: '', name: '', body: '' };
      close();
    }
  }

  // A portal organisation: its people by the mail domain of their address; sharing off unless set.
  const emptyOrganisation = () => ({ key: '', name: '', domains: '', shares: false });
  let organisation = $state(emptyOrganisation());
  async function saveOrganisation(event: SubmitEvent) {
    event.preventDefault();
    const key = organisation.key || keyFrom(organisation.name);
    const saved = await attempt(() => desk.defineOrganisation(key, {
      name: organisation.name.trim(), domains: domainsOf(organisation.domains), shares_requests: organisation.shares,
    }), 'app.desk.organisation_saved');
    if (saved) {
      organisation = emptyOrganisation();
      close();
    }
  }

  // Whose promise a policy is: the requester's, or a team's OLA (UC-SVC-02 check 10). A group
  // already named but gone stays offered by its identifier, so a save drops nothing unseen.
  const groupOptions = $derived([
    { value: '', label: t('app.desk.policy_requester') },
    ...groups.all.map((group) => ({ value: group.id, label: group.name })),
    ...(policy.group && !groups.all.some((group) => group.id === policy.group) ? [{ value: policy.group, label: policy.group }] : []),
  ]);
  // A business day is the chosen calendar's; around the clock has none, so hours suffice there.
  const policyDay = $derived.by(() => {
    const chosen = calendarRows?.find((row) => row.key === policy.calendar);
    return chosen ? dayLengthOf(chosen) : undefined;
  });
  // Pickers fed by the workspace's own values (§5.4): a key already named but gone stays offered
  // by itself, so a save drops nothing unseen.
  $effect(() => untrack(() => entryTypes.open()));
  $effect(() => untrack(() => workflows.open()));
  $effect(() => untrack(() => customFields.open()));
  const kept = (offered: { value: string; label: string }[], chosen: readonly string[]) =>
    [...offered, ...chosen.filter((key) => !offered.some((each) => each.value === key)).map((key) => ({ value: key, label: key }))];
  const typeOptions = $derived(kept(
    entryTypes.all.filter((type) => type.level === 'TASK' && !type.archived).map((type) => ({ value: type.key, label: type.name || type.key })),
    wordsOf(policy.types),
  ));
  const organisationOptions = $derived(kept((organisationRows ?? []).map((row) => ({ value: row.key, label: row.name })), wordsOf(policy.organisations)));
  const statusOptions = $derived.by(() => {
    const seen = new Map<string, string>();
    for (const workflow of workflows.list?.data ?? []) {
      for (const status of workflow.published?.statuses ?? []) if (!seen.has(status.key)) seen.set(status.key, statusLabel(status, t));
    }
    return kept([...seen].map(([value, label]) => ({ value, label })), wordsOf(policy.pauses));
  });
  const choiceFields = $derived(customFields.of().filter((field) => field.kind === 'SELECT'));
  const priorityOptions = $derived([
    { value: '', label: t('app.desk.priority_none') },
    ...choiceFields.map((field) => ({ value: field.key, label: field.name || field.key })),
    ...(policy.priority && !choiceFields.some((field) => field.key === policy.priority) ? [{ value: policy.priority, label: policy.priority }] : []),
  ]);
  const priorityValues = $derived(choiceFields.find((field) => field.key === policy.priority)?.options ?? []);
  const calendarOptions = $derived([{ value: '', label: t('app.desk.around_the_clock') }, ...(calendarRows ?? []).map((row) => ({ value: row.key, label: row.name }))]);

  const countOf = (tab: string): number | undefined =>
    ({ calendars: calendarRows, policies: policyRows, replies: replyRows, organisations: organisationRows } as Record<string, readonly unknown[] | undefined>)[tab]?.length;
  const links = $derived<SectionLink[]>(offered.map((tab) => {
    const count = countOf(tab);
    return { id: tab, label: t(`app.desk.tab.${tab}`), href: `${BASE}/${tab}`, ...(count === undefined ? {} : { status: String(count) }) };
  }));

  // A row's acts are its ⋯ (PS-6): *Edit* opens the row's dialog, *Delete* removes it.
  const rowMenu = (name: string): DataRowMenu | undefined => (mayShape ? { label: t('app.page.more_for', { name }), items: [
    { id: 'edit', label: t('app.desk.edit') },
    { id: 'delete', label: t('app.desk.delete'), isDestructive: true, hasSeparatorBefore: true },
  ] } : undefined);
  const tableProps = $derived({ actionsLabel: t('app.page.row_actions'), sheetDismissLabel: t('app.page.sheet_close'), sheetGrabLabel: t('app.page.sheet_grab') });
  const calendarColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.desk.name'), priority: 1 },
    { id: 'zone', label: t('app.desk.zone'), priority: 2 },
    { id: 'hours', label: t('app.desk.col_hours'), priority: 3 },
  ]);
  const policyColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.desk.name'), priority: 1 },
    { id: 'types', label: t('app.desk.types'), priority: 2 },
    { id: 'for', label: t('app.desk.col_for'), priority: 3 },
    { id: 'state', label: t('app.desk.col_state'), isStatus: true },
  ]);
  const replyColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.desk.name'), priority: 1 },
    { id: 'body', label: t('app.desk.reply_body'), priority: 2 },
  ]);
  const organisationColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.desk.name'), priority: 1 },
    { id: 'domains', label: t('app.desk.organisation_domains'), priority: 2 },
    { id: 'shares', label: t('app.desk.col_shares'), isStatus: true },
  ]);
  const editReply = (row: ReplyTemplate) => { reply = { key: row.key, name: row.name, body: row.body }; editing = 'reply'; };
  const editOrganisation = (row: RequesterOrganisation) => {
    organisation = { key: row.key, name: row.name, domains: row.domains.join(', '), shares: row.shares_requests };
    editing = 'organisation';
  };
</script>

{#snippet itilAct()}
  <Button tone="secondary" icon="layout-template" data-itil-defaults onclick={startFromItil}>{t('app.desk.itil_defaults')}</Button>
{/snippet}

{#snippet tabs()}<ProductTabs module="SERVICE_DESK" current="settings" {onnavigate} />{/snippet}

<SettingsLayout
  head={productHead('SERVICE_DESK')}
  {tabs}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current, onselect: (next) => onreplace?.(`${BASE}/${next}`) }}
  {view}
  back={{ label: t('app.product_home.tab_settings'), onback: () => onreplace?.(BASE) }}
>
  <!-- Every change here is a dialog's own save or a row's act, so nothing waits for a save bar. -->
  <TableSection title={t(`app.desk.tab.${current}`)} description={current === 'organisations' ? t('app.desk.organisations_intro') : undefined}>
    {#snippet acts()}
      {#if mayShape}
        {#if current === 'calendars'}<Button data-list-add icon="plus" onclick={addCalendar}>{t('app.desk.add_calendar')}</Button>
        {:else if current === 'policies'}<Button data-list-add icon="plus" onclick={addPolicy}>{t('app.desk.add_policy')}</Button>
        {:else if current === 'replies'}<Button data-list-add icon="plus" onclick={() => { reply = { key: '', name: '', body: '' }; editing = 'reply'; }}>{t('app.desk.add_reply')}</Button>
        {:else if current === 'organisations'}<Button data-list-add icon="plus" onclick={() => { organisation = emptyOrganisation(); editing = 'organisation'; }}>{t('app.desk.add_organisation')}</Button>{/if}
      {/if}
    {/snippet}
    <div class="part" data-desk-part={current}>
    {#if refusal && editing === undefined}<p class="refusal" role="alert">{refusal}</p>{/if}

    {#if current === 'calendars'}
      <Stack gap="150">
        {#if !calendarRows}<Skeleton lines={3} />{:else if calendarRows.length === 0}<EmptyState kind="unused" title={t('app.desk.calendars_none')}
          description={offersItil ? t('app.desk.itil_defaults_hint') : undefined} action={offersItil ? itilAct : undefined} />{:else}
          <DataTable label={t('app.desk.tab.calendars')} columns={calendarColumns} rows={calendarRows} key={(row: BusinessCalendar) => row.key} rowLabel={(row: BusinessCalendar) => row.name}
            onopen={mayShape ? (row: BusinessCalendar) => editCalendar(row.key) : undefined} menu={(row: BusinessCalendar) => rowMenu(row.name)}
            onmenu={(row: BusinessCalendar, id: string) => (id === 'edit' ? editCalendar(row.key) : void attempt(() => desk.deleteCalendar(row.key), 'app.desk.deleted'))} {...tableProps}>
            {#snippet cell(row: BusinessCalendar, column: DataColumn)}
              {#if column.id === 'name'}{row.name}{:else if column.id === 'zone'}{row.zone}{:else}{t('app.desk.spans', { count: String(row.hours.length) })}{/if}
            {/snippet}
          </DataTable>
        {/if}
        {#if mayShape}
          <Dialog title={calendar.key ? calendar.name : t('app.desk.add_calendar')} isOpen={editing === 'calendar'} onClose={close} dismissLabel={t('app.desk.cancel')} size="m">
            <form onsubmit={(event) => void saveCalendar(event)}>
            <Stack gap="100">
              <Input label={t('app.desk.name')} bind:value={calendar.name} required />
              <ZonePicker label={t('app.desk.zone')} bind:value={calendar.zone} isRequired />
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
              {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
              <Inline gap="100" justify="end">
                <Button tone="secondary" onclick={close}>{t('app.desk.cancel')}</Button>
                <Button type="submit" tone="primary" isBusy={isSaving} busyLabel={t('app.desk.saving')}>{calendar.key ? t('app.desk.replace') : t('app.desk.add_calendar')}</Button>
              </Inline>
            </Stack>
          </form>
          </Dialog>
        {/if}
      </Stack>
    {:else if current === 'policies'}
      <Stack gap="150">
        {#if !policyRows}<Skeleton lines={3} />{:else if policyRows.length === 0}<EmptyState kind="unused" title={t('app.desk.policies_none')}
          description={offersItil ? t('app.desk.itil_defaults_hint') : undefined} action={offersItil ? itilAct : undefined} />{:else}
          <DataTable label={t('app.desk.tab.policies')} columns={policyColumns} rows={policyRows} key={(row: SlaPolicy) => row.key} rowLabel={(row: SlaPolicy) => row.name}
            onopen={mayShape ? (row: SlaPolicy) => editPolicy(row) : undefined} menu={(row: SlaPolicy) => rowMenu(row.name)}
            onmenu={(row: SlaPolicy, id: string) => (id === 'edit' ? editPolicy(row) : void attempt(() => desk.deletePolicy(row.key), 'app.desk.deleted'))} {...tableProps}>
            {#snippet cell(row: SlaPolicy, column: DataColumn)}
              {#if column.id === 'name'}{row.name}
              {:else if column.id === 'types'}{row.entry_types.join(', ')}
              {:else if column.id === 'for'}<span class="quiet">{[
                ...(row.organisations.length > 0 ? [t('app.desk.policy_for', { organisations: row.organisations.join(', ') })] : []),
                ...(row.assignee_group ? [t('app.desk.policy_ola', { group: groups.nameOf(row.assignee_group) ?? row.assignee_group })] : []),
              ].join(' · ')}</span>
              {:else if !row.enabled}<Badge size="sm">{t('app.desk.off')}</Badge>{/if}
            {/snippet}
          </DataTable>
        {/if}
        {#if mayShape}
          <Dialog title={policy.key ? policy.name : t('app.desk.add_policy')} isOpen={editing === 'policy'} onClose={close} dismissLabel={t('app.desk.cancel')} size="l">
            <form onsubmit={(event) => void savePolicy(event)}>
            <Stack gap="100">
              <Input label={t('app.desk.name')} bind:value={policy.name} required />
              <CheckboxGroup label={t('app.desk.types')} hint={t('app.desk.types_pick_hint')} options={typeOptions}
                selected={wordsOf(policy.types)} onChange={(chosen) => (policy.types = textOf(chosen))} />
              <Select label={t('app.desk.policy_group')} hint={t('app.desk.policy_group_hint')} options={groupOptions} bind:value={policy.group} />
              {#if !policy.group}
                {#if organisationOptions.length > 0}
                  <CheckboxGroup label={t('app.desk.policy_organisations')} hint={t('app.desk.policy_organisations_pick_hint')} options={organisationOptions}
                    selected={wordsOf(policy.organisations)} onChange={(chosen) => (policy.organisations = textOf(chosen))} />
                {/if}
              {/if}
              <div class="fields">
                <Select label={t('app.desk.calendar')} options={calendarOptions} bind:value={policy.calendar} />
                <Select label={t('app.desk.priority_field')} hint={t('app.desk.priority_field_pick_hint')} options={priorityOptions} bind:value={policy.priority} />
              </div>
              <div class="goal">
                <DurationInput label={t('app.desk.first_response')} hint={t('app.desk.minutes_hint')} day={policyDay} bind:minutes={policy.first} />
                {#if priorityValues.length > 0}<div class="by-priority">{#each priorityValues as option (option)}
                  <DurationInput label={t('app.desk.goal_for', { goal: t('app.desk.first_response'), option })} day={policyDay}
                    bind:minutes={() => minutesFor(policy.firstBy, option), (minutes: string) => (policy.firstBy = withMinutes(policy.firstBy, option, minutes))} />
                {/each}</div>{/if}

              </div>
              {#if !policy.group}
                <div class="goal">
                  <DurationInput label={t('app.desk.next_response')} hint={t('app.desk.minutes_hint')} day={policyDay} bind:minutes={policy.next} />
                {#if priorityValues.length > 0}<div class="by-priority">{#each priorityValues as option (option)}
                  <DurationInput label={t('app.desk.goal_for', { goal: t('app.desk.next_response'), option })} day={policyDay}
                    bind:minutes={() => minutesFor(policy.nextBy, option), (minutes: string) => (policy.nextBy = withMinutes(policy.nextBy, option, minutes))} />
                {/each}</div>{/if}
  
                </div>
              {/if}
              <div class="goal">
                <DurationInput label={t('app.desk.resolution')} hint={t('app.desk.minutes_hint')} day={policyDay} bind:minutes={policy.resolution} />
                {#if priorityValues.length > 0}<div class="by-priority">{#each priorityValues as option (option)}
                  <DurationInput label={t('app.desk.goal_for', { goal: t('app.desk.resolution'), option })} day={policyDay}
                    bind:minutes={() => minutesFor(policy.resolutionBy, option), (minutes: string) => (policy.resolutionBy = withMinutes(policy.resolutionBy, option, minutes))} />
                {/each}</div>{/if}

              </div>
              <CheckboxGroup label={t('app.desk.pauses')} hint={t('app.desk.pauses_pick_hint')} options={statusOptions}
                selected={wordsOf(policy.pauses)} onChange={(chosen) => (policy.pauses = textOf(chosen))} />
              <Disclosure label={t('app.desk.advanced')}>
              <div class="fields">
                <Input label={t('app.desk.warn')} type="number" min="1" max="99" bind:value={policy.warn} />
                <Input label={t('app.desk.position')} type="number" min="0" bind:value={policy.position} />
              </div>
              </Disclosure>
              <Switch label={t('app.desk.enabled')} checked={policy.enabled}
                onchange={(event: Event) => (policy.enabled = (event.currentTarget as HTMLInputElement).checked)} />
              {#if policy.key}
                {#key policy.key}
                  <DraftPublish section="sla_policies" key={policy.key} blockedReason={policyBlocked}
                    block={() => policyDefinitionOf(policy, policyGoals() ?? []) as unknown as Record<string, unknown>}
                    onLoad={(saved) => editDraftedPolicy(policy.key, saved)} onPublished={() => { policy = emptyPolicy(); close(); }} />
                {/key}
              {:else}
                {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
                <Inline gap="100" justify="end">
                  <Button tone="secondary" onclick={close}>{t('app.desk.cancel')}</Button>
                  <Button type="submit" tone="primary" isBusy={isSaving} busyLabel={t('app.desk.saving')}
                    disabledReason={wordsOf(policy.types).length === 0 ? t('app.desk.types_first') : policyBlocked}>{t('app.desk.add_policy')}</Button>
                </Inline>
              {/if}
            </Stack>
          </form>
          </Dialog>
        {/if}
      </Stack>
    {:else if current === 'replies'}
      <Stack gap="150">
        {#if !replyRows}<Skeleton lines={3} />{:else if replyRows.length === 0}<EmptyState kind="unused" title={t('app.desk.replies_none')}
          description={offersItil ? t('app.desk.itil_defaults_hint') : undefined} action={offersItil ? itilAct : undefined} />{:else}
          <DataTable label={t('app.desk.tab.replies')} columns={replyColumns} rows={replyRows} key={(row: ReplyTemplate) => row.key} rowLabel={(row: ReplyTemplate) => row.name}
            onopen={mayShape ? editReply : undefined} menu={(row: ReplyTemplate) => rowMenu(row.name)}
            onmenu={(row: ReplyTemplate, id: string) => (id === 'edit' ? editReply(row) : void attempt(() => desk.deleteReply(row.key), 'app.desk.deleted'))} {...tableProps}>
            {#snippet cell(row: ReplyTemplate, column: DataColumn)}
              {#if column.id === 'name'}{row.name}{:else}<span class="body">{row.body}</span>{/if}
            {/snippet}
          </DataTable>
        {/if}
        {#if mayShape}
          <Dialog title={reply.key ? reply.name : t('app.desk.add_reply')} isOpen={editing === 'reply'} onClose={close} dismissLabel={t('app.desk.cancel')} size="m">
            <form onsubmit={(event) => void saveReply(event)}>
            <Stack gap="100">
              <Input label={t('app.desk.name')} bind:value={reply.name} required />
              <Textarea label={t('app.desk.reply_body')} hint={t('app.desk.reply_body_hint')} rows={5} bind:value={reply.body} />
              {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
              <Inline gap="100" justify="end">
                <Button tone="secondary" onclick={close}>{t('app.desk.cancel')}</Button>
                <Button type="submit" tone="primary" isBusy={isSaving} busyLabel={t('app.desk.saving')}>{reply.key ? t('app.desk.replace') : t('app.desk.add_reply')}</Button>
              </Inline>
            </Stack>
          </form>
          </Dialog>
        {/if}
      </Stack>
    {:else if current === 'organisations'}
      <Stack gap="150">
        {#if !organisationRows}<Skeleton lines={3} />{:else if organisationRows.length === 0}<EmptyState kind="unused" title={t('app.desk.organisations_none')} />{:else}
          <DataTable label={t('app.desk.tab.organisations')} columns={organisationColumns} rows={organisationRows} key={(row: RequesterOrganisation) => row.key} rowLabel={(row: RequesterOrganisation) => row.name}
            onopen={mayShape ? editOrganisation : undefined} menu={(row: RequesterOrganisation) => rowMenu(row.name)}
            onmenu={(row: RequesterOrganisation, id: string) => (id === 'edit' ? editOrganisation(row) : void attempt(() => desk.deleteOrganisation(row.key), 'app.desk.deleted'))} {...tableProps}>
            {#snippet cell(row: RequesterOrganisation, column: DataColumn)}
              {#if column.id === 'name'}<span data-organisation-row>{row.name}</span>{:else if column.id === 'domains'}{row.domains.join(', ')}
              {:else if row.shares_requests}<Badge tone="info" size="sm">{t('app.desk.organisation_shares')}</Badge>{/if}
            {/snippet}
          </DataTable>{/if}
        {#if mayShape}
          <Dialog title={organisation.key ? organisation.name : t('app.desk.add_organisation')} isOpen={editing === 'organisation'} onClose={close} dismissLabel={t('app.desk.cancel')} size="m">
            <form onsubmit={(event) => void saveOrganisation(event)}>
            <Stack gap="100">
              <Input label={t('app.desk.name')} bind:value={organisation.name} required />
              <Textarea label={t('app.desk.organisation_domains')} hint={t('app.desk.organisation_domains_hint')} rows={2}
                bind:value={organisation.domains} />
              <Switch label={t('app.desk.organisation_share')} checked={organisation.shares}
                onchange={(event: Event) => (organisation.shares = (event.currentTarget as HTMLInputElement).checked)} />
              {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
              <Inline gap="100" justify="end">
                <Button tone="secondary" onclick={close}>{t('app.desk.cancel')}</Button>
                <Button type="submit" tone="primary" isBusy={isSaving} busyLabel={t('app.desk.saving')}
                  disabledReason={domainsOf(organisation.domains).length === 0 ? t('app.desk.organisation_domains_first') : undefined}>
                  {organisation.key ? t('app.desk.replace') : t('app.desk.add_organisation')}</Button>
              </Inline>
            </Stack>
          </form>
          </Dialog>
        {/if}
      </Stack>
    {/if}
    </div>
  </TableSection>
</SettingsLayout>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .part { display: grid; gap: var(--sp-150); }
  .refusal { margin: 0; color: var(--text-danger); }
  .body { display: block; overflow: hidden; color: var(--text-secondary); text-overflow: ellipsis; white-space: nowrap; }
  form { margin: 0; max-inline-size: 64ch; }
  .fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 20ch), 1fr)); gap: var(--sp-100); align-items: end; }
  /* A goal's own target, then one per priority beside each other (user-experience.md §7.4). */
  .goal { display: grid; gap: var(--sp-100); padding-block-start: var(--sp-100); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .by-priority { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 24ch), 1fr)); gap: var(--sp-100); align-items: start; }
  .choices { display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-200); }
</style>
