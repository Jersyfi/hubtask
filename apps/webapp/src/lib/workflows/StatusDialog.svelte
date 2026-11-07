<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A status of a workflow as a Dialog M (page-system §5.1, the mockup's *Status “In progress”*):
  // its name, category and colour, and whether new entries start there. *Add status* opens the same
  // dialog empty. Nothing is sent: *Done* changes the draft, which the page saves and publishes.
  import { labelTokens } from '@hubtask/design-system';
  import { Button, Checkbox, Dialog, Inline, Input, Select, Stack } from '@hubtask/design-system/components';
  import type { WorkflowShape } from '@hubtask/sync-engine';

  import { CATEGORIES, addStatus, changeStatus, statusLabel, type Category } from '../data/workflows.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { initialOf } from './summary.ts';

  interface Props {
    shape: WorkflowShape;
    /** The status's key, `'new'` for *Add status*; closed while undefined. */
    subject: string | undefined;
    onchange: (shape: WorkflowShape) => void;
    onClose: () => void;
  }

  const { shape, subject, onchange, onClose }: Props = $props();

  const status = $derived(subject && subject !== 'new' ? shape.statuses.find((each) => each.key === subject) : undefined);

  let name = $state('');
  let category = $state<string>('IN_PROGRESS');
  let colour = $state<string>(labelTokens[0]);
  let isInitial = $state(false);
  let isNameMissing = $state(false);
  let opened: string | undefined;
  $effect(() => {
    if (subject === opened) return;
    opened = subject;
    const each = subject && subject !== 'new' ? shape.statuses.find((one) => one.key === subject) : undefined;
    name = each?.name ?? '';
    category = each?.category ?? 'IN_PROGRESS';
    colour = each?.color_token ?? labelTokens[0];
    isInitial = each !== undefined && initialOf(shape) === each.key;
    isNameMissing = false;
  });

  const isChanged = () =>
    status
      ? (status.name ?? '') !== name || status.category !== category || status.color_token !== colour || (initialOf(shape) === status.key) !== isInitial
      : name.trim() !== '';
  const guard = new DiscardGuard(isChanged, () => onClose());

  function done() {
    let next = shape;
    let key = status?.key;
    if (!status) {
      if (name.trim() === '') {
        isNameMissing = true;
        return;
      }
      next = addStatus(next, name.trim(), category as Category);
      key = next.statuses[next.statuses.length - 1]?.key;
    }
    if (!key) return;
    next = changeStatus(next, key, { name: name.trim() || null, category: category as Category, color_token: colour });
    if (isInitial && category !== 'DONE') next = { ...next, initial: key };
    onchange(next);
    onClose();
  }

  const categoryOptions = $derived(CATEGORIES.map((each) => ({ value: each, label: t(`workflows.category.${each.toLowerCase()}`) })));
  const colourOptions = $derived(labelTokens.map((token) => ({ value: token, label: t(`app.colour.${token}`) })));
  const isInitialHere = $derived(status !== undefined && initialOf(shape) === status.key);
</script>

<Dialog
  size="m"
  isOpen={subject !== undefined}
  title={status ? t('app.workflow_designer.status_title', { name: statusLabel(status, t) }) : t('app.workflow_designer.add_status')}
  dismissLabel={t('app.workflow_designer.cancel')}
  onDismiss={guard.dismiss}
  {onClose}
  primary={{ label: t('app.workflow_designer.done'), onclick: done }}
>
  <Stack gap="200">
    <Input
      label={t('app.workflow_designer.status_name')}
      bind:value={name}
      isRequired={!status}
      placeholder={status ? statusLabel(status, t) : undefined}
      error={isNameMissing ? t('app.forms.required') : undefined}
      data-status-name
    />
    <Inline gap="200" align="start">
      <Select label={t('app.workflow_designer.category')} options={categoryOptions} bind:value={category} />
      <Select label={t('app.workflow_designer.colour')} options={colourOptions} bind:value={colour} />
    </Inline>
    <Checkbox
      label={t('app.workflow_designer.is_initial')}
      bind:checked={isInitial}
      disabledReason={category === 'DONE' ? t('app.workflow_designer.initial_not_done') : isInitialHere ? t('app.workflow_designer.initial_moves') : undefined}
    />
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) onClose(); }}>{t('app.workflow_designer.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />
