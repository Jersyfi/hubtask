<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Check the chain…* as a Dialog M (page-system §9.3): a period, whether to read the last anchor
  // back, and what the check found.
  //
  // **A break is a finding, not an error.** `:verify` answering `valid: false` is what an audit
  // trail exists to produce; rendering it as a failed request would hide it. So the two outcomes
  // are two renderings, and the break says where it starts.
  import { tick } from 'svelte';

  import { Banner, Button, Checkbox, Dialog, Input, Stack } from '@hubtask/design-system/components';

  import { audit, readAnchor, readVerification } from '../data/audit.svelte.ts';
  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    /** Where the chain's end is anchored, if anywhere: reading the anchor back is offered only then. */
    anchorTargetId: string | null;
  }

  let { isOpen = $bindable(false), anchorTargetId }: Props = $props();

  let verifyFrom = $state('');
  let verifyTo = $state('');
  let verifyAnchors = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let missing = $state<Missing>({});

  const finding = $derived(audit.verification ? readVerification(audit.verification) : undefined);
  const anchorFinding = $derived(audit.verification ? readAnchor(audit.verification) : undefined);
  const when = (at: string | null | undefined) => (at ? formatDateTime(at, messages.locale) : undefined);
  const sentence = (code: string | null | undefined) => (code ? (messages.has(code) ? t(code) : code) : undefined);
  const markOf = (key: string, value: string): string | undefined => (missing[key] && !value ? t(missing[key]) : undefined);

  async function verify(): Promise<void> {
    missing = check([
      { kind: 'field', key: 'from', code: 'app.audit.period_first', isFilled: verifyFrom !== '' },
      { kind: 'field', key: 'to', code: 'app.audit.period_first', isFilled: verifyTo !== '' },
    ]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(document.querySelector('dialog[open]') ?? document));
      return;
    }
    failure = undefined;
    isWorking = true;
    try {
      await audit.verify(new Date(verifyFrom).toISOString(), new Date(verifyTo).toISOString(), verifyAnchors && anchorTargetId !== null);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.audit.verify_title')}
  bind:isOpen
  dismissLabel={t('app.page.close')}
  onClose={() => { failure = undefined; missing = {}; }}
  primary={{ label: t('app.audit.verify'), isBusy: isWorking, busyLabel: t('app.audit.verifying'), onclick: () => void verify() }}
>
  <Stack gap="200">
    <p class="quiet">{t('app.audit.verify_intro')}</p>
    {#if failure}<FailureNote problem={failure} />{/if}
    <div class="fields">
      <Input label={t('app.audit.from')} type="datetime-local" bind:value={verifyFrom} error={markOf('from', verifyFrom)} />
      <Input label={t('app.audit.to')} type="datetime-local" bind:value={verifyTo} error={markOf('to', verifyTo)} />
    </div>
    {#if anchorTargetId}
      <!-- Asked for rather than always done: a read of somebody else's machine. -->
      <Checkbox label={t('app.audit.verify_anchors')} hint={t('app.audit.verify_anchors_hint')} bind:checked={verifyAnchors} />
    {/if}

    {#if finding?.kind === 'holds'}
      <Banner tone="success" title={t('app.audit.chain_holds')}>
        <Stack gap="050">
          <span>{t('app.audit.chain_checked', { count: String(finding.checked) })}</span>
          {#if finding.sealedUntil}
            <span>{t('app.audit.sealed_until', { at: when(finding.sealedUntil) ?? '' })}</span>
          {:else}
            <span>{t('app.audit.never_anchored')}</span>
          {/if}
        </Stack>
      </Banner>
    {:else if finding?.kind === 'broken'}
      <!-- The finding this trail exists to produce: where the chain stops holding. -->
      <Banner tone="danger" title={t('app.audit.chain_broken')}>
        <Stack gap="050">
          {#if finding.firstBrokenSeq !== undefined}<span>{t('app.audit.broken_at', { seq: String(finding.firstBrokenSeq) })}</span>{/if}
          {#if finding.gapCount > 0}
            <span>{t('app.audit.gaps', { count: String(finding.gapCount) })}</span>
            {#if finding.gaps.length > 0}<span class="mono">{finding.gaps.join(', ')}</span>{/if}
          {/if}
          <span>{t('app.audit.break_is_recorded')}</span>
        </Stack>
      </Banner>
    {/if}
    {#if anchorFinding?.kind === 'agrees'}
      <Banner tone="success" title={t('app.audit.anchor_agrees')}>
        {t('app.audit.anchor_read', { at: when(anchorFinding.until) ?? '', seq: String(anchorFinding.seq ?? '') })}
      </Banner>
    {:else if anchorFinding?.kind === 'disagrees'}
      <Banner tone="danger" title={t('app.audit.anchor_disagrees')}>{t('app.audit.anchor_disagrees_explains')}</Banner>
    {:else if anchorFinding?.kind === 'unreadable'}
      <Banner tone="warning" title={t('app.audit.anchor_unreadable')}>{sentence(anchorFinding.code)}</Banner>
    {/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .mono { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
  .fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(20ch, 1fr)); gap: var(--sp-150); }
</style>
