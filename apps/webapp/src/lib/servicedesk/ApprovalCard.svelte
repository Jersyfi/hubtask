<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One approval with its levels and trail (UC-SVC-09 checks 1 to 4): where it stands, how many
  // yeses each level has against what passes it, and who decided - for whom, when, why and on which
  // version of the entry. *Approve* and *Decline* are drawn only where the server says the reader
  // may decide now (`may_decide`, P-05); a decline asks for its reason before it is sent, as the
  // server would refuse one without (`approvals.reason_required`).

  import { Badge, Button, Inline, Stack, Textarea } from '@hubtask/design-system/components';
  import type { Approval } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { approvals, type Verdict } from '../data/approvals.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { decidedOnOlderVersion, levelStanding, stateTone, trailOf } from './approvals.ts';

  interface Props {
    approval: Approval;
    /** Drawn on the reader's own list: the entry's title links to it; without it, the title is text. */
    onnavigate?: (path: string) => void;
  }

  const { approval, onnavigate }: Props = $props();

  $effect(() => accounts.resolve([approval.requested_by, approval.on_behalf_of, ...approval.decisions.flatMap((d) => [d.approver_id, d.on_behalf_of])]));

  const name = (id: string | null | undefined) => accounts.nameOf(id) ?? t('app.approvals.somebody');
  const at = (iso: string | null | undefined) => (iso ? formatDateTime(iso, messages.locale) : '');
  const trail = $derived(trailOf(approval));

  let reason = $state('');
  let deciding = $state<Verdict | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);

  async function decide(verdict: Verdict) {
    refusal = undefined;
    if (verdict === 'DECLINE' && reason.trim() === '') {
      refusal = t('app.approvals.reason_first');
      return;
    }
    deciding = verdict;
    try {
      await approvals.decide(approval, verdict, reason);
      reason = '';
      toast.show({ text: t(verdict === 'APPROVE' ? 'app.approvals.approved_announced' : 'app.approvals.declined_announced'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      deciding = undefined;
    }
  }
</script>

<article class="card" data-approval={approval.id}>
  <Stack gap="100">
    <Inline gap="100" align="center">
      {#if approval.item_title !== undefined && onnavigate}
        <Button tone="subtle" size="sm" onclick={() => onnavigate(`/items/${approval.item_id}`)}>
          {approval.item_number ? `#${approval.item_number} ` : ''}{approval.item_title}
        </Button>
      {:else if approval.item_title !== undefined}
        <!-- The portal's reader decides on the entry's title alone; the team's entry is not theirs to open. -->
        <strong>{approval.item_number ? `#${approval.item_number} ` : ''}{approval.item_title}</strong>
      {/if}
      <Badge tone={stateTone(approval.state)} size="sm">{t(`app.approvals.state.${approval.state.toLowerCase()}`)}</Badge>
      {#if approval.on_behalf_of}<span class="quiet">{t('app.approvals.for_person', { name: name(approval.on_behalf_of) })}</span>{/if}
    </Inline>
    <p class="quiet">
      {t('app.approvals.asked_by', { name: name(approval.requested_by), at: at(approval.requested_at), step: approval.step })}
      {#if approval.state === 'PENDING' && approval.due_at} · {t('app.approvals.due', { at: at(approval.due_at) })}{/if}
    </p>
    <ol class="levels">
      {#each approval.levels as level, index (index)}
        {@const standing = levelStanding(approval, index + 1)}
        <li class:current={approval.state === 'PENDING' && approval.current_level === index + 1}>
          {t('app.approvals.level', { n: String(index + 1) })} · {t(`app.approvals.rule.${level.rule.toLowerCase()}`)} ·
          {t('app.approvals.yes_of', { yes: String(standing.yes), needed: String(standing.needed) })}
        </li>
      {/each}
    </ol>
    {#if trail.length > 0}
      <ol class="trail" aria-label={t('app.approvals.trail')}>
        {#each trail as decision (decision.approver_id + decision.decided_at)}
          <li>
            <Badge tone={decision.verdict === 'APPROVE' ? 'success' : 'danger'} size="sm">
              {t(decision.verdict === 'APPROVE' ? 'app.approvals.verdict_yes' : 'app.approvals.verdict_no')}
            </Badge>
            <span>
              {decision.on_behalf_of
                ? t('app.approvals.decided_for', { name: name(decision.approver_id), for: name(decision.on_behalf_of), at: at(decision.decided_at) })
                : t('app.approvals.decided', { name: name(decision.approver_id), at: at(decision.decided_at) })}
              · {t('app.approvals.on_version', { version: String(decision.item_version) })}
              {#if decidedOnOlderVersion(approval, decision)}<span class="quiet">{t('app.approvals.changed_since')}</span>{/if}
            </span>
            {#if decision.reason}<p class="reason">{decision.reason}</p>{/if}
          </li>
        {/each}
      </ol>
    {/if}
    {#if approval.may_decide && approval.state === 'PENDING'}
      <Textarea label={t('app.approvals.reason')} hint={t('app.approvals.reason_hint')} rows={2} bind:value={reason} />
      <Inline gap="100">
        <Button tone="primary" size="sm" icon="check" isBusy={deciding === 'APPROVE'} busyLabel={t('app.approvals.deciding')}
          disabledReason={deciding ? t('app.approvals.deciding') : undefined} onclick={() => void decide('APPROVE')}>{t('app.approvals.approve')}</Button>
        <Button tone="secondary" size="sm" icon="x" isBusy={deciding === 'DECLINE'} busyLabel={t('app.approvals.deciding')}
          disabledReason={deciding ? t('app.approvals.deciding') : undefined} onclick={() => void decide('DECLINE')}>{t('app.approvals.decline')}</Button>
      </Inline>
    {/if}
    {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
  </Stack>
</article>

<style>
  .card { padding: var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .refusal { margin: 0; color: var(--text-danger); }
  .levels, .trail { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .levels li { color: var(--text-secondary); }
  .levels li.current { color: var(--text-primary); font-weight: var(--fw-medium); }
  .trail li { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: center; }
  .reason { margin: 0; flex-basis: 100%; white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
