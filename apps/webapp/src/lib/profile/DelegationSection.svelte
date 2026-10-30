<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who decides the reader's approvals while they are away (UC-SVC-10 checks 1 and 3): a person
  // and a span of at most a year, set, replaced or ended. The person's own, so on the profile; drawn
  // only where the service desk is on, since nothing else asks anybody's yes.

  import { untrack } from 'svelte';
  import { Button, Inline, Input, Stack } from '@hubtask/design-system/components';
  import type { ApprovalDelegation } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { approvals, delegationPath } from '../data/approvals.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import PersonPicker from '../people/PersonPicker.svelte';
  import { renderProblem } from '../problem.ts';
  import { delegationProblem } from '../servicedesk/approvals.ts';

  const read = untrack(() => resource<{ delegation: ApprovalDelegation | null }>(delegationPath));
  const current = $derived(read.state.status === 'ready' ? read.state.data.delegation : undefined);
  $effect(() => accounts.resolve([current?.delegate_id]));

  let delegate = $state('');
  let from = $state('');
  let until = $state('');
  let isSaving = $state(false);
  let refusal = $state<string | undefined>(undefined);

  const at = (iso: string) => formatDateTime(iso, messages.locale);
  const iso = (local: string) => (local ? new Date(local).toISOString() : '');

  async function save(event: SubmitEvent) {
    event.preventDefault();
    refusal = undefined;
    const problem = delegationProblem(delegate, actor.account?.id, iso(from), iso(until));
    if (problem) {
      refusal = t(problem);
      return;
    }
    isSaving = true;
    try {
      await approvals.delegate({ delegate_id: delegate, from: iso(from), until: iso(until) });
      toast.show({ text: t('app.approvals.delegated_announced'), tone: 'success' });
      delegate = '';
      from = '';
      until = '';
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }

  async function end() {
    refusal = undefined;
    try {
      await approvals.undelegate();
      toast.show({ text: t('app.approvals.undelegated_announced'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }
</script>

<Stack gap="150">
  <h2 class="section">{t('app.approvals.delegation_title')}</h2>
  <p class="quiet">{t('app.approvals.delegation_intro')}</p>
  {#if current}
    <Inline gap="100" align="center">
      <span data-delegation>{t('app.approvals.delegation_current', {
        name: accounts.nameOf(current.delegate_id) ?? t('app.approvals.somebody'), from: at(current.from), until: at(current.until),
      })}</span>
      <Button tone="subtle" size="sm" icon="x" onclick={() => void end()}>{t('app.approvals.undelegate')}</Button>
    </Inline>
  {/if}
  <form class="panel" onsubmit={(event) => void save(event)}>
    <Stack gap="100">
      <PersonPicker label={t('app.approvals.delegate')} noneLabel={t('app.approvals.choose_person')} bind:value={delegate} />
      <div class="fields">
        <Input label={t('app.approvals.from')} type="datetime-local" bind:value={from} />
        <Input label={t('app.approvals.until')} type="datetime-local" bind:value={until} />
      </div>
      <div><Button type="submit" tone="secondary" icon="user-check" isBusy={isSaving} busyLabel={t('app.approvals.saving')}>
        {current ? t('app.approvals.redelegate') : t('app.approvals.delegate_action')}
      </Button></div>
      {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
    </Stack>
  </form>
</Stack>

<style>
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  form { margin: 0; max-inline-size: 52ch; }
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 24ch), 1fr)); gap: var(--sp-100); }
</style>
