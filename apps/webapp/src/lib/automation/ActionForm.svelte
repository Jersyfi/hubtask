<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An action's settings, built from what the manifest declares for its kind (F8-04, decision 2).
  //
  // The manifest's `action_fields` is the use case's own declaration - the same the MCP tool
  // schema is derived from - so a field is rendered by its kind: an identifier whose name the
  // reference table knows becomes a picker over the client's own store of that kind, an enum a
  // select, a boolean a checkbox, an integer a number, a list of identifiers one per line, and a
  // document as JSON. Nothing here knows any action by name.
  //
  // What a rule leaves out, the run supplies (automation.md §2.2): `required` describes the call,
  // not the rule, so a required field is marked and never enforced here.

  import { Checkbox, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';

  import { REFERENCE } from './words.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  /** One declared field, as the manifest answers it. */
  export interface Field {
    readonly name: string;
    readonly kind: string;
    readonly required: boolean;
    readonly enum?: readonly string[];
    readonly description?: string;
  }

  /** What an identifier may point at, offered by name. */
  export type Choice = { readonly value: string; readonly label: string };

  interface Props {
    fields: readonly Field[];
    params: Record<string, unknown>;
    /** The choices per reference kind the client can offer here: `label`, `bucket`, `group`, … */
    pickers: Readonly<Record<string, readonly Choice[]>>;
    /** The server's refusals, by the field's name. */
    errors?: ReadonlyMap<string, string>;
    onchange: (params: Record<string, unknown>) => void;
  }

  const { fields, params, pickers, errors, onchange }: Props = $props();


  /** The JSON of a document field as it is being typed: a half-written object is still visible. */
  let texts = $state<Record<string, string>>({});

  const set = (name: string, value: unknown): void => onchange({ ...params, [name]: value });

  const label = (field: Field): string => field.name.replace(/_/g, ' ');

  const hint = (field: Field): string | undefined => {
    const parts = [field.required ? t('app.flow.field_required') : '', field.description ?? ''].filter(Boolean);
    return parts.length > 0 ? parts.join(' — ') : undefined;
  };

  function documentText(field: Field): string {
    const typed = texts[field.name];
    if (typed !== undefined) return typed;
    const value = params[field.name];
    return value === undefined ? '' : JSON.stringify(value, null, 2);
  }

  function setDocument(field: Field, text: string): void {
    texts = { ...texts, [field.name]: text };
    if (text.trim() === '') {
      set(field.name, undefined);
      return;
    }
    try {
      set(field.name, JSON.parse(text));
    } catch {
      // Left for the server to refuse by name; the text stays on screen as typed.
    }
  }
</script>

{#if fields.length === 0}
  <p class="quiet">{t('app.flow.action_no_fields')}</p>
{:else}
  <Stack gap="150">
    {#each fields as field (field.name)}
      {@const reference = REFERENCE[field.name]}
      {@const choices: readonly Choice[] = reference ? (pickers[reference] ?? []) : []}
      {@const error = errors?.get(field.name)}
      {#if field.kind === 'id' && reference}
        <Select
          label={label(field)}
          hint={choices.length > 0 ? hint(field) : t('app.flow.field_pick_none')}
          {error}
          placeholder={t('app.flow.field_pick')}
          value={String(params[field.name] ?? '')}
          options={[
            ...choices,
            ...(params[field.name] && !choices.some((choice) => choice.value === params[field.name])
              ? [{ value: String(params[field.name]), label: String(params[field.name]) }]
              : []),
          ]}
          onchange={(event: Event) => set(field.name, (event.currentTarget as HTMLSelectElement).value || undefined)}
        />
      {:else if field.enum && field.enum.length > 0}
        <Select
          label={label(field)}
          hint={hint(field)}
          {error}
          placeholder={t('app.flow.field_pick')}
          value={String(params[field.name] ?? '')}
          options={field.enum.map((value) => ({ value, label: value }))}
          onchange={(event: Event) => set(field.name, (event.currentTarget as HTMLSelectElement).value || undefined)}
        />
      {:else if field.kind === 'boolean'}
        <Checkbox
          label={label(field)}
          hint={hint(field)}
          checked={Boolean(params[field.name])}
          onchange={(event: Event) => set(field.name, (event.currentTarget as HTMLInputElement).checked)}
        />
      {:else if field.kind === 'integer'}
        <Input
          label={label(field)}
          hint={hint(field)}
          {error}
          type="number"
          value={params[field.name] === undefined ? '' : String(params[field.name])}
          oninput={(event: Event) => {
            const raw = (event.currentTarget as HTMLInputElement).value;
            set(field.name, raw === '' ? undefined : Number(raw));
          }}
        />
      {:else if field.kind === 'id_list'}
        <Textarea
          label={label(field)}
          hint={t('app.flow.field_ids')}
          {error}
          rows={3}
          value={Array.isArray(params[field.name]) ? (params[field.name] as string[]).join('\n') : ''}
          oninput={(event: Event) => {
            const lines = (event.currentTarget as HTMLTextAreaElement).value.split('\n').map((line) => line.trim()).filter(Boolean);
            set(field.name, lines.length > 0 ? lines : undefined);
          }}
        />
      {:else if field.kind === 'object' || field.kind === 'list' || field.kind === 'any'}
        <Textarea
          label={`${label(field)} — ${t('app.flow.field_json')}`}
          hint={hint(field) ?? t('app.flow.field_json_hint')}
          {error}
          rows={4}
          spellcheck={false}
          value={documentText(field)}
          oninput={(event: Event) => setDocument(field, (event.currentTarget as HTMLTextAreaElement).value)}
        />
      {:else}
        <Input
          label={label(field)}
          hint={field.name.includes('secret') ? t('app.flow.field_secret_kept') : (hint(field) ?? (field.kind === 'id' ? t('app.flow.field_id') : undefined))}
          {error}
          value={params[field.name] === undefined ? '' : String(params[field.name])}
          oninput={(event: Event) => {
            const raw = (event.currentTarget as HTMLInputElement).value;
            set(field.name, raw === '' ? undefined : raw);
          }}
        />
      {/if}
    {/each}
    <p class="quiet">{t('app.flow.action_run_supplies')}</p>
  </Stack>
{/if}

<style>
  .quiet { margin: 0; font-size: var(--fs-075); color: var(--text-subtle); }
</style>
