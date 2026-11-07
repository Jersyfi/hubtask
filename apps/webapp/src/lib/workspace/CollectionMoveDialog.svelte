<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Move to…* for a collection to another hub (the navigation concept's §5.3, UC-WRK-39 checks
  // 6-8): the hubs, those without STRUCTURE greyed with the reason; for the chosen one the server's
  // pre-check (`previewContainerMove`) as `MoveCheck` - each obstacle with its way out, the
  // consequences as warnings, in words. *Move* is enabled once no obstacle remains; on a phone it
  // names the destination. A name taken is resolved in the dialog: the rename is written first.
  // On a desktop the hub is one field, *To: …*, that opens the list; on a phone the list stands.
  import { Button, Dialog, Icon, Input, MoveCheck, Sheet, type MoveCheckItem } from '@hubtask/design-system/components';
  import type { Container, ContainerMovePreview, MoveFinding } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { holds, roleOrder } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { people } from '../data/people.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import ContainerMark from './ContainerMark.svelte';
  import { findingWords, isObstacleResolved } from './moveCheck.ts';

  interface Props {
    container: Container;
    isOpen?: boolean;
    onnavigate: (path: string) => void;
  }

  let { container, isOpen = $bindable(false), onnavigate }: Props = $props();

  const from = $derived(containers.find(container.parent_id ?? ''));
  const mayStructureIn = (hubId: string) =>
    holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'STRUCTURE').status === 'permitted';
  const hubs = $derived(
    containers.hubs
      .filter((each) => each.id !== container.parent_id && !each.effective_archived)
      .map((each) => ({ hub: each, reason: mayStructureIn(each.id) ? undefined : t('app.move.check_no_structure') })),
  );

  let target = $state<string | undefined>(undefined);
  /** The list is open until a hub is chosen, and again from the field. */
  let isPicking = $state(true);
  let newName = $state('');
  let preview = $state<ContainerMovePreview | undefined>(undefined);
  let isMoving = $state(false);
  let failure = $state<string | undefined>(undefined);
  const destination = $derived(target ? containers.find(target) : undefined);

  $effect(() => {
    const at = target;
    preview = undefined;
    if (!at || !isOpen) return;
    return engine.subscribe<ContainerMovePreview>(
      { path: `/containers/${container.id}:move-preview`, body: { target_parent_id: at } },
      (next) => {
        if (next.status === 'ready') preview = next.data;
        if (next.status === 'failed') failure = renderProblem(next.error as never, messages).message;
      },
    );
  });

  // The accounts losing access are named, not counted only.
  $effect(() => {
    const ids = preview?.consequences.find((each) => each.code === 'containers.move_loses_access')?.params['account_ids'];
    if (ids) accounts.resolve(ids.split(','));
  });

  const nameOf = (id: string | undefined) => (id ? containers.find(id)?.name : undefined) ?? '';
  const words = (finding: MoveFinding) =>
    findingWords(finding, { from: from?.name ?? '', to: destination?.name ?? '', nameOf: (id) => accounts.nameOf(id) ?? '', hubName: nameOf }, t);

  const remaining = $derived((preview?.obstacles ?? []).filter((each) => !isObstacleResolved(each, newName, container.name)));
  const isNameTaken = $derived((preview?.obstacles ?? []).some((each) => each.code === 'containers.name_taken'));

  const items = $derived.by((): MoveCheckItem[] => {
    if (!preview || !destination) return [];
    const obstacles = preview.obstacles;
    const list: MoveCheckItem[] = [];
    if (!obstacles.some((each) => each.code === 'access.not_permitted')) list.push({ id: 'right', tone: 'ok', text: t('app.move.check_right') });
    if (!isNameTaken) list.push({ id: 'name', tone: 'ok', text: t('app.move.check_name_free', { hub: destination.name }) });
    for (const finding of obstacles) {
      const said = words(finding);
      const resolved = isObstacleResolved(finding, newName, container.name);
      if (finding.code === 'entry_types.hub_bound') {
        list.push({ id: finding.code, tone: 'obstacle', text: said, way: { label: t('app.move.way_types'), onselect: () => go(`/collections/${container.id}/settings/types`) }, rest: t('app.move.way_types_rest') });
      } else if (finding.code === 'workflows.hub_bound') {
        list.push({ id: finding.code, tone: 'obstacle', text: said, way: { label: t('app.move.way_workflow', { hub: destination.name }), onselect: () => go(`/collections/${container.id}/settings/workflow`) } });
      } else {
        list.push({ id: finding.code, tone: resolved ? 'ok' : 'obstacle', text: resolved ? t('app.move.check_new_name', { name: newName.trim() }) : said });
      }
    }
    for (const finding of preview.consequences) list.push({ id: finding.code, tone: 'warning', text: words(finding) });
    return list;
  });

  function go(path: string) {
    isOpen = false;
    onnavigate(path);
  }

  async function move() {
    if (!target || remaining.length > 0) return;
    const hubName = destination?.name ?? '';
    const moving = container.name;
    isMoving = true;
    failure = undefined;
    try {
      const name = newName.trim();
      if (isNameTaken && name && name !== container.name) await containers.update(container.id, { name }, container.version);
      await containers.move(container.id, target, null, crypto.randomUUID());
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return;
    } finally {
      isMoving = false;
    }
    announcer.say(t('app.move.container_announced', { name: moving, hub: hubName }));
  }

  const confirmLabel = $derived(viewport.isCompact && destination ? t('app.move.confirm_to', { hub: destination.name }) : t('app.move.confirm'));
  const blocked = $derived(!target ? t('app.move.choose_hub_first') : !preview ? t('app.move.checking') : remaining.length > 0 ? t('app.move.obstacles_left', { count: remaining.length }) : undefined);
</script>

{#snippet hubList()}
  <ul class="hubs" id="move-hubs" role="radiogroup" aria-label={t('app.move.hub')} data-touch={viewport.isCompact ? '' : undefined}>
    {#each hubs as row (row.hub.id)}
      <li>
        <button
          type="button"
          class="hub"
          role="radio"
          aria-checked={target === row.hub.id}
          aria-disabled={row.reason ? 'true' : undefined}
          onclick={() => {
            if (row.reason) return;
            target = row.hub.id;
            failure = undefined;
            isPicking = false;
          }}
        >
          <ContainerMark look={row.hub} size="xs" hasDefaultTile />
          <span class="name">{row.hub.name}</span>
          {#if row.reason}<small class="why">{row.reason}</small>{/if}
        </button>
      </li>
    {/each}
  </ul>
{/snippet}

{#snippet body()}
  {#if hubs.length === 0}
    <p class="quiet">{t('app.move.no_hub')}</p>
  {:else if viewport.isCompact}
    <p class="lead">{t('app.move.collection_to', { name: container.name })}</p>
    {@render hubList()}
  {:else}
    <button type="button" class="to" aria-expanded={isPicking} aria-controls="move-hubs" onclick={() => (isPicking = !isPicking)}>
      <span class="to-label">{t('app.move.to')}</span>
      {#if destination}
        <ContainerMark look={destination} size="xs" hasDefaultTile />
        <span class="to-name">{destination.name}</span>
      {:else}
        <span class="quiet">{t('app.move.choose_hub')}</span>
      {/if}
      <Icon name="chevron-down" size="sm" />
    </button>
    {#if isPicking}{@render hubList()}{/if}
  {/if}
  {#if items.length > 0}
    <MoveCheck label={t('app.move.check_label')} {items} toneLabels={{ ok: t('app.move.tone_ok'), obstacle: t('app.move.tone_obstacle'), warning: t('app.move.tone_warning') }} />
  {/if}
  {#if isNameTaken}
    <Input label={t('app.move.new_name')} bind:value={newName} />
  {/if}
  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
{/snippet}

{#snippet actions()}
  {#if !viewport.isCompact}<Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>{/if}
  <Button tone="primary" isBusy={isMoving} disabledReason={blocked} onclick={() => void move()}>{confirmLabel}</Button>
{/snippet}

{#if viewport.isCompact}
  <Sheet title={t('app.move.title', { name: container.name })} closeLabel={t('app.sheet_close')} bind:isOpen>
    {@render body()}
    <div class="sheet-actions">{@render actions()}</div>
  </Sheet>
{:else}
  <Dialog title={t('app.move.title', { name: container.name })} bind:isOpen dismissLabel={t('app.workspace.cancel')} {actions}>
    {@render body()}
  </Dialog>
{/if}

<style>
  .lead { margin: 0 0 var(--sp-100); color: var(--text-subtle); }
  .to {
    display: flex;
    align-items: center;
    gap: calc(var(--sp-050) + var(--sp-025));
    inline-size: 100%;
    min-block-size: var(--layout-nav-row);
    margin: 0 0 var(--sp-100);
    padding: var(--sp-050) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }
  .to:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .to-label { color: var(--text-secondary); }
  .to-name { font-weight: var(--fw-semibold); }
  .quiet { color: var(--text-subtle); }
  .hubs { display: flex; flex-direction: column; gap: var(--sp-025); margin: 0 0 var(--sp-150); padding: 0; list-style: none; }

  .hub {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    inline-size: 100%;
    min-block-size: var(--layout-nav-row);
    padding: var(--sp-050) var(--sp-100);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--layout-nav-radius);
    background: none;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  /* On a phone the hubs are one card of thumb rows, as the Browse stack's (§9: 44 px). */
  .hubs[data-touch] {
    gap: 0;
    overflow: hidden;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .hubs[data-touch] li + li { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .hubs[data-touch] .hub { min-block-size: var(--layout-nav-touchrow); padding-inline: var(--sp-150); border-radius: 0; }
  .hubs[data-touch] .hub[aria-checked='true'] { border-color: transparent; }
  .hub:hover:not([aria-disabled]) { background: var(--bg-surface-hover); }
  .hub[aria-checked='true'] { border-color: var(--focus-ring); background: var(--accent-primary-subtle); font-weight: var(--fw-semibold); }
  .hub:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--bw-ring)); }
  .hub[aria-disabled] { color: var(--text-subtle); cursor: not-allowed; }
  .name { flex: 1; min-width: 0; }
  .why { color: var(--text-subtle); }
  .failure { color: var(--text-danger); }
  /* On a phone one button, the width of the sheet, names the destination; *Close* cancels. */
  .sheet-actions { display: grid; margin-block-start: var(--sp-200); }
</style>
