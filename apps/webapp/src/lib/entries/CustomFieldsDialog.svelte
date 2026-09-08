<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The custom fields in force for a collection: defining one, changing what it permits, and
  // taking it out of use.
  //
  // **The key and the kind are settled once.** `CustomFieldDefinitionUpdate` does not carry
  // either, and the contract gives the reason in its own words — a key that moved would orphan
  // every value stored under it, a kind that changed would reinterpret them. So both controls stay
  // on screen and are off with that reason: a control that vanished after creation would leave the
  // reader wondering whether they had missed it.
  //
  // **The workspace-wide ones are listed and marked.** `GET /custom-fields?collection_id=` answers
  // the collection's own and the ones above it in one list, because what applies here is one
  // question. They are shown without their controls: a definition is changed where it was defined,
  // and offering the buttons here would be offering a write against another scope's screen.
  //
  // **Deleting is soft and the dialog says what stays.** The values remain in the entries and stop
  // being shown, and a field defined again under the same key shows none of them. That is a
  // sentence a reader has to read before pressing, not afterwards.

  import {
    Button,
    CapabilityGate,
    Checkbox,
    Dialog,
    Inline,
    Input,
    Select,
    Stack,
    Textarea,
  } from '@hubtask/design-system/components';
  import type { CustomFieldDefinition } from '@hubtask/sync-engine';

  import { holds, typesWith } from '../data/capability.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { isValidKey, isWorkspaceWide, takesOptions } from '../data/customfields.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    collectionId: string;
    /** The role this reader holds along the collection, which is what `STRUCTURE` is read from. */
    role?: string;
  }

  let { isOpen = $bindable(), collectionId, role }: Props = $props();

  const structure = $derived(holds(role, 'STRUCTURE'));
  const carriers = $derived(typesWith('CUSTOM_FIELDS'));

  const existing = $derived(customFields.of(collectionId));

  /** The eight the contract declares, read from nowhere but the contract's own enum. */
  const KINDS = ['TEXT', 'NUMBER', 'DATE', 'SELECT', 'MULTI_SELECT', 'BOOL', 'USER', 'URL'];

  let key = $state('');
  let kind = $state('TEXT');
  let options = $state('');
  let isRequired = $state(false);
  let chosen = $state<string[]>([]);
  let editing = $state<CustomFieldDefinition | undefined>(undefined);
  let deleting = $state<CustomFieldDefinition | undefined>(undefined);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const keyIsWrong = $derived(key !== '' && !isValidKey(key));
  const canSave = $derived(
    (editing !== undefined || (key !== '' && !keyIsWrong)) && chosen.length > 0 && !isSaving,
  );

  // Opening starts from nothing. Closing leaves the fields alone: they are re-read on the next
  // open, and clearing them here would blank the dialog while it is still fading out.
  $effect(() => {
    if (!isOpen) return;
    reset();
  });

  function reset() {
    key = '';
    kind = 'TEXT';
    options = '';
    isRequired = false;
    // The default is what the column's default is — every carrier rather than a guess at one.
    chosen = [...carriers];
    editing = undefined;
    deleting = undefined;
    failure = undefined;
  }

  function startEdit(definition: CustomFieldDefinition) {
    editing = definition;
    key = definition.key;
    kind = definition.kind;
    options = (definition.options ?? []).join('\n');
    isRequired = definition.is_required ?? false;
    chosen = [...((definition.applies_to ?? []) as readonly string[])];
    failure = undefined;
  }

  /** One per line, blanks dropped: a permitted value of nothing is not one somebody meant. */
  function optionList(): string[] {
    return options
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line !== '');
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
    if (!canSave) return;
    const current = editing;
    void attempt(async () => {
      if (current) {
        await customFields.update(
          current.id,
          {
            options: takesOptions(current.kind) ? optionList() : [],
            is_required: isRequired,
            applies_to: chosen as never,
          },
          current.version,
        );
      } else {
        await customFields.define(
          {
            collection_id: collectionId,
            key,
            kind: kind as never,
            options: takesOptions(kind) ? optionList() : [],
            is_required: isRequired,
            applies_to: chosen as never,
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
      await customFields.remove(target.id, target.version);
      deleting = undefined;
    });
  }

  const kindLabel = (value: string) => t(`app.fields.kind_${value}`);
</script>

<Dialog bind:isOpen title={t('app.fields.title')}>
  <Stack gap="200">
    <p class="hint">{t('app.fields.scope_hint')}</p>

    {#if existing.length === 0}
      <p class="hint">{t('app.fields.none')}</p>
    {:else}
      <ul class="fields">
        {#each existing as definition (definition.id)}
          <li>
            <div class="row">
              <div>
                <span class="key">{definition.key}</span>
                <span class="meta">
                  {kindLabel(definition.kind)} ·
                  {isWorkspaceWide(definition)
                    ? t('app.fields.workspace_wide')
                    : t('app.fields.this_collection')}
                </span>
              </div>
              <!-- A definition is changed where it was defined. The workspace-wide ones are listed
                   here because they apply here, and their controls are not. -->
              {#if !isWorkspaceWide(definition)}
                <Inline gap="050">
                  <Button
                    size="sm"
                    tone="secondary"
                    disabledReason={structure.status === 'permitted'
                      ? undefined
                      : structure.status === 'refused'
                        ? t(structure.code, structure.params)
                        : t('app.fields.deciding')}
                    onclick={() => startEdit(definition)}
                  >
                    {t('app.fields.edit')}
                  </Button>
                  <Button
                    size="sm"
                    tone="secondary"
                    disabledReason={structure.status === 'permitted'
                      ? undefined
                      : structure.status === 'refused'
                        ? t(structure.code, structure.params)
                        : t('app.fields.deciding')}
                    onclick={() => (deleting = definition)}
                  >
                    {t('app.fields.delete')}
                  </Button>
                </Inline>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}

    <CapabilityGate
      status={structure.status}
      reason={structure.status === 'refused' ? t(structure.code, structure.params) : undefined}
      pendingLabel={t('app.fields.deciding')}
    >
      <Stack gap="150">
        <Input
          label={t('app.fields.key')}
          hint={t('app.fields.key_hint')}
          bind:value={key}
          error={keyIsWrong ? t('app.fields.key_invalid') : undefined}
          disabledReason={editing ? t('app.fields.immutable') : undefined}
        />
        <Select
          label={t('app.fields.kind')}
          bind:value={kind}
          options={KINDS.map((value) => ({ value, label: kindLabel(value) }))}
          disabledReason={editing ? t('app.fields.immutable') : undefined}
        />

        {#if takesOptions(editing?.kind ?? kind)}
          <Textarea
            label={t('app.fields.options')}
            hint={t('app.fields.options_hint')}
            bind:value={options}
            rows={4}
          />
        {/if}

        <Checkbox label={t('app.fields.required')} bind:checked={isRequired} />

        <fieldset class="types">
          <legend>{t('app.fields.applies_to')}</legend>
          <p class="hint">{t('app.fields.applies_to_hint')}</p>
          {#if carriers.length === 0}
            <p class="hint">{t('app.fields.applies_to_none')}</p>
          {:else}
            <Inline gap="100">
              {#each carriers as type (type)}
                <!-- A function binding rather than a handler: `checked` is `$bindable` and the
                     spread props are a native input's, so an `onchange` here would be handed an
                     Event rather than the answer. -->
                <Checkbox
                  label={type}
                  bind:checked={
                    () => chosen.includes(type),
                    (next: boolean) =>
                      (chosen = next ? [...chosen, type] : chosen.filter((held) => held !== type))
                  }
                />
              {/each}
            </Inline>
          {/if}
          {#if chosen.length === 0}
            <p class="failure">{t('app.fields.applies_to_empty')}</p>
          {/if}
        </fieldset>

        {#if failure}<p class="failure">{failure.message}</p>{/if}

        <Inline gap="100">
          <Button isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={save}>
            {editing ? t('app.workspace.save') : t('app.fields.define')}
          </Button>
          {#if editing}
            <Button tone="secondary" onclick={reset}>{t('app.workspace.cancel')}</Button>
          {/if}
        </Inline>
      </Stack>
    </CapabilityGate>
  </Stack>
</Dialog>

{#if deleting}
  <Dialog
    isOpen={true}
    title={t('app.fields.delete_title', { key: deleting.key })}
    dismissLabel={t('app.workspace.cancel')}
    onClose={() => (deleting = undefined)}
  >
    <Stack gap="150">
      <p class="hint">{t('app.fields.delete_explains')}</p>
      {#if failure}<p class="failure">{failure.message}</p>{/if}
      <Inline gap="100">
        <Button tone="danger" isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={confirmDelete}>
          {t('app.fields.delete')}
        </Button>
        <Button tone="secondary" onclick={() => (deleting = undefined)}>
          {t('app.workspace.cancel')}
        </Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .fields { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-100); }

  .row { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  .key { display: block; font-family: var(--font-mono); }

  .meta { display: block; color: var(--text-secondary); font-size: var(--fs-075); }

  .types { margin: 0; padding: 0; border: none; }

  .types legend { padding: 0; font-size: var(--fs-075); font-weight: var(--fw-semibold); }

  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
