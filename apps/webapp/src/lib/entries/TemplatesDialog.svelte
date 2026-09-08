<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The templates that apply here: using one, and — where a role may — shaping one.
  //
  // **Two permissions, and they are not the same control.** Defining a template is shaping the
  // workspace and asks `STRUCTURE` at its scope; using one is ordinary work and asks `WRITE_ITEMS`
  // in the collection it lands in. A viewer sees every template and can define none, with the
  // reason — which is the normal case rather than an edge one.
  //
  // **The list is one read of three scopes**, and each row says which it came from. Without the
  // mark it is three lists that look like one, and "delete" on a workspace-wide template from a
  // collection's screen would be a surprise.
  //
  // **What the destination could not carry is reported per kind**, with the move's own codes — the
  // same fact reported by a third operation, and a third set of sentences for it would be a third
  // answer.

  import {
    Button,
    CapabilityGate,
    Dialog,
    Inline,
    Input,
    Select,
    Stack,
    Textarea,
  } from '@hubtask/design-system/components';
  import type { DroppedReference, Template, TemplateNode } from '@hubtask/sync-engine';

  import { holds } from '../data/capability.svelte.ts';
  import { rootTypes } from '../data/capability.svelte.ts';
  import { templates } from '../data/templates.svelte.ts';
  import { scopeCodeOf } from '../data/templates.ts';
  import { type Path } from '../data/people.svelte.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { actor } from '../data/account.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import TemplateEditor from './TemplateEditor.svelte';

  interface Props {
    isOpen: boolean;
    collectionId: string;
    path: Path;
    /** The role this reader holds along the collection. Both permissions are read from it. */
    role?: string;
    onopened: (itemId: string) => void;
  }

  let { isOpen = $bindable(), collectionId, path, role, onopened }: Props = $props();

  const structure = $derived(holds(role, 'STRUCTURE'));
  const writeItems = $derived(holds(role, 'WRITE_ITEMS'));
  const held = $derived(templates.of(collectionId));

  const structureReason = $derived(
    structure.status === 'permitted'
      ? undefined
      : structure.status === 'refused'
        ? t(structure.code, structure.params)
        : t('app.templates.deciding'),
  );
  const useReason = $derived(
    writeItems.status === 'permitted'
      ? undefined
      : writeItems.status === 'refused'
        ? t(writeItems.code, writeItems.params)
        : t('app.templates.deciding'),
  );

  // Defining.
  let isDefining = $state(false);
  let editing = $state<Template | undefined>(undefined);
  let name = $state('');
  let description = $state('');
  let rootType = $state('TASK');
  let nodes = $state<readonly TemplateNode[]>([]);
  let deleting = $state<Template | undefined>(undefined);

  // Using.
  let using = $state<Template | undefined>(undefined);
  let anchor = $state('');
  let rootTitle = $state('');
  let dropped = $state<readonly DroppedReference[]>([]);
  let created = $state<number | undefined>(undefined);
  let madeRoot = $state<string | undefined>(undefined);

  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  $effect(() => {
    if (!isOpen) return;
    reset();
  });

  function reset() {
    isDefining = false;
    editing = undefined;
    name = '';
    description = '';
    rootType = rootTypes()[0] ?? 'TASK';
    nodes = [];
    deleting = undefined;
    using = undefined;
    anchor = '';
    rootTitle = '';
    dropped = [];
    created = undefined;
    madeRoot = undefined;
    failure = undefined;
  }

  function startDefining() {
    reset();
    isDefining = true;
    nodes = [{ type: rootTypes()[0] ?? 'TASK', title: '' } as TemplateNode];
  }

  function startEditing(template: Template) {
    reset();
    isDefining = true;
    editing = template;
    name = template.name;
    description = template.description ?? '';
    rootType = template.root_type as string;
    nodes = template.nodes ?? [];
  }

  function startUsing(template: Template) {
    reset();
    using = template;
    // Today where the reader is: the anchor is a date and "+3 days from Monday" is about days.
    anchor = todayIn(actor.zone);
  }

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    isSaving = true;
    failure = undefined;
    try {
      await work();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  function save() {
    const current = editing;
    void attempt(async () => {
      if (current) {
        // The tree travels whole, which is what `nodes` being one member means.
        await templates.update(
          current.id,
          { name, description: description || null, nodes: nodes as never },
          current.version,
        );
      } else {
        await templates.create(
          {
            scope_type: 'COLLECTION',
            scope_id: collectionId,
            name,
            description: description || null,
            root_type: rootType as never,
            nodes: nodes as never,
          },
          crypto.randomUUID(),
        );
      }
      reset();
    });
  }

  function confirmDelete() {
    const target = deleting;
    if (!target) return;
    void attempt(async () => {
      await templates.remove(target.id, target.version);
      deleting = undefined;
    });
  }

  function instantiate() {
    const template = using;
    if (!template) return;
    void attempt(async () => {
      const made = await templates.instantiate(
        template.id,
        {
          collection_id: collectionId,
          ...(anchor === '' ? {} : { anchor_date: anchor }),
          ...(rootTitle.trim() === '' ? {} : { title: rootTitle.trim() }),
        },
        crypto.randomUUID(),
      );
      dropped = made.dropped_references ?? [];
      created = made.created;
      madeRoot = made.root_item_id;
      // Nothing was left behind, so there is nothing to read: go to what it made.
      if (dropped.length === 0 && madeRoot) onopened(madeRoot);
    });
  }
</script>

<Dialog bind:isOpen title={t('app.templates.title')} dismissLabel={t('app.workspace.cancel')}>
  <Stack gap="200">
    {#if using}
      <Stack gap="150">
        <p class="quiet">{t('app.templates.use_title', { name: using.name })}</p>
        <Input label={t('app.templates.anchor')} type="date" hint={t('app.templates.anchor_hint')} bind:value={anchor} />
        <Input label={t('app.templates.root_title')} hint={t('app.templates.root_title_hint')} bind:value={rootTitle} />

        {#if failure}<p class="failure">{failure.message}</p>{/if}
        {#if created !== undefined}
          <p class="quiet">{t('app.templates.created', { count: String(created) })}</p>
        {/if}

        {#if dropped.length > 0}
          <div class="dropped">
            <p>{t('app.templates.left_behind')}</p>
            <ul>
              {#each dropped as reference, index (`${reference.kind}-${reference.id}-${index}`)}
                <li>
                  <!-- The move's own codes, kind for kind: the same fact reported by a third
                       operation, and a third set of sentences would be a third answer. -->
                  <strong>{t(`app.move.dropped_kind.${reference.kind}`)}</strong>
                  {t(reference.code)}
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        <Inline gap="100">
          {#if madeRoot}
            <Button onclick={() => onopened(madeRoot!)}>{t('app.templates.open_root')}</Button>
          {:else}
            <Button
              isBusy={isSaving}
              busyLabel={t('app.workspace.saving')}
              disabledReason={useReason}
              onclick={instantiate}
            >
              {t('app.templates.go')}
            </Button>
          {/if}
          <Button tone="secondary" onclick={reset}>{t('app.workspace.cancel')}</Button>
        </Inline>
      </Stack>
    {:else if isDefining}
      <Stack gap="150">
        <Input label={t('app.templates.name')} bind:value={name} error={failure?.fields.get('/name')} />
        <Textarea label={t('app.templates.description')} bind:value={description} rows={2} />
        {#if !editing}
          <!-- The root type is settled at definition: changing it would change what the template
               produces, which is a different template. -->
          <Select
            label={t('app.templates.root_type')}
            bind:value={rootType}
            options={rootTypes().map((type) => ({ value: type, label: type }))}
          />
        {/if}

        <TemplateEditor
          template={editing}
          {nodes}
          {path}
          {failure}
          onnodes={(next) => (nodes = next)}
        />

        {#if failure && !failure.fields.get('/name')}<p class="failure">{failure.message}</p>{/if}

        <Inline gap="100">
          <Button
            isBusy={isSaving}
            busyLabel={t('app.workspace.saving')}
            disabledReason={structureReason}
            onclick={save}
          >
            {t('app.templates.save')}
          </Button>
          <Button tone="secondary" onclick={reset}>{t('app.workspace.cancel')}</Button>
        </Inline>
      </Stack>
    {:else}
      {#if held.length === 0}
        <p class="quiet">{t('app.templates.none')}</p>
      {:else}
        <ul class="list">
          {#each held as template (template.id)}
            <li>
              <div class="row">
                <div>
                  <span class="name">{template.name}</span>
                  <span class="meta">
                    {t(scopeCodeOf(template.scope_type as string))} · {template.root_type}
                  </span>
                </div>
                <Inline gap="050">
                  <Button size="sm" disabledReason={useReason} onclick={() => startUsing(template)}>
                    {t('app.templates.use')}
                  </Button>
                  <!-- Changed and deleted where it was defined. A workspace-wide one is listed here
                       because it applies here, and its controls belong to its own scope. -->
                  {#if template.scope_type === 'COLLECTION'}
                    <Button size="sm" tone="secondary" disabledReason={structureReason} onclick={() => startEditing(template)}>
                      {t('app.entries.edit')}
                    </Button>
                    <Button size="sm" tone="secondary" disabledReason={structureReason} onclick={() => (deleting = template)}>
                      {t('app.templates.delete')}
                    </Button>
                  {/if}
                </Inline>
              </div>
            </li>
          {/each}
        </ul>
      {/if}

      {#if failure}<p class="failure">{failure.message}</p>{/if}

      <CapabilityGate
        status={structure.status}
        reason={structure.status === 'refused' ? t(structure.code, structure.params) : undefined}
        pendingLabel={t('app.templates.deciding')}
      >
        <div>
          <Button size="sm" tone="secondary" onclick={startDefining}>
            {t('app.templates.define')}
          </Button>
        </div>
      </CapabilityGate>
    {/if}
  </Stack>
</Dialog>

{#if deleting}
  <Dialog
    isOpen={true}
    title={t('app.templates.delete_title', { name: deleting.name })}
    dismissLabel={t('app.workspace.cancel')}
    onClose={() => (deleting = undefined)}
  >
    <Stack gap="150">
      <p class="quiet">{t('app.templates.delete_explains')}</p>
      {#if failure}<p class="failure">{failure.message}</p>{/if}
      <Inline gap="100">
        <Button tone="danger" isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={confirmDelete}>
          {t('app.templates.delete')}
        </Button>
        <Button tone="secondary" onclick={() => (deleting = undefined)}>{t('app.workspace.cancel')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .list { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-100); }

  .row { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  .name { display: block; font-weight: var(--fw-semibold); }

  .meta { display: block; color: var(--text-secondary); font-size: var(--fs-075); }

  .dropped { color: var(--text-secondary); font-size: var(--fs-075); }

  .dropped ul { margin: var(--sp-050) 0 0; padding-left: var(--sp-200); }

  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
