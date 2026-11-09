<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A step of a workflow as a Dialog L (page-system §5.1, the mockup's *Step “Finish”*), three
  // groups: *The step* - its name, where it leads and where it leaves from -, *Who may take it* -
  // roles, groups and whose yes it waits for (UC-FLW-04 check 1, UC-SVC-08 check 1) -, and
  // *Advanced* folded: the person fields and the condition, the fields it requires and asks for,
  // its checks (checks 2 and 3) and the rules on it, which run in its own change (check 4).
  //
  // **Edits are made on a copy of the draft**, every one a function over it (lib/data/workflows.ts);
  // *Done* hands the copy back, *Cancel* drops it, and nothing is sent from here - the page saves
  // the draft. *Add step* opens the dialog on a copy with the new step in it.
  import { untrack } from 'svelte';
  import { Button, Checkbox, Dialog, Disclosure, IconButton, Inline, Input, Select, Stack, Switch } from '@hubtask/design-system/components';
  import type { WorkflowShape, WorkflowStep } from '@hubtask/sync-engine';

  import {
    APPROVAL_RULES, CHECK_MESSAGE_CODES, addApprovalLevel, addStep, changeApproval, changeApprovalLevel, changeStep, removeApprovalLevel,
    statusLabel, stepLabel, toggleApproval, type ApprovalLevel,
  } from '../data/workflows.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import PersonPicker from '../people/PersonPicker.svelte';

  interface Props {
    shape: WorkflowShape;
    /** The step's key, `'new'` for *Add step*; closed while undefined. */
    subject: string | undefined;
    /** The rules on each step, by step key; absent where the reader may not write rules. */
    stepRules?: (step: string) => readonly { readonly id: string; readonly name: string; readonly enabled: boolean }[];
    onopenrule?: (id: string) => void;
    onaddrule?: (step: string) => void;
    onchange: (shape: WorkflowShape) => void;
    onClose: () => void;
  }

  const { shape, subject, stepRules, onopenrule, onaddrule, onchange, onClose }: Props = $props();

  const ROLES = ['OWNER', 'ADMIN', 'MEMBER', 'CONTRIBUTOR', 'GUEST'] as const;
  /** The app's own languages, each a field of a check's words beside the one text. */
  const LANGUAGES = ['en', 'de'] as const;

  $effect(() => untrack(() => groups.open()));

  let working = $state<WorkflowShape | undefined>(undefined);
  let key = $state<string | undefined>(undefined);
  let opened: string | undefined;
  $effect(() => {
    if (subject === opened) return;
    opened = subject;
    untrack(() => {
      if (subject === undefined) {
        working = undefined;
        key = undefined;
        return;
      }
      let copy = JSON.parse(JSON.stringify(shape)) as WorkflowShape;
      if (subject === 'new') {
        const to = copy.statuses[0]?.key;
        if (to) copy = addStep(copy, [], to);
        key = copy.steps[copy.steps.length - 1]?.key;
      } else {
        key = subject;
      }
      working = copy;
    });
  });

  const step = $derived(working?.steps.find((each) => each.key === key));
  const guard = new DiscardGuard(
    () => subject === 'new' || (working !== undefined && JSON.stringify(working) !== JSON.stringify(shape)),
    () => onClose(),
  );

  function edit(next: WorkflowShape) {
    working = next;
  }

  function done() {
    if (working) onchange(working);
    onClose();
  }

  const statusOptions = $derived((working?.statuses ?? []).map((status) => ({ value: status.key, label: statusLabel(status, t) })));
  const ruleOptions = $derived(APPROVAL_RULES.map((rule) => ({ value: rule, label: t(`app.approvals.rule.${rule.toLowerCase()}`) })));
  const declineOptions = $derived([
    { value: '', label: t('app.approvals.decline_stays') },
    ...(working?.statuses ?? []).map((status) => ({ value: status.key, label: statusLabel(status, t) })),
  ]);
  const messageCodeOptions = $derived([
    { value: '', label: t('app.workflow_designer.check_message_code_none') },
    ...CHECK_MESSAGE_CODES.map((code) => ({ value: code, label: t(`workflows.check_message.${code}`) })),
  ]);

  /** A list written as words separated by commas, as the contract's string lists. */
  const words = (value: string) => value.split(',').map((word) => word.trim()).filter((word) => word !== '');
  const joined = (value: readonly string[] | undefined) => (value ?? []).join(', ');
  const checked = (event: Event) => (event.currentTarget as HTMLInputElement).checked;
  const valueOf = (event: Event) => (event.currentTarget as HTMLInputElement).value;

  function toggleIn(list: readonly string[] | undefined, value: string, on: boolean): string[] {
    const set = new Set(list ?? []);
    if (on) set.add(value);
    else set.delete(value);
    return [...set];
  }

  function change(each: WorkflowStep, next: Partial<WorkflowStep>) {
    if (working) edit(changeStep(working, each.key, next));
  }

  function toggleFrom(each: WorkflowStep, status: string, on: boolean) {
    if (!working) return;
    const from = new Set(each.from ?? []);
    if (on) from.add(status);
    else from.delete(status);
    change(each, { from: working.statuses.map((one) => one.key).filter((one) => from.has(one)) });
  }

  function toggleRole(each: WorkflowStep, role: string, on: boolean) {
    const roles = new Set<string>(each.who?.roles ?? []);
    if (on) roles.add(role);
    else roles.delete(role);
    // The shipped roles in their order, then the workspace's own keys the step already names.
    const ordered = [...ROLES.filter((one) => roles.has(one)), ...[...roles].filter((one) => !(ROLES as readonly string[]).includes(one))];
    change(each, { who: { ...each.who, roles: ordered as never } });
  }

  function changeCheck(each: WorkflowStep, index: number, next: { holds?: string; message?: string | null; messages?: Record<string, string>; message_code?: (typeof CHECK_MESSAGE_CODES)[number] | null }) {
    change(each, { checks: (each.checks ?? []).map((check, at) => (at === index ? { ...check, ...next } : check)) });
  }

  function changeWords(each: WorkflowStep, index: number, language: string, text: string) {
    const next: Record<string, string> = { ...(each.checks?.[index]?.messages ?? {}) };
    if (text.trim()) next[language] = text.trim();
    else delete next[language];
    changeCheck(each, index, { messages: next });
  }

  function addCheck(each: WorkflowStep) {
    const taken = new Set((each.checks ?? []).map((check) => check.key));
    let n = (each.checks?.length ?? 0) + 1;
    while (taken.has(`check_${n}`)) n++;
    change(each, { checks: [...(each.checks ?? []), { key: `check_${n}`, holds: '', message: null }] });
  }

  /** A language's own name in the reader's language. */
  function languageName(language: string): string {
    try {
      return new Intl.DisplayNames([messages.locale], { type: 'language' }).of(language) ?? language;
    } catch {
      return language;
    }
  }

  /** The person being added to each level, by index, until *Add* is pressed. */
  let adding = $state<Record<number, string>>({});

  function addPerson(each: WorkflowStep, index: number, level: ApprovalLevel) {
    const person = adding[index];
    if (!person || !working) return;
    edit(changeApprovalLevel(working, each.key, index, { people: toggleIn(level.people, person, true) }));
    adding[index] = '';
  }

  /** *Advanced* opens by itself where the step already uses something in it. */
  const isAdvancedSet = $derived(
    step !== undefined &&
      ((step.who?.person_fields?.length ?? 0) > 0 || Boolean(step.who?.condition) || (step.requires?.length ?? 0) > 0 || (step.asks?.length ?? 0) > 0 || (step.checks?.length ?? 0) > 0),
  );
</script>

<Dialog
  size="l"
  isOpen={subject !== undefined}
  title={step && working ? (subject === 'new' ? t('app.workflow_designer.add_step') : t('app.workflow_designer.step_title', { name: stepLabel(step, working, t) })) : ''}
  dismissLabel={t('app.workflow_designer.cancel')}
  onDismiss={guard.dismiss}
  {onClose}
  primary={{ label: t('app.workflow_designer.done'), onclick: done }}
>
  {#if step && working}
    {@const shapeNow = working}
    <div class="groups" data-step-dialog={step.key}>
      <section class="group">
        <h3 class="heading">{t('app.workflow_designer.group_step')}</h3>
        <div class="two">
          <Input
            label={t('app.workflow_designer.step_name')}
            value={step.name ?? ''}
            placeholder={stepLabel(step, shapeNow, t)}
            onchange={(event: Event) => change(step, { name: valueOf(event).trim() || null })}
          />
          <Select label={t('app.workflow_designer.step_to')} options={statusOptions} value={step.to} onchange={(event: Event) => change(step, { to: valueOf(event) })} />
        </div>
        <fieldset class="set">
          <legend>{t('app.workflow_designer.step_from')}</legend>
          <div class="choices">
            {#each shapeNow.statuses.filter((status) => status.key !== step.to) as status (status.key)}
              <Checkbox label={statusLabel(status, t)} checked={step.from?.includes(status.key) ?? false} onchange={(event: Event) => toggleFrom(step, status.key, checked(event))} />
            {/each}
          </div>
          <p class="quiet">{t('app.workflow_designer.step_from_hint')}</p>
        </fieldset>
      </section>

      <section class="group">
        <h3 class="heading">{t('app.workflow_designer.who')}</h3>
        <div class="choices" role="group" aria-label={t('app.workflow_designer.who')}>
          {#each ROLES as role (role)}
            <Checkbox label={t(`app.people.role.${role.toLowerCase()}`)} checked={step.who?.roles?.includes(role) ?? false} onchange={(event: Event) => toggleRole(step, role, checked(event))} />
          {/each}
          {#each groups.all as group (group.id)}
            <Checkbox
              label={group.name}
              checked={step.who?.groups?.includes(group.id) ?? false}
              onchange={(event: Event) => change(step, { who: { ...step.who, groups: toggleIn(step.who?.groups, group.id, checked(event)) } })}
            />
          {/each}
        </div>
        <div data-step-approval={step.key}>
          <Switch
            label={t('app.approvals.step_waits')}
            hint={t('app.approvals.step_waits_hint')}
            checked={step.approval != null}
            onchange={(event: Event) => edit(toggleApproval(shapeNow, step.key, checked(event)))}
          />
        </div>
        {#if step.approval}
          {#each step.approval.levels as level, index (index)}
            <div class="level" data-approval-level={index}>
              <Inline gap="100" align="center" justify="between">
                <span class="sub">{t('app.approvals.level', { n: String(index + 1) })}</span>
                {#if step.approval.levels.length > 1}
                  <IconButton icon="trash" size="sm" tone="subtle" label={t('app.approvals.remove_level')} onclick={() => edit(removeApprovalLevel(shapeNow, step.key, index))} />
                {/if}
              </Inline>
              <div class="two">
                <Select label={t('app.approvals.rule_label')} options={ruleOptions} value={level.rule}
                  onchange={(event: Event) => edit(changeApprovalLevel(shapeNow, step.key, index, { rule: valueOf(event) as ApprovalLevel['rule'] }))} />
                {#if level.rule === 'N_OF'}
                  <Input label={t('app.approvals.count')} type="number" min="1" max="50" value={String(level.count ?? 1)}
                    onchange={(event: Event) => edit(changeApprovalLevel(shapeNow, step.key, index, { count: Math.max(1, Number(valueOf(event)) || 1) }))} />
                {/if}
              </div>
              <div class="choices" role="group" aria-label={t('app.approvals.people')}>
                {#each level.people ?? [] as person (person)}
                  <Inline gap="050" align="center">
                    <span>{accounts.nameOf(person) ?? t('app.approvals.somebody')}</span>
                    <IconButton icon="x" size="sm" tone="subtle" label={t('app.approvals.remove_person')}
                      onclick={() => edit(changeApprovalLevel(shapeNow, step.key, index, { people: toggleIn(level.people, person, false) }))} />
                  </Inline>
                {/each}
              </div>
              <div class="add">
                <PersonPicker label={t('app.approvals.add_person')} noneLabel={t('app.approvals.choose_person')} bind:value={() => adding[index] ?? '', (next) => (adding[index] = next)} />
                <Button tone="subtle" size="sm" icon="plus" onclick={() => addPerson(step, index, level)}>{t('app.approvals.add')}</Button>
              </div>
              {#if groups.all.length > 0}
                <div class="choices" role="group" aria-label={t('app.approvals.groups')}>
                  {#each groups.all as group (group.id)}
                    <Checkbox label={group.name} checked={level.groups?.includes(group.id) ?? false}
                      onchange={(event: Event) => edit(changeApprovalLevel(shapeNow, step.key, index, { groups: toggleIn(level.groups, group.id, checked(event)) }))} />
                  {/each}
                </div>
              {/if}
              {#if level.rule === 'ANY' || level.rule === 'N_OF'}
                <div class="choices" role="group" aria-label={t('app.approvals.roles')}>
                  {#each ROLES as role (role)}
                    <Checkbox label={t(`app.people.role.${role.toLowerCase()}`)} checked={level.roles?.includes(role) ?? false}
                      onchange={(event: Event) => edit(changeApprovalLevel(shapeNow, step.key, index, { roles: toggleIn(level.roles, role, checked(event)) }))} />
                  {/each}
                </div>
              {/if}
              <Input label={t('app.approvals.person_fields')} hint={t('app.workflow_designer.person_fields_hint')} value={joined(level.person_fields)}
                onchange={(event: Event) => edit(changeApprovalLevel(shapeNow, step.key, index, { person_fields: words(valueOf(event)) }))} />
            </div>
          {/each}
          {#if step.approval.levels.length < 5}
            <Inline gap="100"><Button tone="subtle" size="sm" icon="plus" onclick={() => edit(addApprovalLevel(shapeNow, step.key))}>{t('app.approvals.add_level')}</Button></Inline>
          {/if}
          <div class="two">
            <Select label={t('app.approvals.decline_to')} options={declineOptions} value={step.approval.decline_to ?? ''}
              onchange={(event: Event) => edit(changeApproval(shapeNow, step.key, { decline_to: valueOf(event) || null }))} />
            <Input label={t('app.approvals.due_hours')} hint={t('app.approvals.due_hours_hint')} type="number" min="0" max="2160"
              value={String(step.approval.due_hours ?? 0)}
              onchange={(event: Event) => edit(changeApproval(shapeNow, step.key, { due_hours: Math.min(2160, Math.max(0, Math.round(Number(valueOf(event)) || 0))) }))} />
          </div>
        {/if}
      </section>

      <Disclosure label={t('app.workflow_designer.advanced')} description={t('app.workflow_designer.advanced_hint')} tone="quiet" isOpen={isAdvancedSet}>
        <Stack gap="200">
          <div class="two">
            <Input
              label={t('app.workflow_designer.person_fields')}
              hint={t('app.workflow_designer.person_fields_hint')}
              value={joined(step.who?.person_fields)}
              onchange={(event: Event) => change(step, { who: { ...step.who, person_fields: words(valueOf(event)) } })}
            />
            <Input
              label={t('app.workflow_designer.who_condition')}
              hint={t('app.workflow_designer.condition_hint')}
              value={step.who?.condition ?? ''}
              onchange={(event: Event) => {
                const condition = valueOf(event).trim();
                const { condition: _dropped, ...rest } = step.who ?? {};
                change(step, { who: condition ? { ...rest, condition } : rest });
              }}
            />
            <Input label={t('app.workflow_designer.requires')} hint={t('app.workflow_designer.fields_hint')} value={joined(step.requires)}
              onchange={(event: Event) => change(step, { requires: words(valueOf(event)) })} />
            <Input label={t('app.workflow_designer.asks')} hint={t('app.workflow_designer.fields_hint')} value={joined(step.asks)}
              onchange={(event: Event) => change(step, { asks: words(valueOf(event)) })} />
          </div>
          <fieldset class="set">
            <legend>{t('app.workflow_designer.checks')}</legend>
            {#each step.checks ?? [] as check, index (check.key)}
              <div class="level" data-check={check.key}>
                <div class="two">
                  <Input label={t('app.workflow_designer.check_holds')} hint={t('app.workflow_designer.condition_hint')} value={check.holds}
                    onchange={(event: Event) => changeCheck(step, index, { holds: valueOf(event) })} />
                  <Input label={t('app.workflow_designer.check_message')} value={check.message ?? ''}
                    onchange={(event: Event) => changeCheck(step, index, { message: valueOf(event).trim() || null })} />
                  {#each LANGUAGES as language (language)}
                    <Input label={t('app.workflow_designer.check_message_in', { language: languageName(language) })} value={check.messages?.[language] ?? ''}
                      onchange={(event: Event) => changeWords(step, index, language, valueOf(event))} />
                  {/each}
                  <Select label={t('app.workflow_designer.check_message_code')} options={messageCodeOptions} value={check.message_code ?? ''}
                    onchange={(event: Event) => changeCheck(step, index, { message_code: (valueOf(event) || null) as (typeof CHECK_MESSAGE_CODES)[number] | null })} />
                </div>
                <Inline gap="100" justify="end">
                  <Button tone="subtle" size="sm" icon="trash" onclick={() => change(step, { checks: (step.checks ?? []).filter((_, at) => at !== index) })}>
                    {t('app.workflow_designer.remove_check')}
                  </Button>
                </Inline>
              </div>
            {/each}
            <Inline gap="100"><Button tone="subtle" size="sm" icon="plus" onclick={() => addCheck(step)}>{t('app.workflow_designer.add_check')}</Button></Inline>
          </fieldset>
          {#if stepRules}
            <fieldset class="set" data-step-then={step.key}>
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
        </Stack>
      </Disclosure>
    </div>
  {/if}
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) onClose(); }}>{t('app.workflow_designer.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  .groups { display: grid; gap: var(--sp-300); }
  .group { display: grid; gap: var(--sp-200); }
  .group + .group { padding-block-start: var(--sp-300); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .sub { font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-secondary); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  .two {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 24ch), 1fr));
    gap: var(--sp-150);
    align-items: end;
  }

  .add { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  /* An approval level and a check are separated by a hairline, not boxed (PS-16). */
  .level { display: grid; gap: var(--sp-100); padding-block: var(--sp-150); border-block-start: var(--bw-hairline) solid var(--border-subtle); }

  .set { margin: 0; padding: 0; border: none; display: grid; gap: var(--sp-100); }
  .set legend { padding: 0; margin-block-end: var(--sp-100); font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-secondary); }
  .choices { display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-200); }
</style>
