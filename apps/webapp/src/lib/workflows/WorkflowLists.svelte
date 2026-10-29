<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workflow as two lists a keyboard reaches (UC-FLW-02 check 2): its statuses and its steps,
  // with every action the diagram offers and the step's who, requires, asks and checks
  // (UC-FLW-04 checks 1 to 3), and its *then*: the rules on the step, which run in its own change
  // (check 4) and are written in the rule editor from the same actions as every rule. Every edit is a function over the draft (lib/data/workflows.ts);
  // nothing is sent from here - the page saves the draft.

  import { tick, untrack } from 'svelte';
  import { Button, Checkbox, IconButton, Inline, Input, Select, Stack } from '@hubtask/design-system/components';
  import { labelTokens } from '@hubtask/design-system';
  import type { WorkflowShape, WorkflowStep } from '@hubtask/sync-engine';

  import {
    CATEGORIES, addStatus, addStep, changeStatus, changeStep, moveStatus, removeStatus, removeStep, statusLabel,
    stepLabel, type Category,
  } from '../data/workflows.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    shape: WorkflowShape;
    selected?: string;
    isReadOnly?: boolean;
    /** The rules on each step, by step key; absent where the reader may not write rules. */
    stepRules?: (step: string) => readonly { readonly id: string; readonly name: string; readonly enabled: boolean }[];
    onopenrule?: (id: string) => void;
    onaddrule?: (step: string) => void;
    onchange: (shape: WorkflowShape) => void;
    onselect: (selected: string) => void;
  }

  const { shape, selected, isReadOnly = false, stepRules, onopenrule, onaddrule, onchange, onselect }: Props = $props();

  const ROLES = ['OWNER', 'ADMIN', 'MEMBER', 'CONTRIBUTOR', 'GUEST'] as const;

  const categoryOptions = $derived(
    CATEGORIES.map((category) => ({ value: category, label: t(`workflows.category.${category.toLowerCase()}`) })),
  );
  const colourOptions = $derived(labelTokens.map((token) => ({ value: token, label: t(`app.colour.${token}`) })));
  const statusOptions = $derived(shape.statuses.map((status) => ({ value: status.key, label: statusLabel(status, t) })));
  const initial = $derived(shape.initial ?? shape.statuses.find((status) => status.category !== 'DONE')?.key);

  let list = $state<HTMLDivElement | undefined>(undefined);

  // A selection made in the diagram moves the focus to its row, so the keyboard carries on there.
  $effect(() => {
    const wanted = selected;
    if (!wanted || !list) return;
    void tick().then(() => {
      const row = list?.querySelector<HTMLElement>(`[data-row="${wanted}"]`);
      row?.scrollIntoView({ block: 'nearest' });
      row?.focus({ preventScroll: true });
    });
  });

  let newName = $state('');
  let newCategory = $state<string>('IN_PROGRESS');
  let newStepTo = $state('');

  function add() {
    if (!newName.trim()) return;
    onchange(addStatus(shape, newName.trim(), newCategory as Category));
    newName = '';
  }

  function addOne() {
    const to = newStepTo || shape.statuses[0]?.key;
    if (!to) return;
    const next = addStep(shape, [], to);
    onchange(next);
    const created = next.steps[next.steps.length - 1];
    if (created) onselect(`step:${created.key}`);
  }

  /** A list written as words separated by commas, as the contract's string lists. */
  const words = (value: string) => value.split(',').map((word) => word.trim()).filter((word) => word !== '');
  const joined = (value: readonly string[] | undefined) => (value ?? []).join(', ');

  function toggleFrom(step: WorkflowStep, key: string, on: boolean) {
    const from = new Set(step.from ?? []);
    if (on) from.add(key);
    else from.delete(key);
    onchange(changeStep(shape, step.key, { from: shape.statuses.map((status) => status.key).filter((each) => from.has(each)) }));
  }

  function toggleRole(step: WorkflowStep, role: string, on: boolean) {
    const roles = new Set<string>(step.who?.roles ?? []);
    if (on) roles.add(role);
    else roles.delete(role);
    // The shipped roles in their order, then the workspace's own keys the step already names.
    const ordered = [...ROLES.filter((each) => roles.has(each)), ...[...roles].filter((each) => !(ROLES as readonly string[]).includes(each))];
    onchange(changeStep(shape, step.key, { who: { ...step.who, roles: ordered as never } }));
  }

  // The workspace's groups, which a step may be limited to (UC-FLW-04 check 1).
  $effect(() => untrack(() => groups.open()));

  function toggleGroup(step: WorkflowStep, groupId: string, on: boolean) {
    const chosen = new Set(step.who?.groups ?? []);
    if (on) chosen.add(groupId);
    else chosen.delete(groupId);
    onchange(changeStep(shape, step.key, { who: { ...step.who, groups: [...chosen] } }));
  }

  function changeCheck(step: WorkflowStep, index: number, change: { holds?: string; message?: string | null }) {
    const checks = (step.checks ?? []).map((check, at) => (at === index ? { ...check, ...change } : check));
    onchange(changeStep(shape, step.key, { checks }));
  }

  function addCheck(step: WorkflowStep) {
    const taken = new Set((step.checks ?? []).map((check) => check.key));
    let n = (step.checks?.length ?? 0) + 1;
    while (taken.has(`check_${n}`)) n++;
    onchange(changeStep(shape, step.key, { checks: [...(step.checks ?? []), { key: `check_${n}`, holds: '', message: null }] }));
  }

  function removeCheck(step: WorkflowStep, index: number) {
    onchange(changeStep(shape, step.key, { checks: (step.checks ?? []).filter((_, at) => at !== index) }));
  }

  const checked = (event: Event) => (event.currentTarget as HTMLInputElement).checked;
  const valueOf = (event: Event) => (event.currentTarget as HTMLInputElement).value;
</script>

<div class="lists" bind:this={list}>
  <section aria-labelledby="workflow-statuses">
    <Stack gap="150">
      <h3 id="workflow-statuses" class="title">{t('app.workflow_designer.statuses')}</h3>
      <ul class="rows">
        {#each shape.statuses as status, index (status.key)}
          <li
            class="row"
            class:selected={selected === `status:${status.key}`}
            data-row={`status:${status.key}`}
            data-status-row={status.key}
            tabindex="-1"
          >
            <div class="fields">
              <Input
                label={t('app.workflow_designer.status_name')}
                value={status.name ?? ''}
                placeholder={statusLabel(status, t)}
                readonly={isReadOnly}
                onfocus={() => onselect(`status:${status.key}`)}
                onchange={(event: Event) => onchange(changeStatus(shape, status.key, { name: valueOf(event).trim() || null }))}
              />
              <Select
                label={t('app.workflow_designer.category')}
                options={categoryOptions}
                value={status.category}
                disabledReason={isReadOnly ? t('app.workflow_designer.read_only') : undefined}
                onchange={(event: Event) => onchange(changeStatus(shape, status.key, { category: valueOf(event) as Category }))}
              />
              <Select
                label={t('app.workflow_designer.colour')}
                options={colourOptions}
                value={status.color_token}
                disabledReason={isReadOnly ? t('app.workflow_designer.read_only') : undefined}
                onchange={(event: Event) => onchange(changeStatus(shape, status.key, { color_token: valueOf(event) }))}
              />
            </div>
            {#if !isReadOnly}
              <Inline gap="100" align="center">
                {#if status.category !== 'DONE'}
                  <Button
                    tone="subtle" size="sm" data-initial={status.key}
                    disabledReason={initial === status.key ? t('app.workflow_designer.is_initial') : undefined}
                    onclick={() => onchange({ ...shape, initial: status.key })}
                  >{initial === status.key ? t('app.workflow_designer.is_initial') : t('app.workflow_designer.make_initial')}</Button>
                {/if}
                <IconButton
                  icon="chevron-up" size="sm" tone="subtle" label={t('app.workflow_designer.move_up', { name: statusLabel(status, t) })}
                  disabledReason={index === 0 ? t('app.workflow_designer.first') : undefined}
                  onclick={() => onchange(moveStatus(shape, status.key, -1))}
                />
                <IconButton
                  icon="chevron-down" size="sm" tone="subtle" label={t('app.workflow_designer.move_down', { name: statusLabel(status, t) })}
                  disabledReason={index === shape.statuses.length - 1 ? t('app.workflow_designer.last') : undefined}
                  onclick={() => onchange(moveStatus(shape, status.key, 1))}
                />
                <IconButton
                  icon="trash" size="sm" tone="subtle" label={t('app.workflow_designer.remove_status', { name: statusLabel(status, t) })}
                  data-remove-status={status.key}
                  disabledReason={shape.statuses.length === 1 ? t('app.workflow_designer.last_status') : undefined}
                  onclick={() => onchange(removeStatus(shape, status.key))}
                />
              </Inline>
            {/if}
          </li>
        {/each}
      </ul>
      {#if !isReadOnly}
        <div class="add">
          <Input label={t('app.workflow_designer.new_status')} bind:value={newName} data-new-status-name />
          <Select label={t('app.workflow_designer.category')} options={categoryOptions} bind:value={newCategory} />
          <Button tone="secondary" size="sm" icon="plus" data-add-status onclick={add}>{t('app.workflow_designer.add_status')}</Button>
        </div>
      {/if}
    </Stack>
  </section>

  <section aria-labelledby="workflow-steps">
    <Stack gap="150">
      <h3 id="workflow-steps" class="title">{t('app.workflow_designer.steps')}</h3>
      {#if shape.steps.length === 0}<p class="quiet">{t('app.workflow_designer.no_steps')}</p>{/if}
      <ul class="rows">
        {#each shape.steps as step (step.key)}
          <li
            class="row step"
            class:selected={selected === `step:${step.key}`}
            data-row={`step:${step.key}`}
            data-step-row={step.key}
            tabindex="-1"
          >
            <div class="fields">
              <Input
                label={t('app.workflow_designer.step_name')}
                value={step.name ?? ''}
                placeholder={stepLabel(step, shape, t)}
                readonly={isReadOnly}
                onfocus={() => onselect(`step:${step.key}`)}
                onchange={(event: Event) => onchange(changeStep(shape, step.key, { name: valueOf(event).trim() || null }))}
              />
              <Select
                label={t('app.workflow_designer.step_to')}
                options={statusOptions}
                value={step.to}
                disabledReason={isReadOnly ? t('app.workflow_designer.read_only') : undefined}
                onchange={(event: Event) => onchange(changeStep(shape, step.key, { to: valueOf(event) }))}
              />
            </div>
            <fieldset class="group">
              <legend>{t('app.workflow_designer.step_from')}</legend>
              <p class="quiet">{t('app.workflow_designer.step_from_hint')}</p>
              <div class="choices">
                {#each shape.statuses.filter((status) => status.key !== step.to) as status (status.key)}
                  <Checkbox
                    label={statusLabel(status, t)}
                    checked={step.from?.includes(status.key) ?? false}
                    disabledReason={isReadOnly ? t('app.workflow_designer.read_only') : undefined}
                    onchange={(event: Event) => toggleFrom(step, status.key, checked(event))}
                  />
                {/each}
              </div>
            </fieldset>
            <fieldset class="group">
              <legend>{t('app.workflow_designer.who')}</legend>
              <div class="choices">
                {#each ROLES as role (role)}
                  <Checkbox
                    label={t(`app.people.role.${role.toLowerCase()}`)}
                    checked={step.who?.roles?.includes(role) ?? false}
                    disabledReason={isReadOnly ? t('app.workflow_designer.read_only') : undefined}
                    onchange={(event: Event) => toggleRole(step, role, checked(event))}
                  />
                {/each}
              </div>
              {#if groups.all.length > 0}
                <div class="choices" role="group" aria-label={t('app.workflow_designer.groups')}>
                  {#each groups.all as group (group.id)}
                    <Checkbox
                      label={group.name}
                      checked={step.who?.groups?.includes(group.id) ?? false}
                      disabledReason={isReadOnly ? t('app.workflow_designer.read_only') : undefined}
                      onchange={(event: Event) => toggleGroup(step, group.id, checked(event))}
                    />
                  {/each}
                </div>
              {/if}
              <Input
                label={t('app.workflow_designer.person_fields')}
                hint={t('app.workflow_designer.person_fields_hint')}
                value={joined(step.who?.person_fields)}
                readonly={isReadOnly}
                onchange={(event: Event) => onchange(changeStep(shape, step.key, { who: { ...step.who, person_fields: words(valueOf(event)) } }))}
              />
              <Input
                label={t('app.workflow_designer.who_condition')}
                hint={t('app.workflow_designer.condition_hint')}
                value={step.who?.condition ?? ''}
                readonly={isReadOnly}
                onchange={(event: Event) => {
                  const condition = valueOf(event).trim();
                  const { condition: _dropped, ...rest } = step.who ?? {};
                  onchange(changeStep(shape, step.key, { who: condition ? { ...rest, condition } : rest }));
                }}
              />
            </fieldset>
            <div class="fields">
              <Input
                label={t('app.workflow_designer.requires')}
                hint={t('app.workflow_designer.fields_hint')}
                value={joined(step.requires)}
                readonly={isReadOnly}
                onchange={(event: Event) => onchange(changeStep(shape, step.key, { requires: words(valueOf(event)) }))}
              />
              <Input
                label={t('app.workflow_designer.asks')}
                hint={t('app.workflow_designer.fields_hint')}
                value={joined(step.asks)}
                readonly={isReadOnly}
                onchange={(event: Event) => onchange(changeStep(shape, step.key, { asks: words(valueOf(event)) }))}
              />
            </div>
            <fieldset class="group">
              <legend>{t('app.workflow_designer.checks')}</legend>
              {#each step.checks ?? [] as check, index (check.key)}
                <div class="fields check" data-check={check.key}>
                  <Input
                    label={t('app.workflow_designer.check_holds')}
                    hint={t('app.workflow_designer.condition_hint')}
                    value={check.holds}
                    readonly={isReadOnly}
                    onchange={(event: Event) => changeCheck(step, index, { holds: valueOf(event) })}
                  />
                  <Input
                    label={t('app.workflow_designer.check_message')}
                    value={check.message ?? ''}
                    readonly={isReadOnly}
                    onchange={(event: Event) => changeCheck(step, index, { message: valueOf(event).trim() || null })}
                  />
                  {#if !isReadOnly}
                    <IconButton icon="trash" size="sm" tone="subtle" label={t('app.workflow_designer.remove_check')} onclick={() => removeCheck(step, index)} />
                  {/if}
                </div>
              {/each}
              {#if !isReadOnly}
                <Inline gap="100"><Button tone="subtle" size="sm" icon="plus" onclick={() => addCheck(step)}>{t('app.workflow_designer.add_check')}</Button></Inline>
              {/if}
            </fieldset>
            {#if stepRules}
              <fieldset class="group" data-step-then={step.key}>
                <legend>{t('app.workflow_designer.then')}</legend>
                <p class="quiet">{t('app.workflow_designer.then_hint')}</p>
                {#each stepRules(step.key) as rule (rule.id)}
                  <Inline gap="100" align="center">
                    <Button tone="subtle" size="sm" icon="zap" data-step-rule={rule.id} onclick={() => onopenrule?.(rule.id)}>{rule.name}</Button>
                    {#if !rule.enabled}<span class="quiet">{t('app.workflow_designer.rule_off')}</span>{/if}
                  </Inline>
                {/each}
                {#if onaddrule}
                  <Inline gap="100"><Button tone="subtle" size="sm" icon="plus" data-add-step-rule={step.key} onclick={() => onaddrule(step.key)}>{t('app.workflow_designer.add_rule')}</Button></Inline>
                {/if}
              </fieldset>
            {/if}
            {#if !isReadOnly}
              <Inline gap="100" justify="end">
                <Button tone="subtle" size="sm" icon="trash" data-remove-step={step.key} onclick={() => onchange(removeStep(shape, step.key))}>
                  {t('app.workflow_designer.remove_step')}
                </Button>
              </Inline>
            {/if}
          </li>
        {/each}
      </ul>
      {#if !isReadOnly}
        <div class="add">
          <Select label={t('app.workflow_designer.step_to')} options={statusOptions} bind:value={newStepTo} />
          <Button tone="secondary" size="sm" icon="plus" data-add-step onclick={addOne}>{t('app.workflow_designer.add_step')}</Button>
        </div>
      {/if}
    </Stack>
  </section>
</div>

<style>
  .lists { display: grid; gap: var(--sp-300); }
  .title { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }

  .row {
    display: grid;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }

  .row.selected { border-color: var(--accent-primary); }
  .row:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }

  .fields, .add {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 24ch), 1fr));
    gap: var(--sp-100);
    align-items: end;
  }

  .check { grid-template-columns: 1fr 1fr auto; }

  .group { margin: 0; padding: 0; border: none; display: grid; gap: var(--sp-050); }
  .group legend { padding: 0; font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-secondary); }
  .choices { display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-200); }
</style>
