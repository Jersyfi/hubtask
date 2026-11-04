<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A type's form, designed under *How we work → Types* (UC-FLW-01 check 2, D-13): its sections, its
  // fields in order with required, read-only and hidden conditions, defaults and help, the checks
  // that must hold, and the channels that narrow it.
  //
  // **A draft, then one publish for the whole form** (ui.md A4). A form every door of the type is
  // held to is saved as a draft, which nobody else meets; publishing plans and applies it whole -
  // a channel may only hide a field the form holds, a condition may only name a field the type
  // has - and shows a refusal on the field it names.
  //
  // **Conditions are CEL, checked by the server.** No evaluator runs here (`entryforms.ts`); a
  // condition that does not compile comes back as a refusal naming where it sits.

  import { untrack } from 'svelte';
  import { Button, Checkbox, Dialog, Disclosure, IconButton, Inline, Input, Select, Stack, Switch } from '@hubtask/design-system/components';
  import type { EntryType } from '@hubtask/sync-engine';

  import { customFields } from '../data/customfields.svelte.ts';
  import { fieldName, valueFor } from '../data/customfields.ts';
  import {
    FORM_CHANNELS, OPT_IN_CHANNELS, addableKeys, channelIn, customKeyOf, draftOfForm, formKeyOfCustom, formOfDraft, moveField, newField,
    isConditional, refusalsByField, removeField, removeSection, type FormDraft,
  } from '../data/entryforms.ts';
  import { keyFrom } from '../data/entrytypes.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import DraftPublish from '../settings/DraftPublish.svelte';
  import ConditionPicker from './ConditionPicker.svelte';

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
  let refusals = $state<ReadonlyMap<string, string>>(new Map());

  /** A field's name as a reader knows it: the entry's own words, or the custom field's key. */
  const labelOf = (key: string) => {
    const custom = customKeyOf(key);
    return custom === undefined ? t(`app.entry_forms.own_${key}`) : fieldName(definitions.find((definition) => definition.key === custom), custom);
  };

  const addable = $derived(addableKeys(draft, definitions.map((definition) => definition.key)));
  const addOptions = $derived([
    { value: '', label: t('app.entry_forms.add_choose') },
    ...addable.map((key) => ({ value: key, label: labelOf(key) })),
  ]);
  const sectionOptions = $derived([
    { value: '', label: t('app.entry_forms.no_section') },
    ...draft.sections.map((section) => ({ value: section.key, label: section.name || section.key })),
  ]);
  /** The custom fields of the form a condition may name, with their closed values (UX-4). */
  const conditionFields = $derived(draft.fields.flatMap((field) => {
    const custom = customKeyOf(field.key);
    if (custom === undefined || !/^[A-Za-z_][A-Za-z0-9_]*$/.test(custom)) return [];
    const definition = definitions.find((each) => each.key === custom);
    return [{ key: custom, label: labelOf(field.key), options: definition?.options as readonly string[] | undefined }];
  }));
  const conditionsOpen = (index: number) => ['required_when', 'read_only_when', 'hidden_when'].some((part) => refusalAt(`fields.${index}.${part}`) !== undefined);

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
    const listed = kind === 'MULTI_SELECT' || kind === 'USER_SET' || kind === 'LABEL' ? typed.split(',').map((each) => each.trim()).filter(Boolean) : typed;
    return valueFor(kind, listed);
  }

  /** Every door the editor draws: the ones open by default, then the ones a form opens. */
  const doors = [...FORM_CHANNELS, ...OPT_IN_CHANNELS] as const;
  const optsIn = (channel: string) => (OPT_IN_CHANNELS as readonly string[]).includes(channel);

  function toggleHidden(channel: (typeof doors)[number], key: string, hidden: boolean) {
    const rule = channelIn(draft, channel);
    rule.hidden = hidden ? [...rule.hidden, key] : rule.hidden.filter((each) => each !== key);
    if (!hidden) delete rule.presets[key];
  }

  /** A refused publish's fields: the use case names its input `form`, the domain the form's own parts. */
  function showRefusals(fields: Parameters<typeof refusalsByField>[0]) {
    refusals = new Map([...refusalsByField(fields)].map(([key, message]) => [key.replace(/^form\./, ''), message]));
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
            <Input label={t('app.entry_forms.section_name', { number: index + 1 })} size="sm" bind:value={section.name} error={refusalAt(`sections.${index}.name`)} />
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
              <Disclosure tone="quiet" label={t('app.entry_forms.conditions')}
                description={isConditional(field) ? t('app.entry_forms.conditions_set') : t('app.entry_forms.conditions_none')}
                isOpen={conditionsOpen(index)}>
                <Stack gap="150">
                  <ConditionPicker label={t('app.entry_forms.required_when')} fields={conditionFields} bind:value={field.required_when} error={refusalAt(`fields.${index}.required_when`)} />
                  <ConditionPicker label={t('app.entry_forms.read_only_when')} fields={conditionFields} bind:value={field.read_only_when} error={refusalAt(`fields.${index}.read_only_when`)} />
                  {#if field.key !== 'title'}
                    <ConditionPicker label={t('app.entry_forms.hidden_when')} fields={conditionFields} bind:value={field.hidden_when} error={refusalAt(`fields.${index}.hidden_when`)} />
                  {/if}
                </Stack>
              </Disclosure>
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
      {#each doors as channel (channel)}
        {@const rule = draft.channels.find((each) => each.channel === channel)}
        <div class="field" data-form-channel={channel}>
          <Stack gap="100">
            <strong>{t(`app.entry_forms.channel_${channel}`)}</strong>
            {#if optsIn(channel)}
              <!-- Closed until named: the switch opens it, and turning it off closes it again. -->
              <Switch label={t('app.entry_forms.channel_opened')} checked={rule !== undefined && !rule.closed}
                onchange={(event) => (channelIn(draft, channel).closed = !(event.currentTarget as HTMLInputElement).checked)} />
            {:else}
              <Switch label={t('app.entry_forms.channel_closed')} checked={rule?.closed ?? false}
                onchange={(event) => (channelIn(draft, channel).closed = (event.currentTarget as HTMLInputElement).checked)} />
            {/if}
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

    <DraftPublish section="types" key={type.key} block={() => ({ form: formOfDraft(draft, shape) })}
      onLoad={(saved) => { if (saved.form) draft = draftOfForm(saved.form as EntryType['form']); }}
      onPublished={() => (isOpen = false)} onProblem={showRefusals} />
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.entry_types.cancel')}</Button>
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
