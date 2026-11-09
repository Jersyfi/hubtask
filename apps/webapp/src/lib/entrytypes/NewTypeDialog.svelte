<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *New type* as a Dialog M (page-system §5.1, §9.1): the level, the name, its icon and colour,
  // the key under *Advanced*. What the type does - a record, dates, a workflow, what may sit
  // underneath, its words in other languages - is set on its own page afterwards, with the starting
  // values a type of work has.
  //
  // **The key is made from the name** (UX-5); *Advanced* opens by itself where the name yields none.
  import { labelTokens } from '@hubtask/design-system';
  import { Button, Dialog, Disclosure, Icon, Inline, Input, LabelChip, Select, Stack } from '@hubtask/design-system/components';
  import type { EntryType } from '@hubtask/sync-engine';

  import { TYPE_ICONS, TYPE_LEVELS, entryTypes, iconOf, keyFrom, levelTypes } from '../data/entrytypes.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
  }

  const { isOpen, onClose }: Props = $props();

  let level = $state<string>(TYPE_LEVELS[0]);
  let name = $state('');
  let key = $state('');
  let isKeyTouched = $state(false);
  let icon = $state('tag');
  let color = $state<string>(labelTokens[0]);
  let isKeyOpen = $state(false);
  let isSaving = $state(false);
  let refusal = $state<string | undefined>(undefined);

  $effect(() => {
    if (name.trim() !== '' && key === '') isKeyOpen = true;
  });

  function reset() {
    level = TYPE_LEVELS[0];
    name = '';
    key = '';
    isKeyTouched = false;
    icon = 'tag';
    color = labelTokens[0];
    isKeyOpen = false;
    refusal = undefined;
  }

  function close() {
    reset();
    onClose();
  }

  const guard = new DiscardGuard(() => name.trim() !== '' || isKeyTouched, close);

  function rename(value: string) {
    name = value;
    if (!isKeyTouched) key = keyFrom(value);
  }

  async function create() {
    isSaving = true;
    refusal = undefined;
    try {
      const position = levelTypes(entryTypes.all, level).filter((each) => !each.system).length + 1;
      await entryTypes.create({
        key, level: level as EntryType['level'], name: name.trim(), icon, color_token: color, position,
        completable: true, workflow: false, record: false, schedulable: true, estimable: false,
        allowed_children: [], description: '', translations: {},
      });
      toast.show({ text: t('app.entry_types.saved', { name: name.trim() }), tone: 'success' });
      close();
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }

  const levelOptions = $derived(TYPE_LEVELS.map((each) => ({ value: each, label: t('app.entry_types.level', { level: each }) })));
  const iconOptions = TYPE_ICONS.map((each) => ({ value: each, label: each }));
  const colourOptions = $derived(labelTokens.map((token) => ({ value: token, label: t(`app.colour.${token}`) })));
</script>

<Dialog
  size="m"
  {isOpen}
  title={t('app.entry_types.new')}
  dismissLabel={t('app.entry_types.cancel')}
  onDismiss={guard.dismiss}
  onClose={close}
  primary={{ label: t('app.entry_types.create'), isBusy: isSaving, onclick: () => void create() }}
>
  <Stack gap="200">
    {#if refusal}<FailureNote problem={refusal} />{/if}
    <Select label={t('app.entry_types.level_label')} options={levelOptions} bind:value={level} />
    <Input label={t('app.entry_types.name')} value={name} isRequired oninput={(event) => rename((event.currentTarget as HTMLInputElement).value)} />
    <Inline gap="200" align="start">
      <Select label={t('app.entry_types.icon')} options={iconOptions} bind:value={icon} />
      <Select label={t('app.entry_types.colour')} options={colourOptions} bind:value={color} />
      <span class="preview" aria-hidden="true">
        <Icon name={iconOf({ icon })} size="sm" />
        <LabelChip name={name || t('app.entry_types.level', { level })} colorToken={color} />
      </span>
    </Inline>
    <Disclosure label={t('app.entry_types.advanced')} tone="quiet" bind:isOpen={isKeyOpen}>
      <Input
        label={t('app.entry_types.key')}
        hint={t('app.entry_types.key_hint')}
        value={key}
        isRequired
        oninput={(event) => { key = (event.currentTarget as HTMLInputElement).value; isKeyTouched = true; }}
      />
    </Disclosure>
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) close(); }}>{t('app.entry_types.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  .preview { display: inline-flex; align-items: center; gap: var(--sp-050); align-self: end; padding-block-end: var(--sp-100); }
</style>
