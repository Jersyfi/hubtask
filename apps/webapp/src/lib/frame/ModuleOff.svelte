<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The page of a module switched off in the workspace: what it is, that its data is kept, and -
  // for who may shape the workspace - the switch at once; for anybody else whom to ask. Drawn in
  // place of the page, so no read is refused and no form is offered that the server would refuse.
  import { untrack } from 'svelte';
  import { Button, EmptyState, Icon, PageHeader, ProductBand, Stack } from '@hubtask/design-system/components';
  import type { ConfigTemplate, ModuleState } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { config } from '../data/config.svelte.ts';
  import { modules } from '../data/modules.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { switchedOn } from '../product/switchedon.ts';
  import { PRODUCTS } from '../navigation.ts';
  import { PRODUCT_TILES } from '../product/tiles.ts';
  import { productTemplates } from '../product/templates.ts';
  import CreateContainerDialog from '../workspace/CreateContainerDialog.svelte';
  import TemplateCards from '../product/TemplateCards.svelte';
  import { WorkspaceStarts } from '../product/workspaceStarts.svelte.ts';
  import { timeWays } from '../time/ways.ts';
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
  const row = $derived(PRODUCTS.find((each) => each.module === module));
  /** The product's sentence as its start page says it, else the module's hint. */
  const sentence = $derived.by(() => {
    const code = `app.product_empty.sentence_${module.toLowerCase()}`;
    return messages.has(code) ? t(code) : t(`app.modules.hint_${module.toLowerCase()}`);
  });
  /** The side way beside the switch, where the product's start page draws one: it switches on and goes there. */
  const SIDE: Readonly<Record<string, { code: string; path: string }>> = {
    GOALS: { code: 'app.goals.empty_side_dashboard', path: '/goals?tab=dashboards' },
    TIME: { code: 'app.time.empty_side_kinds', path: '/time/settings' },
  };
  const side = $derived(SIDE[module]);
  /**
   * The cards under the switch: time's three ways to record (scene emptytime) - the timer, the
   * week, the billable template - every other product its shipped templates.
   */
  const cards = $derived(module === 'TIME' ? timeWays(starts.cards, t, { timer: () => void switchOn('/'), week: () => void switchOn('/time') }) : starts.cards);

  // A product whose templates make a hub - the desk, plans, inventory, pages, whiteboards - starts as
  // the `empty` scene draws the desk: its band, a template as the main way, *start empty* beside it
  // and the templates as cards. Making the hub from one switches the product on (`containers:from-template`).
  // Goals and time keep their own scenes (emptygoals, emptytime), which start from the workspace's.
  let catalogue = $state<readonly ConfigTemplate[]>([]);
  /** Goals and time start as their own scenes draw them. */
  const hasOwnStart = $derived(module === 'GOALS' || module === 'TIME');
  const takesHubStarts = $derived(isProduct && mayShape && !hasOwnStart);
  $effect(() => {
    if (!takesHubStarts) return;
    return untrack(() => config.templates((next) => {
      if (next.status === 'ready') catalogue = next.data.data;
    }));
  });
  const templateName = (template: ConfigTemplate): string =>
    template.origin === 'HUBTASK' ? t(`app.structure.preset.${template.key.toUpperCase()}`) : template.name;
  /** The template the new hub takes; the dialog is open while one is chosen. */
  let startingFrom = $state<string | undefined>(undefined);
  let isCreatingHub = $state(false);
  const hubStarts = $derived(takesHubStarts ? productTemplates(catalogue, module).map((template) => ({
    label: templateName(template),
    description: template.origin === 'HUBTASK' && messages.has(`app.templates.summary.${template.key.toUpperCase()}`) ? t(`app.templates.summary.${template.key.toUpperCase()}`) : undefined,
    icon: row?.icon,
    onclick: () => { startingFrom = template.key; isCreatingHub = true; },
  })) : []);
  const emptyTitle = $derived(module === 'SERVICE_DESK' ? t('app.desk_home.no_desk') : t('app.product_home.none', { product: name }));

  async function switchOn(then?: string) {
    isSwitching = true;
    refusal = undefined;
    try {
      await modules.switch(module as ModuleState['module'], 'ON');
      // The page gives way to the product's home; what it still needs travels with the message
      // (user-experience.md §2.8), as the card under the switch says it.
      const setup = switchedOn(module).setup;
      toast.show({ text: t('app.modules.switched_on', { module: name }), tone: 'success', ...(setup ? { link: { label: t(setup.code), href: setup.href } } : {}) });
      if (then) onnavigate?.(then);
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isSwitching = false;
    }
  }
</script>

{#snippet switchControl()}
  <Button tone="primary" icon={isProduct ? 'plus' : 'layout-template'} isBusy={isSwitching} busyLabel={t('app.workspace.saving')} onclick={() => void switchOn()}>
    {t('app.module_off.switch_on', { module: name })}
  </Button>
  {#if isProduct && side}<Button tone="secondary" data-empty-side disabledReason={isSwitching ? t('app.workspace.saving') : undefined} onclick={() => void switchOn(side.path)}>{t(side.code)}</Button>{/if}
{/snippet}

{#snippet startControl()}
  {#if hubStarts[0]}
    {@const first = hubStarts[0]}
    <Button tone="primary" icon="plus" disabledReason={isSwitching ? t('app.workspace.saving') : undefined} onclick={first.onclick}>{t('app.product_empty.start_with', { template: first.label })}</Button>
  {/if}
  <Button tone="secondary" data-empty-side isBusy={isSwitching} busyLabel={t('app.workspace.saving')} onclick={() => void switchOn()}>{t('app.product_empty.start_empty')}</Button>
{/snippet}

{#snippet offNote()}
  <span class="note" data-module-off-note><Icon name="info" size="sm" />{t(mayShape ? 'app.module_off.kept' : 'app.module_off.ask')}</span>
{/snippet}

<Stack gap="300">
  <!-- A product's start page has no head of its own (scenes emptygoals, emptytime): its name is the
       page's one heading below; on a phone the bar still carries it. -->
  {#if hubStarts.length > 0}
    <ProductBand product={PRODUCT_TILES[module] ?? 'desk'} icon={row?.icon ?? 'layout-template'} {name}
      breadcrumb={{ trail: [{ id: 'products', label: t('app.nav.products') }, { id: 'module', label: name }], label: t('app.admin.trail'), expandLabel: t('app.admin.expand_trail') }} />
    <EmptyState kind="unused" icon={row?.icon ?? 'layout-template'} product={PRODUCT_TILES[module]} title={emptyTitle}
      description={sentence} action={startControl} footnote={offNote} />
    <TemplateCards templates={hubStarts} shape="stacked" tile={PRODUCT_TILES[module]} />
  {:else if isProduct && !hasOwnStart}
    <!-- A product without templates to start from: its band, the sentence and the switch. -->
    <ProductBand product={PRODUCT_TILES[module] ?? 'desk'} icon={row?.icon ?? 'layout-template'} {name}
      breadcrumb={{ trail: [{ id: 'products', label: t('app.nav.products') }, { id: 'module', label: name }], label: t('app.admin.trail'), expandLabel: t('app.admin.expand_trail') }} />
    <EmptyState kind="unused" icon={row?.icon ?? 'layout-template'} product={PRODUCT_TILES[module]} title={t('app.module_off.title', { module: name })}
      description={sentence} action={mayShape ? switchControl : undefined} footnote={offNote} />
  {:else}
  {#if !isProduct || viewport.isCompact}
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
  {/if}
  {#if isProduct}
    <!-- A product switched off is drawn as its first empty state (scenes homeplans, homeassets,
         homeagents): its tile, its sentence, the switch - and, under it, that nothing is lost. -->
    <EmptyState kind="unused" isHome isTitleInBar={viewport.isCompact} icon={row?.icon ?? 'layout-template'} product={PRODUCT_TILES[module]} title={name}
      description={sentence}
      action={mayShape ? switchControl : undefined} footnote={offNote} />
    {#if cards.length > 0}<TemplateCards templates={cards} shape="row" tile={PRODUCT_TILES[module]} />{/if}
  {:else}
    <EmptyState kind="unused" icon="layout-template" title={t('app.module_off.title', { module: name })}
      description={t(mayShape ? 'app.module_off.body' : 'app.module_off.body_ask')}
      action={mayShape ? switchControl : undefined} />
  {/if}
  {/if}
  {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
</Stack>

<CreateContainerDialog bind:isOpen={isCreatingHub} type="HUB" template={startingFrom} oncreated={(_id, landing) => { startingFrom = undefined; onnavigate?.(landing ?? row?.path ?? '/'); }} />

<TemplateDialog template={starts.opened} {onnavigate} onClose={() => (starts.opened = undefined)} />

<style>
  .refusal { margin: 0; color: var(--text-danger); }
  .note { display: inline-flex; align-items: center; gap: var(--sp-050); }
</style>
