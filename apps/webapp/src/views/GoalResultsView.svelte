<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A goal's key results, changed together (UC-GOL-01 check 9) at `/goals/:id/results`: up to
  // twenty repeated groups are past a Dialog L, so they are a page of their own, each result a
  // titled section (page-system §5.1, §7.7). Saving writes them all at once and returns to the goal.
  // A graded goal changes no more; the server says so (`goals.graded`).
  import { untrack } from 'svelte';

  import {
    Button, CreatePage, FormLayout, FormSection, Inline, Input, Select, Skeleton, Stack,
  } from '@hubtask/design-system/components';
  import type { Goal, KeyResultWrite, SavedView } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, holdsHere, roleOrder } from '../lib/data/capability.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { goalPath, goals, goalWrite, hubOf } from '../lib/data/goals.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { views } from '../lib/data/views.svelte.ts';
  import EntryPicker from '../lib/entries/EntryPicker.svelte';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { isObjectId, renderProblem } from '../lib/problem.ts';

  interface Props {
    goalId: string;
    onnavigate: (path: string) => void;
  }

  const { goalId, onnavigate }: Props = $props();

  type ResultDraft = {
    id?: string; title: string; kind: KeyResultWrite['kind']; source: 'MANUAL' | 'VIEW';
    start: string; target: string; current: string; unit: string; viewId: string; entry?: { id: string; title: string };
  };
  const KINDS = ['NUMBER', 'PERCENT', 'BINARY', 'MILESTONE'] as const;
  const AT_MOST = 20;

  let goal = $state<Goal | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let results = $state<ResultDraft[] | undefined>(undefined);
  $effect(() => {
    if (!isObjectId(goalId)) return;
    const path = goalPath(goalId);
    return untrack(() => engine.subscribe<Goal>({ path }, (next) => {
      if (next.status === 'ready') {
        goal = next.data;
        refusal = undefined;
        // The draft is taken once: a later refresh of the goal does not undo what is being typed.
        results ??= next.data.key_results.map((each) => ({
          id: each.id, title: each.title, kind: each.kind, source: each.source,
          start: String(each.start), target: String(each.target), current: String(each.current), unit: each.unit,
          viewId: each.view_id ?? '', ...(each.entry_id ? { entry: { id: each.entry_id, title: t('app.goals.milestone_entry') } } : {}),
        }));
      }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });
  $effect(() => frame.entitle(t('app.goals.edit_results')));
  $effect(() => untrack(() => views.open()));
  const summaryViews = $derived<SavedView[]>(views.of().filter((each) => each.query && 'aggregate' in each.query));

  // Who may write: WRITE_ITEMS at the goal's place - the hub's role, or the workspace's.
  $effect(() => {
    const hub = hubOf(goal);
    if (hub) return untrack(() => people.open({ hubId: hub }));
  });
  const mayWrite = $derived.by(() => {
    if (!goal || goal.grade !== null) return false;
    const hub = hubOf(goal);
    if (hub) return holds(people.roleOf({ hubId: hub }, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted';
    return holdsHere('WRITE_ITEMS') === true;
  });

  function addResult() {
    results = [...(results ?? []), { title: '', kind: 'NUMBER', source: 'MANUAL', start: '0', target: '100', current: '0', unit: '', viewId: '' }];
  }
  const numberOf = (text: string): number | undefined => {
    const value = Number(text.trim().replace(',', '.'));
    return text.trim() === '' || !Number.isFinite(value) ? undefined : value;
  };
  const resultsProblem = $derived.by(() => {
    for (const each of results ?? []) {
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

  const back = () => onnavigate(`/goals/${goalId}`);
  let failure = $state<string | undefined>(undefined);
  let isSaving = $state(false);
  async function save() {
    const at = goal;
    if (!at || !results) return;
    failure = undefined;
    isSaving = true;
    try {
      await goals.update(at, goalWrite(at, { key_results: results.map(resultWrite) }));
      toast.show({ text: t('app.goals.saved'), tone: 'success' });
      back();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }
</script>

<CreatePage
  head={{
    title: t('app.goals.edit_results'),
    subtitle: goal?.title,
    breadcrumb: goal
      ? {
          trail: [
            { id: 'goals', label: t('app.goals.title'), href: `/goals?period=${goal.period_id}` },
            { id: 'goal', label: goal.title, href: `/goals/${goalId}` },
            { id: 'results', label: t('app.goals.edit_results') },
          ],
          label: t('app.goals.trail'),
          expandLabel: t('app.goals.expand_trail'),
          onnavigate: (id) => (id === 'goal' ? back() : onnavigate(`/goals?period=${goal?.period_id ?? ''}`)),
        }
      : undefined,
    primary: mayWrite ? { label: t('app.goals.save'), disabledReason: isSaving ? t('app.workspace.saving') : resultsProblem, onclick: () => void save() } : undefined,
    secondary: [{ label: t('app.links.cancel'), isQuiet: true, onclick: back }],
  }}
  close={{ label: t('app.links.cancel'), onclose: back }}
  refused={refusal ? refusedNote : undefined}
>
  {#if !results}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    <FormLayout place="page">
      {#each results as result, i (result.id ?? `new-${i}`)}
        <FormSection title={t('app.goals.kr_number', { n: String(i + 1) })}>
          <div class="result" data-key-result>
            <Stack gap="150">
              <div class="lg"><Input label={t('app.goals.kr_title')} bind:value={result.title} maxlength={200} /></div>
              <div class="sm"><Select label={t('app.goals.kr_kind')} bind:value={result.kind} options={KINDS.map((each) => ({ value: each, label: t(`app.goals.kind_${each}`) }))} /></div>
              {#if result.kind === 'NUMBER' || result.kind === 'PERCENT'}
                <div class="sm"><Select label={t('app.goals.kr_source')} bind:value={result.source}
                  options={[{ value: 'MANUAL', label: t('app.goals.source_MANUAL') }, { value: 'VIEW', label: t('app.goals.source_VIEW') }]} /></div>
              {/if}
              {#if result.kind === 'NUMBER'}
                <Inline gap="100" isWrapping>
                  <div class="sm"><Input label={t('app.goals.kr_start')} bind:value={result.start} inputmode="decimal" /></div>
                  <div class="sm"><Input label={t('app.goals.kr_target')} bind:value={result.target} inputmode="decimal" /></div>
                  <div class="xs"><Input label={t('app.goals.kr_unit')} bind:value={result.unit} maxlength={16} /></div>
                </Inline>
              {/if}
              {#if result.source === 'VIEW' && (result.kind === 'NUMBER' || result.kind === 'PERCENT')}
                <div class="md"><Select label={t('app.goals.kr_view')} hint={t('app.goals.kr_view_hint')} bind:value={result.viewId} placeholder={t('app.goals.kr_view_pick')}
                  options={summaryViews.map((each) => ({ value: each.id, label: each.name }))} /></div>
              {:else if result.kind === 'MILESTONE'}
                {#if result.entry}<p class="quiet">{t('app.goals.kr_waits_for', { title: result.entry.title })}</p>{/if}
                <EntryPicker onchoose={(item) => { if (item) result.entry = { id: item.id, title: item.title }; }} />
              {:else if result.kind === 'BINARY'}
                <div class="sm"><Select label={t('app.goals.kr_current')} bind:value={result.current}
                  options={[{ value: '0', label: t('app.goals.kr_not_reached') }, { value: '1', label: t('app.goals.kr_reached') }]} /></div>
              {:else if result.source === 'MANUAL'}
                <div class="sm"><Input label={t('app.goals.kr_current')} bind:value={result.current} inputmode="decimal" /></div>
              {/if}
              <Inline justify="end">
                <Button size="sm" tone="subtle" icon="trash" onclick={() => (results = results?.filter((_, j) => j !== i))}>{t('app.goals.kr_remove')}</Button>
              </Inline>
            </Stack>
          </div>
        </FormSection>
      {/each}
    </FormLayout>
    {#if failure}<FailureNote problem={failure} />{/if}
    <div>
      <Button tone="secondary" icon="plus" disabledReason={results.length >= AT_MOST ? t('app.goals.kr_at_most') : undefined} onclick={addResult}>{t('app.goals.kr_add')}</Button>
    </div>
  {/if}
</CreatePage>

{#snippet refusedNote()}<FailureNote problem={refusal ?? ''} />{/snippet}

<style>
  .result { padding: var(--sp-150) var(--sp-200); }
  /* Each field as wide as what it holds (PS-13), never wider than the page. */
  .xs, .sm, .md, .lg { max-inline-size: 100%; }
  .xs { inline-size: var(--layout-field-xs); }
  .sm { inline-size: var(--layout-field-sm); }
  .md { inline-size: var(--layout-field-md); }
  .lg { inline-size: var(--layout-field-lg); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
