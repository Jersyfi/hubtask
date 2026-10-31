<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Types* under *How we work* (UC-FLW-01 check 1): the workspace's entry types by level - added,
  // renamed, recoloured, ordered, archived and, while nothing uses them, deleted. The page is a
  // row of the workflows module, so it is reached only where the module is on (check 8).
  //
  // **Each action acts at once**, as a module switch does: a type is one small record, not a form
  // with a save bar. Deleting asks first; a type in use is refused by the server and can only be
  // archived (check 5), which the refusal says.
  //
  // **What the reader may never do is not drawn** (P-05): without STRUCTURE at the workspace the
  // page lists the types and offers no control.

  import { untrack } from 'svelte';
  import { labelTokens } from '@hubtask/design-system';
  import {
    Banner, Button, Dialog, Icon, IconButton, Inline, Input, LabelChip, PageHeader, Select, Spinner, Stack, Switch,
  } from '@hubtask/design-system/components';
  import type { EntryType } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { hasForm } from '../lib/data/entryforms.ts';
  import EntryFormEditor from '../lib/entries/EntryFormEditor.svelte';
  import {
    TYPE_ICONS, TYPE_LEVELS, entryTypes, iconOf, keyFrom, levelTypes, reorder,
  } from '../lib/data/entrytypes.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import SectionFindings from '../lib/settings/SectionFindings.svelte';

  $effect(() => page.entitle(t('app.entry_types.title')));
  $effect(() => untrack(() => entryTypes.open()));

  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const reading = $derived(entryTypes.state);
  const readRefusal = $derived(
    reading?.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const levelName = (level: string) => t('app.entry_types.level', { level });
  const nameOf = (type: EntryType) => type.name ?? levelName(type.level);

  /** The type an action is running on, so its row shows progress. */
  let busy = $state<string | undefined>(undefined);
  let refusal = $state<{ key: string; message: string } | undefined>(undefined);

  async function act(key: string, run: () => Promise<unknown>, done?: string) {
    refusal = undefined;
    busy = key;
    try {
      await run();
      if (done) toast.show({ text: done, tone: 'success' });
    } catch (error) {
      refusal = { key, message: renderProblem(error as never, messages).message };
    } finally {
      busy = undefined;
    }
  }

  function move(type: EntryType, by: -1 | 1) {
    const moves = reorder(entryTypes.all, type.key, by);
    void act(type.key, async () => {
      for (const { type: each, position } of moves) await entryTypes.change(each, { position });
    });
  }

  function archive(type: EntryType, archived: boolean) {
    const code = archived ? 'app.entry_types.archived_done' : 'app.entry_types.unarchived_done';
    void act(type.key, () => entryTypes.change(type, { archived }), t(code, { name: nameOf(type) }));
  }

  // --- The editor: one dialog for a new type and for a change --------------------------------

  interface Draft {
    level: string;
    name: string;
    key: string;
    isKeyTouched: boolean;
    icon: string;
    color: string;
    completable: boolean;
    workflow: boolean;
    record: boolean;
    schedulable: boolean;
    estimable: boolean;
  }

  let editing = $state<EntryType | undefined>(undefined);
  let draft = $state<Draft | undefined>(undefined);
  let isEditorOpen = $state(false);
  let isSaving = $state(false);
  let editorRefusal = $state<string | undefined>(undefined);

  function startNew(level: string) {
    editing = undefined;
    draft = {
      level, name: '', key: '', isKeyTouched: false, icon: 'tag',
      color: labelTokens[0], completable: true, workflow: false, record: false, schedulable: true, estimable: false,
    };
    editorRefusal = undefined;
    isEditorOpen = true;
  }

  function startEdit(type: EntryType) {
    editing = type;
    draft = {
      level: type.level, name: type.name ?? '', key: type.key, isKeyTouched: true, icon: type.icon,
      color: type.color_token, completable: type.completable, workflow: type.workflow, record: type.record,
      schedulable: type.schedulable, estimable: type.estimable,
    };
    editorRefusal = undefined;
    isEditorOpen = true;
  }

  function rename(value: string) {
    if (!draft) return;
    draft.name = value;
    if (!editing && !draft.isKeyTouched) draft.key = keyFrom(value);
  }

  async function save() {
    if (!draft) return;
    const d = draft;
    const behaviours = {
      completable: d.record ? false : d.completable, workflow: d.workflow, record: d.record,
      schedulable: d.schedulable, estimable: d.estimable,
    };
    isSaving = true;
    editorRefusal = undefined;
    try {
      if (editing) {
        const before = editing;
        const change = {
          ...(d.name.trim() !== (before.name ?? '') && d.name.trim() !== '' ? { name: d.name.trim() } : {}),
          ...(d.icon !== before.icon ? { icon: d.icon } : {}),
          ...(d.color !== before.color_token ? { color_token: d.color } : {}),
          ...(Object.fromEntries(
            Object.entries(behaviours).filter(([name, on]) => before[name as keyof typeof behaviours] !== on),
          ) as Partial<typeof behaviours>),
        };
        if (Object.keys(change).length > 0) await entryTypes.change(before, change);
      } else {
        const position = levelTypes(entryTypes.all, d.level).filter((each) => !each.system).length + 1;
        await entryTypes.create({
          key: d.key, level: d.level as EntryType['level'], name: d.name.trim(), icon: d.icon, color_token: d.color, position, ...behaviours,
        });
      }
      isEditorOpen = false;
      toast.show({ text: t('app.entry_types.saved', { name: d.name.trim() || levelName(d.level) }), tone: 'success' });
    } catch (error) {
      editorRefusal = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }

  const iconOptions = TYPE_ICONS.map((icon) => ({ value: icon, label: icon }));
  const colourOptions = labelTokens.map((token) => ({ value: token, label: t(`app.colour.${token}`) }));

  // --- The type's form: its own dialog, one save for the whole form ---------------------------

  let forming = $state<EntryType | undefined>(undefined);
  let isFormOpen = $state(false);

  // --- Deleting asks first ----------------------------------------------------------------------

  let deleting = $state<EntryType | undefined>(undefined);
  let isDeleteOpen = $state(false);

  function confirmDelete() {
    const type = deleting;
    isDeleteOpen = false;
    deleting = undefined;
    if (type) void act(type.key, () => entryTypes.remove(type), t('app.entry_types.deleted', { name: nameOf(type) }));
  }
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.entry_types.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'entry-types', label: t('app.entry_types.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  <p class="quiet">{t('app.entry_types.intro')}</p>
  <div class="file"><ViewSectionsAsFile sections={['types']} /></div>
  <SectionFindings sections={['types']} />

  {#if !reading || reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting">
      <Spinner label={t('app.entry_types.reading')} />
      <span>{t('app.entry_types.reading')}</span>
    </p>
  {:else if readRefusal}
    <Banner tone="danger" title={readRefusal.message}>
      {#if readRefusal.reference}{t('app.error_reference', { request_id: readRefusal.reference })}{/if}
    </Banner>
  {:else}
    {#each TYPE_LEVELS as level (level)}
      {@const types = levelTypes(entryTypes.all, level)}
      {@const movable = types.filter((each) => !each.system && !each.archived)}
      <section class="panel" data-level={level}>
        <Stack gap="200">
          <Inline gap="150" align="center" justify="between">
            <h2 class="heading">{t('app.entry_types.level_heading', { level })}</h2>
            {#if mayShape}
              <Button tone="secondary" size="sm" icon="plus" data-new-type={level} onclick={() => startNew(level)}>
                {t('app.entry_types.new')}
              </Button>
            {/if}
          </Inline>
          <ul class="types">
            {#each types as type (type.key)}
              {@const index = movable.findIndex((each) => each.key === type.key)}
              <li class="type" class:archived={type.archived} data-entry-type={type.key}>
                <div class="identity">
                  <Icon name={iconOf(type)} size="sm" />
                  <LabelChip name={nameOf(type)} colorToken={type.color_token} />
                  <code class="key">{type.key}</code>
                  {#if type.system}<span class="note">{t('app.entry_types.system')}</span>{/if}
                  {#if type.record}<span class="note">{t('app.entry_types.record')}</span>{/if}
                  {#if !type.schedulable}<span class="note">{t('app.entry_types.not_scheduled')}</span>{/if}
                  {#if type.archived}<span class="note">{t('app.entry_types.archived')}</span>{/if}
                  {#if hasForm(type.form)}<span class="note" data-has-form>{t('app.entry_types.has_form')}</span>{/if}
                </div>
                {#if mayShape}
                  <Inline gap="100" align="center">
                    {#if index >= 0}
                      <IconButton
                        icon="chevron-up" size="sm" tone="subtle" label={t('app.entry_types.move_up', { name: nameOf(type) })}
                        disabledReason={index === 0 ? t('app.entry_types.first') : undefined}
                        isBusy={busy === type.key} onclick={() => move(type, -1)}
                      />
                      <IconButton
                        icon="chevron-down" size="sm" tone="subtle" label={t('app.entry_types.move_down', { name: nameOf(type) })}
                        disabledReason={index === movable.length - 1 ? t('app.entry_types.last') : undefined}
                        isBusy={busy === type.key} onclick={() => move(type, 1)}
                      />
                    {/if}
                    <Button tone="subtle" size="sm" data-edit={type.key} onclick={() => startEdit(type)}>
                      {t('app.entry_types.edit')}
                    </Button>
                    <Button tone="subtle" size="sm" data-edit-form={type.key} onclick={() => { forming = type; isFormOpen = true; }}>
                      {t('app.entry_types.form')}
                    </Button>
                    {#if !type.system}
                      <Button tone="subtle" size="sm" data-archive={type.key} onclick={() => archive(type, !type.archived)}>
                        {type.archived ? t('app.entry_types.unarchive') : t('app.entry_types.archive')}
                      </Button>
                      <IconButton
                        icon="trash" size="sm" tone="subtle" label={t('app.entry_types.delete', { name: nameOf(type) })}
                        data-delete={type.key}
                        onclick={() => { deleting = type; isDeleteOpen = true; }}
                      />
                    {/if}
                  </Inline>
                {/if}
                {#if refusal?.key === type.key}<p class="refusal" role="alert">{refusal.message}</p>{/if}
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/each}
  {/if}
</Stack>

{#if draft}
  <Dialog
    bind:isOpen={isEditorOpen}
    title={editing ? t('app.entry_types.edit_title', { name: nameOf(editing) }) : t('app.entry_types.new_title', { level: draft.level })}
    dismissLabel={t('app.entry_types.cancel')}
    onClose={() => { draft = undefined; editing = undefined; }}
  >
    <Stack gap="200">
      <Input
        label={t('app.entry_types.name')}
        value={draft.name}
        isRequired={!editing?.system}
        placeholder={editing?.system ? levelName(editing.level) : undefined}
        oninput={(event) => rename((event.currentTarget as HTMLInputElement).value)}
      />
      {#if !editing}
        <Input
          label={t('app.entry_types.key')}
          hint={t('app.entry_types.key_hint')}
          value={draft.key}
          isRequired
          oninput={(event) => { if (draft) { draft.key = (event.currentTarget as HTMLInputElement).value; draft.isKeyTouched = true; } }}
        />
      {/if}
      <Inline gap="200" align="start">
        <Select label={t('app.entry_types.icon')} options={iconOptions} bind:value={draft.icon} />
        <Select label={t('app.entry_types.colour')} options={colourOptions} bind:value={draft.color} />
        <span class="preview" aria-hidden="true">
          <Icon name={iconOf({ icon: draft.icon })} size="sm" />
          <LabelChip name={draft.name || levelName(draft.level)} colorToken={draft.color} />
        </span>
      </Inline>
      {#if !editing?.system}
        <Switch label={t('app.entry_types.behaviour_record')} hint={t('app.entry_types.behaviour_record_hint')} bind:checked={draft.record} />
      {/if}
      <Switch
        label={t('app.entry_types.behaviour_completable')}
        bind:checked={draft.completable}
        disabledReason={draft.record ? t('app.entry_types.record_not_completable') : undefined}
      />
      <Switch label={t('app.entry_types.behaviour_schedulable')} hint={t('app.entry_types.behaviour_schedulable_hint')} bind:checked={draft.schedulable} />
      <Switch label={t('app.entry_types.behaviour_workflow')} bind:checked={draft.workflow} />
      <Switch label={t('app.entry_types.behaviour_estimable')} bind:checked={draft.estimable} />
      {#if editorRefusal}<p class="refusal" role="alert">{editorRefusal}</p>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isEditorOpen = false)}>{t('app.entry_types.cancel')}</Button>
      <Button tone="primary" data-save-type isBusy={isSaving} onclick={save}>{t('app.entry_types.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if forming}
  {#key forming.key}
    <EntryFormEditor type={forming} name={nameOf(forming)} bind:isOpen={isFormOpen} onClose={() => (forming = undefined)} />
  {/key}
{/if}

{#if deleting}
  <Dialog
    bind:isOpen={isDeleteOpen}
    title={t('app.entry_types.delete_title', { name: nameOf(deleting) })}
    dismissLabel={t('app.entry_types.cancel')}
    onClose={() => (deleting = undefined)}
  >
    <p class="quiet">{t('app.entry_types.delete_explain')}</p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isDeleteOpen = false)}>{t('app.entry_types.cancel')}</Button>
      <Button tone="danger" data-confirm-delete onclick={confirmDelete}>{t('app.entry_types.delete_confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .waiting {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .panel {
    padding: var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }

  .types {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: var(--sp-100);
  }

  .type {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-100);
    padding-block: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  .type:first-child { border-block-start: none; }
  .type.archived .identity { color: var(--text-subtle); }

  .identity { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); min-inline-size: 0; }
  .key { color: var(--text-subtle); font-size: var(--fs-075); }
  .note { color: var(--text-subtle); font-size: var(--fs-075); }
  .preview { display: inline-flex; align-items: center; gap: var(--sp-050); align-self: end; padding-block-end: var(--sp-100); }
  .refusal { margin: 0; flex-basis: 100%; color: var(--text-danger); }
  .file { display: flex; justify-content: flex-end; }
</style>
