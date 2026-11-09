<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Workflows* under *How we work* (UC-FLW-02, UC-FLW-04, UC-FLW-05) as an editor page
  // (page-system §7.4, §9.1, the mockup's scene): the workflows with a search at the start, each at
  // its own address; the open one's head with *Design · Where it runs · Try a step*, its diagram on a
  // sunken backdrop, and its statuses and steps one line each - a status opening its Dialog M, a
  // step its Dialog L. Below `expanded` the list is the page and a workflow opens alone with
  // *‹ Workflows*.
  //
  // **Edits are a draft** (configuration/ui.md A4): what is in force stays until *Publish*, which
  // shows first what moves and asks where the entries of a removed status go. *Save draft* keeps
  // the draft on the server beside what is in force; *Discard* drops it. The draft bar says so and
  // stays in place while the page scrolls (PS-17).
  //
  // **Shown only to who may shape the workspace** (UC-FLW-02 check 8): the row is drawn with
  // STRUCTURE only, and a reader who arrives without it is told so and offered nothing.

  import { untrack } from 'svelte';
  import {
    Banner, BlockList, Button, Dialog, EditorPage, EmptyState, IconButton, Menu, SearchField, Spinner, ViewSwitcher,
    type Block, type MenuItem, type SectionLink,
  } from '@hubtask/design-system/components';
  import type { Workflow, WorkflowShape } from '@hubtask/sync-engine';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { holdsHere, includedHere, mayHere } from '../lib/data/capability.svelte.ts';
  import { onStep } from '../lib/automation/model.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { rules } from '../lib/data/rules.svelte.ts';
  import {
    SYSTEM_WORKFLOW, changesBetween, moveStatus, moveStatusTo, removeStatus, removeStep, startingDraft, statusLabel, stepLabel, workflows,
  } from '../lib/data/workflows.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import NewWorkflowDialog from '../lib/workflows/NewWorkflowDialog.svelte';
  import PublishDialog from '../lib/workflows/PublishDialog.svelte';
  import SchemePanel from '../lib/workflows/SchemePanel.svelte';
  import StatusDialog from '../lib/workflows/StatusDialog.svelte';
  import StepDialog from '../lib/workflows/StepDialog.svelte';
  import TryStep from '../lib/workflows/TryStep.svelte';
  import WorkflowDiagram from '../lib/workflows/WorkflowDiagram.svelte';
  import { initialOf, statusSummary, statusTone, stepSummary } from '../lib/workflows/summary.ts';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import SectionFindings from '../lib/settings/SectionFindings.svelte';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    /** The workflow at the address (`/administration/workflows/:key`); none for the list. */
    workflow?: string;
    onnavigate: (path: string) => void;
    onreplace?: (path: string) => void;
  }

  const { workflow: asked, onnavigate, onreplace }: Props = $props();

  const BASE = '/administration/workflows';
  const pathOf = (key: string) => `${BASE}/${encodeURIComponent(key)}`;

  $effect(() => page.entitle(t('app.workflow_designer.title')));
  $effect(() => untrack(() => workflows.open()));
  $effect(() => untrack(() => groups.open()));

  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const reading = $derived(workflows.state);
  const readRefusal = $derived(reading?.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const all = $derived(workflows.list?.data ?? []);

  // A step's *then* is offered only to who may write rules where the plan includes them (P-05).
  const mayAutomate = $derived(mayHere('AUTOMATION').status === 'permitted' && includedHere('AUTOMATION_RULES') === true);
  $effect(() => {
    if (!mayAutomate) return;
    return untrack(() => rules.open());
  });
  const rulesOnStep = (step: string) => (chosen ? rules.all.filter((rule) => onStep(rule, chosen.key, step)) : []);
  function addRule(step: string) {
    if (!chosen) return;
    onnavigate(`/administration/rules/new?workflow=${encodeURIComponent(chosen.key)}&step=${encodeURIComponent(step)}`);
  }

  const nameOf = (workflow: Pick<Workflow, 'name'>) => workflow.name ?? t('workflows.default_name');
  const placesOf = (key: string) => (workflows.list?.scheme ?? []).filter((row) => row.workflow === key).length;

  const chosen = $derived(all.find((each) => each.key === asked) ?? all.find((each) => each.key === SYSTEM_WORKFLOW) ?? all[0]);
  /** Below `expanded` the list is the page until a workflow is at the address. */
  const view = $derived<'list' | 'detail'>(asked && chosen?.key === asked ? 'detail' : 'list');

  // The draft being edited: started from the server's draft or what is in force whenever another
  // workflow is opened or the server's copy moves on (a save, a publish, someone else).
  let draft = $state<WorkflowShape | undefined>(undefined);
  let isDirty = $state(false);
  let openedAt = '';
  $effect(() => {
    const workflow = chosen;
    if (!workflow) return;
    const stamp = `${workflow.key}@${workflow.version}`;
    if (stamp === openedAt) return;
    openedAt = stamp;
    untrack(() => {
      draft = startingDraft(workflow);
      isDirty = false;
    });
  });

  let segment = $state('design');

  function edit(next: WorkflowShape) {
    draft = next;
    isDirty = true;
  }

  let busy = $state<string | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);

  async function act(what: string, run: () => Promise<unknown>, done?: string) {
    busy = what;
    refusal = undefined;
    try {
      await run();
      if (done) toast.show({ text: done, tone: 'success' });
      return true;
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
      return false;
    } finally {
      busy = undefined;
    }
  }

  function save() {
    const workflow = chosen;
    const shape = draft;
    if (!workflow || !shape) return;
    void act('save', async () => {
      await workflows.saveDraft(workflow, shape);
      isDirty = false;
    }, t('app.workflow_designer.draft_saved'));
  }

  function discard() {
    const workflow = chosen;
    if (!workflow) return;
    if (!workflow.draft) {
      draft = startingDraft(workflow);
      isDirty = false;
      return;
    }
    void act('discard', () => workflows.discardDraft(workflow), t('app.workflow_designer.draft_discarded'));
  }

  let isPublishing = $state(false);

  /** A draft not yet saved is saved first: the server publishes the draft it holds. */
  async function startPublish() {
    const workflow = chosen;
    const shape = draft;
    if (!workflow || !shape) return;
    if (isDirty || !workflow.draft) {
      const saved = await act('save', async () => {
        await workflows.saveDraft(workflow, shape);
        isDirty = false;
      });
      if (!saved) return;
    }
    isPublishing = true;
  }

  const changes = $derived(chosen && draft ? changesBetween(draft, chosen.published) : 0);
  const hasDraft = $derived(isDirty || Boolean(chosen?.draft));

  // --- A new workflow, and deleting one -------------------------------------------------------

  let isNewOpen = $state(false);
  let isDeleteOpen = $state(false);

  function remove() {
    const workflow = chosen;
    isDeleteOpen = false;
    if (!workflow) return;
    void act('delete', async () => {
      await workflows.remove(workflow.key);
      onreplace?.(BASE);
    }, t('app.workflow_designer.deleted', { name: nameOf(workflow) }));
  }

  // --- The list of workflows ------------------------------------------------------------------

  let search = $state('');
  const sections = $derived<SectionLink[]>(
    all
      .filter((each) => matchesSearch(search, [nameOf(each)]))
      .map((each) => ({
        id: each.key,
        label: nameOf(each),
        href: pathOf(each.key),
        status: each.draft ? t('app.workflow_designer.has_draft') : t('app.workflow_designer.places', { count: String(placesOf(each.key)) }),
        statusTone: each.draft ? 'warning' : 'quiet',
      })),
  );

  // --- Statuses and steps as blocks, each opening its dialog ----------------------------------

  let statusOpen = $state<string | undefined>(undefined);
  let stepOpen = $state<string | undefined>(undefined);

  const groupName = (id: string) => groups.all.find((group) => group.id === id)?.name;
  const statusBlocks = $derived<Block[]>(
    draft ? draft.statuses.map((status) => ({ id: status.key, name: statusLabel(status, t), tone: statusTone(status.category), summary: statusSummary(draft as WorkflowShape, status, t) })) : [],
  );
  const stepBlocks = $derived<Block[]>(
    draft ? draft.steps.map((step) => ({ id: step.key, name: stepLabel(step, draft as WorkflowShape, t), summary: stepSummary(draft as WorkflowShape, step, t, groupName) })) : [],
  );

  function statusMenu(block: Block) {
    if (!draft) return undefined;
    const index = draft.statuses.findIndex((status) => status.key === block.id);
    const status = draft.statuses[index];
    const items: MenuItem[] = [
      { id: 'edit', label: t('app.workflow_designer.edit_menu') },
      ...(status && status.category !== 'DONE' && initialOf(draft) !== status.key ? [{ id: 'initial', label: t('app.workflow_designer.make_initial') }] : []),
      { id: 'up', label: t('app.workflow_designer.move_up_menu'), disabledReason: index === 0 ? t('app.workflow_designer.first') : undefined, hasSeparatorBefore: true },
      { id: 'down', label: t('app.workflow_designer.move_down_menu'), disabledReason: index === draft.statuses.length - 1 ? t('app.workflow_designer.last') : undefined },
      {
        id: 'remove', label: t('app.workflow_designer.remove_menu'), isDestructive: true, hasSeparatorBefore: true,
        disabledReason: draft.statuses.length === 1 ? t('app.workflow_designer.last_status') : undefined,
      },
    ];
    return { label: t('app.page.more_for', { name: block.name }), items };
  }

  function onStatusMenu(block: Block, id: string) {
    if (!draft) return;
    if (id === 'edit') statusOpen = block.id;
    else if (id === 'initial') edit({ ...draft, initial: block.id });
    else if (id === 'up') edit(moveStatus(draft, block.id, -1));
    else if (id === 'down') edit(moveStatus(draft, block.id, 1));
    else if (id === 'remove') edit(removeStatus(draft, block.id));
  }

  function stepMenu(block: Block) {
    const items: MenuItem[] = [
      { id: 'edit', label: t('app.workflow_designer.edit_menu') },
      { id: 'remove', label: t('app.workflow_designer.remove_menu'), isDestructive: true, hasSeparatorBefore: true },
    ];
    return { label: t('app.page.more_for', { name: block.name }), items };
  }

  function onStepMenu(block: Block, id: string) {
    if (!draft) return;
    if (id === 'edit') stepOpen = block.id;
    else if (id === 'remove') edit(removeStep(draft, block.id));
  }

  /** The diagram's choice opens the block's dialog, as pressing the block does. */
  function fromDiagram(selected: string) {
    const [kind, key] = selected.split(':');
    if (kind === 'status') statusOpen = key;
    else if (kind === 'step') stepOpen = key;
  }

  const segments = $derived([
    { id: 'design', label: t('app.workflow_designer.tab_design') },
    { id: 'used', label: t('app.workflow_designer.tab_used') },
    { id: 'try', label: t('app.workflow_designer.tab_try') },
  ]);

  const objectMenu = $derived<MenuItem[]>(
    chosen && !chosen.system ? [{ id: 'delete', label: t('app.workflow_designer.delete_menu'), isDestructive: true }] : [],
  );

  let isFileOpen = $state(false);
  const fileAction = $derived(viewAsFileAction(() => (isFileOpen = true)));
  const head = $derived({
    title: t('app.workflow_designer.title'),
    subtitle: reading?.status === 'ready' ? t('app.workflow_designer.figure', { count: String(all.length) }) : undefined,
    breadcrumb: adminTrail({ id: 'workflows', label: t('app.workflow_designer.title') }),
    primary: mayShape && !readRefusal ? { label: t('app.workflow_designer.new'), icon: 'plus' as const, opener: 'new-workflow', onclick: () => (isNewOpen = true) } : undefined,
    secondary: fileAction ? ([fileAction] as const) : ([] as const),
  });
</script>

<EditorPage
  {head}
  nav={{ label: t('app.workflow_designer.list'), sections, current: chosen?.key, onselect: (key) => onreplace?.(pathOf(key)), search: searchBox }}
  {view}
  back={{ label: t('app.workflow_designer.title'), onback: () => onreplace?.(BASE) }}
  draft={mayShape && chosen && draft && hasDraft ? { label: isDirty ? t('app.workflow_designer.draft_bar_unsaved', { count: String(changes) }) : t('app.workflow_designer.draft_bar', { count: String(changes) }), actions: draftActions } : undefined}
  refused={!mayShape ? notYours : readRefusal ? failed : undefined}
>
  <SectionFindings sections={['workflows']} />
  {#if !reading || reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet"><Spinner label={t('app.workflow_designer.reading')} /><span>{t('app.workflow_designer.reading')}</span></p>
  {:else if chosen && draft}
    <header class="object-head">
      <div class="object-title">
        <h2 class="name" data-workflow={chosen.key}>{nameOf(chosen)}</h2>
        <p class="facts">{t('app.workflow_designer.used_facts', { count: String(placesOf(chosen.key)) })}</p>
      </div>
      <div class="object-acts">
        <ViewSwitcher label={t('app.workflow_designer.tabs')} views={segments} bind:selected={segment} />
        {#if objectMenu.length > 0}
          <Menu label={t('app.page.more_for', { name: nameOf(chosen) })} items={objectMenu} placement={{ side: 'block-end', align: 'end' }} onselect={() => (isDeleteOpen = true)}>
            {#snippet trigger(props)}
              <IconButton icon="ellipsis" size="sm" label={t('app.page.more_for', { name: nameOf(chosen) })} {...props} data-workflow-menu />
            {/snippet}
          </Menu>
        {/if}
      </div>
    </header>
    {#if refusal}<FailureNote problem={refusal} />{/if}

    {#if segment === 'design'}
      <div class="flow"><WorkflowDiagram shape={draft} onselect={fromDiagram} /></div>
      <BlockList
        title={t('app.workflow_designer.statuses')}
        count={String(draft.statuses.length)}
        blocks={statusBlocks}
        onopen={(id) => (statusOpen = id)}
        menu={statusMenu}
        onmenu={onStatusMenu}
        onmove={(id, to) => { if (draft) edit(moveStatusTo(draft, id, to)); }}
        gripLabel={(block) => t('app.workflow_designer.move', { name: block.name })}
        add={{ label: t('app.workflow_designer.add_status'), onclick: () => (statusOpen = 'new') }}
      />
      <BlockList
        title={t('app.workflow_designer.steps')}
        count={String(draft.steps.length)}
        blocks={stepBlocks}
        onopen={(id) => (stepOpen = id)}
        menu={stepMenu}
        onmenu={onStepMenu}
        add={{ label: t('app.workflow_designer.add_step'), onclick: () => (stepOpen = 'new') }}
      />
      {#if draft.steps.length === 0}<p class="quiet">{t('app.workflow_designer.no_steps')}</p>{/if}
    {:else if segment === 'used'}
      <SchemePanel workflow={chosen} />
    {:else}
      <TryStep workflow={chosen} shape={(chosen.draft as WorkflowShape | null) ?? chosen.published} isDraft={Boolean(chosen.draft)} />
    {/if}
  {:else}
    <EmptyState kind="unused" title={t('app.workflow_designer.none')} />
  {/if}
</EditorPage>

{#snippet searchBox()}
  <SearchField label={t('app.workflow_designer.search')} isLabelHidden placeholder={t('app.workflow_designer.search')} clearLabel={t('app.page.clear_search')} size="sm" bind:value={search} />
{/snippet}

{#snippet draftActions()}
  <Button tone="subtle" size="sm" data-discard-draft isBusy={busy === 'discard'} onclick={discard}>{t('app.workflow_designer.discard')}</Button>
  <Button tone="secondary" size="sm" data-save-draft isBusy={busy === 'save'}
    disabledReason={isDirty ? undefined : t('app.workflow_designer.nothing_to_save')} onclick={save}>{t('app.workflow_designer.save_draft')}</Button>
  <Button tone="secondary" size="sm" onclick={() => (segment = 'try')}>{t('app.workflow_designer.tab_try')}</Button>
  <Button tone="primary" size="sm" data-publish onclick={startPublish}>{t('app.workflow_designer.publish_open')}</Button>
{/snippet}

{#snippet notYours()}<Banner tone="info" title={t('app.workflow_designer.not_yours')} />{/snippet}
{#snippet failed()}{#if readRefusal}<FailureNote problem={readRefusal} />{/if}{/snippet}

{#if mayShape && draft}
  <StatusDialog shape={draft} subject={statusOpen} onchange={edit} onClose={() => (statusOpen = undefined)} />
  <StepDialog
    shape={draft}
    subject={stepOpen}
    stepRules={mayAutomate ? rulesOnStep : undefined}
    onopenrule={(id) => onnavigate(`/administration/rules/${id}`)}
    onaddrule={addRule}
    onchange={edit}
    onClose={() => (stepOpen = undefined)}
  />
{/if}

{#if isPublishing && chosen && draft}
  <PublishDialog
    workflow={chosen}
    {draft}
    onclose={() => (isPublishing = false)}
    onpublished={(published) => {
      isPublishing = false;
      toast.show({ text: t('app.workflow_designer.published', { name: nameOf(published), count: published.entries_moved ?? 0 }), tone: 'success' });
    }}
  />
{/if}

{#if mayShape}
  <NewWorkflowDialog isOpen={isNewOpen} {all} onClose={() => (isNewOpen = false)} oncreated={(key) => onnavigate(pathOf(key))} />
{/if}

{#if chosen}
  <Dialog
    size="s"
    isOpen={isDeleteOpen}
    title={t('app.workflow_designer.delete_title', { name: nameOf(chosen) })}
    dismissLabel={t('app.workflow_designer.cancel')}
    onClose={() => (isDeleteOpen = false)}
    primary={{ label: t('app.workflow_designer.delete_confirm'), tone: 'danger', onclick: remove }}
  >
    <p class="quiet">{t('app.workflow_designer.delete_explain')}</p>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => (isDeleteOpen = false)}>{t('app.workflow_designer.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

<ViewSectionsAsFile sections={['workflows']} bind:isOpen={isFileOpen} isTriggerHidden />

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .object-head { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: var(--sp-150) var(--sp-300); }
  .object-title { display: flex; flex-direction: column; gap: var(--sp-050); min-inline-size: 0; }
  .name { margin: 0; font-family: var(--font-display); font-size: var(--fs-500); line-height: var(--lh-tight); }
  .facts { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .object-acts { display: flex; align-items: center; gap: var(--sp-100); }

  /* The diagram on a sunken backdrop inside the canvas (page-system §4.2). */
  .flow { padding: var(--sp-200); border-radius: var(--r-lg); background: var(--bg-surface-sunken); overflow-x: auto; }
</style>
