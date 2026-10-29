<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Workflows* under *How we work* (UC-FLW-02, UC-FLW-04, UC-FLW-05): the workspace's workflows,
  // one open in the designer - the diagram and, beside it, the same as lists a keyboard reaches.
  //
  // **Edits are a draft** (configuration/ui.md A4): what is in force stays until *Publish*, which
  // shows first what moves and asks where the entries of a removed status go. *Save draft* keeps
  // the draft on the server beside what is in force; *Discard* drops it.
  //
  // **Shown only to who may shape the workspace** (UC-FLW-02 check 8): the row is drawn with
  // STRUCTURE only, and a reader who arrives without it is told so and offered nothing.

  import { untrack } from 'svelte';
  import { Banner, Button, Dialog, IconButton, Inline, Input, PageHeader, Spinner, Stack, Tabs } from '@hubtask/design-system/components';
  import type { Workflow, WorkflowShape } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { SYSTEM_WORKFLOW, keyOf, startingDraft, workflows } from '../lib/data/workflows.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import PublishDialog from '../lib/workflows/PublishDialog.svelte';
  import SchemePanel from '../lib/workflows/SchemePanel.svelte';
  import TryStep from '../lib/workflows/TryStep.svelte';
  import WorkflowDiagram from '../lib/workflows/WorkflowDiagram.svelte';
  import WorkflowLists from '../lib/workflows/WorkflowLists.svelte';

  $effect(() => page.entitle(t('app.workflow_designer.title')));
  $effect(() => untrack(() => workflows.open()));

  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const reading = $derived(workflows.state);
  const readRefusal = $derived(reading?.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const all = $derived(workflows.list?.data ?? []);

  const nameOf = (workflow: Pick<Workflow, 'name'>) => workflow.name ?? t('workflows.default_name');

  let chosenKey = $state<string | undefined>(undefined);
  const chosen = $derived(all.find((each) => each.key === chosenKey) ?? all.find((each) => each.key === SYSTEM_WORKFLOW) ?? all[0]);

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
      selected = undefined;
    });
  });

  let selected = $state<string | undefined>(undefined);
  let tab = $state('design');

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

  // --- A new workflow, and deleting one -------------------------------------------------------

  let isNewOpen = $state(false);
  let newName = $state('');

  function create() {
    const name = newName.trim();
    const base = all.find((each) => each.key === SYSTEM_WORKFLOW) ?? chosen;
    if (!name || !base) return;
    const key = keyOf(name, all.map((each) => each.key));
    void act('create', async () => {
      await workflows.create(key, name, structuredClone(base.published));
      chosenKey = key;
      isNewOpen = false;
      newName = '';
    }, t('app.workflow_designer.created', { name }));
  }

  let isDeleteOpen = $state(false);

  function remove() {
    const workflow = chosen;
    isDeleteOpen = false;
    if (!workflow) return;
    void act('delete', async () => {
      await workflows.remove(workflow.key);
      chosenKey = undefined;
    }, t('app.workflow_designer.deleted', { name: nameOf(workflow) }));
  }

  const tabs = $derived([
    { id: 'design', label: t('app.workflow_designer.tab_design') },
    { id: 'used', label: t('app.workflow_designer.tab_used') },
    { id: 'try', label: t('app.workflow_designer.tab_try') },
  ]);
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.workflow_designer.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'workflows', label: t('app.workflow_designer.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  <p class="quiet">{t('app.workflow_designer.intro')}</p>

  {#if !mayShape}
    <Banner tone="info" title={t('app.workflow_designer.not_yours')} />
  {:else if !reading || reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting"><Spinner label={t('app.workflow_designer.reading')} /><span>{t('app.workflow_designer.reading')}</span></p>
  {:else if readRefusal}
    <Banner tone="danger" title={readRefusal.message}>
      {#if readRefusal.reference}{t('app.error_reference', { request_id: readRefusal.reference })}{/if}
    </Banner>
  {:else}
    <div class="layout">
      <nav class="chooser" aria-label={t('app.workflow_designer.list')}>
        <ul>
          {#each all as workflow (workflow.key)}
            <li>
              <button
                type="button" class="choice" class:current={workflow.key === chosen?.key}
                aria-current={workflow.key === chosen?.key ? 'true' : undefined}
                data-workflow={workflow.key}
                onclick={() => (chosenKey = workflow.key)}
              >
                <span>{nameOf(workflow)}</span>
                {#if workflow.draft}<span class="note">{t('app.workflow_designer.has_draft')}</span>{/if}
              </button>
            </li>
          {/each}
        </ul>
        <Button tone="secondary" size="sm" icon="plus" data-new-workflow onclick={() => (isNewOpen = true)}>
          {t('app.workflow_designer.new')}
        </Button>
      </nav>

      {#if chosen && draft}
        <section class="designer" aria-label={nameOf(chosen)}>
          <Stack gap="200">
            <Inline gap="100" align="center" justify="between">
              <h2 class="name">{nameOf(chosen)}</h2>
              <Inline gap="100" align="center">
                {#if isDirty || chosen.draft}<span class="note" data-draft-state>{isDirty ? t('app.workflow_designer.unsaved') : t('app.workflow_designer.draft_kept')}</span>{/if}
                <Button tone="secondary" size="sm" data-save-draft isBusy={busy === 'save'}
                  disabledReason={isDirty ? undefined : t('app.workflow_designer.nothing_to_save')} onclick={save}>
                  {t('app.workflow_designer.save_draft')}
                </Button>
                <Button tone="subtle" size="sm" data-discard-draft isBusy={busy === 'discard'}
                  disabledReason={isDirty || chosen.draft ? undefined : t('app.workflow_designer.nothing_to_discard')} onclick={discard}>
                  {t('app.workflow_designer.discard')}
                </Button>
                <Button tone="primary" size="sm" data-publish
                  disabledReason={isDirty || chosen.draft ? undefined : t('app.workflow_designer.nothing_to_publish')} onclick={startPublish}>
                  {t('app.workflow_designer.publish_open')}
                </Button>
                {#if !chosen.system}
                  <IconButton icon="trash" size="sm" tone="subtle" label={t('app.workflow_designer.delete', { name: nameOf(chosen) })}
                    data-delete-workflow onclick={() => (isDeleteOpen = true)} />
                {/if}
              </Inline>
            </Inline>
            {#if refusal}<Banner tone="danger" title={refusal} />{/if}
            <Tabs {tabs} selected={tab} onselect={(id) => (tab = id)} label={t('app.workflow_designer.tabs')}>
            {#if tab === 'design'}
              <WorkflowDiagram shape={draft} {selected} onselect={(next) => (selected = next)} />
              <WorkflowLists shape={draft} {selected} onchange={edit} onselect={(next) => (selected = next)} />
            {:else if tab === 'used'}
              <SchemePanel workflow={chosen} />
            {:else}
              <TryStep workflow={chosen} shape={(chosen.draft as WorkflowShape | null) ?? chosen.published} isDraft={Boolean(chosen.draft)} />
            {/if}
            </Tabs>
          </Stack>
        </section>
      {/if}
    </div>
  {/if}
</Stack>

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

<Dialog bind:isOpen={isNewOpen} title={t('app.workflow_designer.new_title')} dismissLabel={t('app.workflow_designer.cancel')}>
  <Stack gap="150">
    <Input label={t('app.workflow_designer.workflow_name')} bind:value={newName} isRequired data-new-workflow-name />
    <p class="quiet">{t('app.workflow_designer.new_hint')}</p>
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isNewOpen = false)}>{t('app.workflow_designer.cancel')}</Button>
    <Button tone="primary" data-create-workflow isBusy={busy === 'create'} onclick={create}>{t('app.workflow_designer.create')}</Button>
  {/snippet}
</Dialog>

{#if chosen}
  <Dialog bind:isOpen={isDeleteOpen} title={t('app.workflow_designer.delete_title', { name: nameOf(chosen) })} dismissLabel={t('app.workflow_designer.cancel')}>
    <p class="quiet">{t('app.workflow_designer.delete_explain')}</p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isDeleteOpen = false)}>{t('app.workflow_designer.cancel')}</Button>
      <Button tone="danger" data-confirm-delete onclick={remove}>{t('app.workflow_designer.delete_confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .layout { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-300); }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) {
    .layout { grid-template-columns: minmax(0, max-content) minmax(0, 1fr); }
  }

  .chooser { display: grid; align-content: start; gap: var(--sp-150); }
  .chooser ul { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }

  .choice {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    inline-size: 100%;
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-md);
    background: none;
    font: inherit;
    color: inherit;
    text-align: start;
    cursor: pointer;
  }

  .choice.current { border-color: var(--border-subtle); background: var(--bg-surface); font-weight: var(--fw-medium); }
  .choice:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }

  .note { color: var(--text-subtle); font-size: var(--fs-075); font-weight: normal; }
  .name { margin: 0; font-size: var(--fs-200); }
  .designer { min-inline-size: 0; }
</style>
