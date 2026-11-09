<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Registering an agent, or changing one (UC-AGT-01, UC-AGT-02): a Dialog L of three titled groups
  // - who it is, what it may do, what it may spend (page-system §5.1). Scopes are ticked in words
  // (user-experience.md §5.4); the typed list is the one fold, for a scope the installation does
  // not declare. A refusal is said inside, which keeps what was typed (§5.10).

  import { untrack } from 'svelte';

  import { Banner, Button, CheckboxGroup, Dialog, Disclosure, Input, Select, Stack, Textarea, type SelectOption } from '@hubtask/design-system/components';
  import type { Agent, AgentModel, AgentWrite } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { SHIPPED_ROLES, agents, draftOf, scopeChoices, scopeWordCode, scopesFrom, scopesTicked, type AgentDraft } from '../data/agents.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** What is open: an agent to change, a shipped role to start from, or a blank new agent. */
    opened: { agent?: Agent; role?: (typeof SHIPPED_ROLES)[number] } | undefined;
    models: readonly AgentModel[];
    onclose: () => void;
    /** The new agent, once registered. */
    onregistered?: (agent: Agent) => void;
  }

  const { opened, models, onclose, onregistered }: Props = $props();

  const editing = $derived(opened?.agent);
  let draft = $state<AgentDraft | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let isWorking = $state(false);

  $effect(() => {
    const now = opened;
    untrack(() => {
      failure = undefined;
      if (!now) return (draft = undefined);
      const fresh = draftOf(now.agent, actor.account?.id ?? '', now.role);
      draft = now.role ? { ...fresh, name: t(`app.agents.kind.${now.role}`), purpose: t(`app.agents.role_purpose.${now.role}`) } : fresh;
    });
  });

  $effect(() => untrack(() => people.openScope({ scopeType: 'TENANT' })));
  const ownerOptions = $derived<SelectOption[]>(people.known().map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') })));
  const modelOptions = $derived<SelectOption[]>([
    { value: '', label: t('app.agents.no_model') },
    ...models.filter((each) => !each.retired).map((each) => ({ value: each.id, label: each.name })),
  ]);

  const scopeOffered = $derived(scopeChoices(manifest.value?.token_scopes ?? [], editing?.scopes ?? []));
  const scopeOptions = $derived(scopeOffered.map((scope) => {
    const code = scopeWordCode(scope);
    return messages.has(code) ? { value: scope, label: t(code), hint: scope } : { value: scope, label: scope };
  }));

  const kindOptions = $derived<SelectOption[]>(
    (['TICKET_TRIAGE', 'CI_SUGGESTION', 'DUPLICATE_REVIEW', 'KNOWLEDGE_DRAFT', 'EXTERNAL'] as const).map((each) => ({ value: each, label: t(`app.agents.kind.${each}`) })),
  );
  const riskOptions = $derived<SelectOption[]>((['MINIMAL', 'LIMITED'] as const).map((each) => ({ value: each, label: t(`app.agents.risk.${each}`) })));
  const reversibleOptions = $derived<SelectOption[]>((['ALLOW', 'NOTIFY', 'APPROVAL', 'NEVER'] as const).map((each) => ({ value: each, label: t(`app.agents.rule.${each}`) })));
  const irreversibleOptions = $derived<SelectOption[]>((['APPROVAL', 'NEVER'] as const).map((each) => ({ value: each, label: t(`app.agents.rule.${each}`) })));

  const whole = (text: string) => (/^\d+$/.test(text.trim()) ? Number.parseInt(text, 10) : Number.NaN);
  const reason = $derived(
    !draft ? undefined
    : !draft.name.trim() ? t('agents.name_required')
    : !draft.purpose.trim() ? t('agents.purpose_required')
    : [draft.tokens, draft.actions, draft.runs].some((each) => Number.isNaN(whole(each))) ? t('agents.budget_invalid')
    : !(whole(draft.accuracy) >= 0 && whole(draft.accuracy) <= 100) ? t('agents.min_accuracy_invalid')
    : undefined,
  );

  async function save() {
    const d = draft;
    if (!d) return;
    const body: AgentWrite = {
      name: d.name.trim(),
      purpose: d.purpose.trim(),
      kind: d.kind,
      risk_class: d.risk,
      scopes: scopesFrom(d.scopes),
      policy: { reversible: d.reversible, irreversible: d.irreversible },
      budget: { tokens_per_day: whole(d.tokens), actions_per_hour: whole(d.actions), concurrent_runs: whole(d.runs) },
      min_accuracy: whole(d.accuracy),
      ...(d.owner ? { owner_id: d.owner } : {}),
      ...(d.model ? { model_id: d.model } : {}),
    };
    isWorking = true;
    failure = undefined;
    try {
      if (editing) {
        await agents.change(editing, body);
        toast.show({ text: t('app.agents.saved'), tone: 'success' });
      } else {
        const made = await agents.register(body);
        toast.show({ text: t('app.agents.registered'), tone: 'success' });
        if (made) onregistered?.(made as Agent);
      }
      onclose();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="l"
  isOpen={opened !== undefined && draft !== undefined}
  title={editing ? t('app.agents.edit_named', { name: editing.name }) : t('app.agents.add_agent')}
  dismissLabel={t('app.agents.cancel')}
  onClose={onclose}
>
  {#if draft}
    <Stack gap="200">
      {#if editing && editing.status !== 'DRAFT'}<Banner tone="info">{t('app.agents.change_hint')}</Banner>{/if}
      <fieldset class="group">
        <legend>{t('app.agents.group_who')}</legend>
        <Stack gap="150">
          <Input label={t('app.agents.name')} bind:value={draft.name} maxlength={120} />
          <Textarea label={t('app.agents.purpose')} hint={t('app.agents.purpose_hint')} rows={3} bind:value={draft.purpose} />
          <div class="line">
            <Select label={t('app.agents.kind_label')} options={kindOptions} bind:value={draft.kind} />
            <Select label={t('app.agents.owner')} hint={t('app.agents.owner_hint')} options={ownerOptions} bind:value={draft.owner} />
          </div>
        </Stack>
      </fieldset>
      <fieldset class="group">
        <legend>{t('app.agents.group_may')}</legend>
        <Stack gap="150">
          <div class="line">
            <Select label={t('app.agents.risk_label')} hint={t('app.agents.risk_hint')} options={riskOptions} bind:value={draft.risk} />
            <Select label={t('app.agents.model')} options={modelOptions} bind:value={draft.model} />
          </div>
          {#if scopeOptions.length > 0}
            <CheckboxGroup
              label={t('app.agents.scopes')}
              hint={t('app.agents.scopes_pick_hint')}
              options={scopeOptions}
              selected={scopesFrom(draft.scopes)}
              onChange={(next) => { if (draft) draft.scopes = scopesTicked(draft.scopes, scopeOffered, next); }}
            />
          {/if}
          <Disclosure tone="quiet" label={t('app.agents.scopes_advanced')} isOpen={scopeOptions.length === 0}>
            <Input label={t('app.agents.scopes_typed')} hint={t('app.agents.scopes_hint')} bind:value={draft.scopes} />
          </Disclosure>
          <div class="line">
            <Select label={t('app.agents.reversible')} options={reversibleOptions} bind:value={draft.reversible} />
            <Select label={t('app.agents.irreversible')} options={irreversibleOptions} bind:value={draft.irreversible} />
          </div>
        </Stack>
      </fieldset>
      <fieldset class="group">
        <legend>{t('app.agents.group_spend')}</legend>
        <Stack gap="100">
          <div class="line">
            <Input label={t('app.agents.tokens_per_day')} inputmode="numeric" bind:value={draft.tokens} />
            <Input label={t('app.agents.actions_per_hour')} inputmode="numeric" bind:value={draft.actions} />
            <Input label={t('app.agents.concurrent_runs')} inputmode="numeric" bind:value={draft.runs} />
          </div>
          <p class="meta">{t('app.agents.budget_hint')}</p>
          <Input label={t('app.agents.min_accuracy')} hint={t('app.agents.min_accuracy_hint')} inputmode="numeric" bind:value={draft.accuracy} />
        </Stack>
      </fieldset>
      {#if failure}<FailureNote problem={failure} />{/if}
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="secondary" onclick={onclose}>{t('app.agents.cancel')}</Button>
    <Button tone="primary" isBusy={isWorking} busyLabel={t('app.agents.working')} disabledReason={reason} onclick={() => void save()}>{editing ? t('app.agents.save') : t('app.agents.add')}</Button>
  {/snippet}
</Dialog>

<style>
  .group { margin: 0; padding: 0; border: 0; min-width: 0; }
  legend { padding: 0; margin-block-end: var(--sp-100); font-weight: var(--fw-semibold); }
  .line { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, var(--layout-field-sm)), 1fr)); gap: var(--sp-100); }
  .meta { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
