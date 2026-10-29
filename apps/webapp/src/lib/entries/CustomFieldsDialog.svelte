<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The custom fields in force for a collection: defining one, changing what it permits, and
  // taking it out of use.
  //
  // **The key, the kind and a MONEY field's currency are settled once.**
  // `CustomFieldDefinitionUpdate` carries none of them, and the contract gives the reason in its own words — a key that moved would orphan
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
    Checkbox,
    CheckboxGroup,
    Dialog,
    Inline,
    Input,
    Select,
    Stack,
    Textarea,
  } from '@hubtask/design-system/components';
  import type { CustomFieldDefinition } from '@hubtask/sync-engine';

  import { holds, offerIn, typesWith } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { isCalculated, isValidCurrency, isValidKey, isWorkspaceWide, takesCurrency, takesOptions } from '../data/customfields.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { check, focusFirst, isComplete, without, type Missing } from '../forms/missing.ts';
  import PanelOrDialog from '../workspace/PanelOrDialog.svelte';

  interface Props {
    /** Drawn as a tab of the *Structure* panel rather than as a dialog of its own. */
    isEmbedded?: boolean;
    isOpen: boolean;
    collectionId: string;
    /** The role this reader holds along the collection, which is what `STRUCTURE` is read from. */
    role?: string;
  }

  let { isEmbedded = false, isOpen = $bindable(), collectionId, role }: Props = $props();

  const structure = $derived(holds(role, 'STRUCTURE'));
  const carriers = $derived(typesWith('CUSTOM_FIELDS'));
  // A level by its name here, never the manifest's identifier (UC-WRK-23 check 6).
  const offer = $derived(offerIn(containers.find(collectionId)?.structure?.settings));

  const existing = $derived(customFields.of(collectionId));

  /** The fourteen the contract declares, in the contract's own order. */
  const KINDS = [
    'TEXT', 'NUMBER', 'DATE', 'SELECT', 'MULTI_SELECT', 'BOOL', 'USER', 'URL',
    'MONEY', 'DURATION', 'REFERENCE', 'USER_SET', 'RICH_TEXT', 'CALCULATED',
  ];

  let key = $state('');
  let kind = $state('TEXT');
  let options = $state('');
  let currency = $state('');
  let formula = $state('');
  let isRequired = $state(false);
  /** Whether only holders of `FIELD_WRITE:<key>` write it (UC-ID-27 check 1). */
  let isRestricted = $state(false);
  /** Whether it is hidden from readers without `FIELD_READ:<key>` as well (UC-ID-27 check 3). */
  let isHidden = $state(false);
  // A hidden field is restricted as well: a reader who could write it would overwrite it blind.
  $effect(() => {
    if (isHidden) isRestricted = true;
  });
  let chosen = $state<string[]>([]);
  let editing = $state<CustomFieldDefinition | undefined>(undefined);
  let deleting = $state<CustomFieldDefinition | undefined>(undefined);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const keyIsWrong = $derived(key !== '' && !isValidKey(key));
  const currencyIsWrong = $derived(currency !== '' && !isValidCurrency(currency));
  /** Whether the reader may shape fields here: the controls that do are drawn only then (P-05). */
  const mayShape = $derived(structure.status === 'permitted');
  /** What *Define* found missing when it was pressed; each mark goes when its field changes. */
  let missing = $state<Missing>({});
  let form = $state<HTMLElement | null>(null);

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
    currency = '';
    formula = '';
    isRequired = false;
    isRestricted = false;
    isHidden = false;
    // The default is what the column's default is — every carrier rather than a guess at one.
    chosen = [...carriers];
    editing = undefined;
    deleting = undefined;
    failure = undefined;
    missing = {};
  }

  function startEdit(definition: CustomFieldDefinition) {
    editing = definition;
    key = definition.key;
    kind = definition.kind;
    options = (definition.options ?? []).join('\n');
    currency = definition.currency ?? '';
    formula = definition.formula ?? '';
    isRequired = definition.is_required ?? false;
    isRestricted = definition.restricted ?? false;
    isHidden = definition.read_restricted ?? false;
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

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    isSaving = true;
    failure = undefined;
    try {
      await work();
      // Said out loud on success (4.1.3): what changed is on the screen, and a reader who cannot
      // see the screen is told the same thing once, through the one live region.
      if (said) announcer.say(said);
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  function save() {
    if (isSaving) return;
    // Never switched off for what is missing (design-system.md §4, case 3): pressed, it marks the
    // key and the types it still needs and moves focus to the first.
    missing = check([
      { kind: 'field', key: 'key', code: 'fields.key_required', isFilled: editing !== undefined || key !== '' },
      { kind: 'at-least-one', key: 'types', code: 'app.fields.applies_to_empty', chosen: chosen.length },
      {
        kind: 'field', key: 'currency', code: 'app.fields.currency_required',
        isFilled: editing !== undefined || !takesCurrency(kind) || currency !== '',
      },
      {
        kind: 'field', key: 'formula', code: 'fields.formula_required',
        isFilled: editing !== undefined || !isCalculated(kind) || formula.trim() !== '',
      },
    ]);
    if (!isComplete(missing)) {
      queueMicrotask(() => focusFirst(form));
      return;
    }
    if (keyIsWrong && editing === undefined) return;
    if (currencyIsWrong && editing === undefined) return;
    const current = editing;
    void attempt(async () => {
      if (current) {
        await customFields.update(
          current.id,
          {
            options: takesOptions(current.kind) ? optionList() : [],
            is_required: isRequired,
            ...(isRestricted !== (current.restricted ?? false) ? { restricted: isRestricted } : {}),
            ...(isHidden !== (current.read_restricted ?? false) ? { read_restricted: isHidden } : {}),
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
            ...(takesCurrency(kind) ? { currency } : {}),
            ...(isCalculated(kind) ? { formula } : {}),
            is_required: isCalculated(kind) ? false : isRequired,
            restricted: isCalculated(kind) ? false : isRestricted || isHidden,
            read_restricted: isCalculated(kind) ? false : isHidden,
            applies_to: chosen as never,
          },
          crypto.randomUUID(),
        );
      }
      reset();
    }, current ? t('app.fields.saved_announced') : t('app.fields.defined_announced'));
  }

  function confirmDelete() {
    const target = deleting;
    if (!target) return;
    void attempt(async () => {
      await customFields.remove(target.id, target.version);
      deleting = undefined;
    }, t('app.fields.removed_announced'));
  }

  const kindLabel = (value: string) => t(`app.fields.kind_${value}`);
</script>

<PanelOrDialog {isEmbedded} bind:isOpen title={t('app.fields.title')}>
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
                  {kindLabel(definition.kind)}{#if definition.currency} ({definition.currency}){/if} ·
                  {isWorkspaceWide(definition)
                    ? t('app.fields.workspace_wide')
                    : t('app.fields.this_collection')}
                </span>
              </div>
              <!-- A definition is changed where it was defined. The workspace-wide ones are listed
                   here because they apply here, and their controls are not. -->
              {#if !isWorkspaceWide(definition) && mayShape}
                <Inline gap="050">
                  <Button size="sm" tone="secondary" onclick={() => startEdit(definition)}>
                    {t('app.fields.edit')}
                  </Button>
                  <Button size="sm" tone="secondary" onclick={() => (deleting = definition)}>
                    {t('app.fields.delete')}
                  </Button>
                </Inline>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}

    <!-- Defining and changing are the shape of the workspace: drawn for a role that may, absent for
         any other and while the role is still being read (P-05, UC-WRK-22 check 4). -->
    {#if mayShape}
      <div bind:this={form}>
      <Stack gap="150">
        <Input
          label={t('app.fields.key')}
          hint={t('app.fields.key_hint')}
          bind:value={key}
          oninput={() => (missing = without(missing, 'key'))}
          error={missing.key ? t(missing.key) : keyIsWrong ? t('app.fields.key_invalid') : undefined}
          disabledReason={editing ? t('app.fields.immutable') : undefined}
        />
        <Select
          label={t('app.fields.kind')}
          bind:value={kind}
          options={KINDS.map((value) => ({ value, label: kindLabel(value) }))}
          disabledReason={editing ? t('app.fields.immutable') : undefined}
        />

        {#if takesCurrency(editing?.kind ?? kind)}
          <Input
            label={t('app.fields.currency')}
            hint={t('app.fields.currency_hint')}
            bind:value={currency}
            maxlength={3}
            autocapitalize="characters"
            oninput={() => {
              currency = currency.toUpperCase();
              missing = without(missing, 'currency');
            }}
            error={missing.currency ? t(missing.currency) : currencyIsWrong ? t('app.fields.currency_invalid') : undefined}
            disabledReason={editing ? t('app.fields.currency_immutable') : undefined}
          />
        {/if}

        {#if isCalculated(editing?.kind ?? kind)}
          <Textarea
            label={t('app.fields.formula')}
            hint={t('app.fields.formula_hint')}
            bind:value={formula}
            rows={2}
            oninput={() => (missing = without(missing, 'formula'))}
            error={missing.formula ? t(missing.formula) : undefined}
            disabledReason={editing ? t('app.fields.formula_immutable') : undefined}
          />
        {/if}

        {#if takesOptions(editing?.kind ?? kind)}
          <Textarea
            label={t('app.fields.options')}
            hint={t('app.fields.options_hint')}
            bind:value={options}
            rows={4}
          />
        {/if}

        {#if !isCalculated(editing?.kind ?? kind)}
          <Checkbox label={t('app.fields.required')} bind:checked={isRequired} />
          <Checkbox label={t('app.fields.restricted')} hint={t('app.fields.restricted_hint')} bind:checked={isRestricted} />
          <Checkbox label={t('app.fields.hidden')} hint={t('app.fields.hidden_hint')} bind:checked={isHidden} />
        {/if}

        {#if carriers.length === 0}
          <fieldset class="types">
            <legend>{t('app.fields.applies_to')}</legend>
            <p class="hint">{t('app.fields.applies_to_none')}</p>
          </fieldset>
        {:else}
          <!-- Nothing said in advance when none is ticked: *Define* says it if it is pressed so. -->
          <CheckboxGroup
            label={t('app.fields.applies_to')}
            hint={t('app.fields.applies_to_hint')}
            options={carriers.map((type) => ({ value: type, label: offer.level(type).label }))}
            selected={chosen}
            error={missing.types ? t(missing.types) : undefined}
            onChange={(next) => {
              chosen = [...next];
              missing = without(missing, 'types');
            }}
          />
        {/if}

        {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}

        <Inline gap="100">
          <Button isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={save}>
            {editing ? t('app.workspace.save') : t('app.fields.define')}
          </Button>
          {#if editing}
            <Button tone="secondary" onclick={reset}>{t('app.workspace.cancel')}</Button>
          {/if}
        </Inline>
      </Stack>
      </div>
    {/if}
  </Stack>
</PanelOrDialog>

{#if deleting}
  <Dialog
    isOpen={true}
    title={t('app.fields.delete_title', { key: deleting.key })}
    dismissLabel={t('app.workspace.cancel')}
    onClose={() => (deleting = undefined)}
  >
    <Stack gap="150">
      <p class="hint">{t('app.fields.delete_explains')}</p>
      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
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
