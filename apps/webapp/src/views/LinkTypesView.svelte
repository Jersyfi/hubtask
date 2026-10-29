<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Link types* under *How we work* (UC-PLN-01 check 5): the six every workspace has and its own -
  // added with their two names, direction, waiting end, limit and cycle rule; renamed, ordered,
  // archived and, while no link uses them, deleted. What a type means is fixed once it is made, so
  // the editor of an existing type offers its names only.
  //
  // **Each action acts at once**, as on *Types*: a link type is one small record. A type in use is
  // refused by the server on delete and can only be archived, which the refusal says.
  //
  // **What the reader may never do is not drawn** (P-05): without STRUCTURE at the workspace the
  // page lists the types and offers no control.

  import { untrack } from 'svelte';
  import {
    Banner, Button, Dialog, IconButton, Inline, Input, PageHeader, Select, Spinner, Stack, Switch,
  } from '@hubtask/design-system/components';
  import type { LinkType } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { keyFrom } from '../lib/data/entrytypes.svelte.ts';
  import { linkTypes, sideName } from '../lib/data/links.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  $effect(() => page.entitle(t('app.link_types.title')));
  $effect(() => untrack(() => linkTypes.open()));

  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const reading = $derived(linkTypes.state);
  const readRefusal = $derived(
    reading?.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const ordered = $derived([...linkTypes.all].sort((a, b) => a.position - b.position));
  const movable = $derived(ordered.filter((each) => !each.system && !each.archived));

  const nameOf = (type: LinkType) => sideName(type, type.key, true, t);
  const inverseOf = (type: LinkType) => sideName(type, type.key, false, t);

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

  /** Swaps a type with its neighbour among the movable ones, writing both positions. */
  function move(type: LinkType, by: -1 | 1) {
    const at = movable.findIndex((each) => each.key === type.key);
    const other = movable[at + by];
    if (!other) return;
    void act(type.key, async () => {
      await linkTypes.change(type, { position: other.position });
      await linkTypes.change(other, { position: type.position });
    });
  }

  function archive(type: LinkType, archived: boolean) {
    const code = archived ? 'app.link_types.archived_done' : 'app.link_types.unarchived_done';
    void act(type.key, () => linkTypes.change(type, { archived }), t(code, { name: nameOf(type) }));
  }

  // --- The editor ------------------------------------------------------------------------------

  interface Draft {
    name: string;
    inverseName: string;
    key: string;
    isKeyTouched: boolean;
    // Strings, as a select list binds them; narrowed when the type is sent.
    direction: string;
    waits: string;
    isOne: boolean;
    areLoopsAllowed: boolean;
  }

  let editing = $state<LinkType | undefined>(undefined);
  let draft = $state<Draft | undefined>(undefined);
  let isEditorOpen = $state(false);
  let isSaving = $state(false);
  let editorRefusal = $state<string | undefined>(undefined);

  function startNew() {
    editing = undefined;
    draft = {
      name: '', inverseName: '', key: '', isKeyTouched: false, direction: 'DIRECTED', waits: 'NONE',
      isOne: false, areLoopsAllowed: true,
    };
    editorRefusal = undefined;
    isEditorOpen = true;
  }

  function startEdit(type: LinkType) {
    editing = type;
    draft = {
      name: type.name ?? '', inverseName: type.inverse_name ?? '', key: type.key, isKeyTouched: true,
      direction: type.direction, waits: type.waits, isOne: type.cardinality === 'ONE',
      areLoopsAllowed: type.cycles === 'ALLOWED',
    };
    editorRefusal = undefined;
    isEditorOpen = true;
  }

  function rename(value: string) {
    if (!draft) return;
    draft.name = value;
    if (!editing && !draft.isKeyTouched) draft.key = keyFrom(value);
  }

  // A symmetric type has no waiting end and no limit; a waiting type forbids loops. The draft
  // follows, so the dialog never offers a combination the server refuses.
  const isSymmetric = $derived(draft?.direction === 'SYMMETRIC');
  const isWaiting = $derived(draft !== undefined && draft.waits !== 'NONE');

  async function save() {
    if (!draft) return;
    const d = draft;
    isSaving = true;
    editorRefusal = undefined;
    try {
      if (editing) {
        const before = editing;
        const change = {
          ...(d.name.trim() !== (before.name ?? '') ? { name: d.name.trim() } : {}),
          ...(!isSymmetric && d.inverseName.trim() !== (before.inverse_name ?? '') ? { inverse_name: d.inverseName.trim() } : {}),
        };
        if (Object.keys(change).length > 0) await linkTypes.change(before, change);
      } else {
        const position = Math.max(-1, ...linkTypes.all.map((each) => each.position)) + 1;
        await linkTypes.create({
          key: d.key,
          name: d.name.trim(),
          ...(isSymmetric ? {} : { inverse_name: d.inverseName.trim() }),
          direction: d.direction as LinkType['direction'],
          waits: isSymmetric ? 'NONE' : (d.waits as LinkType['waits']),
          cardinality: !isSymmetric && d.isOne ? 'ONE' : 'MANY',
          cycles: isWaiting || !d.areLoopsAllowed ? 'FORBIDDEN' : 'ALLOWED',
          position,
        });
      }
      isEditorOpen = false;
      toast.show({ text: t('app.link_types.saved', { name: d.name.trim() || (editing ? nameOf(editing) : d.key) }), tone: 'success' });
    } catch (error) {
      editorRefusal = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }

  const directionOptions = $derived([
    { value: 'DIRECTED', label: t('app.link_types.direction_directed') },
    { value: 'SYMMETRIC', label: t('app.link_types.direction_symmetric') },
  ]);
  const waitsOptions = $derived([
    { value: 'NONE', label: t('app.link_types.waits_none') },
    { value: 'SOURCE', label: t('app.link_types.waits_source_option') },
    { value: 'TARGET', label: t('app.link_types.waits_target_option') },
  ]);

  // --- Deleting asks first ----------------------------------------------------------------------

  let deleting = $state<LinkType | undefined>(undefined);
  let isDeleteOpen = $state(false);

  function confirmDelete() {
    const type = deleting;
    isDeleteOpen = false;
    deleting = undefined;
    if (type) void act(type.key, () => linkTypes.remove(type), t('app.link_types.deleted', { name: nameOf(type) }));
  }
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.link_types.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'link-types', label: t('app.link_types.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  <p class="quiet">{t('app.link_types.intro')}</p>

  {#if !reading || reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting">
      <Spinner label={t('app.link_types.reading')} />
      <span>{t('app.link_types.reading')}</span>
    </p>
  {:else if readRefusal}
    <Banner tone="danger" title={readRefusal.message}>
      {#if readRefusal.reference}{t('app.error_reference', { request_id: readRefusal.reference })}{/if}
    </Banner>
  {:else}
    <section class="panel">
      <Stack gap="200">
        {#if mayShape}
          <Inline gap="150" align="center" justify="end">
            <Button tone="secondary" size="sm" icon="plus" data-new-link-type onclick={startNew}>{t('app.link_types.new')}</Button>
          </Inline>
        {/if}
        <ul class="types">
          {#each ordered as type (type.key)}
            {@const index = movable.findIndex((each) => each.key === type.key)}
            <li class="type" class:archived={type.archived} data-link-type={type.key}>
              <div class="identity">
                <span class="name">{nameOf(type)}</span>
                {#if type.direction === 'DIRECTED'}<span class="inverse">/ {inverseOf(type)}</span>{/if}
                <code class="key">{type.key}</code>
                {#if type.system}<span class="note">{t('app.link_types.system')}</span>{/if}
                {#if type.direction === 'SYMMETRIC'}<span class="note">{t('app.link_types.symmetric')}</span>{/if}
                {#if type.waits === 'SOURCE'}<span class="note">{t('app.link_types.waits_source')}</span>{/if}
                {#if type.waits === 'TARGET'}<span class="note">{t('app.link_types.waits_target')}</span>{/if}
                {#if type.cardinality === 'ONE'}<span class="note">{t('app.link_types.one')}</span>{/if}
                {#if type.cycles === 'ALLOWED'}<span class="note">{t('app.link_types.loops')}</span>{/if}
                {#if type.archived}<span class="note">{t('app.link_types.archived')}</span>{/if}
              </div>
              {#if mayShape && !type.system}
                <Inline gap="100" align="center">
                  {#if index >= 0}
                    <IconButton
                      icon="chevron-up" size="sm" tone="subtle" label={t('app.link_types.move_up', { name: nameOf(type) })}
                      disabledReason={index === 0 ? t('app.link_types.first') : undefined}
                      isBusy={busy === type.key} onclick={() => move(type, -1)}
                    />
                    <IconButton
                      icon="chevron-down" size="sm" tone="subtle" label={t('app.link_types.move_down', { name: nameOf(type) })}
                      disabledReason={index === movable.length - 1 ? t('app.link_types.last') : undefined}
                      isBusy={busy === type.key} onclick={() => move(type, 1)}
                    />
                  {/if}
                  <Button tone="subtle" size="sm" data-edit={type.key} onclick={() => startEdit(type)}>{t('app.link_types.edit')}</Button>
                  <Button tone="subtle" size="sm" data-archive={type.key} onclick={() => archive(type, !type.archived)}>
                    {type.archived ? t('app.link_types.unarchive') : t('app.link_types.archive')}
                  </Button>
                  <IconButton
                    icon="trash" size="sm" tone="subtle" label={t('app.link_types.delete', { name: nameOf(type) })}
                    data-delete={type.key}
                    onclick={() => { deleting = type; isDeleteOpen = true; }}
                  />
                </Inline>
              {/if}
              {#if refusal?.key === type.key}<p class="refusal" role="alert">{refusal.message}</p>{/if}
            </li>
          {/each}
        </ul>
      </Stack>
    </section>
  {/if}
</Stack>

{#if draft}
  <Dialog
    bind:isOpen={isEditorOpen}
    title={editing ? t('app.link_types.edit_title', { name: nameOf(editing) }) : t('app.link_types.new_title')}
    dismissLabel={t('app.link_types.cancel')}
    onClose={() => { draft = undefined; editing = undefined; }}
  >
    <Stack gap="200">
      {#if !editing}
        <Select label={t('app.link_types.direction')} options={directionOptions} bind:value={draft.direction} />
      {/if}
      <Input
        label={t('app.link_types.name')}
        value={draft.name}
        isRequired
        oninput={(event) => rename((event.currentTarget as HTMLInputElement).value)}
      />
      {#if !isSymmetric}
        <Input label={t('app.link_types.inverse_name')} bind:value={draft.inverseName} isRequired />
      {/if}
      {#if !editing}
        <Input
          label={t('app.link_types.key')}
          hint={t('app.link_types.key_hint')}
          value={draft.key}
          isRequired
          oninput={(event) => { if (draft) { draft.key = (event.currentTarget as HTMLInputElement).value; draft.isKeyTouched = true; } }}
        />
        {#if !isSymmetric}
          <Select label={t('app.link_types.waits')} options={waitsOptions} bind:value={draft.waits} />
          <Switch label={t('app.link_types.one_label')} bind:checked={draft.isOne} />
        {/if}
        <Switch
          label={t('app.link_types.loops_label')}
          checked={!isWaiting && draft.areLoopsAllowed}
          onchange={(event: Event) => { if (draft) draft.areLoopsAllowed = (event.currentTarget as HTMLInputElement).checked; }}
          disabledReason={isWaiting ? t('app.link_types.loops_waiting') : undefined}
        />
        <p class="quiet">{t('app.link_types.fixed_note')}</p>
      {/if}
      {#if editorRefusal}<p class="refusal" role="alert">{editorRefusal}</p>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isEditorOpen = false)}>{t('app.link_types.cancel')}</Button>
      <Button tone="primary" data-save-link-type isBusy={isSaving} onclick={save}>{t('app.link_types.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if deleting}
  <Dialog
    bind:isOpen={isDeleteOpen}
    title={t('app.link_types.delete_title', { name: nameOf(deleting) })}
    dismissLabel={t('app.link_types.cancel')}
    onClose={() => (deleting = undefined)}
  >
    <p class="quiet">{t('app.link_types.delete_explain')}</p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isDeleteOpen = false)}>{t('app.link_types.cancel')}</Button>
      <Button tone="danger" data-confirm-delete onclick={confirmDelete}>{t('app.link_types.delete_confirm')}</Button>
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
  .name { font-weight: var(--fw-medium); }
  .inverse { color: var(--text-secondary); }
  .key { color: var(--text-subtle); font-size: var(--fs-075); }
  .note { color: var(--text-subtle); font-size: var(--fs-075); }
  .refusal { margin: 0; flex-basis: 100%; color: var(--text-danger); }
</style>
