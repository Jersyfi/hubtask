<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection's *Entry templates* as a settings section (page-system §9.6, PS-4, §5.1): one
  // table of the templates that apply here, *Use this template* the row's verb, *Edit* and
  // *Delete* behind its ⋯; using one is a Dialog M, defining or changing one a Dialog L (its form
  // and its tree), *Generate from a description* a Dialog M, a deletion a Dialog S. Capture and
  // the collection's own dialog keep `TemplatesDialog`, where a table may not stand.
  //
  // The permissions are `TemplatesDialog`'s: defining asks `STRUCTURE`, using `WRITE_ITEMS`, and a
  // control the role does not allow is absent rather than off (P-05, UC-WRK-16 check 7). A
  // workspace-wide template is listed because it applies here; it is changed where it was defined.

  import { Button, DataTable, Dialog, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';
  import type { DataColumn } from '@hubtask/design-system/components';
  import type { DroppedReference, Template, TemplateNode } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { holds, offerIn, rootTypes } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { type Path } from '../data/people.svelte.ts';
  import { suggestions } from '../data/suggestions.svelte.ts';
  import { isPending } from '../data/suggestions.ts';
  import { templates } from '../data/templates.svelte.ts';
  import { scopeCodeOf } from '../data/templates.ts';
  import { announcer } from '../announce.svelte.ts';
  import { focusFirst } from '../forms/missing.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import TableSection from '../page/TableSection.svelte';
  import { renderProblem } from '../problem.ts';
  import CollectionSummary from './CollectionSummary.svelte';
  import TemplateEditor from './TemplateEditor.svelte';

  interface Props {
    collectionId: string;
    path: Path;
    /** The role this reader holds along the collection. Both permissions are read from it. */
    role?: string;
    onopened: (itemId: string) => void;
  }

  const { collectionId, path, role, onopened }: Props = $props();

  // The collection's levels by the names it gave them (UC-WRK-23 check 6), never the identifiers.
  const offer = $derived(offerIn(containers.find(collectionId)?.structure?.settings));
  const levelLabel = (type: string): string => offer.level(type).label;

  const mayShape = $derived(holds(role, 'STRUCTURE').status === 'permitted');
  const mayUse = $derived(holds(role, 'WRITE_ITEMS').status === 'permitted');
  const held = $derived(templates.of(collectionId));
  const hasAi = $derived(manifest.value?.features?.ai_suggestions === true);
  const generation = $derived(suggestions.askingOf(collectionId));

  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  async function attempt(work: () => Promise<unknown>, said: string): Promise<boolean> {
    isSaving = true;
    failure = undefined;
    try {
      await work();
      announcer.say(said);
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages);
      return false;
    } finally {
      isSaving = false;
    }
  }

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.templates.name'), priority: 1 },
    { id: 'scope', label: t('app.templates.scope'), priority: 2 },
    { id: 'root', label: t('app.templates.root_type'), priority: 3 },
  ]);
  const menuOf = (template: Template) =>
    template.scope_type === 'COLLECTION' && mayShape
      ? {
          label: t('app.page.more_for', { name: template.name }),
          items: [
            { id: 'edit', label: t('app.entries.edit') },
            { id: 'delete', label: t('app.templates.delete'), isDestructive: true, hasSeparatorBefore: true },
          ],
        }
      : undefined;

  // --- Defining and changing one: a Dialog L, its fields and its tree. ---------------------------
  let isDefining = $state(false);
  let editing = $state<Template | undefined>(undefined);
  let name = $state('');
  let description = $state('');
  let rootType = $state('TASK');
  let nodes = $state<readonly TemplateNode[]>([]);
  let startedFrom = '';

  const snapshot = () => JSON.stringify([name, description, rootType, nodes]);
  const definingGuard = new DiscardGuard(() => snapshot() !== startedFrom, () => (isDefining = false));

  function startDefining(template?: Template) {
    failure = undefined;
    editing = template;
    name = template?.name ?? '';
    description = template?.description ?? '';
    rootType = (template?.root_type as string | undefined) ?? rootTypes()[0] ?? 'TASK';
    nodes = template ? (template.nodes ?? []) : [{ type: rootTypes()[0] ?? 'TASK', title: '' } as TemplateNode];
    startedFrom = snapshot();
    isDefining = true;
  }

  async function save() {
    const current = editing;
    const done = await attempt(
      () =>
        current
          ? // The tree travels whole, which is what `nodes` being one member means.
            templates.update(current.id, { name, description: description || null, nodes: nodes as never }, current.version)
          : templates.create(
              { scope_type: 'COLLECTION', scope_id: collectionId, name, description: description || null, root_type: rootType as never, nodes: nodes as never },
              crypto.randomUUID(),
            ),
      current ? t('app.templates.saved_announced') : t('app.templates.defined_announced'),
    );
    if (done) isDefining = false;
  }

  // --- Using one: a Dialog M, the anchor and the first entry's title. -----------------------------
  let using = $state<Template | undefined>(undefined);
  let isUsing = $state(false);
  let anchor = $state('');
  let rootTitle = $state('');
  let dropped = $state<readonly DroppedReference[]>([]);
  let created = $state<number | undefined>(undefined);
  let madeRoot = $state<string | undefined>(undefined);

  function startUsing(template: Template) {
    failure = undefined;
    using = template;
    // Today where the reader is: the anchor is a date and "+3 days from Monday" is about days.
    anchor = todayIn(actor.zone);
    rootTitle = '';
    dropped = [];
    created = undefined;
    madeRoot = undefined;
    isUsing = true;
  }

  async function instantiate() {
    const template = using;
    if (!template) return;
    await attempt(async () => {
      const made = await templates.instantiate(
        template.id,
        { collection_id: collectionId, ...(anchor === '' ? {} : { anchor_date: anchor }), ...(rootTitle.trim() === '' ? {} : { title: rootTitle.trim() }) },
        crypto.randomUUID(),
      );
      dropped = made.dropped_references ?? [];
      created = made.created;
      madeRoot = made.root_item_id;
      // Nothing was left behind, so there is nothing to read: go to what it made.
      if (dropped.length === 0 && madeRoot) onopened(madeRoot);
    }, t('app.templates.used_announced'));
  }

  // --- Generating a draft from a description: a Dialog M. ----------------------------------------
  let isGenerating = $state(false);
  let wanted = $state('');
  let isWantedMissing = $state(false);
  let generateBody = $state<HTMLElement | null>(null);

  function startGenerating() {
    failure = undefined;
    wanted = '';
    isWantedMissing = false;
    isGenerating = true;
  }

  async function generate() {
    const asked = wanted.trim();
    // Never switched off for an empty description (design-system.md §4, case 3): pressed, the
    // field says what it needs and takes focus.
    if (asked === '') {
      isWantedMissing = true;
      queueMicrotask(() => focusFirst(generateBody));
      return;
    }
    // Answered `202` at once; the draft lands above the table when it arrives.
    if (await attempt(() => suggestions.generateTemplate(collectionId, asked), t('app.templates.generation_asked_announced'))) isGenerating = false;
  }

  // --- Deleting one: a Dialog S. -----------------------------------------------------------------
  let deleting = $state<Template | undefined>(undefined);
  let isDeleting = $state(false);

  async function confirmDelete() {
    const target = deleting;
    if (target && (await attempt(() => templates.remove(target.id, target.version), t('app.templates.removed_announced')))) isDeleting = false;
  }
</script>

{#snippet acts()}
  {#if mayShape}
    {#if hasAi}
      <Button size="sm" tone="subtle" icon="sparkles" disabledReason={isPending(generation) ? t('app.suggestions.pending') : undefined} onclick={startGenerating}>
        {t('app.templates.generate_from_description')}
      </Button>
    {/if}
    <Button size="sm" tone="secondary" onclick={() => startDefining()}>{t('app.templates.define')}</Button>
  {/if}
{/snippet}

<!-- The drafts asked for here, while they run and once they arrive. -->
{#if hasAi}<CollectionSummary containerId={collectionId} part="template" />{/if}

<TableSection title={t('app.templates.title')} note={String(held.length)} {acts}>
  {#if failure && !isDefining && !isUsing && !isGenerating && !isDeleting}<p class="failure" role="alert">{failure.message}</p>{/if}
  <div data-entry-templates>
    <DataTable
      label={t('app.templates.applying_label')}
      {columns}
      rows={held}
      key={(template) => template.id}
      rowLabel={(template) => template.name}
      menu={menuOf}
      onmenu={(template, id) => {
        if (id === 'edit') startDefining(template);
        else {
          deleting = template;
          failure = undefined;
          isDeleting = true;
        }
      }}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
    >
      {#snippet cell(template, column)}
        {#if column.id === 'name'}<span class="name">{template.name}</span>
        {:else if column.id === 'scope'}<span class="quiet">{t(scopeCodeOf(template.scope_type as string))}</span>
        {:else}<span class="quiet">{levelLabel(template.root_type as string)}</span>{/if}
      {/snippet}
      {#snippet verb(template)}
        {#if mayUse}<Button size="sm" tone="secondary" onclick={() => startUsing(template)}>{t('app.templates.use')}</Button>{/if}
      {/snippet}
      {#snippet empty()}<p class="quiet">{t('app.templates.none')}</p>{/snippet}
    </DataTable>
  </div>
</TableSection>

<Dialog
  size="l"
  title={editing ? editing.name : t('app.templates.define')}
  bind:isOpen={isDefining}
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={definingGuard.dismiss}
  primary={{ label: t('app.templates.save'), isBusy: isSaving, busyLabel: t('app.workspace.saving'), onclick: () => void save() }}
>
  <Stack gap="150">
    <Input label={t('app.templates.name')} bind:value={name} error={failure?.fields.get('/name')} />
    <Textarea label={t('app.templates.description')} bind:value={description} rows={2} />
    {#if !editing}
      <!-- The root type is settled at definition: changing it would make a different template. -->
      <Select label={t('app.templates.root_type')} bind:value={rootType} options={rootTypes().map((type) => ({ value: type, label: levelLabel(type) }))} />
    {/if}
    <TemplateEditor template={editing} {nodes} {path} {failure} {levelLabel} onnodes={(next) => (nodes = next)} />
    {#if failure && !failure.fields.get('/name')}<p class="failure" role="alert">{failure.message}</p>{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (definingGuard.dismiss()) isDefining = false; }}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog guard={definingGuard} />

<Dialog
  size="m"
  title={using ? t('app.templates.use_title', { name: using.name }) : t('app.templates.use')}
  bind:isOpen={isUsing}
  dismissLabel={t('app.workspace.cancel')}
  primary={madeRoot
    ? { label: t('app.templates.open_root'), onclick: () => onopened(madeRoot!) }
    : { label: t('app.templates.go'), isBusy: isSaving, busyLabel: t('app.workspace.saving'), onclick: () => void instantiate() }}
>
  <Stack gap="150">
    <Input label={t('app.templates.anchor')} type="date" hint={t('app.templates.anchor_hint')} bind:value={anchor} />
    <Input label={t('app.templates.root_title')} hint={t('app.templates.root_title_hint')} bind:value={rootTitle} />
    {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
    {#if created !== undefined}<p class="quiet">{t('app.templates.created', { count: String(created) })}</p>{/if}
    {#if dropped.length > 0}
      <div class="dropped">
        <p>{t('app.templates.left_behind')}</p>
        <ul>
          {#each dropped as reference, index (`${reference.kind}-${reference.id}-${index}`)}
            <!-- The move's own codes, kind for kind: a third set of sentences would be a third answer. -->
            <li><strong>{t(`app.move.dropped_kind.${reference.kind}`)}</strong> {t(reference.code)}</li>
          {/each}
        </ul>
      </div>
    {/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isUsing = false)}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<Dialog
  size="m"
  title={t('app.templates.generate_from_description')}
  bind:isOpen={isGenerating}
  dismissLabel={t('app.workspace.cancel')}
  primary={{ label: t('app.templates.generate'), isBusy: isSaving, busyLabel: t('app.templates.generating'), onclick: () => void generate() }}
>
  <div bind:this={generateBody}>
    <Stack gap="150">
      <Textarea
        label={t('app.templates.generate_description')}
        hint={t('app.templates.generate_hint')}
        bind:value={wanted}
        oninput={() => (isWantedMissing = false)}
        rows={4}
        error={isWantedMissing ? t('app.templates.generate_empty') : failure?.fields.get('/description')}
      />
      <p class="quiet">{t('app.templates.generate_note')}</p>
      {#if failure && !failure.fields.get('/description')}<p class="failure" role="alert">{failure.message}</p>{/if}
    </Stack>
  </div>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isGenerating = false)}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<Dialog
  size="s"
  title={deleting ? t('app.templates.delete_title', { name: deleting.name }) : t('app.templates.delete')}
  bind:isOpen={isDeleting}
  dismissLabel={t('app.workspace.cancel')}
  primary={{ label: t('app.templates.delete'), tone: 'danger', isBusy: isSaving, busyLabel: t('app.workspace.saving'), onclick: () => void confirmDelete() }}
>
  <Stack gap="150">
    <p class="quiet">{t('app.templates.delete_explains')}</p>
    {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isDeleting = false)}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .name { font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); max-inline-size: 64ch; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .dropped { color: var(--text-secondary); font-size: var(--fs-075); }
  .dropped p { margin: 0; }
  .dropped ul { margin: var(--sp-050) 0 0; padding-inline-start: var(--sp-200); }
</style>
