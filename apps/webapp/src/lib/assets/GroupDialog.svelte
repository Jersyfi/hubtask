<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A saved group defined or changed (UC-AST-11 check 5): its key, name, filter and the service it
  // stands for - a Dialog L of the page system's ladder. Opened from *Save as group*, prefilled from
  // the line, or from a saved group's ⋯. What the line held that the dialog does not edit (owners,
  // locations, numbers) is carried and said; what a group cannot hold is said with its reason.
  //
  // **A group is a filter, never a list of objects** (UC-AST-11, *Where it ends*).
  import { untrack } from 'svelte';
  import { Button, CheckboxGroup, Dialog, Input, Select, Stack, Textarea, type SelectOption } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { CiClass, CiGroup, CiGroupFilter } from '@hubtask/sync-engine';

  import { CI_STAGES, classWord, cmdb, groupDefine, groupDraft, groupMissing, instantiable, type GroupDraft, type Words } from '../data/cmdb.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    /** The hub's services a group may stand for. */
    services: readonly SelectOption[];
    /** The group changed; absent for a new one. */
    editing?: CiGroup;
    /** A new group's filter, from the line. */
    prefill?: CiGroupFilter;
    /** What the filter holds beyond the dialog's fields, in words: *Owner: nobody*. */
    keptText?: string;
    /** Why part of the line could not become the group's filter. */
    note?: string;
    onsaved?: (group: CiGroup) => void;
  }

  let { isOpen = $bindable(false), hubId, classes, words, services, editing, prefill, keptText, note, onsaved }: Props = $props();

  let draft = $state<GroupDraft>(groupDraft());
  let isSaving = $state(false);
  let saveFailure = $state<string | undefined>(undefined);
  $effect(() => {
    if (!isOpen) return;
    untrack(() => {
      const from = editing ?? (prefill ? ({ filter: prefill } as CiGroup) : undefined);
      draft = { ...groupDraft(from), key: editing?.key ?? '', name: editing?.name ?? '' };
      saveFailure = undefined;
    });
  });

  const missing = $derived(groupMissing(draft, !editing));
  const classChecks = $derived(instantiable(classes).map((each) => ({ value: each.key, label: classWord(each.key, classes, words) })));
  const stageChecks = $derived(CI_STAGES.map((each) => ({ value: each, label: t(`app.assets.stage.${each}`) })));

  async function save() {
    isSaving = true;
    try {
      const saved = await cmdb.defineGroup(hubId, editing?.key ?? draft.key, groupDefine(draft), editing?.version);
      toast.show({ text: t('app.ci_groups.saved'), tone: 'success' });
      isOpen = false;
      onsaved?.(saved);
    } catch (error) {
      // Said inside the dialog, which keeps what was typed (user-experience.md §5.10).
      saveFailure = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }
</script>

{#if isOpen}
  <Dialog size="l" bind:isOpen title={editing ? t('app.cmdb_admin.edit_named', { name: editing.name }) : t('app.ci_groups.new')} dismissLabel={t('app.assets.cancel')} primary={{ label: t('app.assets.save'), isBusy: isSaving, disabledReason: missing[0] ? t(missing[0]) : undefined, onclick: () => void save() }}>
    <Stack gap="150">
      {#if note}<p class="note" data-group-note>{note}</p>{/if}
      {#if !editing}<Input label={t('app.cmdb_admin.key')} hint={t('app.cmdb_admin.key_hint')} bind:value={draft.key} maxlength={63} isRequired />{/if}
      <Input label={t('app.ci_groups.name')} bind:value={draft.name} maxlength={200} isRequired />
      <CheckboxGroup label={t('app.ci_groups.classes')} hint={t('app.ci_groups.classes_hint')} options={classChecks} selected={draft.classes} onChange={(next) => (draft.classes = [...next])} />
      <CheckboxGroup label={t('app.ci_groups.stages')} options={stageChecks} selected={draft.stages} onChange={(next) => (draft.stages = [...next] as GroupDraft['stages'])} />
      <Input label={t('app.ci_groups.name_has')} bind:value={draft.nameHas} maxlength={200} />
      <Textarea label={t('app.ci_groups.conditions')} hint={t('app.ci_groups.conditions_hint')} rows={3} bind:value={draft.conditions} />
      {#if keptText}<p class="meta" data-group-kept>{t('app.inventory.group_kept', { conditions: keptText })}</p>{/if}
      <Select label={t('app.ci_groups.service')} options={[{ value: '', label: t('app.ci_groups.no_service') }, ...services]} bind:value={draft.service} />
      {#if saveFailure}<FailureNote problem={saveFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.assets.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .meta { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
  .note { margin: 0; color: var(--text-secondary); font-size: var(--fs-090); }
</style>
