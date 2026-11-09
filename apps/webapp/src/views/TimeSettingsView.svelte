<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The time's settings (page-system §9.6), where the time module is on: the workspace's time policy - the last
  // locked day and whether other people's time is seen in full or only as sums (UC-TIM-03 checks 4
  // and 5) - and its kinds of work, each with whether its time starts billable, in their order,
  // archived rather than deleted because records keep them (UC-TIM-01 check 2).
  //
  // **What the reader may never do is not drawn** (P-05): the policy is set with APPROVE_TIME or
  // MANAGE_MEMBERS at the workspace, a kind of work written with STRUCTURE; without, the page
  // shows the values. The cost rates (UC-TIM-08) are drawn only for MANAGE_COSTS or MANAGE_MEMBERS.
  //
  // Each part is a section at `/time?tab=settings&section=<id>`: the time's last tab, under the
  // home's head and `ViewBar` (the navigation concept §4); *Record time* is not offered on it.

  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    Button, DataTable, Dialog, EmptyState, Inline, Input, SaveBar, SettingsLayout, SettingsSection, Skeleton, Stack, Switch,
    type DataColumn, type DataRowMenu, type SectionLink,
  } from '@hubtask/design-system/components';
  import type { SettingDefinition, TimePolicy, WorkType, WorkTypeList } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { keyFrom } from '../lib/data/entrytypes.svelte.ts';
  import { POLICY, WORK_TYPES, time } from '../lib/data/time.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { leaveGuard } from '../lib/frame/leave.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import CostRates from '../lib/time/CostRates.svelte';
  import { partOf } from '../lib/time/policy.ts';
  import { settingsCatalogue } from '../lib/data/settingsCatalogue.svelte.ts';
  import SettingControl from '../lib/settings/SettingControl.svelte';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { pagePart } from '../lib/settings/asFile.ts';
  import TableSection from '../lib/page/TableSection.svelte';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';
  import ProductTabs from '../lib/product/ProductTabs.svelte';
  import { productHead } from '../lib/product/frame.svelte.ts';

  interface Props {
    /** The section the address names (`&section=`); none is the list of sections. */
    section?: string;
    /** Replaces the address: a move between sections. */
    onreplace?: (path: string) => void;
    onnavigate?: (path: string) => void;
  }
  const { section, onreplace, onnavigate }: Props = $props();
  const BASE = '/time?tab=settings';
  const title = $derived(t('app.time_settings.page_title'));
  $effect(() => page.entitle(title));

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
  // Each part is drawn from its catalogue declaration (configuration/ui.md A1, A2): a part the
  // installation or the plan locked is unavailable with who decides it, beside its source chip.
  let definitions = $state<Readonly<Record<string, SettingDefinition>>>({});
  $effect(() => untrack(() => settingsCatalogue.read((next) => {
    if (next.status === 'ready') definitions = Object.fromEntries(next.data.settings.map((each) => [each.key, each]));
  })));
  // The save bar's count and the leave guard's (user-experience.md §5.4).
  const changes = $derived(policy === undefined ? 0
    : [(policy.locked_through ?? '') !== draft.lockedThrough, policy.visibility !== draft.visibility].filter(Boolean).length);
  let isSaving = $state(false);
  // A press with nothing changed writes nothing (UC-WRK-32 check 3: the save is never unavailable for it).
  async function savePolicy() {
    const read = policy;
    if (!read || changes === 0 || isSaving) return;
    isSaving = true;
    await act(() => time.setPolicy(read, { locked_through: draft.lockedThrough || null, visibility: draft.visibility === 'AGGREGATED' ? 'AGGREGATED' : 'FULL' }), t('app.time_settings.policy_saved'));
    isSaving = false;
  }
  function discardPolicy() {
    const read = policy;
    if (!read) return;
    draft.lockedThrough = read.locked_through ?? '';
    draft.visibility = read.visibility;
  }
  $effect(() => leaveGuard.hold(() => changes));

  const offered = $derived(['policy', 'kinds', ...(mayCost ? ['rates'] : [])]);
  const named = $derived(offered.find((each) => each === section));
  const shown = $derived(named ?? 'policy');
  const view = $derived<'list' | 'detail'>(named ? 'detail' : 'list');
  const sectionLabel = (id: string) => t(id === 'policy' ? 'app.time_settings.policy' : id === 'kinds' ? 'app.time_settings.kinds' : 'app.rates.title');
  const links = $derived<SectionLink[]>(offered.map((id) => ({
    id, label: sectionLabel(id), href: `${BASE}&section=${id}`,
    ...(id === 'kinds' && kinds ? { status: String(kinds.length) } : {}),
    ...(id === 'policy' && changes > 0 ? { status: t('app.save_bar.changes', { count: String(changes) }), statusTone: 'set' as const } : {}),
  })));
  let isFileOpen = $state(false);
  const fileAction = $derived(viewAsFileAction(() => (isFileOpen = true)));

  // --- The kinds of work --------------------------------------------------------------------------

  const ordered = $derived([...(kinds ?? [])].sort((a, b) => a.position - b.position));
  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.time_settings.col_name'), priority: 1 },
    { id: 'billable', label: t('app.time_settings.col_billable'), priority: 2 },
    { id: 'state', label: t('app.time_settings.col_state'), priority: 2, isStatus: true },
  ]);
  /** A kind's ⋯: its order and whether it is offered (PS-6). */
  function kindMenu(kind: WorkType): DataRowMenu | undefined {
    if (!mayShape) return undefined;
    const at = ordered.findIndex((each) => each.key === kind.key);
    return {
      label: t('app.page.more_for', { name: kind.name }),
      items: [
        ...(at > 0 ? [{ id: 'up', label: t('app.time_settings.move_up', { name: kind.name }), icon: 'chevron-up' as const }] : []),
        ...(at < ordered.length - 1 ? [{ id: 'down', label: t('app.time_settings.move_down', { name: kind.name }), icon: 'chevron-down' as const }] : []),
        { id: 'archive', label: kind.archived ? t('app.time_settings.restore') : t('app.time_settings.archive'), icon: 'archive' as const, hasSeparatorBefore: at > 0 || at < ordered.length - 1 },
      ],
    };
  }
  function onKindMenu(kind: WorkType, id: string) {
    if (id === 'up') move(kind, -1);
    else if (id === 'down') move(kind, 1);
    else archive(kind, !kind.archived);
  }

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

{#snippet tabs()}<ProductTabs module="TIME" current="settings" {onnavigate} />{/snippet}

<SettingsLayout
  head={{ ...productHead('TIME'), secondary: fileAction ? ([fileAction] as const) : ([] as const) }}
  {tabs}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current: shown, onselect: (next) => onreplace?.(`${BASE}&section=${next}`) }}
  {view}
  back={{ label: t('app.product_home.tab_settings'), onback: () => onreplace?.(BASE) }}
>
  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !policy || !kinds}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else if shown === 'policy'}
    <div data-time-part="policy">
      <SettingsSection title={t('app.time_settings.policy')} save={mayLock ? saveBar : undefined}>
        {#if mayLock}
          {#if definitions['time.locked_through']}
            <SettingControl definition={definitions['time.locked_through']} cell={{ ...partOf(policy, 'time.locked_through'), value: policy.locked_through ?? null }}
              value={draft.lockedThrough || null} holds={() => mayLock} onchange={(next) => (draft.lockedThrough = typeof next === 'string' ? next : '')} />
          {/if}
          {#if definitions['time.visibility']}
            <SettingControl definition={definitions['time.visibility']} cell={{ ...partOf(policy, 'time.visibility'), value: policy.visibility }}
              value={draft.visibility} holds={() => mayLock} onchange={(next) => (draft.visibility = String(next))} />
          {/if}
        {:else}
          <p class="value">{policy.locked_through ? t('app.time_settings.locked_until', { day: policy.locked_through }) : t('app.time_settings.nothing_locked')}</p>
          <p class="value">{t(`app.time_settings.visibility_${policy.visibility}`)}</p>
        {/if}
      </SettingsSection>
    </div>
  {:else if shown === 'kinds'}
    <div data-time-part="kinds">
      <TableSection title={t('app.time_settings.kinds')} note={String(ordered.length)}>
        {#snippet acts()}
          {#if mayShape}<Button icon="plus" data-list-add onclick={() => openKind(undefined)}>{t('app.time_settings.add_kind')}</Button>{/if}
        {/snippet}
        <DataTable
          label={t('app.time_settings.kinds')}
          {columns}
          rows={ordered}
          key={(kind: WorkType) => kind.key}
          rowLabel={(kind: WorkType) => kind.name}
          onopen={mayShape ? (kind: WorkType) => openKind(kind) : undefined}
          menu={kindMenu}
          onmenu={onKindMenu}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          actionsLabel={t('app.time_settings.col_actions')}
        >
          {#snippet cell(kind: WorkType, column: DataColumn)}
            {#if column.id === 'name'}{kind.name}
            {:else if column.id === 'billable'}{kind.billable ? t('app.time.billable_yes') : t('app.time.billable_no')}
            {:else}{kind.archived ? t('app.time_settings.archived') : t('app.time_settings.offered')}{/if}
          {/snippet}
          {#snippet empty()}<EmptyState kind="unused" title={t('app.time_settings.no_kinds')} />{/snippet}
        </DataTable>
      </TableSection>
    </div>
  {:else}
    <CostRates />
  {/if}
</SettingsLayout>

<ViewSectionsAsFile {...pagePart('/time?tab=settings')} bind:isOpen={isFileOpen} isTriggerHidden />

{#snippet saveBar()}
  <SaveBar
    label={t('app.save_bar.label')}
    changesLabel={changes > 0 ? t('app.save_bar.changes', { count: String(changes) }) : undefined}
    discardLabel={t('app.save_bar.discard')}
    saveLabel={t('app.time_settings.save')}
    isSaving={isSaving}
    savingLabel={t('app.time_settings.saving')}
    onDiscard={discardPolicy}
    onSave={() => void savePolicy()}
  />
{/snippet}

{#if isEditing}
  <Dialog bind:isOpen={isEditing} title={editing ? t('app.time_settings.edit') : t('app.time_settings.add_kind')} dismissLabel={t('app.links.cancel')} size="m">
    <Stack gap="150">
      <Input label={t('app.time_settings.col_name')} value={kindDraft.name} maxlength={80} oninput={(event: Event) => rename((event.currentTarget as HTMLInputElement).value)} />
      <!-- The key is made from the name; asked only where a name yields none (a script without Latin letters). -->
      {#if !editing && kindDraft.name.trim() !== '' && (kindDraft.isKeyTouched || keyFrom(kindDraft.name) === '')}
        <Input label={t('app.time_settings.col_key')} hint={t('app.time_settings.key_hint')} value={kindDraft.key}
          oninput={(event: Event) => { kindDraft.key = (event.currentTarget as HTMLInputElement).value; kindDraft.isKeyTouched = true; }} />
      {/if}
      <Switch label={t('app.time_settings.billable_default')} bind:checked={kindDraft.billable} />
      <Inline justify="end"><Button onclick={() => void saveKind()}>{t('app.time_settings.save')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .value { margin: 0; padding: var(--sp-150) var(--sp-200); }
</style>
