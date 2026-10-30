<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One goal (UC-GOL-01 check 9, UC-GOL-02 check 6): its status, confidence and progress, its key
  // results, the entries linked to it and its check-ins, the latest first.
  //
  // **Writes** where the reader may write at the goal's place: change it and its key results (a
  // number from a start to a target, a percentage, reached or not, a milestone waiting on an entry,
  // or a saved summary view's total), link and unlink entries, check in, delete it. A graded goal
  // changes no more; the server says so (`goals.graded`).
  //
  // **The server computes the numbers**: a view's total and a milestone's entry are counted for
  // this reader, and an entry they may not read is not listed (UC-GOL-01 check 5).

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Button, Dialog, IconButton, Inline, Input, PageHeader, ProgressBar, Select, Skeleton, Stack, Table, Textarea,
    type Column,
  } from '@hubtask/design-system/components';
  import type { Goal, GoalCheckInList, GoalList, KeyResultWrite, SavedView, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { holds, holdsHere, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import {
    checkInsPath, goalPath, goals, goalsPath, goalWrite, parentChoices, percentOf, percentText, type CheckIn,
  } from '../lib/data/goals.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { views } from '../lib/data/views.svelte.ts';
  import EntryPicker from '../lib/entries/EntryPicker.svelte';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { goalTone } from '../lib/goals/tone.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    goalId: string;
    onnavigate: (path: string) => void;
  }

  const { goalId, onnavigate }: Props = $props();

  let goal = $state<Goal | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = goalPath(goalId);
    return untrack(() => engine.subscribe<Goal>({ path }, (next) => {
      if (next.status === 'ready') { goal = next.data; refusal = undefined; }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });
  $effect(() => frame.entitle(goal ? goal.title : t('app.goals.goal')));
  $effect(() => accounts.resolve([goal?.owner_id, ...(goal?.check_ins ?? []).map((each) => each.author_id)]));

  // The period's other goals, for the parent a goal hangs under.
  let siblings = $state<Goal[]>([]);
  $effect(() => {
    const period = goal?.period_id;
    if (!period) return;
    const path = goalsPath(period);
    return untrack(() => engine.subscribe<GoalList>({ path }, (next) => { if (next.status === 'ready') siblings = [...next.data.data]; }));
  });
  const parentName = $derived(goal?.parent_id ? siblings.find((each) => each.id === goal?.parent_id)?.title : undefined);
  const children = $derived(goal ? siblings.filter((each) => each.parent_id === goal?.id) : []);

  // Every check-in, beyond the latest twenty the goal carries.
  let history = $state<GoalCheckInList['data'] | undefined>(undefined);
  let isHistoryAsked = $state(false);
  $effect(() => {
    if (!isHistoryAsked) return;
    const path = checkInsPath(goalId);
    return untrack(() => engine.subscribe<GoalCheckInList>({ path }, (next) => { if (next.status === 'ready') history = next.data.data; }));
  });
  const checkIns = $derived(history ?? goal?.check_ins ?? []);

  // Who may write: WRITE_ITEMS at the goal's place - the hub's role, or the workspace's.
  $effect(() => {
    const hub = goal?.hub_id;
    if (hub) return untrack(() => people.open({ hubId: hub }));
  });
  const mayWrite = $derived.by(() => {
    if (!goal || goal.grade !== null) return false;
    if (goal.hub_id) return holds(people.roleOf({ hubId: goal.hub_id }, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted';
    return holdsHere('WRITE_ITEMS') === true;
  });

  const placeName = $derived(goal?.hub_id ? (containers.find(goal.hub_id)?.name ?? t('app.goals.hub_unread')) : t('app.goals.workspace'));
  const personName = (id: string | null | undefined) => (id ? (accounts.nameOf(id) ?? t('app.people.unnamed')) : t('app.goals.nobody'));

  let failure = $state<string | undefined>(undefined);
  async function act(run: () => Promise<unknown>, done?: string): Promise<boolean> {
    failure = undefined;
    try {
      await run();
      if (done) toast.show({ text: done, tone: 'success' });
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return false;
    }
  }

  const resultColumns = $derived<Column[]>([
    { id: 'title', label: t('app.goals.kr_title') },
    { id: 'kind', label: t('app.goals.kr_kind') },
    { id: 'start', label: t('app.goals.kr_start'), align: 'end' },
    { id: 'target', label: t('app.goals.kr_target'), align: 'end' },
    { id: 'current', label: t('app.goals.kr_current'), align: 'end' },
    { id: 'progress', label: t('app.goals.col_progress') },
  ]);
  const entryColumns = $derived<Column[]>([
    { id: 'entry', label: t('app.goals.entry') },
    { id: 'done', label: t('app.goals.entry_state') },
    { id: 'actions', label: t('app.goals.actions'), isLabelHidden: true },
  ]);
  const checkInColumns = $derived<Column[]>([
    { id: 'when', label: t('app.goals.checkin_when') },
    { id: 'who', label: t('app.goals.checkin_who') },
    { id: 'status', label: t('app.goals.col_status') },
    { id: 'confidence', label: t('app.goals.col_confidence'), align: 'end' },
    { id: 'note', label: t('app.goals.checkin_note') },
  ]);
  const locale = $derived(messages.locale);

  // --- Changing the goal ------------------------------------------------------------------------

  let isChanging = $state(false);
  const draft = $state({ title: '', description: '', status: 'NONE' as Goal['status'], parent: '', owner: '' });
  function openChange() {
    if (!goal) return;
    draft.title = goal.title;
    draft.description = goal.description;
    draft.status = goal.status;
    draft.parent = goal.parent_id ?? '';
    draft.owner = goal.owner_id ?? '';
    isChanging = true;
  }
  const ownerChoices = $derived([
    { value: '', label: t('app.goals.nobody') },
    ...(goal?.owner_id && goal.owner_id !== actor.account?.id ? [{ value: goal.owner_id, label: personName(goal.owner_id) }] : []),
    ...(actor.account ? [{ value: actor.account.id, label: t('app.goals.owner_me') }] : []),
  ]);
  async function saveChange() {
    const at = goal;
    if (!at) return;
    const ok = await act(() => goals.update(at, goalWrite(at, {
      title: draft.title.trim(), description: draft.description, status: draft.status, parent_id: draft.parent, owner_id: draft.owner,
    })), t('app.goals.saved'));
    if (ok) isChanging = false;
  }

  let isDeleting = $state(false);
  async function remove() {
    const at = goal;
    if (!at) return;
    const ok = await act(() => goals.remove(at), t('app.goals.deleted', { title: at.title }));
    if (ok) { isDeleting = false; onnavigate(`/goals?period=${at.period_id}`); }
  }

  // --- Key results ------------------------------------------------------------------------------

  type ResultDraft = {
    id?: string; title: string; kind: KeyResultWrite['kind']; source: 'MANUAL' | 'VIEW';
    start: string; target: string; current: string; unit: string; viewId: string; entry?: { id: string; title: string };
  };
  let isEditingResults = $state(false);
  let results = $state<ResultDraft[]>([]);
  $effect(() => {
    if (!isEditingResults) return;
    return untrack(() => views.open());
  });
  const summaryViews = $derived<SavedView[]>(isEditingResults ? views.of().filter((each) => each.query && 'aggregate' in each.query) : []);
  function openResults() {
    if (!goal) return;
    results = goal.key_results.map((each) => ({
      id: each.id, title: each.title, kind: each.kind, source: each.source,
      start: String(each.start), target: String(each.target), current: String(each.current), unit: each.unit,
      viewId: each.view_id ?? '', ...(each.entry_id ? { entry: { id: each.entry_id, title: t('app.goals.milestone_entry') } } : {}),
    }));
    isEditingResults = true;
  }
  function addResult() {
    results = [...results, { title: '', kind: 'NUMBER', source: 'MANUAL', start: '0', target: '100', current: '0', unit: '', viewId: '' }];
  }
  const numberOf = (text: string): number | undefined => {
    const value = Number(text.trim().replace(',', '.'));
    return text.trim() === '' || !Number.isFinite(value) ? undefined : value;
  };
  const resultsProblem = $derived.by(() => {
    for (const each of results) {
      if (each.title.trim() === '') return t('app.goals.kr_title_needed');
      if (each.kind === 'MILESTONE' && !each.entry) return t('app.goals.kr_entry_needed');
      if (each.source === 'VIEW' && each.viewId === '') return t('app.goals.kr_view_needed');
      if (each.kind === 'NUMBER' && (numberOf(each.start) === undefined || numberOf(each.target) === undefined)) return t('app.goals.kr_numbers_needed');
    }
    return undefined;
  });
  function resultWrite(each: ResultDraft): KeyResultWrite {
    const write: KeyResultWrite = { title: each.title.trim(), kind: each.kind, source: each.kind === 'NUMBER' || each.kind === 'PERCENT' ? each.source : 'MANUAL' };
    if (each.id) write.id = each.id;
    if (each.kind === 'NUMBER') {
      write.start = numberOf(each.start) ?? 0;
      write.target = numberOf(each.target) ?? 0;
      write.unit = each.unit.trim();
    }
    if (each.kind === 'PERCENT') { write.start = 0; write.target = 100; }
    if (write.source === 'MANUAL' && each.kind !== 'MILESTONE') {
      const current = numberOf(each.current);
      if (current !== undefined) write.current = current;
    }
    if (write.source === 'VIEW') write.view_id = each.viewId;
    if (each.kind === 'MILESTONE' && each.entry) write.entry_id = each.entry.id;
    return write;
  }
  async function saveResults() {
    const at = goal;
    if (!at) return;
    const ok = await act(() => goals.update(at, goalWrite(at, { key_results: results.map(resultWrite) })), t('app.goals.saved'));
    if (ok) isEditingResults = false;
  }
  const valueText = (kind: string, value: number, unit: string) =>
    kind === 'BINARY' ? (value >= 1 ? t('app.goals.kr_reached') : t('app.goals.kr_not_reached'))
      : kind === 'PERCENT' ? t('app.goals.percent', { percent: String(value) })
        : unit ? t('app.goals.kr_value_unit', { value: String(value), unit }) : String(value);

  // --- Linked entries ---------------------------------------------------------------------------

  let isLinking = $state(false);
  let linking = $state<WorkItem | undefined>(undefined);
  const linked = $derived(new Set((goal?.linked_entries ?? []).map((each) => each.id)));
  async function link() {
    if (!goal || !linking) return;
    const ok = await act(() => goals.link(goalId, [linking!.id]), t('app.goals.linked', { title: linking.title }));
    if (ok) { isLinking = false; linking = undefined; }
  }

  // --- Checking in ------------------------------------------------------------------------------

  let isCheckingIn = $state(false);
  const checkIn = $state({ status: 'ON_TRACK' as CheckIn['status'], confidence: '7', note: '', values: {} as Record<string, string> });
  const manualResults = $derived((goal?.key_results ?? []).filter((each) => each.source === 'MANUAL' && each.kind !== 'MILESTONE'));
  function openCheckIn() {
    if (!goal) return;
    checkIn.status = goal.status === 'NONE' ? 'ON_TRACK' : goal.status;
    checkIn.confidence = String(goal.confidence ?? 7);
    checkIn.note = '';
    checkIn.values = Object.fromEntries(manualResults.map((each) => [each.id, String(each.current)]));
    isCheckingIn = true;
  }
  const confidence = $derived(numberOf(checkIn.confidence));
  const confidenceError = $derived(confidence === undefined || !Number.isInteger(confidence) || confidence < 0 || confidence > 10 ? t('app.goals.confidence_range') : undefined);
  async function saveCheckIn() {
    if (confidence === undefined || confidenceError) return;
    const values = manualResults
      .map((each) => ({ key_result_id: each.id, value: numberOf(checkIn.values[each.id] ?? '') }))
      .filter((each): each is { key_result_id: string; value: number } => each.value !== undefined);
    const ok = await act(() => goals.checkIn(goalId, { status: checkIn.status, confidence, note: checkIn.note, values }), t('app.goals.checked_in'));
    if (ok) isCheckingIn = false;
  }

  const STATUSES = ['NONE', 'ON_TRACK', 'AT_RISK', 'OFF_TRACK', 'DONE', 'DROPPED'] as const;
  const KINDS = ['NUMBER', 'PERCENT', 'BINARY', 'MILESTONE'] as const;
</script>

<Stack gap="300">
  <PageHeader
    title={goal?.title ?? t('app.goals.goal')}
    subtitle={goal ? t('app.goals.goal_subtitle', { place: placeName }) : undefined}
    isTitleInBar={viewport.isCompact}
    breadcrumb={goal
      ? {
          trail: [{ id: 'goals', label: t('app.goals.title'), href: `/goals?period=${goal.period_id}` }, { id: 'goal', label: goal.title }],
          label: t('app.goals.trail'),
          expandLabel: t('app.goals.expand_trail'),
          onnavigate: () => onnavigate(`/goals?period=${goal?.period_id ?? ''}`),
        }
      : undefined}
    secondary={mayWrite ? [{ label: t('app.goals.check_in'), icon: 'circle-check', onclick: openCheckIn }, { label: t('app.goals.change'), icon: 'pencil', onclick: openChange }] : []}
    menu={mayWrite ? { label: t('app.goals.more'), items: [{ id: 'delete', label: t('app.goals.delete'), icon: 'trash', isDestructive: true }], onselect: () => (isDeleting = true) } : undefined}
  />

  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !goal}
    <div aria-busy="true"><Skeleton lines={8} /></div>
  {:else}
    <section class="summary" aria-label={t('app.goals.summary')}>
      <div class="facts">
        {#if goal.status !== 'NONE'}<Badge tone={goalTone(goal.status)}>{t(`app.goals.status_${goal.status}`)}</Badge>{/if}
        {#if goal.confidence !== null}<span>{t('app.goals.confidence_of', { value: String(goal.confidence) })}</span>{/if}
        <span>{t('app.goals.owned_by', { name: personName(goal.owner_id) })}</span>
        {#if parentName && goal.parent_id}<span>{t('app.goals.serves')} <a href={`/goals/${goal.parent_id}`}>{parentName}</a></span>{/if}
        {#if goal.grade !== null}<Badge tone="info">{t('app.goals.graded', { percent: percentText(goal.grade) })}</Badge>{/if}
      </div>
      {#if goal.progress === null}
        <p class="quiet">{t('app.goals.no_progress_hint')}</p>
      {:else}
        <ProgressBar label={t('app.goals.progress_of', { title: goal.title })} value={percentOf(goal.progress)} valueLabel={t('app.goals.percent', { percent: percentText(goal.progress) })} />
      {/if}
      {#if goal.description}<p class="description">{goal.description}</p>{/if}
    </section>

    <section class="block" aria-labelledby="goal-results">
      <div class="head">
        <h2 id="goal-results" class="heading">{t('app.goals.key_results')}</h2>
        {#if mayWrite}<Button size="sm" tone="secondary" icon="pencil" onclick={openResults}>{t('app.goals.edit_results')}</Button>{/if}
      </div>
      <div class="scroll">
        <Table label={t('app.goals.key_results')} isLabelHidden columns={resultColumns}>
          {#snippet empty()}{t('app.goals.no_results')}{/snippet}
          {#each goal.key_results as result (result.id)}
            <tr>
              <th scope="row">{result.title}{#if result.live}<span class="note">{t('app.goals.kr_live')}</span>{/if}</th>
              <td>{t(`app.goals.kind_${result.kind}`)}{#if result.source === 'VIEW'}<span class="note">{t('app.goals.kr_from_view')}</span>{/if}</td>
              <td class="number">{result.kind === 'NUMBER' ? valueText(result.kind, result.start, result.unit) : ''}</td>
              <td class="number">{result.kind === 'NUMBER' ? valueText(result.kind, result.target, result.unit) : ''}</td>
              <td class="number">
                {#if result.kind === 'MILESTONE'}
                  {#if result.entry_id}<a href={`/items/${result.entry_id}`}>{result.progress >= 1 ? t('app.goals.kr_reached') : t('app.goals.kr_not_reached')}</a>{/if}
                {:else}{valueText(result.kind, result.current, result.unit)}{/if}
              </td>
              <td class="progress"><ProgressBar size="sm" label={t('app.goals.progress_of', { title: result.title })} value={percentOf(result.progress)} valueLabel={t('app.goals.percent', { percent: percentText(result.progress) })} /></td>
            </tr>
          {/each}
        </Table>
      </div>
    </section>

    {#if children.length > 0 || goal.child_count > 0}
      <section class="block" aria-labelledby="goal-children">
        <h2 id="goal-children" class="heading">{t('app.goals.children', { count: String(goal.child_count) })}</h2>
        <ul class="plain">
          {#each children as child (child.id)}
            <li><a href={`/goals/${child.id}`}>{child.title}</a> <span class="note">{t('app.goals.percent', { percent: percentText(child.progress) || '0' })}</span></li>
          {/each}
        </ul>
      </section>
    {/if}

    <section class="block" aria-labelledby="goal-entries">
      <div class="head">
        <h2 id="goal-entries" class="heading">{t('app.goals.entries', { done: String(goal.entries.done), total: String(goal.entries.total) })}</h2>
        {#if mayWrite}<Button size="sm" tone="secondary" icon="link" onclick={() => { linking = undefined; isLinking = true; }}>{t('app.goals.link_entry')}</Button>{/if}
      </div>
      <div class="scroll">
        <Table label={t('app.goals.entries_label')} isLabelHidden columns={entryColumns}>
          {#snippet empty()}{t('app.goals.no_entries')}{/snippet}
          {#each goal.linked_entries ?? [] as entry (entry.id)}
            <tr>
              <th scope="row"><a href={`/items/${entry.id}`}>{entry.title}</a>{#if entry.key}<span class="note">{entry.key}</span>{/if}</th>
              <td>{entry.completed ? t('app.goals.entry_done') : t('app.goals.entry_open')}</td>
              <td>{#if mayWrite}<IconButton icon="x" size="sm" label={t('app.goals.unlink', { title: entry.title })} onclick={() => void act(() => goals.unlink(goalId, entry.id), t('app.goals.unlinked', { title: entry.title }))} />{/if}</td>
            </tr>
          {/each}
        </Table>
      </div>
    </section>

    <section class="block" aria-labelledby="goal-checkins">
      <h2 id="goal-checkins" class="heading">{t('app.goals.checkins')}</h2>
      <div class="scroll">
        <Table label={t('app.goals.checkins')} isLabelHidden columns={checkInColumns}>
          {#snippet empty()}{t('app.goals.no_checkins')}{/snippet}
          {#each checkIns as each (each.id)}
            <tr>
              <td>{formatDateTime(each.created_at, locale)}</td>
              <th scope="row">{personName(each.author_id)}</th>
              <td><Badge size="sm" tone={goalTone(each.status)}>{t(`app.goals.status_${each.status}`)}</Badge></td>
              <td class="number">{t('app.goals.confidence_of', { value: String(each.confidence) })}</td>
              <td class="clip">{each.note}</td>
            </tr>
          {/each}
        </Table>
      </div>
      {#if !isHistoryAsked && (goal.check_ins?.length ?? 0) >= 20}
        <Inline><Button size="sm" tone="subtle" onclick={() => (isHistoryAsked = true)}>{t('app.goals.all_checkins')}</Button></Inline>
      {/if}
    </section>
  {/if}
</Stack>

{#if isChanging && goal}
  <Dialog bind:isOpen={isChanging} title={t('app.goals.change')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.goals.goal_title')} bind:value={draft.title} maxlength={200} />
      <Textarea label={t('app.goals.description')} bind:value={draft.description} maxlength={10000} />
      <Select label={t('app.goals.col_status')} bind:value={draft.status} options={STATUSES.map((each) => ({ value: each, label: t(`app.goals.status_${each}`) }))} />
      <Select label={t('app.goals.col_owner')} bind:value={draft.owner} options={ownerChoices} />
      <Select label={t('app.goals.parent')} hint={t('app.goals.parent_hint')} bind:value={draft.parent}
        options={[{ value: '', label: t('app.goals.no_parent') }, ...parentChoices(siblings, goal.id).map((each) => ({ value: each.id, label: each.title }))]} />
      <Inline justify="end"><Button disabledReason={draft.title.trim() === '' ? t('app.goals.title_needed') : undefined} onclick={() => void saveChange()}>{t('app.goals.save')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isDeleting && goal}
  <Dialog bind:isOpen={isDeleting} title={t('app.goals.delete_title', { title: goal.title })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.goals.delete_hint')}</p>
      <Inline justify="end"><Button tone="danger" onclick={() => void remove()}>{t('app.goals.delete')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isEditingResults}
  <Dialog bind:isOpen={isEditingResults} title={t('app.goals.edit_results')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="200">
      {#each results as result, i (result.id ?? `new-${i}`)}
        <fieldset class="result">
          <legend>{t('app.goals.kr_number', { n: String(i + 1) })}</legend>
          <Stack gap="100">
            <Input label={t('app.goals.kr_title')} bind:value={result.title} maxlength={200} />
            <Select label={t('app.goals.kr_kind')} bind:value={result.kind} options={KINDS.map((each) => ({ value: each, label: t(`app.goals.kind_${each}`) }))} />
            {#if result.kind === 'NUMBER' || result.kind === 'PERCENT'}
              <Select label={t('app.goals.kr_source')} bind:value={result.source}
                options={[{ value: 'MANUAL', label: t('app.goals.source_MANUAL') }, { value: 'VIEW', label: t('app.goals.source_VIEW') }]} />
            {/if}
            {#if result.kind === 'NUMBER'}
              <Inline gap="100" isWrapping>
                <Input label={t('app.goals.kr_start')} bind:value={result.start} inputmode="decimal" />
                <Input label={t('app.goals.kr_target')} bind:value={result.target} inputmode="decimal" />
                <Input label={t('app.goals.kr_unit')} bind:value={result.unit} maxlength={16} />
              </Inline>
            {/if}
            {#if result.source === 'VIEW' && (result.kind === 'NUMBER' || result.kind === 'PERCENT')}
              <Select label={t('app.goals.kr_view')} hint={t('app.goals.kr_view_hint')} bind:value={result.viewId} placeholder={t('app.goals.kr_view_pick')}
                options={summaryViews.map((each) => ({ value: each.id, label: each.name }))} />
            {:else if result.kind === 'MILESTONE'}
              {#if result.entry}<p class="quiet">{t('app.goals.kr_waits_for', { title: result.entry.title })}</p>{/if}
              <EntryPicker onchoose={(item) => { if (item) result.entry = { id: item.id, title: item.title }; }} />
            {:else if result.kind === 'BINARY'}
              <Select label={t('app.goals.kr_current')} bind:value={result.current}
                options={[{ value: '0', label: t('app.goals.kr_not_reached') }, { value: '1', label: t('app.goals.kr_reached') }]} />
            {:else if result.source === 'MANUAL'}
              <Input label={t('app.goals.kr_current')} bind:value={result.current} inputmode="decimal" />
            {/if}
            <Inline justify="end"><Button size="sm" tone="subtle" icon="trash" onclick={() => (results = results.filter((_, j) => j !== i))}>{t('app.goals.kr_remove')}</Button></Inline>
          </Stack>
        </fieldset>
      {/each}
      <Inline justify="between">
        <Button size="sm" tone="secondary" icon="plus" disabledReason={results.length >= 20 ? t('app.goals.kr_at_most') : undefined} onclick={addResult}>{t('app.goals.kr_add')}</Button>
        <Button disabledReason={resultsProblem} onclick={() => void saveResults()}>{t('app.goals.save')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

{#if isLinking}
  <Dialog bind:isOpen={isLinking} title={t('app.goals.link_entry')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <EntryPicker excluded={linked} chosen={linking} onchoose={(item) => (linking = item)} />
      <Inline justify="end"><Button disabledReason={linking ? undefined : t('app.goals.pick_entry')} onclick={() => void link()}>{t('app.goals.link')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isCheckingIn && goal}
  <Dialog bind:isOpen={isCheckingIn} title={t('app.goals.check_in_title', { title: goal.title })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Select label={t('app.goals.col_status')} bind:value={checkIn.status} options={STATUSES.filter((each) => each !== 'NONE').map((each) => ({ value: each, label: t(`app.goals.status_${each}`) }))} />
      <Input label={t('app.goals.col_confidence')} hint={t('app.goals.confidence_hint')} bind:value={checkIn.confidence} inputmode="numeric" error={confidenceError} />
      {#each manualResults as result (result.id)}
        {#if result.kind === 'BINARY'}
          <Select label={result.title} bind:value={checkIn.values[result.id]}
            options={[{ value: '0', label: t('app.goals.kr_not_reached') }, { value: '1', label: t('app.goals.kr_reached') }]} />
        {:else}
          <Input label={result.title} hint={result.kind === 'NUMBER' ? t('app.goals.kr_towards', { target: valueText(result.kind, result.target, result.unit) }) : undefined} bind:value={checkIn.values[result.id]} inputmode="decimal" />
        {/if}
      {/each}
      <Textarea label={t('app.goals.checkin_note')} hint={t('app.goals.checkin_note_hint')} bind:value={checkIn.note} maxlength={5000} />
      <Inline justify="end"><Button disabledReason={confidenceError} onclick={() => void saveCheckIn()}>{t('app.goals.check_in')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .summary, .block { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }
  .facts { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); color: var(--text-secondary); font-size: var(--fs-075); }
  .head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .description { margin: 0; white-space: pre-wrap; }
  .scroll { overflow-x: auto; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .progress { min-width: var(--layout-board-column); }
  .clip { max-inline-size: var(--layout-panel-width); white-space: pre-wrap; }
  .plain { margin: 0; padding-inline-start: var(--sp-200); }
  .result { margin: 0; padding: var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); }
  .result legend { padding-inline: var(--sp-050); font-size: var(--fs-075); color: var(--text-secondary); }
</style>
