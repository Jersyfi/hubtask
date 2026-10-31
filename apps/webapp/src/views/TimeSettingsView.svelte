<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Time* under *How we work*, where the time module is on: the workspace's time policy - the last
  // locked day and whether other people's time is seen in full or only as sums (UC-TIM-03 checks 4
  // and 5) - and its kinds of work, each with whether its time starts billable, in their order,
  // archived rather than deleted because records keep them (UC-TIM-01 check 2).
  //
  // **What the reader may never do is not drawn** (P-05): the policy is set with APPROVE_TIME or
  // MANAGE_MEMBERS at the workspace, a kind of work written with STRUCTURE; without, the page
  // shows the values. The cost rates (UC-TIM-08) are drawn only for MANAGE_COSTS or MANAGE_MEMBERS.

  import { untrack } from 'svelte';
  import {
    Banner, Button, Dialog, IconButton, Inline, Input, PageHeader, Radio, Skeleton, Stack, Switch, Table, type Column,
  } from '@hubtask/design-system/components';
  import type { TimePolicy, WorkType, WorkTypeList } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { keyFrom } from '../lib/data/entrytypes.svelte.ts';
  import { POLICY, WORK_TYPES, time } from '../lib/data/time.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import CostRates from '../lib/time/CostRates.svelte';

  $effect(() => page.entitle(t('app.time_settings.title')));

  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const mayLock = $derived(holdsHere('APPROVE_TIME') === true || holdsHere('MANAGE_MEMBERS') === true);
  const mayCost = $derived(holdsHere('MANAGE_COSTS') === true || holdsHere('MANAGE_MEMBERS') === true);

  let policy = $state<TimePolicy | undefined>(undefined);
  let kinds = $state<WorkType[] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => untrack(() => {
    const stops = [
      engine.subscribe<TimePolicy>({ path: POLICY }, (next) => {
        if (next.status === 'ready') policy = next.data;
        if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
      }),
      engine.subscribe<WorkTypeList>({ path: WORK_TYPES }, (next) => { if (next.status === 'ready') kinds = next.data.data; }),
    ];
    return () => stops.forEach((stop) => stop());
  }));

  let failure = $state<string | undefined>(undefined);
  async function act(run: () => Promise<unknown>, done: string): Promise<boolean> {
    failure = undefined;
    try {
      await run();
      toast.show({ text: done, tone: 'success' });
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return false;
    }
  }

  // --- The policy ---------------------------------------------------------------------------------

  const draft = $state({ lockedThrough: '', visibility: 'FULL' as string });
  $effect(() => {
    const read = policy;
    if (!read) return;
    untrack(() => { draft.lockedThrough = read.locked_through ?? ''; draft.visibility = read.visibility; });
  });
  const isPolicyChanged = $derived(policy !== undefined && ((policy.locked_through ?? '') !== draft.lockedThrough || policy.visibility !== draft.visibility));
  // A press with nothing changed writes nothing (UC-WRK-32 check 3: the save is never unavailable for it).
  function savePolicy() {
    const read = policy;
    if (!read || !isPolicyChanged) return;
    void act(() => time.setPolicy(read, { locked_through: draft.lockedThrough || null, visibility: draft.visibility === 'AGGREGATED' ? 'AGGREGATED' : 'FULL' }), t('app.time_settings.policy_saved'));
  }

  // --- The kinds of work --------------------------------------------------------------------------

  const ordered = $derived([...(kinds ?? [])].sort((a, b) => a.position - b.position));
  const columns = $derived<Column[]>([
    { id: 'name', label: t('app.time_settings.col_name') },
    { id: 'key', label: t('app.time_settings.col_key') },
    { id: 'billable', label: t('app.time_settings.col_billable') },
    { id: 'state', label: t('app.time_settings.col_state') },
    { id: 'actions', label: t('app.time_settings.col_actions'), isLabelHidden: true },
  ]);

  let isEditing = $state(false);
  let editing = $state<WorkType | undefined>(undefined);
  const kindDraft = $state({ key: '', name: '', billable: true, isKeyTouched: false });
  function openKind(kind: WorkType | undefined) {
    editing = kind;
    kindDraft.key = kind?.key ?? '';
    kindDraft.name = kind?.name ?? '';
    kindDraft.billable = kind?.billable ?? true;
    kindDraft.isKeyTouched = kind !== undefined;
    isEditing = true;
  }
  function rename(value: string) {
    kindDraft.name = value;
    if (!editing && !kindDraft.isKeyTouched) kindDraft.key = keyFrom(value);
  }
  async function saveKind() {
    const name = kindDraft.name.trim();
    if (!name || !kindDraft.key) return;
    const position = editing?.position ?? (ordered.at(-1)?.position ?? 0) + 1;
    const ok = await act(
      () => time.defineWorkType(kindDraft.key, { name, billable: kindDraft.billable, archived: editing?.archived ?? false, position }),
      t('app.time_settings.kind_saved', { name }),
    );
    if (ok) isEditing = false;
  }
  function archive(kind: WorkType, archived: boolean) {
    void act(
      () => time.defineWorkType(kind.key, { name: kind.name, billable: kind.billable, archived, position: kind.position }),
      t(archived ? 'app.time_settings.kind_archived' : 'app.time_settings.kind_restored', { name: kind.name }),
    );
  }
  function move(kind: WorkType, by: -1 | 1) {
    const at = ordered.findIndex((each) => each.key === kind.key);
    const other = ordered[at + by];
    if (!other) return;
    void act(async () => {
      await time.defineWorkType(kind.key, { name: kind.name, billable: kind.billable, archived: kind.archived, position: other.position });
      await time.defineWorkType(other.key, { name: other.name, billable: other.billable, archived: other.archived, position: kind.position });
    }, t('app.time_settings.kind_moved', { name: kind.name }));
  }
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.time_settings.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'time', label: t('app.time_settings.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
    secondary={mayShape ? [{ label: t('app.time_settings.add_kind'), icon: 'plus', onclick: () => openKind(undefined) }] : []}
  />

  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !policy || !kinds}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    <section class="block" aria-labelledby="time-policy">
      <h2 id="time-policy" class="heading">{t('app.time_settings.policy')}</h2>
      {#if mayLock}
        <Input label={t('app.time_settings.locked_through')} hint={t('app.time_settings.locked_hint')} type="date" bind:value={draft.lockedThrough} />
        <Radio label={t('app.time_settings.visibility')} name="visibility" bind:value={draft.visibility} options={[
          { value: 'FULL', label: t('app.time_settings.visibility_FULL'), hint: t('app.time_settings.visibility_FULL_hint') },
          { value: 'AGGREGATED', label: t('app.time_settings.visibility_AGGREGATED'), hint: t('app.time_settings.visibility_AGGREGATED_hint') },
        ]} />
        <Inline justify="end"><Button onclick={savePolicy}>{t('app.time_settings.save')}</Button></Inline>
      {:else}
        <p>{policy.locked_through ? t('app.time_settings.locked_until', { day: policy.locked_through }) : t('app.time_settings.nothing_locked')}</p>
        <p>{t(`app.time_settings.visibility_${policy.visibility}`)}</p>
      {/if}
    </section>

    <section class="block" aria-labelledby="time-kinds">
      <h2 id="time-kinds" class="heading">{t('app.time_settings.kinds')}</h2>
      <Table label={t('app.time_settings.kinds')} isLabelHidden {columns}>
        {#snippet empty()}{t('app.time_settings.no_kinds')}{/snippet}
        {#each ordered as kind, i (kind.key)}
          <tr>
            <th scope="row">{kind.name}</th>
            <td><code>{kind.key}</code></td>
            <td>{kind.billable ? t('app.time.billable_yes') : t('app.time.billable_no')}</td>
            <td>{kind.archived ? t('app.time_settings.archived') : t('app.time_settings.offered')}</td>
            <td>
              {#if mayShape}
                <Inline gap="050">
                  <IconButton icon="chevron-up" size="sm" label={t('app.time_settings.move_up', { name: kind.name })} disabledReason={i === 0 ? t('app.time_settings.first') : undefined} onclick={() => move(kind, -1)} />
                  <IconButton icon="chevron-down" size="sm" label={t('app.time_settings.move_down', { name: kind.name })} disabledReason={i === ordered.length - 1 ? t('app.time_settings.last') : undefined} onclick={() => move(kind, 1)} />
                  <Button size="sm" tone="subtle" icon="pencil" onclick={() => openKind(kind)}>{t('app.time_settings.edit')}</Button>
                  <Button size="sm" tone="subtle" icon="archive" onclick={() => archive(kind, !kind.archived)}>{kind.archived ? t('app.time_settings.restore') : t('app.time_settings.archive')}</Button>
                </Inline>
              {/if}
            </td>
          </tr>
        {/each}
      </Table>
    </section>

    {#if mayCost}<CostRates />{/if}
  {/if}
</Stack>

{#if isEditing}
  <Dialog bind:isOpen={isEditing} title={editing ? t('app.time_settings.edit') : t('app.time_settings.add_kind')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.time_settings.col_name')} value={kindDraft.name} maxlength={80} oninput={(event: Event) => rename((event.currentTarget as HTMLInputElement).value)} />
      <Input label={t('app.time_settings.col_key')} hint={t('app.time_settings.key_hint')} value={kindDraft.key} disabledReason={editing ? t('app.time_settings.key_fixed') : undefined}
        oninput={(event: Event) => { kindDraft.key = (event.currentTarget as HTMLInputElement).value; kindDraft.isKeyTouched = true; }} />
      <Switch label={t('app.time_settings.billable_default')} bind:checked={kindDraft.billable} />
      <Inline justify="end"><Button onclick={() => void saveKind()}>{t('app.time_settings.save')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .block { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  p { margin: 0; }
</style>
