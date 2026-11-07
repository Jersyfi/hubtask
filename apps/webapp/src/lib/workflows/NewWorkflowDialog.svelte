<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *New workflow* as a Dialog M (page-system §9.1, the mockup's dialog): its name and which
  // workflow it starts as a copy of - the standard one unless another is chosen. The new workflow
  // opens in the editor at its own address.
  import { Button, Dialog, Input, Select, Stack } from '@hubtask/design-system/components';
  import type { Workflow, WorkflowShape } from '@hubtask/sync-engine';

  import { SYSTEM_WORKFLOW, keyOf, workflows } from '../data/workflows.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    all: readonly Workflow[];
    onClose: () => void;
    oncreated: (key: string) => void;
  }

  const { isOpen, all, onClose, oncreated }: Props = $props();

  let name = $state('');
  let from = $state('');
  let isMissing = $state(false);
  let isWriting = $state(false);
  let refusal = $state<string | undefined>(undefined);

  const nameOf = (workflow: Pick<Workflow, 'name'>) => workflow.name ?? t('workflows.default_name');
  const standard = $derived(all.some((each) => each.key === SYSTEM_WORKFLOW) ? SYSTEM_WORKFLOW : (all[0]?.key ?? ''));
  const options = $derived(all.map((each) => ({ value: each.key, label: t('app.workflow_designer.copy_of', { name: nameOf(each) }) })));

  function close() {
    name = '';
    from = '';
    isMissing = false;
    refusal = undefined;
    onClose();
  }
  const guard = new DiscardGuard(() => name.trim() !== '', close);

  async function create() {
    const trimmed = name.trim();
    const base = all.find((each) => each.key === (from || standard));
    if (!trimmed) {
      isMissing = true;
      return;
    }
    if (!base) return;
    const key = keyOf(trimmed, all.map((each) => each.key));
    isWriting = true;
    refusal = undefined;
    try {
      await workflows.create(key, trimmed, JSON.parse(JSON.stringify(base.published)) as WorkflowShape);
      toast.show({ text: t('app.workflow_designer.created', { name: trimmed }), tone: 'success' });
      close();
      oncreated(key);
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isWriting = false;
    }
  }
</script>

<Dialog
  size="m"
  {isOpen}
  title={t('app.workflow_designer.new')}
  dismissLabel={t('app.workflow_designer.cancel')}
  onDismiss={guard.dismiss}
  onClose={close}
  primary={{ label: t('app.workflow_designer.create_workflow'), isBusy: isWriting, onclick: () => void create() }}
>
  <Stack gap="200">
    {#if refusal}<FailureNote problem={refusal} />{/if}
    <Input label={t('app.workflow_designer.workflow_name')} bind:value={name} isRequired data-new-workflow-name error={isMissing && !name.trim() ? t('app.forms.required') : undefined} />
    <Select label={t('app.workflow_designer.start_from')} options={options} value={from || standard} onchange={(event: Event) => (from = (event.currentTarget as HTMLSelectElement).value)} />
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) close(); }}>{t('app.workflow_designer.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />
