<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's *Working hours* in its settings, where the service desk is on (UC-SVC-01 check 5): each
  // of the workspace's calendars, with the hub's own version where it keeps one - its own hours,
  // holidays and zone under the same key, which every SLA policy naming the key counts on for the
  // hub's entries. Keeping or dropping one needs STRUCTURE on the hub; the others read.

  import { untrack } from 'svelte';
  import { Badge, Button, Checkbox, Inline, Input, Skeleton, Stack, Textarea } from '@hubtask/design-system/components';
  import type { BusinessCalendarList } from '@hubtask/sync-engine';

  import { calendarsPath, desk, hubCalendarsPath } from '../data/desk.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { WEEKDAYS, formOf, holidaysOf, hoursOf, hubCalendarRows, type HubCalendarRow } from './desk.ts';

  interface Props {
    hubId: string;
    mayShape: boolean;
  }

  let { hubId, mayShape }: Props = $props();

  const workspace = untrack(() => resource<BusinessCalendarList>(calendarsPath));
  const own = untrack(() => resource<BusinessCalendarList>(hubCalendarsPath(hubId)));
  const rows = $derived(
    workspace.state.status === 'ready' && own.state.status === 'ready'
      ? hubCalendarRows(workspace.state.data.data, own.state.data.data)
      : undefined,
  );

  let editing = $state<string | undefined>(undefined);
  let form = $state({ name: '', zone: '', days: [] as string[], from: '08:00', to: '17:00', holidays: '' });
  let refusal = $state<string | undefined>(undefined);
  let isSaving = $state(false);

  function edit(row: HubCalendarRow) {
    const from = row.own ?? row.workspace;
    const span = formOf(from);
    editing = row.key;
    form = { name: from.name, zone: from.zone, days: span.days, from: span.from, to: span.to, holidays: from.holidays.join('\n') };
  }

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

  async function save(event: SubmitEvent) {
    event.preventDefault();
    const key = editing;
    if (!key) return;
    const saved = await attempt(() => desk.defineHubCalendar(hubId, key, {
      name: form.name.trim(), zone: form.zone.trim(), hours: hoursOf(form.days, form.from, form.to), holidays: holidaysOf(form.holidays),
    }), 'app.desk.calendar_saved');
    if (saved) editing = undefined;
  }
</script>

<Stack gap="150">
  <p class="quiet">{t('app.desk.hub_calendars_hint')}</p>
  {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
  {#if !rows}<Skeleton lines={3} />{:else if rows.length === 0}<p class="quiet">{t('app.desk.calendars_none')}</p>{/if}
  <ul class="rows">
    {#each rows ?? [] as row (row.key)}
      <li>
        <Inline gap="100" align="center" justify="between">
          <span>
            <strong>{(row.own ?? row.workspace).name}</strong> · {(row.own ?? row.workspace).zone}
            · {t('app.desk.spans', { count: String((row.own ?? row.workspace).hours.length) })}
            {#if row.own}<Badge size="sm" tone="info">{t('app.desk.hub_own')}</Badge>{:else}<Badge size="sm">{t('app.desk.hub_workspace')}</Badge>{/if}
          </span>
          {#if mayShape}
            <Inline gap="050">
              <Button size="sm" tone="secondary" icon="pencil" onclick={() => edit(row)}>
                {row.own ? t('app.desk.edit') : t('app.desk.hub_keep_own')}</Button>
              {#if row.own}
                <Button size="sm" tone="subtle" icon="rotate-ccw" isBusy={isSaving} busyLabel={t('app.desk.saving')}
                  onclick={() => void attempt(() => desk.deleteHubCalendar(hubId, row.key), 'app.desk.hub_back_to_workspace')}>
                  {t('app.desk.hub_use_workspace')}</Button>
              {/if}
            </Inline>
          {/if}
        </Inline>
        {#if mayShape && editing === row.key}
          <form class="panel" onsubmit={(event) => void save(event)}>
            <Stack gap="100">
              <Input label={t('app.desk.name')} bind:value={form.name} required />
              <Input label={t('app.desk.zone')} hint={t('app.desk.zone_hint')} bind:value={form.zone} required />
              <div class="choices" role="group" aria-label={t('app.desk.days')}>
                {#each WEEKDAYS as weekday (weekday)}
                  <Checkbox label={t(`app.desk.day.${weekday.toLowerCase()}`)} checked={form.days.includes(weekday)}
                    onchange={(event: Event) => {
                      const on = (event.currentTarget as HTMLInputElement).checked;
                      form.days = on ? [...form.days, weekday] : form.days.filter((each) => each !== weekday);
                    }} />
                {/each}
              </div>
              <div class="fields">
                <Input label={t('app.desk.from')} type="time" bind:value={form.from} />
                <Input label={t('app.desk.to')} type="time" bind:value={form.to} />
              </div>
              <Textarea label={t('app.desk.holidays')} hint={t('app.desk.holidays_hint')} rows={3} bind:value={form.holidays} />
              <Inline gap="100">
                <Button type="submit" tone="primary" icon="calendar" isBusy={isSaving} busyLabel={t('app.desk.saving')}>{t('app.desk.replace')}</Button>
                <Button tone="subtle" onclick={() => (editing = undefined)}>{t('app.desk.cancel')}</Button>
              </Inline>
            </Stack>
          </form>
        {/if}
      </li>
    {/each}
  </ul>
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .rows li { display: grid; gap: var(--sp-100); padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); }
  form { margin: 0; max-inline-size: 64ch; }
  .panel { padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 20ch), 1fr)); gap: var(--sp-100); align-items: end; }
  .choices { display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-200); }
</style>
