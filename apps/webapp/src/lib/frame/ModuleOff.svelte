<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The page of a module switched off in the workspace: what it is, that its data is kept, and -
  // for who may shape the workspace - the switch at once; for anybody else whom to ask. Drawn in
  // place of the page, so no read is refused and no form is offered that the server would refuse.
  import { Button, EmptyState, PageHeader, Stack } from '@hubtask/design-system/components';
  import type { ModuleState } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { modules } from '../data/modules.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { page } from './page.svelte.ts';
  import { toast } from './toasts.svelte.ts';
  import { viewport } from './viewport.svelte.ts';

  const { module }: { module: string } = $props();

  const name = $derived(t(`app.modules.name_${module.toLowerCase()}`));
  $effect(() => page.entitle(name));
  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  let isSwitching = $state(false);
  let refusal = $state<string | undefined>(undefined);

  async function switchOn() {
    isSwitching = true;
    refusal = undefined;
    try {
      await modules.switch(module as ModuleState['module'], 'ON');
      toast.show({ text: t('app.modules.switched_on', { module: name }), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isSwitching = false;
    }
  }
</script>

{#snippet switchControl()}
  <Button tone="primary" icon="layout-template" isBusy={isSwitching} busyLabel={t('app.workspace.saving')} onclick={() => void switchOn()}>
    {t('app.module_off.switch_on', { module: name })}
  </Button>
{/snippet}

<Stack gap="300">
  <PageHeader title={name} isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'module', label: name },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }} />
  <EmptyState kind="unused" icon="layout-template" title={t('app.module_off.title', { module: name })}
    description={t(mayShape ? 'app.module_off.body' : 'app.module_off.body_ask')}
    action={mayShape ? switchControl : undefined} />
  {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
</Stack>

<style>
  .refusal { margin: 0; color: var(--text-danger); }
</style>
