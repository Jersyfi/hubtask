<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What the target type's form asks for, under capture's line (UC-FLW-01 check 2, D-13): the fields
  // the form requires and the ones a condition may require, unfolded as the line is typed.
  //
  // **The server answers the conditions.** While something changes, the draft is sent to
  // `form:evaluate` and the fields are drawn as the answer says; the create is held to the same
  // answer, and its refusal lands here on the field it names. Without an answer (offline, or before
  // the first one arrives) the form's own `required` is drawn and the server decides on arrival.

  import { untrack } from 'svelte';
  import { CustomFieldRenderer, Stack, Textarea } from '@hubtask/design-system/components';
  import type { FieldValue } from '@hubtask/design-system/components';
  import type { EntryFormState, EntryType } from '@hubtask/sync-engine';

  import { customFields } from '../data/customfields.svelte.ts';
  import { definitionsFor, valueFor } from '../data/customfields.ts';
  import { customKeyOf, standingOf, unfoldedIn } from '../data/entryforms.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    type: EntryType;
    /** The level the entry is created on, whose definitions it carries. */
    level: string;
    collectionId: string;
    title: string;
    values: { notes: string; custom: Record<string, unknown> };
    /** The server's answer, for capture to refuse before it sends. */
    answer?: EntryFormState;
    /** A refused create's messages by form key. */
    refusals: ReadonlyMap<string, string>;
  }

  let { type, level, collectionId, title, values = $bindable(), answer = $bindable(), refusals }: Props = $props();

  $effect(() => {
    const id = collectionId;
    return untrack(() => customFields.open(id));
  });

  const definitions = $derived(definitionsFor(customFields.of(collectionId), level));
  const fields = $derived(
    unfoldedIn(type.form, answer).filter((field) => customKeyOf(field.key) !== undefined || field.key === 'notes'),
  );

  // A new answer a moment after the last change; one that arrives late for an older draft is dropped.
  let asked = 0;
  $effect(() => {
    const draft = {
      collection_id: collectionId, title, notes: values.notes,
      custom_fields: $state.snapshot(values.custom) as Record<string, unknown>,
    };
    const key = type.key;
    const mine = ++asked;
    const timer = setTimeout(() => {
      entryTypes.evaluate(key, draft).then(
        (answered) => { if (mine === asked) answer = answered; },
        () => { if (mine === asked) answer = undefined; },
      );
    }, 250);
    return () => clearTimeout(timer);
  });

  function drawn(key: string, required: boolean) {
    const definition = definitions.find((each) => each.key === key);
    const help = type.form?.fields?.find((field) => field.key === `custom_fields.${key}`)?.help;
    return {
      key,
      label: key,
      kind: (definition?.kind as string | undefined) ?? 'TEXT',
      options: (definition?.options ?? []) as readonly string[],
      isRequired: required,
      ...(help ? { hint: help } : {}),
    };
  }
</script>

{#if fields.length > 0}
  <section class="form" data-capture-form={type.key} aria-label={t('app.capture.form_heading', { type: type.name ?? type.key })}>
    <h3 class="heading">{t('app.capture.form_heading', { type: type.name ?? type.key })}</h3>
    <Stack gap="100">
      {#each fields as field (field.key)}
        {@const standing = standingOf(field, answer)}
        {@const custom = customKeyOf(field.key)}
        <div data-capture-field={field.key}>
          {#if custom !== undefined}
            {@const shape = drawn(custom, standing.required)}
            <CustomFieldRenderer
              definition={shape}
              value={(values.custom[custom] ?? null) as FieldValue}
              unknownKindLabel={t('app.fields.unknown_kind')}
              emptyValueLabel={t('app.fields.empty_value')}
              onChange={(next) => (values.custom = { ...values.custom, [custom]: valueFor(shape.kind, next) })}
            />
          {:else if field.key === 'notes'}
            <Textarea
              label={standing.required ? t('app.entry_forms.required_label', { name: t('app.entry_forms.own_notes') }) : t('app.entry_forms.own_notes')}
              rows={2}
              bind:value={values.notes}
              hint={field.help}
            />
          {/if}
          {#if refusals.get(field.key)}<p class="refusal" role="alert">{refusals.get(field.key)}</p>{/if}
        </div>
      {/each}
    </Stack>
  </section>
{/if}

<style>
  .form {
    display: grid;
    gap: var(--sp-100);
    padding-block-start: var(--sp-150);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  .heading { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-semibold); color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
