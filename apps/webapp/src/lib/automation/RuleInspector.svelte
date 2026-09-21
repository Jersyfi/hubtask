<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The inspector: what the selected piece of the canvas is, and its settings (F8-04).
  //
  // One panel per kind of selection. It never holds the draft - every edit goes back through
  // `onupdate` with a function over the draft, so that the canvas and the head redraw from the
  // one copy the view holds. The server's refusals arrive by pointer and are shown at the field
  // they name, as the old form showed them.

  import { Button, Callout, Checkbox, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';

  import ActionForm, { type Choice, type Field } from './ActionForm.svelte';
  import Composer from './Composer.svelte';
  import { pointerOf, replaceAt, stepAt, type Draft } from './model.ts';
  import type { Selection } from './selection.ts';
  import { eventGroups, kindWord } from './words.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    draft: Draft;
    selection: Selection;
    /** The *Rule* tab (decision 16): the name, where it applies, whose rights it acts with, the guardrails - together. */
    section?: 'rule';
    /** Whether the rule has been saved at least once: what a manual trigger's button needs. */
    ruleId?: string;
    generatedName: string;
    /** What the manifest serves. */
    triggers: readonly string[];
    eventTypes: readonly string[];
    actionFields: Readonly<Record<string, readonly Field[]>>;
    /** The choices the workspace offers. */
    scopes: readonly Choice[];
    runners: readonly Choice[];
    pickers: Readonly<Record<string, readonly Choice[]>>;
    itemTypes: readonly Choice[];
    /** The server's refusals, by JSON pointer into the rule's document. */
    errors: ReadonlyMap<string, string>;
    onupdate: (change: (draft: Draft) => Draft) => void;
    onremovestep: (path: string) => void;
    onremovecondition: (index: number) => void;
    onaddcondition: () => void;
    /** The inbound address and the manual start, both the view's to perform. */
    inbound?: { rotatedAt?: string; onmint: () => void; isWorking: boolean };
    onstart?: () => void;
  }

  const {
    draft,
    selection,
    section,
    ruleId,
    generatedName,
    triggers,
    eventTypes,
    actionFields,
    scopes,
    runners,
    pickers,
    itemTypes,
    errors,
    onupdate,
    onremovestep,
    onremovecondition,
    onaddcondition,
    inbound,
    onstart,
  }: Props = $props();

  const words = { t, has: (code: string) => messages.has(code) };

  const CHANGED_FIELDS = ['title', 'notes', 'due_at', 'start_at', 'assignee_id', 'bucket_id', 'parent_id', 'completed'];

  const composerChoices = $derived({
    types: itemTypes,
    buckets: pickers.bucket ?? [],
    accounts: pickers.account ?? [],
  });

  const value = (event: Event): string => (event.currentTarget as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement).value;

  /** The refusals at one step's parameters, by field name. */
  function errorsAt(path: string): ReadonlyMap<string, string> {
    const prefix = pointerOf(path, 'params/');
    const found = new Map<string, string>();
    for (const [pointer, message] of errors) {
      if (pointer.startsWith(prefix)) found.set(pointer.slice(prefix.length), message);
    }
    return found;
  }

  const step = $derived(selection.kind === 'step' ? stepAt(draft.actions, selection.path) : undefined);
</script>

{#snippet nameSection()}
    <h3>{t('app.flow.inspector_rule')}</h3>
    <Input
      label={t('app.flow.name_own')}
      hint={t('app.flow.name_automatic_hint')}
      error={errors.get('/name')}
      value={draft.name}
      placeholder={generatedName}
      oninput={(event: Event) => {
        const name = value(event);
        onupdate((current) => ({ ...current, name }));
      }}
    />
{/snippet}
{#snippet scopeSection()}
    <h3>{t('app.flow.scope')}</h3>
    <Select
      label={t('app.rules.scope')}
      hint={t('app.rules.scope_hint')}
      error={errors.get('/scope')}
      value={draft.scope.id ? `${draft.scope.type}:${draft.scope.id}` : draft.scope.type}
      options={scopes}
      onchange={(event: Event) => {
        const [type, id] = value(event).split(':');
        onupdate((current) => ({ ...current, scope: { type: type ?? 'TENANT', ...(id ? { id } : {}) } }));
      }}
    />
{/snippet}
{#snippet runAsSection()}
    <h3>{t('app.flow.runs_as')}</h3>
    <Select
      label={t('app.rules.runs_as')}
      hint={t('app.flow.run_as_hint')}
      error={errors.get('/run_as')}
      placeholder={t('app.rules.choose_runner')}
      value={draft.runAs}
      options={runners}
      onchange={(event: Event) => {
        const runAs = value(event);
        onupdate((current) => ({ ...current, runAs }));
      }}
    />
{/snippet}
{#snippet guardrailsSection()}
    <h3>{t('app.flow.card_guardrails')}</h3>
    <Select
      label={t('app.rules.on_error')}
      hint={t('app.rules.on_error_hint')}
      value={draft.onError}
      options={[
        { value: 'STOP', label: t('app.rules.on_error_stop') },
        { value: 'CONTINUE', label: t('app.rules.on_error_continue') },
        { value: 'RETRY', label: t('app.rules.on_error_retry') },
      ]}
      onchange={(event: Event) => {
        const onError = value(event);
        onupdate((current) => ({ ...current, onError }));
      }}
    />
    <Input
      label={t('app.rules.max_runs')}
      hint={t('app.rules.max_runs_hint')}
      error={errors.get('/throttle/max_runs_per_hour')}
      type="number"
      value={draft.throttle.maxRunsPerHour ? String(draft.throttle.maxRunsPerHour) : ''}
      oninput={(event: Event) => {
        const raw = value(event);
        onupdate((current) => ({ ...current, throttle: { ...current.throttle, maxRunsPerHour: raw ? Number(raw) : undefined } }));
      }}
    />
    <Textarea
      label={t('app.rules.dedupe')}
      hint={t('app.rules.dedupe_hint')}
      error={errors.get('/throttle/dedupe_key_expr')}
      rows={2}
      spellcheck={false}
      value={draft.throttle.dedupeKeyExpr ?? ''}
      oninput={(event: Event) => {
        const raw = value(event);
        onupdate((current) => ({ ...current, throttle: { ...current.throttle, dedupeKeyExpr: raw || undefined } }));
      }}
    />
    <Callout>{t('app.flow.guardrails_hint')}</Callout>
{/snippet}

<div class="panel">
  {#if section === 'rule'}
    <p class="quiet">{t('app.flow.rule_tab_hint')}</p>
    {@render nameSection()}
    {@render scopeSection()}
    {@render runAsSection()}
    {@render guardrailsSection()}
  {:else if selection.kind === 'rule'}
    {@render nameSection()}
  {:else if selection.kind === 'trigger'}
    <h3>{t('app.flow.card_starts_on')}</h3>
    <Select
      label={t('app.flow.trigger_kind')}
      error={errors.get('/trigger/kind')}
      value={draft.trigger.kind}
      options={triggers.map((kind) => ({ value: kind, label: messages.has(`app.rules.trigger_${kind.toLowerCase()}`) ? t(`app.rules.trigger_${kind.toLowerCase()}`) : kind }))}
      onchange={(event: Event) => {
        const kind = value(event);
        onupdate((current) => ({ ...current, trigger: { kind, ...(kind === 'EVENT' ? { event_type: current.trigger.event_type ?? eventTypes[0] ?? '' } : {}) } }));
      }}
    />
    {#if draft.trigger.kind === 'EVENT'}
      <Select
        label={t('app.rules.event_type')}
        hint={draft.trigger.event_type ? t('app.flow.event_wire_name', { type: draft.trigger.event_type }) : t('app.rules.event_type_hint')}
        error={errors.get('/trigger/event_type')}
        placeholder={t('app.rules.choose_event')}
        value={draft.trigger.event_type ?? ''}
        options={[]}
        groups={eventGroups(words, eventTypes)}
        onchange={(event: Event) => {
          const event_type = value(event);
          onupdate((current) => ({ ...current, trigger: { ...current.trigger, event_type, changed_fields: [] } }));
        }}
      />
      {#if draft.trigger.event_type?.includes('.updated.')}
        <Stack gap="050">
          <span class="label">{t('app.flow.changed_fields')}</span>
          <span class="hint">{t('app.flow.changed_fields_hint')}</span>
          {#each CHANGED_FIELDS as field (field)}
            <Checkbox
              label={field.replace(/_/g, ' ')}
              checked={draft.trigger.changed_fields?.includes(field) ?? false}
              onchange={(event: Event) => {
                const checked = (event.currentTarget as HTMLInputElement).checked;
                onupdate((current) => {
                  const present = current.trigger.changed_fields ?? [];
                  const changed_fields = checked ? [...present, field] : present.filter((each) => each !== field);
                  return { ...current, trigger: { ...current.trigger, changed_fields } };
                });
              }}
            />
          {/each}
        </Stack>
      {/if}
    {:else if draft.trigger.kind === 'SCHEDULE'}
      <Input
        label={t('app.rules.rrule')}
        hint={t('app.flow.rrule_hint')}
        error={errors.get('/trigger/rrule')}
        value={draft.trigger.rrule ?? ''}
        oninput={(event: Event) => {
          const rrule = value(event);
          onupdate((current) => ({ ...current, trigger: { ...current.trigger, rrule } }));
        }}
      />
      <Input
        label={t('app.rules.timezone')}
        hint={t('app.rules.timezone_hint')}
        error={errors.get('/trigger/timezone')}
        value={draft.trigger.timezone ?? ''}
        oninput={(event: Event) => {
          const timezone = value(event);
          onupdate((current) => ({ ...current, trigger: { ...current.trigger, timezone } }));
        }}
      />
    {:else if draft.trigger.kind === 'RELATIVE_DATE'}
      <Input
        label={t('app.flow.relative_offset')}
        hint={t('app.flow.relative_offset_hint')}
        error={errors.get('/trigger/offset')}
        value={draft.trigger.offset ?? ''}
        oninput={(event: Event) => {
          const offset = value(event);
          onupdate((current) => ({ ...current, trigger: { ...current.trigger, offset } }));
        }}
      />
      <Select
        label={t('app.flow.relative_anchor')}
        error={errors.get('/trigger/anchor')}
        value={draft.trigger.anchor ?? 'DUE_DATE'}
        options={[
          { value: 'DUE_DATE', label: t('app.flow.anchor_due_date') },
          { value: 'CREATED_AT', label: t('app.flow.anchor_created_at') },
        ]}
        onchange={(event: Event) => {
          const anchor = value(event);
          onupdate((current) => ({ ...current, trigger: { ...current.trigger, anchor } }));
        }}
      />
      <Callout>{t('app.flow.relative_hint')}</Callout>
    {:else if draft.trigger.kind === 'INBOUND_WEBHOOK'}
      {#if inbound && ruleId}
        <Callout tone="warning">{t(inbound.rotatedAt ? 'app.rules.rotate_cost' : 'app.rules.rotate_first')}</Callout>
        <div>
          <Button tone={inbound.rotatedAt ? 'danger' : 'primary'} size="sm" isBusy={inbound.isWorking} busyLabel={t('app.rules.rotating')} onclick={inbound.onmint}>
            {t(inbound.rotatedAt ? 'app.rules.rotate' : 'app.rules.mint')}
          </Button>
        </div>
      {:else}
        <Callout>{t('app.rules.inbound_none')}</Callout>
      {/if}
    {:else if draft.trigger.kind === 'MANUAL'}
      <Callout>{t('app.flow.manual_hint')}</Callout>
      {#if onstart && ruleId}
        <div><Button size="sm" icon="play" onclick={onstart}>{t('app.flow.manual_button')}</Button></div>
      {/if}
    {:else}
      <Callout>{t('app.flow.jumble_hint')}</Callout>
    {/if}
    <Callout tone="info">{t('app.flow.trigger_all_six')}</Callout>
  {:else if selection.kind === 'scope'}
    {@render scopeSection()}
  {:else if selection.kind === 'runas'}
    {@render runAsSection()}
  {:else if selection.kind === 'gate'}
    <h3>{t('app.flow.card_only_when')}</h3>
    <p class="quiet">{t('app.flow.gate_hint')}</p>
    <div><Button size="sm" icon="plus" onclick={onaddcondition}>{t('app.flow.add_condition')}</Button></div>
    <Callout>{t('app.flow.gate_before_writes')}</Callout>
  {:else if selection.kind === 'condition'}
    {@const index = selection.index}
    <h3>{t('app.flow.condition_n', { n: index + 1 })}</h3>
    <Composer
      expr={draft.conditions[index] ?? ''}
      error={errors.get(`/conditions/${index}/expr`)}
      choices={composerChoices}
      onchange={(expr) => onupdate((current) => ({ ...current, conditions: current.conditions.map((each, at) => (at === index ? expr : each)) }))}
    />
    <div><Button size="sm" tone="subtle" icon="trash" onclick={() => onremovecondition(index)}>{t('app.rules.remove_condition')}</Button></div>
  {:else if selection.kind === 'step' && step}
    {@const path = selection.path}
    <h3>{kindWord(words, step.kind)}</h3>
    <span class="hint mono">{t('app.flow.kind_path', { kind: step.kind, path })}</span>
    {#if step.kind === 'BRANCH'}
      <span class="label">{t('app.flow.branch_condition')}</span>
      <Composer
        expr={String(step.params.condition ?? '')}
        error={errors.get(pointerOf(path, 'params/condition'))}
        choices={composerChoices}
        onchange={(condition) => onupdate((current) => ({ ...current, actions: replaceAt(current.actions, path, { ...step, params: { ...step.params, condition } }) }))}
      />
      <Callout>{t('app.flow.branch_hint')}</Callout>
    {:else if step.kind === 'STOP'}
      <Callout>{t('app.flow.stop_hint')}</Callout>
    {:else if step.kind === 'WAIT'}
      <Input
        label={t('app.rules.wait_duration')}
        hint={t('app.rules.wait_duration_hint')}
        error={errors.get(pointerOf(path, 'params/duration'))}
        value={String(step.params.duration ?? '')}
        oninput={(event: Event) => {
          const duration = value(event);
          onupdate((current) => ({ ...current, actions: replaceAt(current.actions, path, { ...step, params: { ...step.params, duration } }) }));
        }}
      />
      <Callout>{t('app.flow.wait_hint')}</Callout>
    {:else}
      {#if errors.get(pointerOf(path, 'kind'))}<Callout tone="danger">{errors.get(pointerOf(path, 'kind'))}</Callout>{/if}
      <ActionForm
        fields={actionFields[step.kind] ?? []}
        params={step.params}
        {pickers}
        errors={errorsAt(path)}
        onchange={(params) => onupdate((current) => ({ ...current, actions: replaceAt(current.actions, path, { ...step, params }) }))}
      />
    {/if}
    <div><Button size="sm" tone="subtle" icon="trash" onclick={() => onremovestep(path)}>{t('app.flow.remove_step')}</Button></div>
  {:else if selection.kind === 'guardrails'}
    {@render guardrailsSection()}
  {:else}
    <p class="quiet">{t('app.flow.inspector_nothing')}</p>
  {/if}
</div>

<style>
  .panel { display: flex; flex-direction: column; gap: var(--sp-200); padding: var(--sp-200); }

  h3 { margin: 0; font-family: var(--font-ui); font-size: var(--fs-200); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }

  .label { font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .hint { font-size: var(--fs-075); color: var(--text-subtle); }

  .mono { font-family: var(--font-mono); }
</style>
