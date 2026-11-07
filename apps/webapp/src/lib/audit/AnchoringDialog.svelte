<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Anchoring…* as a Dialog M (page-system §9.3): where the chain's end is written once a day,
  // outside this database. Read from the workspace, written through the audit's own route, which
  // records the change.
  import { tick } from 'svelte';

  import { Button, Dialog, Select, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { audit } from '../data/audit.svelte.ts';
  import { backup } from '../data/backup.svelte.ts';
  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import WorkspaceGate from '../workspace/WorkspaceGate.svelte';

  interface Props {
    isOpen: boolean;
    anchorTargetId: string | null;
  }

  let { isOpen = $bindable(false), anchorTargetId }: Props = $props();

  let anchorChoice = $state('');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let missing = $state<Missing>({});

  const anchorTargetName = $derived(anchorTargetId ? (backup.all.find((target) => target.id === anchorTargetId)?.name ?? anchorTargetId) : undefined);

  async function attempt(work: () => Promise<unknown>, said: string): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
      announcer.say(said);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  function anchor(): void {
    missing = check([{ kind: 'field', key: 'anchor', code: 'app.audit.target_first', isFilled: anchorChoice !== '' }]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(document.querySelector('dialog[open]') ?? document));
      return;
    }
    void attempt(() => audit.configureAnchoring(anchorChoice), t('app.audit.anchoring_on_announced'));
  }
</script>

<Dialog
  size="m"
  title={t('app.audit.anchoring_title')}
  bind:isOpen
  dismissLabel={t('app.page.close')}
  onClose={() => { failure = undefined; missing = {}; anchorChoice = ''; }}
>
  <Stack gap="200">
    <p class="quiet">{t('app.audit.anchoring_intro')}</p>
    <p><strong>{anchorTargetId ? t('app.audit.anchoring_on', { target: anchorTargetName ?? '' }) : t('app.audit.anchoring_off')}</strong></p>
    {#if failure}<FailureNote problem={failure} />{/if}
    <WorkspaceGate permission="STRUCTURE">
      {#snippet actions()}
        <Stack gap="150">
          <Select
            label={t('app.audit.anchoring_target')}
            hint={t('app.audit.anchoring_target_hint')}
            bind:value={anchorChoice}
            placeholder={t('app.audit.choose_target')}
            options={backup.all.map((target) => ({ value: target.id, label: target.name }))}
            error={missing['anchor'] && !anchorChoice ? t(missing['anchor']) : undefined}
          />
          <div class="row">
            <!-- Unavailable only where there is nothing to do; an empty choice is marked when pressed. -->
            <Button
              tone="primary"
              isBusy={isWorking}
              busyLabel={t('app.audit.anchoring_saving')}
              disabledReason={anchorChoice !== '' && anchorChoice === anchorTargetId ? t('app.audit.anchoring_choose_first') : undefined}
              onclick={anchor}
            >
              {t('app.audit.anchor_here')}
            </Button>
            {#if anchorTargetId}
              <Button
                tone="secondary"
                isBusy={isWorking}
                busyLabel={t('app.audit.anchoring_saving')}
                onclick={() => void attempt(() => audit.configureAnchoring(null), t('app.audit.anchoring_off_announced'))}
              >
                {t('app.audit.anchoring_switch_off')}
              </Button>
            {/if}
          </div>
        </Stack>
      {/snippet}
    </WorkspaceGate>
    <p class="quiet small">{t('app.audit.anchoring_is_audited')}</p>
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  p { margin: 0; }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
