<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A type's form, designed under *How we work → Types* (UC-FLW-01 check 2, D-13): its sections, its
  // fields in order with required, read-only and hidden conditions, defaults and help, the checks
  // that must hold, and the channels that narrow it.
  //
  // **One save for the whole form.** Unlike a type's own switches, a form is one document the
  // server checks as a whole - a channel may only hide a field the form holds, a condition may only
  // name a field the type has - so the editor sends it once and shows the refusal on the field it
  // names.
  //
  // **Conditions are CEL, checked by the server.** No evaluator runs here (`entryforms.ts`); a
  // condition that does not compile comes back as a refusal naming where it sits.

  import { untrack } from 'svelte';
  import { Button, Checkbox, Dialog, IconButton, Inline, Input, Select, Stack, Switch } from '@hubtask/design-system/components';
  import type { EntryType } from '@hubtask/sync-engine';

  import { customFields } from '../data/customfields.svelte.ts';
  import { valueFor } from '../data/customfields.ts';
  import {
    FORM_CHANNELS, addableKeys, channelIn, customKeyOf, draftOfForm, formKeyOfCustom, formOfDraft, moveField, newField,
    refusalsByField, removeField, removeSection, type FormDraft,
  } from '../data/entryforms.ts';
  import { entryTypes, keyFrom } from '../data/entrytypes.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    type: EntryType;
    name: string;
    isOpen: boolean;
    onClose: () => void;
  }

  let { type, name, isOpen = $bindable(), onClose }: Props = $props();

  $effect(() => untrack(() => customFields.open()));

  const definitions = $derived(customFields.of());
  let draft = $state<FormDraft>(untrack(() => draftOfForm(type.form)));
  let adding = $state('');
  let otherKey = $state('');
  let newSection = $state('');
  let isSaving = $state(false);
  let refusal = $state<string | undefined>(undefined);
  let refusals = $state<ReadonlyMap<string, string>>(new Map());

  /** A field's name as a reader knows it: the entry's own words, or the custom field's key. */
  const labelOf = (key: string) => customKeyOf(key) ?? t(`app.entry_forms.own_${key}`);

  const addable = $derived(addableKeys(draft, definitions.map((definition) => definition.key)));
  const addOptions = $derived([
    { value: '', label: t('app.entry_forms.add_choose') },
    ...addable.map((key) => ({ value: key, label: labelOf(key) })),
  ]);
  const sectionOptions = $derived([
    { value: '', label: t('app.entry_forms.no_section') },
    ...draft.sections.map((section) => ({ value: section.key, label: section.name || section.key })),
  ]);
  const fieldOptions = $derived([
    { value: '', label: t('app.entry_forms.whole_entry') },
    ...draft.fields.map((field) => ({ value: field.key, label: labelOf(field.key) })),
  ]);

  function add(key: string) {
    if (key === '' || draft.fields.some((field) => field.key === key)) return;
    draft.fields.push(newField(key));
    adding = '';
  }

  function addOther() {
    const key = keyFrom(otherKey);
    if (key === '') return;
    add(formKeyOfCustom(key));
    otherKey = '';
  }

  function addSection() {
    const key = keyFrom(newSection);
    if (key === '' || draft.sections.some((section) => section.key === key)) return;
    draft.sections.push({ key, name: newSection.trim() });
    newSection = '';
  }

  function addValidation() {
    draft.validations.push({ key: `check_${draft.validations.length + 1}`, field: '', holds: '', message: '' });
  }

  /** What a typed default or preset becomes: the custom field's kind decides, a list split by commas. */
  function shape(key: string, typed: string): unknown {
    const custom = customKeyOf(key);
    const kind = definitions.find((definition) => definition.key === custom)?.kind as string | undefined;
    if (!kind) return typed.trim();
    const listed = kind === 'MULTI_SELECT' || kind === 'USER_SET' ? typed.split(',').map((each) => each.trim()).filter(Boolean) : typed;
    return valueFor(kind, listed);
  }

  function toggleHidden(channel: (typeof FORM_CHANNELS)[number], key: string, hidden: boolean) {
    const rule = channelIn(draft, channel);
    rule.hidden = hidden ? [...rule.hidden, key] : rule.hidden.filter((each) => each !== key);
    if (!hidden) delete rule.presets[key];
  }

  async function save() {
    isSaving = true;
    refusal = undefined;
    refusals = new Map();
    try {
      await entryTypes.setForm(type, formOfDraft(draft, shape));
      toast.show({ text: t('app.entry_forms.saved', { name }), tone: 'success' });
      isOpen = false;
    } catch (error) {
      const rendered = renderProblem(error as never, messages);
      refusal = rendered.message;
      // The use case names its input `form`, the domain the form's own parts: both land the same.
      refusals = new Map([...refusalsByField(rendered.fields)].map(([key, message]) => [key.replace(/^form\./, ''), message]));
    } finally {
      isSaving = false;
    }
  }

  /** A refusal of a part of the form: the server names `/fields/<n>/<part>` and the like. */
  const refusalAt = (path: string) => refusals.get(path);
  const channelRefusal = $derived([...refusals].find(([key]) => key.startsWith('channels'))?.[1]);
</script>

<Dialog bind:isOpen title={t('app.entry_forms.title', { name })} dismissLabel={t('app.entry_types.cancel')} isWide {onClose}>
  <Stack gap="300">
    <p class="quiet">{t('app.entry_forms.intro')}</p>

    <section class="part" data-form-sections>
      <h3 class="heading">{t('app.entry_forms.sections')}</h3>
      <Stack gap="100">
        {#each draft.sections as section, index (section.key)}
          <Inline gap="100" align="end">
            <Input label={t('app.entry_forms.section_name', { key: section.key })} size="sm" bind:value={section.name} error={refusalAt(`sections.${index}.name`)} />
            <IconButton icon="trash" size="sm" tone="subtle" label={t('app.entry_forms.remove_section', { name: section.name })} onclick={() => removeSection(draft, section.key)} />
          </Inline>
        {/each}
        <Inline gap="100" align="end">
          <Input label={t('app.entry_forms.new_section')} size="sm" bind:value={newSection} />
          <Button size="sm" tone="secondary" icon="plus" data-add-section onclick={addSection}>{t('app.entry_forms.add_section')}</Button>
        </Inline>
      </Stack>
    </section>

    <section class="part" data-form-fields>
      <h3 class="heading">{t('app.entry_forms.fields')}</h3>
      {#if draft.fields.length === 0}<p class="quiet">{t('app.entry_forms.fields_none')}</p>{/if}
      <ol class="fields">
        {#each draft.fields as field, index (field.key)}
          {@const custom = customKeyOf(field.key)}
          <li class="field" data-form-field={field.key}>
            <Stack gap="100">
              <Inline gap="100" align="center" justify="between">
                <strong>{labelOf(field.key)}</strong>
                <Inline gap="050" align="center">
                  <IconButton icon="chevron-up" size="sm" tone="subtle" label={t('app.entry_forms.move_up', { name: labelOf(field.key) })}
                    disabledReason={index === 0 ? t('app.entry_types.first') : undefined} onclick={() => moveField(draft, field.key, -1)} />
                  <IconButton icon="chevron-down" size="sm" tone="subtle" label={t('app.entry_forms.move_down', { name: labelOf(field.key) })}
                    disabledReason={index === draft.fields.length - 1 ? t('app.entry_types.last') : undefined} onclick={() => moveField(draft, field.key, 1)} />
                  <IconButton icon="trash" size="sm" tone="subtle" label={t('app.entry_forms.remove_field', { name: labelOf(field.key) })} onclick={() => removeField(draft, field.key)} />
                </Inline>
              </Inline>
              <Inline gap="150" align="end">
                <Select label={t('app.entry_forms.section')} size="sm" options={sectionOptions} bind:value={field.section} />
                <Switch label={t('app.entry_forms.required')} bind:checked={field.required} />
              </Inline>
              <Input label={t('app.entry_forms.required_when')} hint={t('app.entry_forms.condition_hint')} size="sm" bind:value={field.required_when} error={refusalAt(`fields.${index}.required_when`)} />
              <Input label={t('app.entry_forms.read_only_when')} size="sm" bind:value={field.read_only_when} error={refusalAt(`fields.${index}.read_only_when`)} />
              {#if field.key !== 'title'}
                <Input label={t('app.entry_forms.hidden_when')} size="sm" bind:value={field.hidden_when} error={refusalAt(`fields.${index}.hidden_when`)} />
              {/if}
              {#if custom !== undefined}
                <Input label={t('app.entry_forms.default')} size="sm" bind:value={field.default} error={refusalAt(`fields.${index}.default`)} />
              {/if}
              <Input label={t('app.entry_forms.help')} size="sm" bind:value={field.help} />
              {#if refusalAt(`fields.${index}`) ?? refusalAt(`fields.${index}.key`)}
                <p class="refusal" role="alert">{refusalAt(`fields.${index}`) ?? refusalAt(`fields.${index}.key`)}</p>
              {/if}
            </Stack>
          </li>
        {/each}
      </ol>
      <Inline gap="100" align="end">
        <Select label={t('app.entry_forms.add_field')} size="sm" options={addOptions} bind:value={adding} />
        <Button size="sm" tone="secondary" icon="plus" data-add-field onclick={() => add(adding)}>{t('app.entry_forms.add')}</Button>
      </Inline>
      <Inline gap="100" align="end">
        <Input label={t('app.entry_forms.other_key')} hint={t('app.entry_forms.other_key_hint')} size="sm" bind:value={otherKey} />
        <Button size="sm" tone="subtle" icon="plus" onclick={addOther}>{t('app.entry_forms.add')}</Button>
      </Inline>
    </section>

    <section class="part" data-form-validations>
      <h3 class="heading">{t('app.entry_forms.validations')}</h3>
      <p class="quiet">{t('app.entry_forms.validations_intro')}</p>
      {#each draft.validations as validation, index (index)}
        <div class="field" data-form-validation={validation.key}>
          <Stack gap="100">
            <Inline gap="100" align="end">
              <Input label={t('app.entry_forms.validation_key')} size="sm" bind:value={validation.key} error={refusalAt(`validations.${index}.key`)} />
              <Select label={t('app.entry_forms.validation_field')} size="sm" options={fieldOptions} bind:value={validation.field} />
              <IconButton icon="trash" size="sm" tone="subtle" label={t('app.entry_forms.remove_validation', { key: validation.key })}
                onclick={() => (draft.validations = draft.validations.filter((_, at) => at !== index))} />
            </Inline>
            <Input label={t('app.entry_forms.validation_holds')} hint={t('app.entry_forms.condition_hint')} size="sm" bind:value={validation.holds} error={refusalAt(`validations.${index}.holds`)} />
            <Input label={t('app.entry_forms.validation_message')} size="sm" bind:value={validation.message} />
          </Stack>
        </div>
      {/each}
      <div><Button size="sm" tone="secondary" icon="plus" data-add-validation onclick={addValidation}>{t('app.entry_forms.add_validation')}</Button></div>
    </section>

    <section class="part" data-form-channels>
      <h3 class="heading">{t('app.entry_forms.channels')}</h3>
      <p class="quiet">{t('app.entry_forms.channels_intro')}</p>
      {#each FORM_CHANNELS as channel (channel)}
        {@const rule = draft.channels.find((each) => each.channel === channel)}
        <div class="field" data-form-channel={channel}>
          <Stack gap="100">
            <strong>{t(`app.entry_forms.channel_${channel}`)}</strong>
            <Switch label={t('app.entry_forms.channel_closed')} checked={rule?.closed ?? false}
              onchange={(event) => (channelIn(draft, channel).closed = (event.currentTarget as HTMLInputElement).checked)} />
            {#if draft.fields.some((field) => field.key !== 'title')}
              <Inline gap="150">
                {#each draft.fields.filter((field) => field.key !== 'title') as field (field.key)}
                  <Checkbox density="sm" label={t('app.entry_forms.channel_hides', { name: labelOf(field.key) })}
                    checked={rule?.hidden.includes(field.key) ?? false}
                    onchange={(event) => toggleHidden(channel, field.key, (event.currentTarget as HTMLInputElement).checked)} />
                {/each}
              </Inline>
              {#each rule?.hidden ?? [] as key (key)}
                <Input label={t('app.entry_forms.channel_preset', { name: labelOf(key) })} size="sm"
                  value={rule?.presets[key] ?? ''}
                  oninput={(event) => (channelIn(draft, channel).presets[key] = (event.currentTarget as HTMLInputElement).value)} />
              {/each}
            {/if}
          </Stack>
        </div>
      {/each}
      {#if channelRefusal}<p class="refusal" role="alert">{channelRefusal}</p>{/if}
    </section>

    {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.entry_types.cancel')}</Button>
    <Button tone="primary" data-save-form isBusy={isSaving} onclick={save}>{t('app.entry_forms.save')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .part { display: grid; gap: var(--sp-150); }

  .fields {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: var(--sp-150);
  }

  .field {
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
  }

  .refusal { margin: 0; color: var(--text-danger); }
</style>
