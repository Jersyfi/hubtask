<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The collection's labels: making them, renaming them, taking them away.
  //
  // `labels` has `create`, `update` and `remove`, and without this the set a `LabelPicker`
  // offers could only be made outside the application: the picker's own empty state - "No labels
  // in this collection yet." - would be a dead end rather than a beginning.
  //
  // **Here rather than in the picker.** Putting a create control inside `LabelPicker` would mean
  // changing a design system component, and the design system grows in the order of
  // design-system.md §4, never as a side effect of application work. It also belongs here on its own
  // merits: a label is a property of the collection, and this is the collection's screen. The
  // picker stays what it is - a chooser among what exists.
  //
  // **Ten colours, and no more.** `design-system.md` §4 says it in five words: "ten colorToken
  // values, nothing else". Each is a measured pair of background and foreground, legible in both
  // themes, which is what a colour wheel could not promise - so the choice is a list of the ten
  // rather than a picker, and `labelTokens` is where the ten are.

  import { untrack } from 'svelte';

  import { labelTokens } from '@hubtask/design-system';
  import { Button, DataTable, Dialog, Input, LabelChip, Stack } from '@hubtask/design-system/components';
  import type { DataColumn } from '@hubtask/design-system/components';

  import { labels } from '../data/labels.svelte.ts';
  import { offerUndo } from './undo.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { focusFirst } from '../forms/missing.ts';

  import type { Label } from '@hubtask/sync-engine';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import TableSection from '../page/TableSection.svelte';

  interface Props {
    collectionId: string;
  }

  const { collectionId }: Props = $props();

  /** The label dialog: a new one, or `editing`'s name and colour (a Dialog S, page-system §5.1). */
  let isOpen = $state(false);

  let name = $state('');
  let colour = $state<string>(labelTokens[0]);
  let editing = $state<Label | undefined>(undefined);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const existing = $derived(labels.of(collectionId));
  /**
   * What was taken out of the vocabulary, with the way back (UC-WRK-28 check 3): a deletion's
   * *Undo* stands only as long as its message, so the labels themselves are the place it stays.
   */
  const deleted = $derived(labels.deletedOf(collectionId));

  $effect(() => {
    const wanted = collectionId;
    return untrack(() => labels.openDeleted(wanted));
  });

  async function restore(label: Label) {
    failure = undefined;
    try {
      await labels.restore(collectionId, label.id, crypto.randomUUID());
      announcer.say(t('app.labels.restored_announced', { name: label.name }));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }

  function reset() {
    name = '';
    colour = labelTokens[0];
    editing = undefined;
    failure = undefined;
    isNameMissing = false;
  }

  function startNew() {
    reset();
    isOpen = true;
  }

  function startEditing(label: Label) {
    reset();
    editing = label;
    name = label.name;
    colour = label.color_token ?? labelTokens[0];
    isOpen = true;
  }

  const guard = new DiscardGuard(
    () => (editing ? name.trim() !== editing.name || colour !== (editing.color_token ?? labelTokens[0]) : name.trim() !== ''),
    () => (isOpen = false),
  );

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.labels.name'), priority: 1 },
    { id: 'colour', label: t('app.workspace.colour'), priority: 2 },
  ]);
  const menuOf = (label: Label) => ({
    label: t('app.page.more_for', { name: label.name }),
    items: [
      { id: 'rename', label: t('app.labels.rename', { name: label.name }) },
      { id: 'delete', label: t('app.labels.delete', { name: label.name }), isDestructive: true, hasSeparatorBefore: true },
    ],
  });

  /** The save was pressed without a name: the field says so (design-system.md §4, case 3). */
  let isNameMissing = $state(false);
  let form = $state<HTMLFormElement | null>(null);

  async function save() {
    if (isSaving) return;
    if (name.trim() === '') {
      isNameMissing = true;
      queueMicrotask(() => focusFirst(form));
      return;
    }
    isSaving = true;
    failure = undefined;
    try {
      if (editing) {
        // The version the reader had when they started typing (ADR-0025).
        await labels.update(
          collectionId,
          editing.id,
          { name: name.trim(), color_token: colour },
          editing.version,
        );
      } else {
        await labels.create(
          collectionId,
          { name: name.trim(), color_token: colour },
          crypto.randomUUID(),
        );
      }
      announcer.say(t('app.labels.saved_announced'));
      isOpen = false;
      reset();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  async function remove(label: Label) {
    failure = undefined;
    try {
      await labels.remove(collectionId, label.id, label.version);
      // At once, with the way back (UC-WRK-28 check 3): the entries that carried it carry it again.
      offerUndo({ kind: 'label-deleted', collectionId, labelId: label.id, name: label.name });

    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }
</script>

<!-- The collection's labels as one table (page-system §9.6, PS-4): rename and delete behind each
     row's ⋯, a new one and a change in a Dialog S; what was deleted below with its way back. -->
{#snippet newLabel()}
  <Button size="sm" tone="secondary" onclick={startNew}>{t('app.labels.new')}</Button>
{/snippet}

<TableSection title={t('app.labels.manage')} note={String(existing.length)} acts={newLabel}>
  {#if failure && !isOpen}<p class="failure" role="alert">{failure.message}</p>{/if}
  <DataTable
    label={t('app.labels.choose')}
    {columns}
    rows={existing}
    key={(label) => label.id}
    rowLabel={(label) => label.name}
    menu={menuOf}
    onmenu={(label, id) => (id === 'delete' ? void remove(label) : startEditing(label))}
    actionsLabel={t('app.page.row_actions')}
    sheetDismissLabel={t('app.page.sheet_close')}
    sheetGrabLabel={t('app.page.sheet_grab')}
  >
    {#snippet cell(label, column)}
      {#if column.id === 'name'}<LabelChip name={label.name} colorToken={label.color_token} />
      {:else}<span class="quiet">{label.color_token ? t(`app.colour.${label.color_token}`) : ''}</span>{/if}
    {/snippet}
    {#snippet empty()}<p class="quiet">{t('app.labels.none_yet')}</p>{/snippet}
  </DataTable>
</TableSection>

{#if deleted.length > 0}
  <!-- What was taken out of the vocabulary stays with its way back (UC-WRK-28 check 3). -->
  <TableSection title={t('app.labels.deleted_heading')} note={String(deleted.length)}>
    <DataTable
      label={t('app.labels.deleted_heading')}
      columns={columns.slice(0, 1)}
      rows={deleted}
      key={(label) => label.id}
      rowLabel={(label) => label.name}
      actionsLabel={t('app.page.row_actions')}
    >
      {#snippet cell(label)}<LabelChip name={label.name} colorToken={label.color_token} />{/snippet}
      {#snippet verb(label)}
        <Button size="sm" tone="secondary" onclick={() => void restore(label)}>{t('app.labels.restore', { name: label.name })}</Button>
      {/snippet}
    </DataTable>
  </TableSection>
{/if}

<Dialog
  size="s"
  title={editing ? t('app.labels.rename', { name: editing.name }) : t('app.labels.new')}
  bind:isOpen
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={guard.dismiss}
  onClose={reset}
  primary={{ label: t(editing ? 'app.workspace.save' : 'app.labels.create'), isBusy: isSaving, busyLabel: t('app.workspace.saving'), onclick: () => void save() }}
>
  <form bind:this={form} onsubmit={(event) => { event.preventDefault(); void save(); }}>
    <Stack gap="150">
      <Input
        label={t('app.labels.name')}
        bind:value={name}
        isRequired
        oninput={() => (isNameMissing = false)}
        error={isNameMissing ? t('app.labels.name_required') : undefined}
      />

      <!-- A radio group by hand rather than ten `Radio`s: what a reader is choosing between is the
           chips themselves, drawn with the label's own name. The radio is named by its colour, or
           every swatch would be read out as the name field's label. -->
      <fieldset class="colours">
        <legend>{t('app.workspace.colour')}</legend>
        {#each labelTokens as token (token)}
          <label class="swatch">
            <input type="radio" name="label-colour" value={token} aria-label={t(`app.colour.${token}`)} bind:group={colour} />
            <LabelChip name={name.trim() === '' ? t('app.labels.name') : name.trim()} colorToken={token} />
          </label>
        {/each}
      </fieldset>

      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) isOpen = false; }}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
  .colours {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-050);
    margin: 0;
    padding: 0;
    border: 0;
  }
  .colours legend {
    padding: 0;
    margin-block-end: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }
  .swatch { display: inline-flex; align-items: center; gap: var(--sp-050); cursor: pointer; }
  /* The radio stays in the tab order and keeps the arrow-key behaviour a group has; what marks the
     chosen one is the ring, which is the same ring every other control here focuses with. */
  .swatch input { position: absolute; opacity: 0; pointer-events: none; }
  .swatch:has(input:checked) { outline: var(--bw-ring) solid var(--accent-primary); border-radius: var(--r-sm); }
  .swatch:has(input:focus-visible) { outline: var(--bw-ring) solid var(--focus-ring); border-radius: var(--r-sm); }
</style>
