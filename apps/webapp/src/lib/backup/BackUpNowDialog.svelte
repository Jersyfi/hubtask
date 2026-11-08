<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Back up now* where there is more than one target: a Dialog S of its one field, the target
  // (page-system §5.1). With one target the head's press starts it at once.
  import { Button, Dialog, Select } from '@hubtask/design-system/components';

  import type { Target } from '../data/backup.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    isOpen: boolean;
    targets: readonly Target[];
    isWorking: boolean;
    onstart: (targetId: string) => void;
  }

  let { isOpen = $bindable(false), targets, isWorking, onstart }: Props = $props();

  let chosen = $state('');
  let isMissing = $state(false);

  function start(): void {
    if (chosen === '') {
      isMissing = true;
      return;
    }
    onstart(chosen);
    isOpen = false;
  }
</script>

<Dialog
  size="s"
  title={t('app.backup.start')}
  bind:isOpen
  dismissLabel={t('app.workspace.cancel')}
  onClose={() => { chosen = ''; isMissing = false; }}
  primary={{ label: t('app.backup.start'), isBusy: isWorking, busyLabel: t('app.backup.starting'), onclick: start }}
>
  <Select
    label={t('app.backup.schedule_target')}
    bind:value={chosen}
    placeholder={t('app.backup.choose_target')}
    options={targets.map((each) => ({ value: each.id, label: each.name }))}
    error={isMissing && chosen === '' ? t('app.backup.start_target_needed') : undefined}
  />
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
