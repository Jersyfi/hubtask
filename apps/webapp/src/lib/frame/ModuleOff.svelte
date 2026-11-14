<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The page of a module switched off in the workspace: what it is, that its data is kept, and -
  // for who may shape the workspace - the switch at once; for anybody else whom to ask. Drawn in
  // place of the page, so no read is refused and no form is offered that the server would refuse.
  import { Button, EmptyState, Icon, PageHeader, Stack } from '@hubtask/design-system/components';
  import type { ModuleState } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { modules } from '../data/modules.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { switchedOn } from '../product/switchedon.ts';
  import { PRODUCT_TILES } from '../product/tiles.ts';
  import TemplateCards from '../product/TemplateCards.svelte';
  import { WorkspaceStarts } from '../product/workspaceStarts.svelte.ts';
  import TemplateDialog from '../settings/TemplateDialog.svelte';
  import { page } from './page.svelte.ts';
  import { toast } from './toasts.svelte.ts';
  import { viewport } from './viewport.svelte.ts';

  // `isProduct`: a product's home, whose trail is *Products* rather than the administration's.
  // `onnavigate`: where a template taken from its cards opens the product (user-experience.md §3.2).
  const { module, isProduct = false, onnavigate }: { module: string; isProduct?: boolean; onnavigate?: (path: string) => void } = $props();

  const name = $derived(t(`app.modules.name_${module.toLowerCase()}`));
  $effect(() => page.entitle(name));
  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  let isSwitching = $state(false);
  // A product whose start is a template of the workspace alone - goals, time - offers those too:
  // taking one switches the product on with it (scenes emptygoals, emptytime).
  const starts = new WorkspaceStarts(() => module, () => isProduct && mayShape);
  let refusal = $state<string | undefined>(undefined);

  async function switchOn() {
    isSwitching = true;
    refusal = undefined;
    try {
      await modules.switch(module as ModuleState['module'], 'ON');
      // The page gives way to the product's home; what it still needs travels with the message
      // (user-experience.md §2.8), as the card under the switch says it.
      const setup = switchedOn(module).setup;
      toast.show({ text: t('app.modules.switched_on', { module: name }), tone: 'success', ...(setup ? { link: { label: t(setup.code), href: setup.href } } : {}) });
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

{#snippet offNote()}
  <span class="note" data-module-off-note><Icon name="info" size="sm" />{t(mayShape ? 'app.module_off.kept' : 'app.module_off.ask')}</span>
{/snippet}

<Stack gap="300">
  <PageHeader title={name} isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        isProduct
          ? { id: 'products', label: t('app.nav.products') }
          : { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'module', label: name },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }} />
  {#if isProduct}
    <!-- A product switched off is drawn as its first empty state (scenes homeplans, homeassets,
         homeagents): its tile, its sentence, the switch - and, under it, that nothing is lost. -->
    <EmptyState kind="unused" icon="layout-template" product={PRODUCT_TILES[module]} title={t('app.module_off.title', { module: name })}
      description={t(`app.modules.hint_${module.toLowerCase()}`)}
      action={mayShape ? switchControl : undefined} footnote={offNote} />
    {#if starts.cards.length > 0}<TemplateCards templates={starts.cards} shape="row" tile={PRODUCT_TILES[module]} />{/if}
  {:else}
    <EmptyState kind="unused" icon="layout-template" title={t('app.module_off.title', { module: name })}
      description={t(mayShape ? 'app.module_off.body' : 'app.module_off.body_ask')}
      action={mayShape ? switchControl : undefined} />
  {/if}
  {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
</Stack>

<TemplateDialog template={starts.opened} {onnavigate} onClose={() => (starts.opened = undefined)} />

<style>
  .refusal { margin: 0; color: var(--text-danger); }
  .note { display: inline-flex; align-items: center; gap: var(--sp-050); }
</style>
