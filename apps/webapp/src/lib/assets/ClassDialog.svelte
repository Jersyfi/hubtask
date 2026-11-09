<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A class's editor, a Dialog L (page-system §5.1): *New class* on the classes' list and *Change*
  // on a class's record page draw the same one. Only what the class itself declares is edited, not
  // what it inherits; its refusal is said inside it, which keeps what was typed
  // (user-experience.md §5.10).
  import { Button, Checkbox, CheckboxGroup, Dialog, IconButton, Input, Select, Stack, Switch, Textarea, type SelectOption } from '@hubtask/design-system/components';
  import { untrack } from 'svelte';
  import type { CiAttribute, CiClass, CiRelationType } from '@hubtask/sync-engine';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { classDefine, classDraft, classWord, cmdb, keyIsWellFormed, relationWord, type ClassDraft, type Words } from '../data/cmdb.svelte.ts';

  interface Props {
    isOpen: boolean;
    /** The class changed; none for a new one. */
    editing?: CiClass;
    /** Every class, in reading order, for *Beneath*. */
    classes: readonly CiClass[];
    types: readonly CiRelationType[];
  }
  let { isOpen = $bindable(), editing, classes, types }: Props = $props();
  const words: Words = (code) => (messages.has(code) ? t(code) : undefined);
  const nameOf = (key: string) => classWord(key, classes, words);

  let classDraftState = $state<ClassDraft | undefined>(undefined);
  let dialogFailure = $state<string | undefined>(undefined);
  // A fresh draft each time it opens, of the class as it is then; a read arriving while it is
  // open does not overwrite what is being typed.
  $effect.pre(() => {
    if (!isOpen) return;
    untrack(() => {
      classDraftState = classDraft(editing);
      dialogFailure = undefined;
    });
  });

  const classOptions = $derived<SelectOption[]>(classes.map((each) => ({ value: each.key, label: nameOf(each.key) })));
  const kinds: readonly CiAttribute['kind'][] = ['TEXT', 'NUMBER', 'BOOLEAN', 'DATE', 'CHOICE', 'URL', 'GROUP', 'ACCOUNT'];
  const kindOptions = $derived<SelectOption[]>(kinds.map((each) => ({ value: each, label: t(`app.cmdb_admin.kind.${each}`) })));
  const needOptions = $derived<SelectOption[]>((['OPTIONAL', 'RECOMMENDED', 'REQUIRED'] as const).map((each) => ({ value: each, label: t(`app.cmdb_admin.need.${each}`) })));
  const typeKeys = $derived<SelectOption[]>(types.map((each) => ({ value: each.key, label: relationWord(each, each.key, true, words) })));

  const classReason = $derived(
    !classDraftState ? undefined
    : !editing && !keyIsWellFormed(classDraftState.key) ? t('app.cmdb_admin.key_hint')
    : !classDraftState.name.trim() ? t('app.cmdb_admin.needs_name')
    : undefined,
  );

  async function saveClass() {
    const draft = classDraftState;
    if (!draft) return;
    dialogFailure = undefined;
    try {
      await cmdb.defineClass(editing?.key ?? draft.key, classDefine(draft), editing?.version);
      toast.show({ text: t('app.cmdb_admin.saved'), tone: 'success' });
      isOpen = false;
    } catch (error) {
      dialogFailure = renderProblem(error as never, messages).message;
    }
  }
</script>

{#if isOpen && classDraftState}
  <Dialog bind:isOpen title={editing ? t('app.cmdb_admin.edit_named', { name: nameOf(editing.key) }) : t('app.cmdb_admin.new_class')} dismissLabel={t('app.assets.cancel')} size="l">
    <Stack gap="150">
      {#if !editing}<Input label={t('app.cmdb_admin.key')} hint={t('app.cmdb_admin.key_hint')} bind:value={classDraftState.key} maxlength={63} />{/if}
      <Input label={t('app.assets.name_label')} bind:value={classDraftState.name} maxlength={120} />
      <Select label={t('app.cmdb_admin.parent')} options={classOptions.filter((each) => each.value !== editing?.key)} bind:value={classDraftState.parent} />
      <Switch label={t('app.cmdb_admin.abstract')} bind:checked={classDraftState.abstract} />
      <fieldset class="group">
        <legend>{t('app.assets.attributes')}</legend>
        {#each classDraftState.attributes as attribute, index (index)}
          <div class="line">
            <Input label={t('app.cmdb_admin.key')} size="sm" bind:value={attribute.key} maxlength={63} />
            <Select label={t('app.cmdb_admin.kind_label')} size="sm" options={kindOptions} bind:value={attribute.kind} />
            <Select label={t('app.cmdb_admin.need_label')} size="sm" options={needOptions} bind:value={attribute.need} />
            <Checkbox label={t('app.cmdb_admin.identifying')} bind:checked={attribute.identifier} />
            {#if attribute.kind === 'CHOICE'}<Textarea label={t('app.cmdb_admin.options')} rows={2} bind:value={attribute.options} />{/if}
            <IconButton icon="x" size="sm" tone="subtle" label={t('app.cmdb_admin.remove')} onclick={() => classDraftState?.attributes.splice(index, 1)} />
          </div>
        {/each}
        <div><Button size="sm" tone="subtle" icon="plus" onclick={() => classDraftState?.attributes.push({ key: '', kind: 'TEXT', need: 'OPTIONAL', identifier: false, options: '' })}>{t('app.cmdb_admin.add_attribute')}</Button></div>
      </fieldset>
      <Textarea label={t('app.cmdb_admin.rules')} hint={t('app.cmdb_admin.rules_hint')} rows={3} bind:value={classDraftState.rules} />
      <fieldset class="group">
        <legend>{t('app.cmdb_admin.required')}</legend>
        {#each classDraftState.required as needed, index (index)}
          <div class="line">
            <CheckboxGroup label={t('app.cmdb_admin.required_types')} options={typeKeys.map((each) => ({ value: each.value, label: each.label }))} selected={needed.types} onChange={(next) => (needed.types = [...next])} />
            <Checkbox label={t('app.cmdb_admin.outgoing')} bind:checked={needed.outgoing} />
            <IconButton icon="x" size="sm" tone="subtle" label={t('app.cmdb_admin.remove')} onclick={() => classDraftState?.required.splice(index, 1)} />
          </div>
        {/each}
        <div><Button size="sm" tone="subtle" icon="plus" onclick={() => classDraftState?.required.push({ types: [], outgoing: true })}>{t('app.cmdb_admin.add_required')}</Button></div>
      </fieldset>
      <Input label={t('app.cmdb_admin.staleness_label')} hint={t('app.cmdb_admin.staleness_hint')} inputmode="numeric" bind:value={classDraftState.staleness} />
      {#if editing}<Switch label={t('app.cmdb_admin.archived')} bind:checked={classDraftState.archived} />{/if}
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={classReason} onclick={() => void saveClass()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .group { display: grid; gap: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); padding: var(--sp-100) var(--sp-150); margin: 0; }
  .line { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
</style>
