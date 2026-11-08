<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One retention rule as a Dialog M (page-system §5.1): why it is kept that long, what it would do
  // now, and switching it on - only once somebody has seen what it would do - or out of
  // enforcement. Withdrawing it is the row's ⋯, behind its own confirmation.
  import { Badge, Banner, Button, Dialog, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { policies, type Policy } from '../data/policies.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { actionWord, kindWord } from './words.svelte.ts';

  interface Props {
    policy: Policy | undefined;
    /** Where the rule applies, in words. */
    scope: string;
    /** Arming and disarming are the owner's (P-05). */
    mayArm: boolean;
    onClose: () => void;
  }

  const { policy, scope, mayArm, onClose }: Props = $props();

  /** §7's five per cent: above this on a first run and the rule is stored as an announcement. */
  const BROAD_SHARE = 0.05;

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  const preview = $derived(policy ? policies.previewOf(policy.id) : undefined);
  const when = (at: string | null | undefined) => (at ? formatDateTime(at, messages.locale) : undefined);

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
      if (said) announcer.say(said);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  const primary = $derived.by(() => {
    if (!policy || !mayArm) return undefined;
    const id = policy.id;
    // Armed only after somebody has seen what it would do.
    return policy.enabled === false
      ? {
          label: t('app.retention.arm'),
          tone: 'danger' as const,
          isBusy: isWorking,
          busyLabel: t('app.retention.arming'),
          disabledReason: preview ? undefined : t('app.retention.preview_first'),
          onclick: (): void => void attempt(() => policies.update(id, { enabled: true }), t('app.retention.armed_announced')),
        }
      : {
          label: t('app.retention.disarm'),
          isBusy: isWorking,
          busyLabel: t('app.retention.saving'),
          onclick: (): void => void attempt(() => policies.update(id, { enabled: false }), t('app.retention.disarmed_announced')),
        };
  });
</script>

<Dialog
  size="m"
  isOpen={policy !== undefined}
  title={policy ? kindWord(policy.data_kind) : ''}
  dismissLabel={t('app.dismiss')}
  onClose={() => { failure = undefined; onClose(); }}
  {primary}
>
  {#if policy}
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <div class="line">
        <Badge tone={policy.action === 'NOTIFY_ONLY' ? 'info' : 'warning'}>{actionWord(policy.action)}</Badge>
        <Badge tone={policy.enabled === false ? 'neutral' : 'success'}>{policy.enabled === false ? t('app.retention.off') : t('app.retention.on')}</Badge>
        {#if policy.last_failure}<Badge tone="danger">{t('app.retention.failing_badge')}</Badge>{/if}
      </div>
      <p class="quiet">{t('app.retention.after_days', { days: String(policy.retain_days) })} · {t('app.retention.at_scope', { scope })}</p>
      {#if policy.justification}
        <!-- The sentence somebody will read in an audit, kept beside the period it justifies. -->
        <p class="quiet">{t('app.retention.justified', { reason: policy.justification })}</p>
      {/if}
      {#if policy.last_failure}
        <!-- UC-LIF-04 check 10: the code's own sentence says what failed. -->
        <Banner tone="danger" title={t('app.retention.failing_title')}>
          {t('app.retention.failing', { at: when(policy.last_failure.at) ?? policy.last_failure.at })}
          {#if messages.has(policy.last_failure.code)}{t(policy.last_failure.code)}{/if}
        </Banner>
      {/if}

      <div class="preview">
        <Stack gap="050">
          {#if preview}
            <p class="quiet">
              {t('app.retention.preview_matched', { count: String(preview.matched ?? 0) })}
              {#if preview.share_of_scope !== undefined}
                · {t('app.retention.preview_share', { percent: String(Math.round((preview.share_of_scope ?? 0) * 100)) })}
              {/if}
            </p>
            {#if (preview.share_of_scope ?? 0) > BROAD_SHARE}
              <p class="warning">{t('app.retention.preview_broad')}</p>
            {/if}
            {#each Object.entries(preview.blocked ?? {}) as [reason, count] (reason)}
              <!-- What the rule would *not* take, and why. A legal hold is the one an operator is
                   usually looking for. -->
              <p class="quiet">
                {messages.has(`app.retention.blocked_${reason}`)
                  ? t(`app.retention.blocked_${reason}`, { count: String(count) })
                  : t('app.retention.blocked_other', { reason, count: String(count) })}
              </p>
            {/each}
            {#if (preview.samples ?? []).length > 0}
              <ul class="samples">
                {#each preview.samples ?? [] as sample (sample.id)}
                  <li class="quiet">{sample.title} · {when(sample.effective_at)}</li>
                {/each}
              </ul>
            {/if}
            <p class="quiet">{t('app.retention.preview_wrote_nothing')}</p>
          {/if}
          <div>
            <Button size="sm" tone="secondary" isBusy={isWorking} busyLabel={t('app.retention.previewing')} onclick={() => void attempt(() => policies.preview(policy.id))}>
              {t('app.retention.preview')}
            </Button>
          </div>
        </Stack>
      </div>
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .warning { margin: 0; color: var(--text-warning); font-size: var(--fs-075); }
  .preview { padding: var(--sp-150); border-radius: var(--r-md); background: var(--bg-surface-sunken); }
  .samples { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
</style>
